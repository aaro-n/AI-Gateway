package openrouter

import (
	"ai-gateway/internal/core/registry"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

func init() {
	registry.Register(registry.ProtocolDescriptor{
		Name:      "openrouter",
		Label:     "OpenRouter",
		KeyPrefix: "sk-or-",
		KeyLength: 24,
		KeyEncoder: func(b []byte) string {
			return hex.EncodeToString(b)
		},
		AuthExtractor: func(c *gin.Context) string {
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				return auth[7:]
			}
			return ""
		},
		NewProvider: func(cfg *registry.Config) registry.Provider {
			return NewOpenRouterProvider(cfg)
		},
		TestExecutor:   &OpenRouterTestExecutor{},
		DefaultBaseURL: "https://openrouter.ai/api/v1",
		FormSchema: []registry.FormField{
			{
				Key: "base_url", Label: "Base URL", Type: "url",
				Placeholder: "https://openrouter.ai/api/v1",
				Default:     "https://openrouter.ai/api/v1",
				Required:    true,
			},
		},
	})
}
