package gemini

import (
	"ai-gateway/internal/core/registry"
	"encoding/base64"

	"github.com/gin-gonic/gin"
)

func init() {
	registry.Register(registry.ProtocolDescriptor{
		Name:      "gemini",
		Label:     "Google Gemini",
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
		NewProvider: func(cfg *registry.Config) registry.Provider {
			return NewGeminiProvider(cfg)
		},
		TestExecutor: &GeminiTestExecutor{},
		DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta",
		FormSchema: []registry.FormField{
			{
				Key: "base_url", Label: "Base URL", Type: "url",
				Placeholder: "https://generativelanguage.googleapis.com/v1beta",
				Default:     "https://generativelanguage.googleapis.com/v1beta",
				Required:    true,
			},
			{
				Key: "region", Label: "Region", Type: "text",
				Placeholder: "us-central1",
				Default:     "",
				Required:    false,
			},
		},
	})
}
