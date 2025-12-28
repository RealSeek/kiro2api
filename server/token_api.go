package server

import (
	"net/http"
	"strconv"

	"kiro2api/auth"
	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

// AddTokenRequest 添加 Token 的请求结构
type AddTokenRequest struct {
	AuthType     string `json:"auth_type"`               // Social 或 IdC
	RefreshToken string `json:"refresh_token"`           // 刷新令牌
	ClientID     string `json:"client_id,omitempty"`     // IdC 认证需要
	ClientSecret string `json:"client_secret,omitempty"` // IdC 认证需要
	Disabled     bool   `json:"disabled,omitempty"`      // 是否禁用
}

// TokenAPIResponse 通用 API 响应结构
type TokenAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Count   int    `json:"count,omitempty"` // 当前配置数量
}

// registerTokenManagementRoutes 注册 Token 管理路由
func registerTokenManagementRoutes(r *gin.Engine, authService *auth.AuthService) {
	// 添加 Token
	r.POST("/api/tokens", func(c *gin.Context) {
		handleAddToken(c, authService)
	})

	// 删除 Token
	r.DELETE("/api/tokens/:index", func(c *gin.Context) {
		handleDeleteToken(c, authService)
	})
}

// handleAddToken 处理添加 Token 请求
func handleAddToken(c *gin.Context, authService *auth.AuthService) {
	var req AddTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("解析添加Token请求失败", logger.Err(err))
		c.JSON(http.StatusBadRequest, TokenAPIResponse{
			Success: false,
			Message: "请求格式错误: " + err.Error(),
		})
		return
	}

	// 验证必填字段
	if req.RefreshToken == "" {
		c.JSON(http.StatusBadRequest, TokenAPIResponse{
			Success: false,
			Message: "refresh_token 不能为空",
		})
		return
	}

	// 设置默认认证类型
	if req.AuthType == "" {
		req.AuthType = auth.AuthMethodSocial
	}

	// 验证认证类型
	if req.AuthType != auth.AuthMethodSocial && req.AuthType != auth.AuthMethodIdC {
		c.JSON(http.StatusBadRequest, TokenAPIResponse{
			Success: false,
			Message: "auth_type 必须是 Social 或 IdC",
		})
		return
	}

	// IdC 认证需要额外字段
	if req.AuthType == auth.AuthMethodIdC {
		if req.ClientID == "" || req.ClientSecret == "" {
			c.JSON(http.StatusBadRequest, TokenAPIResponse{
				Success: false,
				Message: "IdC 认证需要 client_id 和 client_secret",
			})
			return
		}
	}

	// 构建 AuthConfig
	config := auth.AuthConfig{
		AuthType:     req.AuthType,
		RefreshToken: req.RefreshToken,
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
		Disabled:     req.Disabled,
	}

	// 添加配置
	if err := authService.AddConfig(config); err != nil {
		logger.Error("添加Token配置失败", logger.Err(err))
		c.JSON(http.StatusInternalServerError, TokenAPIResponse{
			Success: false,
			Message: "添加配置失败: " + err.Error(),
		})
		return
	}

	logger.Info("成功添加Token配置",
		logger.String("auth_type", req.AuthType),
		logger.Int("total_count", authService.GetConfigCount()))

	c.JSON(http.StatusOK, TokenAPIResponse{
		Success: true,
		Message: "Token 添加成功",
		Count:   authService.GetConfigCount(),
	})
}

// handleDeleteToken 处理删除 Token 请求
func handleDeleteToken(c *gin.Context, authService *auth.AuthService) {
	indexStr := c.Param("index")
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, TokenAPIResponse{
			Success: false,
			Message: "无效的索引: " + indexStr,
		})
		return
	}

	// 删除配置
	if err := authService.RemoveConfig(index); err != nil {
		logger.Warn("删除Token配置失败",
			logger.Int("index", index),
			logger.Err(err))
		c.JSON(http.StatusBadRequest, TokenAPIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	logger.Info("成功删除Token配置",
		logger.Int("removed_index", index),
		logger.Int("remaining_count", authService.GetConfigCount()))

	c.JSON(http.StatusOK, TokenAPIResponse{
		Success: true,
		Message: "Token 删除成功",
		Count:   authService.GetConfigCount(),
	})
}
