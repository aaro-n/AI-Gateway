package deepseek

import (
	"net/http"

	"ai-gateway/internal/core/httpclient"
	"ai-gateway/internal/core/registry"
)

type DeepSeekProvider struct {
	cfg             *registry.Config
	httpPool        *http.Client
	lastRawResponse []byte // 最近一次 FromUnified 的原始响应体（供调试）
}

func (p *DeepSeekProvider) LastRawResponse() []byte { return p.lastRawResponse }

func NewDeepSeekProvider(cfg *registry.Config) *DeepSeekProvider {
	return &DeepSeekProvider{cfg: cfg, httpPool: httpclient.Pool()}
}
