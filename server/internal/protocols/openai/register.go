package openai

import (
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/protocols/capabilities"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

func init() {
	registry.Register(registry.ProtocolDescriptor{
		Name:      "openai",
		Label:     "OpenAI",
		KeyPrefix: "sk-",
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
			return NewOpenAIProvider(cfg)
		},
		TestExecutor:   &OpenAITestExecutor{},
		DefaultBaseURL: "https://api.openai.com/v1", Capabilities: capabilities.Get("openai"), FormSchema: []registry.FormField{
			{
				Key: "base_url", Label: "Base URL", Type: "url",
				Placeholder: "https://api.openai.com/v1",
				Default:     "https://api.openai.com/v1",
				Required:    true,
			},
		},
	})
}
