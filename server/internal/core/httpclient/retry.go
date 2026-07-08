package httpclient

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v5"
)

// RetryConfig 重试配置。
type RetryConfig struct {
	MaxRetries     uint64
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	RequestTimeout time.Duration
}

func defaults(cfg *RetryConfig) *RetryConfig {
	if cfg == nil {
		cfg = &RetryConfig{}
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.InitialBackoff == 0 {
		cfg.InitialBackoff = 500 * time.Millisecond
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 10 * time.Second
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 2 * time.Minute
	}
	return cfg
}

// DoWithRetry 使用指数退避执行 HTTP 请求，自动重试临时性错误。
func DoWithRetry(req *http.Request, cfg *RetryConfig) (*http.Response, error) {
	cfg = defaults(cfg)

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = cfg.InitialBackoff
	bo.MaxInterval = cfg.MaxBackoff
	maxElapsed := cfg.RequestTimeout * time.Duration(cfg.MaxRetries+1)

	ctx, cancel := context.WithTimeout(req.Context(), maxElapsed)
	defer cancel()

	var resp *http.Response
	_, err := backoff.Retry(ctx, func() (*http.Response, error) {
		r := req.Clone(ctx)
		res, err := Pool().Do(r)
		if err != nil {
			return nil, err
		}
		if isRetryableStatus(res.StatusCode) {
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			return nil, ErrRetryableStatus{res.StatusCode}
		}
		return res, nil
	},
		backoff.WithBackOff(bo),
		backoff.WithMaxElapsedTime(maxElapsed),
	)
	return resp, err
}

type ErrRetryableStatus struct{ Code int }

func (e ErrRetryableStatus) Error() string { return http.StatusText(e.Code) }

func isRetryableStatus(code int) bool {
	return code == 429 || code == 502 || code == 503 || code == 504
}
