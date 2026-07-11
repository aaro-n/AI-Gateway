package maxtoken

import (
	"context"
	"fmt"

	"ai-gateway/internal/core/unified"
	"ai-gateway/internal/protocols/pipeline"
)

func EnsureMaxTokens(defaultValue, maxValue int) pipeline.Middleware {
	return pipeline.OnRequest("max-tokens", func(ctx context.Context, req *unified.Request) (*unified.Request, error) {
		if req.MaxTokens <= 0 {
			req.MaxTokens = defaultValue
		}
		if req.MaxTokens > maxValue {
			req.MaxTokens = maxValue
		}
		return req, nil
	})
}

func CapMaxTokens(maxValue int) pipeline.Middleware {
	return pipeline.OnRequest("max-tokens-cap", func(ctx context.Context, req *unified.Request) (*unified.Request, error) {
		if req.MaxTokens <= 0 {
			return nil, fmt.Errorf("max_tokens required, capped at %d", maxValue)
		}
		if req.MaxTokens > maxValue {
			req.MaxTokens = maxValue
		}
		return req, nil
	})
}
