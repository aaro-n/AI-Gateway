package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// UpstreamError 标记上游供应商返回的错误，与本地错误（参数校验、路由失败等）区分。
// 熔断器、重试逻辑可通过 errors.As 判断是否应针对上游重试。
type UpstreamError struct {
	StatusCode int
	Body       []byte
	Err        error
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "upstream error"
	}
	if e.Err != nil {
		return fmt.Sprintf("upstream %d: %s", e.StatusCode, e.Err.Error())
	}
	return fmt.Sprintf("upstream %d: %s", e.StatusCode, string(e.Body))
}

func (e *UpstreamError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsRateLimit 判定上游是否返回速率限制（429 或 529）。
func (e *UpstreamError) IsRateLimit() bool {
	return e != nil && (e.StatusCode == http.StatusTooManyRequests || e.StatusCode == 529)
}

// IsRetryable 判定上游错误是否可以重试（5xx 或 429）。
func (e *UpstreamError) IsRetryable() bool {
	if e == nil {
		return false
	}
	return e.StatusCode >= 500 || e.StatusCode == http.StatusTooManyRequests || e.StatusCode == 529
}

// WrapUpstreamError 包装上游错误，避免重复包装。
func WrapUpstreamError(statusCode int, body []byte, err error) error {
	if err == nil {
		return nil
	}
	var upstream *UpstreamError
	if errors.As(err, &upstream) {
		return err
	}
	return &UpstreamError{StatusCode: statusCode, Body: body, Err: err}
}

// IsUpstreamError 判断是否为上游错误。
func IsUpstreamError(err error) bool {
	var upstream *UpstreamError
	return errors.As(err, &upstream)
}
