package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"kiro2api/auth"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// createTestClientTokenManager 创建测试用的 ClientTokenManager
func createTestClientTokenManager(tokens ...string) *auth.ClientTokenManager {
	manager, _ := auth.NewClientTokenManager()
	for _, token := range tokens {
		manager.AddToken(token, "test")
	}
	return manager
}

func TestPathBasedAuthMiddleware_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, router := gin.CreateTestContext(w)

	// 配置中间件
	manager := createTestClientTokenManager("test-token-123")
	protectedPrefixes := []string{"/v1/"}

	router.Use(PathBasedAuthMiddleware(manager, protectedPrefixes))
	router.POST("/v1/messages", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// 创建请求
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	c.Request.Header.Set("Authorization", "Bearer test-token-123")

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPathBasedAuthMiddleware_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, router := gin.CreateTestContext(w)

	manager := createTestClientTokenManager("test-token-123")
	protectedPrefixes := []string{"/v1/"}

	router.Use(PathBasedAuthMiddleware(manager, protectedPrefixes))
	router.POST("/v1/messages", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	c.Request.Header.Set("Authorization", "Bearer wrong-token")

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPathBasedAuthMiddleware_MissingAuthHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, router := gin.CreateTestContext(w)

	manager := createTestClientTokenManager("test-token-123")
	protectedPrefixes := []string{"/v1/"}

	router.Use(PathBasedAuthMiddleware(manager, protectedPrefixes))
	router.POST("/v1/messages", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPathBasedAuthMiddleware_UnprotectedPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, router := gin.CreateTestContext(w)

	manager := createTestClientTokenManager("test-token-123")
	protectedPrefixes := []string{"/v1/"}

	router.Use(PathBasedAuthMiddleware(manager, protectedPrefixes))
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	c.Request = httptest.NewRequest("GET", "/health", nil)

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPathBasedAuthMiddleware_EmptyProtectedPrefixes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, router := gin.CreateTestContext(w)

	manager := createTestClientTokenManager("test-token-123")
	protectedPrefixes := []string{}

	router.Use(PathBasedAuthMiddleware(manager, protectedPrefixes))
	router.POST("/any/path", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	c.Request = httptest.NewRequest("POST", "/any/path", nil)

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPathBasedAuthMiddleware_InvalidBearerFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, router := gin.CreateTestContext(w)

	manager := createTestClientTokenManager("test-token-123")
	protectedPrefixes := []string{"/v1/"}

	router.Use(PathBasedAuthMiddleware(manager, protectedPrefixes))
	router.POST("/v1/messages", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	c.Request.Header.Set("Authorization", "Invalid test-token-123")

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPathBasedAuthMiddleware_MultipleProtectedPrefixes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, router := gin.CreateTestContext(w)

	manager := createTestClientTokenManager("test-token-123")
	protectedPrefixes := []string{"/v1/", "/api/"}

	router.Use(PathBasedAuthMiddleware(manager, protectedPrefixes))
	router.POST("/api/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	c.Request = httptest.NewRequest("POST", "/api/data", nil)
	c.Request.Header.Set("Authorization", "Bearer test-token-123")

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
}
