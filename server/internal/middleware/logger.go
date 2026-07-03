package middleware

import (
	"bytes"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	coreErrors "ai-gateway/internal/core/errors"
)

// bodyLogWriter 捕获响应体以便日志记录
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		traceID := GetTraceID(c)

		// 读取并恢复请求体
		var reqBody []byte
		if c.Request.Body != nil {
			reqBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewReader(reqBody))
		}

		// 拦截响应体
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()

		trimmedReq := trimBody(string(reqBody))
		trimmedResp := trimBody(blw.body.String())

		if status >= 400 {
			coreErrors.TraceError(traceID,
				"http_request method=%s path=%s status=%d latency=%v client_ip=%s req=%q resp=%q errors=%q",
				method, path, status, latency, clientIP, trimmedReq, trimmedResp, c.Errors.String())
		} else {
			coreErrors.TraceDebug(traceID,
				"http_request method=%s path=%s status=%d latency=%v client_ip=%s req=%q resp=%q",
				method, path, status, latency, clientIP, trimmedReq, trimmedResp)
		}
	}
}

func trimBody(s string) string {
	s = stringsReplace(s, "\n", " ")
	s = stringsReplace(s, "\r", "")
	s = stringsReplace(s, "\t", " ")
	if len(s) > 600 {
		return s[:600] + "..."
	}
	return s
}

func stringsReplace(s, old, new string) string {
	return strings.ReplaceAll(s, old, new)
}
