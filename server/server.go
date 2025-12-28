package server

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"kiro2api/auth"
	"kiro2api/config"
	"kiro2api/converter"
	"kiro2api/logger"
	"kiro2api/types"
	"kiro2api/utils"

	"github.com/gin-gonic/gin"
)

// 移除全局httpClient，使用utils包中的共享客户端

// StartServer 启动HTTP代理服务器
func StartServer(port string, authToken string, authService *auth.AuthService) {
	// 设置 gin 模式
	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "" {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)

	// 获取管理员凭据
	adminUser := os.Getenv("ADMIN_USERNAME")
	adminPass := os.Getenv("ADMIN_PASSWORD")
	if adminUser == "" {
		adminUser = "admin"
	}

	// 检查是否启用 Dashboard 认证
	dashboardAuthEnabled := adminPass != ""
	if dashboardAuthEnabled {
		logger.Info("Dashboard 认证已启用")
	} else {
		logger.Warn("Dashboard 认证未启用，请设置 ADMIN_PASSWORD 环境变量以保护管理面板")
	}

	// 创建会话管理器（30分钟空闲超时，24小时绝对超时）
	sessionManager := NewSessionManager(30*time.Minute, 24*time.Hour)

	// 创建认证处理器
	authHandlers := NewAuthHandlers(sessionManager, adminUser, adminPass, 30*time.Minute)

	// 判断是否使用 Secure cookie
	secureCookie := gin.Mode() == gin.ReleaseMode

	r := gin.New()

	// 添加中间件
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	// 注入请求ID，便于日志追踪
	r.Use(RequestIDMiddleware())
	r.Use(corsMiddleware())
	// 会话中间件（解析会话cookie）
	r.Use(SessionMiddleware(sessionManager))
	// CSRF 保护（跳过 /v1 API）
	if dashboardAuthEnabled {
		r.Use(CSRFMiddleware(secureCookie))
	}
	// 只对 /v1 开头的端点进行认证
	r.Use(PathBasedAuthMiddleware(authToken, []string{"/v1"}))

	// 静态资源服务 - 前后端完全分离
	r.Static("/static", "./static")

	// Dashboard 首页（需要认证）
	r.GET("/", func(c *gin.Context) {
		if dashboardAuthEnabled {
			if _, exists := c.Get(sessionUserKey); !exists {
				c.Redirect(http.StatusFound, "/static/login.html")
				return
			}
		}
		c.File("./static/index.html")
	})

	// 认证相关 API
	r.POST("/api/login", authHandlers.HandleLogin)
	r.POST("/api/logout", authHandlers.HandleLogout)
	r.GET("/api/session", authHandlers.HandleSessionCheck)

	// API端点 - 纯数据服务（需要认证保护）
	apiGroup := r.Group("/api")
	if dashboardAuthEnabled {
		apiGroup.Use(AdminAPIAuthGuard())
	}
	apiGroup.GET("/tokens", func(c *gin.Context) {
		handleTokenPoolAPI(c, authService)
	})

	// Token 管理 API（动态添加/删除）
	registerTokenManagementRoutes(r, authService, dashboardAuthEnabled)

	// GET /v1/models 端点
	r.GET("/v1/models", func(c *gin.Context) {
		// 构建模型列表
		models := []types.Model{}
		for anthropicModel := range config.ModelMap {
			model := types.Model{
				ID:          anthropicModel,
				Object:      "model",
				Created:     1234567890,
				OwnedBy:     "anthropic",
				DisplayName: anthropicModel,
				Type:        "text",
				MaxTokens:   200000,
			}
			models = append(models, model)
		}

		response := types.ModelsResponse{
			Object: "list",
			Data:   models,
		}

		c.JSON(http.StatusOK, response)
	})

	r.POST("/v1/messages", func(c *gin.Context) {
		// 使用RequestContext统一处理token获取和请求体读取
		reqCtx := &RequestContext{
			GinContext:  c,
			AuthService: authService,
			RequestType: "Anthropic",
		}

		tokenWithUsage, body, err := reqCtx.GetTokenWithUsageAndBody()
		if err != nil {
			return // 错误已在GetTokenWithUsageAndBody中处理
		}

		// 先解析为通用map以便处理工具格式
		var rawReq map[string]any
		if err := utils.SafeUnmarshal(body, &rawReq); err != nil {
			logger.Error("解析请求体失败", logger.Err(err))
			respondError(c, http.StatusBadRequest, "解析请求体失败: %v", err)
			return
		}

		// 标准化工具格式处理
		if tools, exists := rawReq["tools"]; exists && tools != nil {
			if toolsArray, ok := tools.([]any); ok {
				normalizedTools := make([]map[string]any, 0, len(toolsArray))
				for _, tool := range toolsArray {
					if toolMap, ok := tool.(map[string]any); ok {
						// 检查是否是简化的工具格式（直接包含name, description, input_schema）
						if name, hasName := toolMap["name"]; hasName {
							if description, hasDesc := toolMap["description"]; hasDesc {
								if inputSchema, hasSchema := toolMap["input_schema"]; hasSchema {
									// 转换为标准Anthropic工具格式
									normalizedTool := map[string]any{
										"name":         name,
										"description":  description,
										"input_schema": inputSchema,
									}
									normalizedTools = append(normalizedTools, normalizedTool)
									continue
								}
							}
						}
						// 如果不是简化格式，保持原样
						normalizedTools = append(normalizedTools, toolMap)
					}
				}
				rawReq["tools"] = normalizedTools
			}
		}

		// 重新序列化并解析为AnthropicRequest
		normalizedBody, err := utils.SafeMarshal(rawReq)
		if err != nil {
			logger.Error("重新序列化请求失败", logger.Err(err))
			respondError(c, http.StatusBadRequest, "处理请求格式失败: %v", err)
			return
		}

		var anthropicReq types.AnthropicRequest
		if err := utils.SafeUnmarshal(normalizedBody, &anthropicReq); err != nil {
			logger.Error("解析标准化请求体失败", logger.Err(err))
			respondError(c, http.StatusBadRequest, "解析请求体失败: %v", err)
			return
		}

		// 验证请求的有效性
		if len(anthropicReq.Messages) == 0 {
			logger.Error("请求中没有消息")
			respondError(c, http.StatusBadRequest, "%s", "messages 数组不能为空")
			return
		}

		// 验证最后一条消息有有效内容
		lastMsg := anthropicReq.Messages[len(anthropicReq.Messages)-1]
		content, err := utils.GetMessageContent(lastMsg.Content)
		if err != nil {
			logger.Error("获取消息内容失败",
				logger.Err(err),
				logger.String("raw_content", fmt.Sprintf("%v", lastMsg.Content)))
			respondError(c, http.StatusBadRequest, "获取消息内容失败: %v", err)
			return
		}

		trimmedContent := strings.TrimSpace(content)
		if trimmedContent == "" || trimmedContent == "answer for user question" {
			logger.Error("消息内容为空或无效",
				logger.String("content", content),
				logger.String("trimmed_content", trimmedContent))
			respondError(c, http.StatusBadRequest, "%s", "消息内容不能为空")
			return
		}

		if anthropicReq.Stream {
			handleStreamRequest(c, anthropicReq, tokenWithUsage)
			return
		}

		handleNonStreamRequest(c, anthropicReq, tokenWithUsage.TokenInfo)
	})

	// Token计数端点
	r.POST("/v1/messages/count_tokens", handleCountTokens)

	// 新增：OpenAI兼容的 /v1/chat/completions 端点
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		// 使用RequestContext统一处理token获取和请求体读取
		reqCtx := &RequestContext{
			GinContext:  c,
			AuthService: authService,
			RequestType: "OpenAI",
		}

		tokenInfo, body, err := reqCtx.GetTokenAndBody()
		if err != nil {
			return // 错误已在GetTokenAndBody中处理
		}

		var openaiReq types.OpenAIRequest
		if err := utils.SafeUnmarshal(body, &openaiReq); err != nil {
			logger.Error("解析OpenAI请求体失败", logger.Err(err))
			respondError(c, http.StatusBadRequest, "解析请求体失败: %v", err)
			return
		}

		logger.Debug("OpenAI请求解析成功",
			logger.String("model", openaiReq.Model),
			logger.Bool("stream", openaiReq.Stream != nil && *openaiReq.Stream),
			logger.Int("max_tokens", func() int {
				if openaiReq.MaxTokens != nil {
					return *openaiReq.MaxTokens
				}
				return 16384
			}()))

		// 转换为Anthropic格式
		anthropicReq := converter.ConvertOpenAIToAnthropic(openaiReq)

		if anthropicReq.Stream {
			handleOpenAIStreamRequest(c, anthropicReq, tokenInfo)
			return
		}
		handleOpenAINonStreamRequest(c, anthropicReq, tokenInfo)
	})

	r.NoRoute(func(c *gin.Context) {
		logger.Warn("访问未知端点",
			logger.String("path", c.Request.URL.Path),
			logger.String("method", c.Request.Method))
		respondError(c, http.StatusNotFound, "%s", "404 未找到")
	})

	logger.Info("启动Anthropic API代理服务器",
		logger.String("port", port),
		logger.String("auth_token", "***"))
	logger.Info("AuthToken 验证已启用")
	if dashboardAuthEnabled {
		logger.Info("Dashboard 认证已启用，用户名: " + adminUser)
	}
	logger.Info("可用端点:")
	logger.Info("  GET  /                          - Dashboard (需要登录)")
	logger.Info("  GET  /static/*                  - 静态资源服务")
	logger.Info("  POST /api/login                 - 登录")
	logger.Info("  POST /api/logout                - 登出")
	logger.Info("  GET  /api/session               - 会话状态")
	logger.Info("  GET  /api/tokens                - Token池状态API")
	logger.Info("  POST /api/tokens                - 添加Token")
	logger.Info("  DELETE /api/tokens/:index       - 删除Token")
	logger.Info("  GET  /v1/models                 - 模型列表")
	logger.Info("  POST /v1/messages               - Anthropic API代理")
	logger.Info("  POST /v1/messages/count_tokens  - Token计数接口")
	logger.Info("  POST /v1/chat/completions       - OpenAI API代理")
	logger.Info("按Ctrl+C停止服务器")

	// 创建自定义HTTP服务器以支持长时间请求
	server := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	logger.Info("启动HTTP服务器", logger.String("port", port))

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("启动服务器失败", logger.Err(err), logger.String("port", port))
		os.Exit(1)
	}
}

// corsMiddleware CORS中间件
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}
