package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

const (
	// Context keys
	sessionUserKey = "session_user"
	sessionIDKey   = "session_id"

	// CSRF 配置
	csrfTokenCookieName = "csrf_token"
	csrfHeaderName      = "X-CSRF-Token"
	csrfTokenLength     = 32 // 256 bits
)

// SessionMiddleware 解析会话cookie并附加用户信息到context
func SessionMiddleware(manager *SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Request.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			if session, ok := manager.Validate(cookie.Value); ok {
				c.Set(sessionUserKey, session.User)
				c.Set(sessionIDKey, session.ID)
			}
		}
		c.Next()
	}
}

// AdminAPIAuthGuard 保护管理API，未认证返回401 JSON
func AdminAPIAuthGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get(sessionUserKey); !exists {
			logger.Debug("管理API访问被拒绝: 未认证",
				logger.String("path", c.Request.URL.Path),
				logger.String("ip", c.ClientIP()))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "未登录，请先登录",
			})
			return
		}
		c.Next()
	}
}

// DashboardAuthGuard 保护Dashboard页面，未认证重定向到登录页
func DashboardAuthGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get(sessionUserKey); !exists {
			c.Redirect(http.StatusFound, "/static/login.html")
			c.Abort()
			return
		}
		c.Next()
	}
}

// GetSessionUser 从context获取当前登录用户
func GetSessionUser(c *gin.Context) string {
	if user, exists := c.Get(sessionUserKey); exists {
		if u, ok := user.(string); ok {
			return u
		}
	}
	return ""
}

// GetSessionID 从context获取当前会话ID
func GetSessionID(c *gin.Context) string {
	if sid, exists := c.Get(sessionIDKey); exists {
		if s, ok := sid.(string); ok {
			return s
		}
	}
	return ""
}

// CSRFMiddleware 使用双提交 Cookie 模式验证 CSRF token
// 保护所有非安全 HTTP 方法（POST, PUT, PATCH, DELETE）
// 跳过 /v1 开头的 API 路由（外部客户端 API 使用 Authorization header）
func CSRFMiddleware(secureCookie bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过外部 API 路由（使用 token 认证，不需要 CSRF）
		if strings.HasPrefix(c.Request.URL.Path, "/v1") ||
			strings.HasPrefix(c.Request.URL.Path, "/api/event_logging") {
			c.Next()
			return
		}

		// 从 cookie 获取现有 token
		token := ""
		if cookie, err := c.Request.Cookie(csrfTokenCookieName); err == nil && cookie.Value != "" {
			token = cookie.Value
		}

		// 如果没有 token，生成新的并设置 cookie
		if token == "" {
			newToken, err := generateCSRFToken()
			if err != nil {
				logger.Error("生成 CSRF token 失败", logger.Err(err))
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error":   "服务器内部错误",
				})
				return
			}
			token = newToken
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     csrfTokenCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: false, // 前端需要读取
				SameSite: http.SameSiteLaxMode,
				Secure:   secureCookie,
				MaxAge:   3600, // 1小时
			})
		}

		// 对于非安全方法，验证 header 中的 token
		if isUnsafeMethod(c.Request.Method) {
			headerToken := c.GetHeader(csrfHeaderName)
			if headerToken == "" || token == "" ||
				subtle.ConstantTimeCompare([]byte(headerToken), []byte(token)) != 1 {
				logger.Warn("CSRF 校验失败",
					logger.String("path", c.Request.URL.Path),
					logger.String("method", c.Request.Method),
					logger.String("ip", c.ClientIP()))
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"success": false,
					"error":   "CSRF token 无效，请刷新页面后重试",
				})
				return
			}
		}

		c.Next()
	}
}

// generateCSRFToken 生成安全的随机 CSRF token
func generateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// isUnsafeMethod 检查是否是需要 CSRF 保护的方法
func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
