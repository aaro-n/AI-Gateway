package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/unified"
)

type Option func(*Pipeline)

type Middleware interface {
	Name() string
	OnRequest(ctx context.Context, req *unified.Request) (*unified.Request, error)
	OnResponse(ctx context.Context, resp *unified.Response) (*unified.Response, error)
	OnError(ctx context.Context, err error) error
}

type Pipeline struct {
	inbound     registry.Provider
	outbound    registry.Provider
	middlewares []Middleware
	maxRetries  int
	retryDelay  time.Duration
	emptyDetect bool
	timeout     time.Duration
}

func New(inbound, outbound registry.Provider, opts ...Option) *Pipeline {
	p := &Pipeline{
		inbound:    inbound,
		outbound:   outbound,
		retryDelay: time.Second,
		timeout:    30 * time.Second,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func WithRetry(maxRetries int, retryDelay time.Duration) Option {
	return func(p *Pipeline) { p.maxRetries = maxRetries; p.retryDelay = retryDelay }
}

func WithTimeout(d time.Duration) Option {
	return func(p *Pipeline) { p.timeout = d }
}

func WithEmptyResponseDetection() Option {
	return func(p *Pipeline) { p.emptyDetect = true }
}

func WithMiddlewares(mws ...Middleware) Option {
	return func(p *Pipeline) { p.middlewares = append(p.middlewares, mws...) }
}

func (p *Pipeline) Process(ctx context.Context, body []byte, modelID string) (*unified.Response, error) {
	req, err := p.inbound.ToUnified(body, modelID)
	if err != nil {
		return nil, fmt.Errorf("to_unified: %w", err)
	}
	req.Stream = false

	for _, mw := range p.middlewares {
		req, err = mw.OnRequest(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("middleware %s: %w", mw.Name(), err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			slog.Warn("pipeline retry", "attempt", attempt, "delay", p.retryDelay)
			time.Sleep(p.retryDelay)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		resp, _, err := p.outbound.FromUnified(req)
		if err != nil {
			lastErr = err
			for i := len(p.middlewares) - 1; i >= 0; i-- {
				lastErr = p.middlewares[i].OnError(ctx, lastErr)
			}
			continue
		}
		if p.emptyDetect && resp.Content == "" && len(resp.ToolCalls) == 0 {
			lastErr = fmt.Errorf("empty response")
			continue
		}
		for i := len(p.middlewares) - 1; i >= 0; i-- {
			resp, err = p.middlewares[i].OnResponse(ctx, resp)
			if err != nil {
				lastErr = err
				break
			}
		}
		if err != nil {
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("pipeline failed after %d attempts: %w", p.maxRetries+1, lastErr)
}

type simpleMiddleware struct {
	name       string
	onRequest  func(ctx context.Context, req *unified.Request) (*unified.Request, error)
	onResponse func(ctx context.Context, resp *unified.Response) (*unified.Response, error)
	onError    func(ctx context.Context, err error) error
}

func (m *simpleMiddleware) Name() string { return m.name }
func (m *simpleMiddleware) OnRequest(ctx context.Context, req *unified.Request) (*unified.Request, error) {
	if m.onRequest != nil {
		return m.onRequest(ctx, req)
	}
	return req, nil
}
func (m *simpleMiddleware) OnResponse(ctx context.Context, resp *unified.Response) (*unified.Response, error) {
	if m.onResponse != nil {
		return m.onResponse(ctx, resp)
	}
	return resp, nil
}
func (m *simpleMiddleware) OnError(ctx context.Context, err error) error {
	if m.onError != nil {
		return m.onError(ctx, err)
	}
	return err
}

func OnRequest(name string, fn func(ctx context.Context, req *unified.Request) (*unified.Request, error)) Middleware {
	return &simpleMiddleware{name: name, onRequest: fn}
}

func OnResponse(name string, fn func(ctx context.Context, resp *unified.Response) (*unified.Response, error)) Middleware {
	return &simpleMiddleware{name: name, onResponse: fn}
}

func OnError(name string, fn func(ctx context.Context, err error) error) Middleware {
	return &simpleMiddleware{name: name, onError: fn}
}
