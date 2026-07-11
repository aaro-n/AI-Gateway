package pipeline

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	coreErrors "ai-gateway/internal/core/errors"
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/unified"
)

type RetryableProvider struct {
	inner      registry.Provider
	cfg        *registry.Config
	maxRetries int
	retryCount int
	CanRetry   func(err error) bool
}

func NewRetryableProvider(inner registry.Provider, cfg *registry.Config, maxRetries int) *RetryableProvider {
	return &RetryableProvider{inner: inner, cfg: cfg, maxRetries: maxRetries}
}

func (r *RetryableProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	return r.inner.SyncModels(providerID)
}

func (r *RetryableProvider) ToUnified(body []byte, modelID string) (*unified.Request, error) {
	return r.inner.ToUnified(body, modelID)
}

func (r *RetryableProvider) FromUnified(req *unified.Request) (*unified.Response, <-chan unified.StreamEvent, error) {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		resp, events, err := r.inner.FromUnified(req)
		if err == nil {
			return resp, events, nil
		}
		lastErr = err
		if !r.shouldRetry(err) || attempt >= r.maxRetries {
			break
		}
		r.retryCount++
	}
	return nil, nil, fmt.Errorf("retry exhausted (%d/%d): %w", r.retryCount, r.maxRetries, lastErr)
}

func (r *RetryableProvider) FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, c *gin.Context, usage *registry.Usage) error {
	return r.inner.FormatUnified(resp, events, c, usage)
}

func (r *RetryableProvider) shouldRetry(err error) bool {
	if r.CanRetry != nil {
		return r.CanRetry(err)
	}
	if coreErrors.IsUpstreamError(err) {
		if ue, ok := err.(*coreErrors.UpstreamError); ok {
			return ue.StatusCode >= http.StatusInternalServerError ||
				ue.StatusCode == http.StatusTooManyRequests
		}
		return false
	}
	return true
}
