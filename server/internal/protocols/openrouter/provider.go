package openrouter

import (
	"net/http"

	"ai-gateway/internal/core/httpclient"
	"ai-gateway/internal/core/registry"
)

type OpenRouterProvider struct {
	cfg      *registry.Config
	httpPool *http.Client
}

func NewOpenRouterProvider(cfg *registry.Config) *OpenRouterProvider {
	return &OpenRouterProvider{cfg: cfg, httpPool: httpclient.Pool()}
}
