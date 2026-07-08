package deepseek

import (
	"net/http"

	"ai-gateway/internal/core/httpclient"
	"ai-gateway/internal/core/registry"
)

type DeepSeekProvider struct {
	cfg      *registry.Config
	httpPool *http.Client
}

func NewDeepSeekProvider(cfg *registry.Config) *DeepSeekProvider {
	return &DeepSeekProvider{cfg: cfg, httpPool: httpclient.Pool()}
}
