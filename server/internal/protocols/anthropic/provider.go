package anthropic

import (
	"net/http"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"ai-gateway/internal/core/registry"
)

// AnthropicProvider 封装 Anthropic API 调用（使用官方 SDK 做 HTTP 传输和响应解析）。
type AnthropicProvider struct {
	cfg      *registry.Config
	sdk      anthropic.Client // SDK 客户端（连接池复用、自动 Auth header）
	httpPool *http.Client     // 共享 HTTP 客户端（给流式手动解析用）
}

func NewAnthropicProvider(cfg *registry.Config) *AnthropicProvider {
	hc := &http.Client{
		Timeout: 10 * time.Minute, // 流式可能很长时间
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	return &AnthropicProvider{
		cfg: cfg,
		sdk: anthropic.NewClient(
			option.WithBaseURL(cfg.BaseURL),
			option.WithAPIKey(cfg.APIKey),
			option.WithHTTPClient(hc),
		),
		httpPool: hc,
	}
}
