package registry

import "fmt"

// ErrorDetail 统一错误详情。
type ErrorDetail struct {
	Message   string `json:"message"`
	Type      string `json:"type"`
	Param     string `json:"param,omitempty"`
	Code      string `json:"code,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// ResponseError 统一上游错误响应，所有协议可转为该结构。
type ResponseError struct {
	StatusCode int         `json:"-"`
	Detail     ErrorDetail `json:"error"`
}

func (e *ResponseError) Error() string {
	if e == nil {
		return "response error"
	}
	return fmt.Sprintf("[%d] %s: %s", e.StatusCode, e.Detail.Type, e.Detail.Message)
}

// NewResponseError 快速构建统一错误。
func NewResponseError(statusCode int, msg, typ string) *ResponseError {
	return &ResponseError{
		StatusCode: statusCode,
		Detail: ErrorDetail{
			Message: msg,
			Type:    typ,
		},
	}
}
