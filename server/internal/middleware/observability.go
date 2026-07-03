// Package middleware 提供可观测性 HTTP 中间件
package middleware

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/monitor"
)

// PrometheusMiddleware 为每个 HTTP 请求记录 Prometheus 指标。
//
// 指标包括：
//   - ai_gateway_http_active_requests      → 当前活跃请求数（gauge）
//   - ai_gateway_http_request_duration_seconds → 请求延迟分布（histogram）
//   - ai_gateway_http_requests_total        → 请求计数（counter）
//
// 路径会做规范化处理，避免高基数问题（如 /api/v1/keys/123 → /api/v1/keys/:id）。
func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		normalizedPath := normalizePath(path)

		// 增加活跃请求计数
		monitor.GlobalRecorder.RecordHTTPActiveRequest(normalizedPath, method, 1)
		defer monitor.GlobalRecorder.RecordHTTPActiveRequest(normalizedPath, method, -1)

		c.Next()

		// 记录请求延迟和状态
		statusCode := strconv.Itoa(c.Writer.Status())
		monitor.GlobalRecorder.RecordHTTPRequest(start, normalizedPath, method, statusCode)
	}
}

// normalizePath 将路径中的动态参数替换为占位符，降低指标基数。
//
// 规则：
//   - UUID / 数字 ID → :id
//   - 保持 API 前缀如 /api/v1/keys/:id
func normalizePath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "" {
			continue
		}

		// UUID 格式: 8-4-4-4-12
		if len(p) == 36 && strings.Count(p, "-") == 4 {
			parts[i] = ":id"
			continue
		}
		// 纯数字
		if isNumeric(p) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
