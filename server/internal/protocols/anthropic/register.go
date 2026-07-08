package anthropic

import (
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/protocols/capabilities"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

func init() {
	registry.Register(registry.ProtocolDescriptor{
		Name:      "anthropic",
		Label:     "Anthropic",
		KeyPrefix: "sk-ant-",
		KeyLength: 24,
		KeyEncoder: func(b []byte) string {
			return hex.EncodeToString(b)
		},
		AuthExtractor: func(c *gin.Context) string {
			return c.GetHeader("x-api-key")
		},
		NewProvider: func(cfg *registry.Config) registry.Provider {
			return NewAnthropicProvider(cfg)
		},
		TestExecutor:   &AnthropicTestExecutor{},
		DefaultBaseURL: "https://api.anthropic.com", Capabilities: capabilities.Get("anthropic"), FormSchema: []registry.FormField{
			{
				Key: "base_url", Label: "Base URL", Type: "url",
				Placeholder: "https://api.anthropic.com",
				Default:     "https://api.anthropic.com",
				Required:    true,
			},
		},
	})
}
