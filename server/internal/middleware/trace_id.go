// Package middleware 提供请求级 trace-id 注入与传播。
package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const (
	// TraceIDHeader 响应头中返回的 trace id
	TraceIDHeader = "X-Trace-ID"

	// TraceIDKey gin.Context 中存储 trace id 的 key
	TraceIDKey = "trace_id"
)

// TraceID 为每个请求生成 8 字节 hex trace-id。
//
// 注入规则：
//   - 优先使用请求头 X-Trace-ID（允许上游传入，便于跨服务追踪）
//   - 无请求头时随机生成 8 字节
//   - 写入 gin.Context (c.GetString("trace_id"))
//   - 写入响应头 X-Trace-ID
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader(TraceIDHeader)
		if traceID == "" {
			traceID = generateTraceID()
		}
		c.Set(TraceIDKey, traceID)
		c.Header(TraceIDHeader, traceID)
		c.Next()
	}
}

func generateTraceID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GetTraceID 从 gin.Context 获取 trace id
func GetTraceID(c *gin.Context) string {
	id, _ := c.Get(TraceIDKey)
	if s, ok := id.(string); ok {
		return s
	}
	return ""
}
