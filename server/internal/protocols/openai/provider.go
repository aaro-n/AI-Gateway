package openai

import (
	"net/http"

	"ai-gateway/internal/core/httpclient"
	"ai-gateway/internal/core/registry"
)

type OpenAIProvider struct {
	cfg             *registry.Config
	httpPool        *http.Client
	lastRawResponse []byte // 最近一次 FromUnified 的原始响应体（供调试）
}

func (p *OpenAIProvider) LastRawResponse() []byte { return p.lastRawResponse }

func NewOpenAIProvider(cfg *registry.Config) *OpenAIProvider {
	return &OpenAIProvider{cfg: cfg, httpPool: httpclient.Pool()}
}
