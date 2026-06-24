package provider

import (
	"encoding/base64"
	"strings"

	"github.com/gin-gonic/gin"
)

func init() {
	Register(ProtocolDescriptor{
		Name:      "gemini",
		KeyPrefix: "AIza",
		KeyLength: 26,
		KeyEncoder: func(b []byte) string {
			s := base64.RawURLEncoding.EncodeToString(b)
			if len(s) > 35 {
				return s[:35]
			}
			return s
		},
		AuthExtractor: func(c *gin.Context) string {
			key := c.GetHeader("x-goog-api-key")
			if key == "" {
				key = c.Query("key")
			}
			return key
		},
		ModelExtractor: func(c *gin.Context) (string, error) {
			// In Gemini, the model ID is passed in the URL path.
			// E.g., /gateway/gemini/v1beta/models/gemini-2.5-flash:generateContent
			path := c.Request.URL.Path
			parts := strings.Split(path, "/models/")
			if len(parts) < 2 {
				return "", nil
			}
			modelPart := parts[1]
			modelID := strings.Split(modelPart, ":")[0]
			return modelID, nil
		},
		DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta",
		NewProvider: func(cfg *Config) Provider {
			return NewGeminiProvider(cfg)
		},
	})
}
