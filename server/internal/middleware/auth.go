package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"ai-gateway/internal/model"
)

func SetupSessionStore(secret string, maxAge int, secure, httpOnly bool, sameSite string) gin.HandlerFunc {
	store := cookie.NewStore([]byte(secret))

	options := sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: httpOnly,
		Secure:   secure,
	}

	switch sameSite {
	case "strict":
		options.SameSite = http.SameSiteStrictMode
	case "none":
		options.SameSite = http.SameSiteNoneMode
	default:
		options.SameSite = http.SameSiteLaxMode
	}

	store.Options(options)

	return sessions.Sessions("ai-gateway-session", store)
}

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		userID := session.Get("user_id")

		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}

func authenticateKey(c *gin.Context, rawKey string, expectedFormat string) (*model.Key, bool) {
	var kFormat model.KeyFormat
	if err := model.DB.Where("formatted_key = ?", rawKey).First(&kFormat).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		c.Abort()
		return nil, false
	}

	if kFormat.Format != expectedFormat {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Key format " + kFormat.Format + " is not allowed on " + expectedFormat + " endpoint",
		})
		c.Abort()
		return nil, false
	}

	var apiKey model.Key
	if err := model.DB.Where("id = ? AND enabled = ?", kFormat.KeyID, true).First(&apiKey).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		c.Abort()
		return nil, false
	}

	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "API key expired"})
		c.Abort()
		return nil, false
	}

	c.Set("api_key", &apiKey)
	c.Set("key_id", apiKey.ID)
	c.Set("key_name", apiKey.Name)
	return &apiKey, true
}

func RequireAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		key := strings.TrimPrefix(authHeader, "Bearer ")
		authenticateKey(c, key, "openai")
	}
}

func RequireAPIKeyForAnthropic() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKeyHeader := c.GetHeader("x-api-key")
		if apiKeyHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing x-api-key header"})
			c.Abort()
			return
		}

		authenticateKey(c, apiKeyHeader, "anthropic")
	}
}

func RequireAPIKeyForGemini() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("x-goog-api-key")
		if key == "" {
			key = c.Query("key")
		}
		if key == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing API key header (x-goog-api-key) or query parameter (key)"})
			c.Abort()
			return
		}

		authenticateKey(c, key, "gemini")
	}
}

func RequireAPIKeyForMCP() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		key := ""
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			key = strings.TrimPrefix(authHeader, "Bearer ")
		}
		if key == "" {
			key = c.GetHeader("x-api-key")
		}
		if key == "" {
			key = c.GetHeader("x-goog-api-key")
			if key == "" {
				key = c.Query("key")
			}
		}

		if key == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing API key"})
			c.Abort()
			return
		}

		// MCP accepts any valid key format
		var kFormat model.KeyFormat
		if err := model.DB.Where("formatted_key = ?", key).First(&kFormat).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			c.Abort()
			return
		}

		var apiKey model.Key
		if err := model.DB.Where("id = ? AND enabled = ?", kFormat.KeyID, true).First(&apiKey).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			c.Abort()
			return
		}

		if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "API key expired"})
			c.Abort()
			return
		}

		c.Set("api_key", &apiKey)
		c.Set("key_id", apiKey.ID)
		c.Set("key_name", apiKey.Name)
		c.Next()
	}
}
