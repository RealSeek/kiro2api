package server

import (
	"net/http"
	"strconv"

	"kiro2api/auth"
	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

// AddClientTokenRequest 添加客户端令牌的请求结构
type AddClientTokenRequest struct {
	Token string `json:"token"` // 令牌值
	Name  string `json:"name"`  // 可选名称
}

// ClientTokenAPIResponse 通用 API 响应结构
type ClientTokenAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Count   int    `json:"count,omitempty"`
}

// ClientTokenListResponse 客户端令牌列表响应
type ClientTokenListResponse struct {
	Success bool                     `json:"success"`
	Tokens  []auth.ClientTokenStats  `json:"tokens"`
	Total   int                      `json:"total"`
}

// registerClientTokenRoutes 注册客户端令牌管理路由
func registerClientTokenRoutes(r *gin.Engine, manager *auth.ClientTokenManager, requireAuth bool) {
	// 创建路由组
	group := r.Group("/api/client-tokens")
	if requireAuth {
		group.Use(AdminAPIAuthGuard())
	}

	// 获取所有客户端令牌
	group.GET("", func(c *gin.Context) {
		handleGetClientTokens(c, manager)
	})

	// 添加客户端令牌
	group.POST("", func(c *gin.Context) {
		handleAddClientToken(c, manager)
	})

	// 删除客户端令牌
	group.DELETE("/:index", func(c *gin.Context) {
		handleDeleteClientToken(c, manager)
	})

	// 切换客户端令牌状态
	group.POST("/:index/toggle", func(c *gin.Context) {
		handleToggleClientToken(c, manager)
	})
}

// handleGetClientTokens 获取所有客户端令牌
func handleGetClientTokens(c *gin.Context, manager *auth.ClientTokenManager) {
	stats := manager.GetAllStats()
	c.JSON(http.StatusOK, ClientTokenListResponse{
		Success: true,
		Tokens:  stats,
		Total:   len(stats),
	})
}

// handleAddClientToken 添加客户端令牌
func handleAddClientToken(c *gin.Context, manager *auth.ClientTokenManager) {
	var req AddClientTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("解析添加客户端令牌请求失败", logger.Err(err))
		c.JSON(http.StatusBadRequest, ClientTokenAPIResponse{
			Success: false,
			Message: "请求格式错误: " + err.Error(),
		})
		return
	}

	// 验证必填字段
	if req.Token == "" {
		c.JSON(http.StatusBadRequest, ClientTokenAPIResponse{
			Success: false,
			Message: "token 不能为空",
		})
		return
	}

	// 添加令牌
	if err := manager.AddToken(req.Token, req.Name); err != nil {
		logger.Error("添加客户端令牌失败", logger.Err(err))
		c.JSON(http.StatusBadRequest, ClientTokenAPIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	logger.Info("成功添加客户端令牌",
		logger.String("name", req.Name),
		logger.Int("total_count", manager.GetTokenCount()))

	c.JSON(http.StatusOK, ClientTokenAPIResponse{
		Success: true,
		Message: "客户端令牌添加成功",
		Count:   manager.GetTokenCount(),
	})
}

// handleDeleteClientToken 删除客户端令牌
func handleDeleteClientToken(c *gin.Context, manager *auth.ClientTokenManager) {
	indexStr := c.Param("index")
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClientTokenAPIResponse{
			Success: false,
			Message: "无效的索引: " + indexStr,
		})
		return
	}

	// 删除令牌
	if err := manager.RemoveToken(index); err != nil {
		logger.Warn("删除客户端令牌失败",
			logger.Int("index", index),
			logger.Err(err))
		c.JSON(http.StatusBadRequest, ClientTokenAPIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	logger.Info("成功删除客户端令牌",
		logger.Int("removed_index", index),
		logger.Int("remaining_count", manager.GetTokenCount()))

	c.JSON(http.StatusOK, ClientTokenAPIResponse{
		Success: true,
		Message: "客户端令牌删除成功",
		Count:   manager.GetTokenCount(),
	})
}

// handleToggleClientToken 切换客户端令牌状态
func handleToggleClientToken(c *gin.Context, manager *auth.ClientTokenManager) {
	indexStr := c.Param("index")
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClientTokenAPIResponse{
			Success: false,
			Message: "无效的索引: " + indexStr,
		})
		return
	}

	// 切换状态
	if err := manager.ToggleToken(index); err != nil {
		logger.Warn("切换客户端令牌状态失败",
			logger.Int("index", index),
			logger.Err(err))
		c.JSON(http.StatusBadRequest, ClientTokenAPIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	logger.Info("成功切换客户端令牌状态",
		logger.Int("index", index))

	c.JSON(http.StatusOK, ClientTokenAPIResponse{
		Success: true,
		Message: "状态切换成功",
		Count:   manager.GetTokenCount(),
	})
}
