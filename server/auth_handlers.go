package server

import (
	"crypto/subtle"
	"net/http"
	"sync"
	"time"

	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

// 安全配置常量
const (
	// 登录失败后的固定延迟，防止暴力破解时序攻击
	failedLoginDelay = 300 * time.Millisecond
)

// AuthHandlers 认证相关的HTTP处理器
type AuthHandlers struct {
	manager      *SessionManager
	adminUser    string
	adminPass    string
	secureCookie bool
	idleTimeout  time.Duration
	limiter      *loginRateLimiter
}

// NewAuthHandlers 创建认证处理器
func NewAuthHandlers(manager *SessionManager, adminUser, adminPass string, idleTimeout time.Duration, secureCookie bool) *AuthHandlers {
	return &AuthHandlers{
		manager:      manager,
		adminUser:    adminUser,
		adminPass:    adminPass,
		secureCookie: secureCookie,
		idleTimeout:  idleTimeout,
		limiter:      newLoginRateLimiter(10, 10*time.Minute), // 10分钟内最多10次尝试
	}
}

// LoginRequest 登录请求结构
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HandleLogin 处理登录请求
func (h *AuthHandlers) HandleLogin(c *gin.Context) {
	ip := c.ClientIP()

	// 检查限流
	if !h.limiter.Allow(ip) {
		logger.Warn("登录请求被限流",
			logger.String("ip", ip))
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"error":   "登录尝试过于频繁，请稍后再试",
		})
		return
	}

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求格式无效",
		})
		return
	}

	// 验证凭据（使用常数时间比较防止时序攻击）
	userMatch := subtle.ConstantTimeCompare([]byte(req.Username), []byte(h.adminUser)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(req.Password), []byte(h.adminPass)) == 1

	if !userMatch || !passMatch {
		// 固定延迟防止时序分析
		time.Sleep(failedLoginDelay)
		logger.Warn("登录失败: 凭据无效",
			logger.String("username", req.Username),
			logger.String("ip", ip))
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "用户名或密码错误",
		})
		return
	}

	// 创建会话
	session, err := h.manager.CreateSession(req.Username)
	if err != nil {
		logger.Error("创建会话失败",
			logger.Err(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "服务器内部错误",
		})
		return
	}

	// 设置cookie
	maxAge := int(h.idleTimeout.Seconds())
	if maxAge <= 0 {
		maxAge = 1800 // 默认30分钟
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, session.ID, maxAge, "/", "", h.secureCookie, true)

	logger.Info("用户登录成功",
		logger.String("username", req.Username),
		logger.String("ip", ip))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "登录成功",
	})
}

// HandleLogout 处理登出请求
func (h *AuthHandlers) HandleLogout(c *gin.Context) {
	// 删除服务端会话
	if sid := GetSessionID(c); sid != "" {
		h.manager.Delete(sid)
	}

	// 清除cookie
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, "", -1, "/", "", h.secureCookie, true)

	user := GetSessionUser(c)
	if user != "" {
		logger.Info("用户登出",
			logger.String("username", user),
			logger.String("ip", c.ClientIP()))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已登出",
	})
}

// HandleSessionCheck 检查会话状态
func (h *AuthHandlers) HandleSessionCheck(c *gin.Context) {
	user := GetSessionUser(c)
	authenticated := user != ""

	c.JSON(http.StatusOK, gin.H{
		"authenticated": authenticated,
		"user":          user,
	})
}

// loginRateLimiter 简单的登录限流器（按IP）
type loginRateLimiter struct {
	mu          sync.Mutex
	limit       int
	window      time.Duration
	buckets     map[string]rateBucket
	maxBuckets  int       // 最大桶数量
	lastCleanup time.Time // 上次清理时间
}

type rateBucket struct {
	count int
	reset time.Time
}

func newLoginRateLimiter(limit int, window time.Duration) *loginRateLimiter {
	return &loginRateLimiter{
		limit:       limit,
		window:      window,
		buckets:     make(map[string]rateBucket),
		maxBuckets:  10000, // 最多保留10000个IP记录
		lastCleanup: time.Now(),
	}
}

func (l *loginRateLimiter) Allow(key string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	// 定期清理过期桶（每分钟检查一次）
	if now.Sub(l.lastCleanup) > time.Minute {
		l.cleanupExpiredLocked(now)
		l.lastCleanup = now
	}

	bucket := l.buckets[key]

	// 窗口已过期，重置计数
	if now.After(bucket.reset) {
		bucket = rateBucket{
			count: 0,
			reset: now.Add(l.window),
		}
	}

	bucket.count++
	l.buckets[key] = bucket

	return bucket.count <= l.limit
}

// cleanupExpiredLocked 清理过期的桶（调用时需持有锁）
func (l *loginRateLimiter) cleanupExpiredLocked(now time.Time) {
	// 如果桶数量超过限制，强制清理
	if len(l.buckets) > l.maxBuckets {
		// 清理所有过期桶
		for key, bucket := range l.buckets {
			if now.After(bucket.reset) {
				delete(l.buckets, key)
			}
		}
	}
}
