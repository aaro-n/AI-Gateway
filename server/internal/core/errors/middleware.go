package errors

import (
	"github.com/gin-gonic/gin"
)

// GinRecovery panic 恢复 Gin 中间件
func GinRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				Recover()
				c.AbortWithStatusJSON(500, gin.H{
					"error": gin.H{
						"code":    ErrInternal.Code,
						"message": ErrInternal.Message,
					},
				})
			}
		}()
		c.Next()
	}
}

// GinErrorReporter 自动上报 4xx/5xx 到日志（带 Trace ID）
func GinErrorReporter() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		status := c.Writer.Status()
		if status < 400 {
			return
		}

		traceID := c.GetString("trace_id")
		errs := c.Errors
		detail := ""
		if len(errs) > 0 {
			detail = errs.Last().Error()
		}

		method := c.Request.Method
		path := c.Request.URL.Path
		ip := c.ClientIP()

		if status >= 500 {
			TraceError(traceID, "[%d] %s %s | %s | %s", status, method, path, ip, detail)
		} else if status == 404 {
			// 404 降级为 DEBUG
			TraceDebug(traceID, "[%d] %s %s | %s", status, method, path, ip)
		} else {
			TraceWarn(traceID, "[%d] %s %s | %s | %s", status, method, path, ip, detail)
		}
	}
}
