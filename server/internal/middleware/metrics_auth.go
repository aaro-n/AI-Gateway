// Package middleware 提供 Prometheus /metrics 端点的 Bearer token 认证中间件
package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MetricsAuth 保护 /metrics 端点。
// 返回 401 如果 token 不匹配。
func MetricsAuth(expectedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid metrics token"})
			c.Abort()
			return
		}

		provided := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expectedToken)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid metrics token"})
			c.Abort()
			return
		}

		c.Next()
	}
}
