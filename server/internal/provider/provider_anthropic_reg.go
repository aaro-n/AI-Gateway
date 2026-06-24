package provider

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"

	"github.com/gin-gonic/gin"
)

func init() {
	Register(ProtocolDescriptor{
		Name:      "anthropic",
		KeyPrefix: "sk-ant-",
		KeyLength: 24,
		KeyEncoder: func(b []byte) string {
			return hex.EncodeToString(b)
		},
		AuthExtractor: func(c *gin.Context) string {
			return c.GetHeader("x-api-key")
		},
		ModelExtractor: func(c *gin.Context) (string, error) {
			body, err := c.GetRawData()
			if err != nil {
				return "", err
			}
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			var req struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				return "", err
			}
			return req.Model, nil
		},
		DefaultBaseURL: "https://api.anthropic.com/v1",
		NewProvider: func(cfg *Config) Provider {
			return NewAnthropicProvider(cfg)
		},
	})
}
