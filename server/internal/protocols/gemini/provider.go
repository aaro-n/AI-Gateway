package gemini

import (
	"net/http"

	"ai-gateway/internal/core/httpclient"
	"ai-gateway/internal/core/registry"
)

// GeminiProvider Gemini 协议实现
type GeminiProvider struct {
	cfg      *registry.Config
	httpPool *http.Client
}

// NewGeminiProvider 创建 Gemini Provider
func NewGeminiProvider(cfg *registry.Config) *GeminiProvider {
	return &GeminiProvider{cfg: cfg, httpPool: httpclient.Pool()}
}
