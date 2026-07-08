package openai

import (
	"net/http"

	"ai-gateway/internal/core/httpclient"
	"ai-gateway/internal/core/registry"
)

type OpenAIProvider struct {
	cfg      *registry.Config
	httpPool *http.Client
}

func NewOpenAIProvider(cfg *registry.Config) *OpenAIProvider {
	return &OpenAIProvider{cfg: cfg, httpPool: httpclient.Pool()}
}
