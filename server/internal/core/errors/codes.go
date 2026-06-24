package errors

import "fmt"

// ErrorCode 统一错误码
type ErrorCode struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// 通用错误 (1000-1999)
var (
	ErrInternal     = &ErrorCode{Code: 1000, Message: "internal server error"}
	ErrBadRequest   = &ErrorCode{Code: 1001, Message: "bad request"}
	ErrUnauthorized = &ErrorCode{Code: 1002, Message: "unauthorized"}
	ErrForbidden    = &ErrorCode{Code: 1003, Message: "forbidden"}
	ErrNotFound     = &ErrorCode{Code: 1004, Message: "not found"}
	ErrConflict     = &ErrorCode{Code: 1005, Message: "conflict"}
	ErrTimeout      = &ErrorCode{Code: 1006, Message: "request timeout"}
)

// 认证相关 (2000-2999)
var (
	ErrInvalidAPIKey  = &ErrorCode{Code: 2001, Message: "invalid API key"}
	ErrKeyExpired     = &ErrorCode{Code: 2002, Message: "API key expired"}
	ErrKeyFormatWrong = &ErrorCode{Code: 2003, Message: "key format not allowed on this endpoint"}
	ErrMissingAuth    = &ErrorCode{Code: 2004, Message: "missing authorization"}
)

// 路由相关 (3000-3999)
var (
	ErrModelNotFound       = &ErrorCode{Code: 3001, Message: "model not found"}
	ErrNoMapping           = &ErrorCode{Code: 3002, Message: "no model mapping found"}
	ErrNoProvider          = &ErrorCode{Code: 3003, Message: "no available provider"}
	ErrUnsupportedProtocol = &ErrorCode{Code: 3004, Message: "unsupported protocol"}
	ErrCrossProtocol       = &ErrorCode{Code: 3005, Message: "cross-protocol not supported from this entry"}
)

// Provider 相关 (4000-4999)
var (
	ErrProviderDown       = &ErrorCode{Code: 4001, Message: "upstream provider unavailable"}
	ErrRateLimited        = &ErrorCode{Code: 4002, Message: "rate limited by upstream provider"}
	ErrConversionFailed   = &ErrorCode{Code: 4003, Message: "protocol conversion failed"}
	ErrProviderTestFailed = &ErrorCode{Code: 4004, Message: "provider connection test failed"}
)

// GatewayError 网关统一错误
type GatewayError struct {
	Code   *ErrorCode `json:"error_code"`
	Detail string     `json:"detail,omitempty"`
	Raw    error      `json:"-"`
}

func (e *GatewayError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("[%d] %s: %s", e.Code.Code, e.Code.Message, e.Detail)
	}
	return fmt.Sprintf("[%d] %s", e.Code.Code, e.Code.Message)
}

// Unwrap 支持 errors.Is / errors.As
func (e *GatewayError) Unwrap() error {
	return e.Raw
}

// New 创建带详情的错误
func New(code *ErrorCode, detail string) *GatewayError {
	return &GatewayError{Code: code, Detail: detail}
}

// Newf 创建格式化详情的错误
func Newf(code *ErrorCode, format string, args ...interface{}) *GatewayError {
	return &GatewayError{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// Wrap 包装底层错误
func Wrap(code *ErrorCode, err error) *GatewayError {
	return &GatewayError{Code: code, Detail: err.Error(), Raw: err}
}
