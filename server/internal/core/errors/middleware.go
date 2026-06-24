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

// GinErrorReporter 自动上报 4xx/5xx 到终端日志
func GinErrorReporter() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		status := c.Writer.Status()
		if status < 400 {
			return
		}

		errs := c.Errors
		detail := ""
		if len(errs) > 0 {
			detail = errs.Last().Error()
		}

		method := c.Request.Method
		path := c.Request.URL.Path
		ip := c.ClientIP()

		if status >= 500 {
			Error("[%d] %s %s | %s | %s", status, method, path, ip, detail)
		} else if status == 404 {
			// 404 降为 DEBUG，太多噪音
			Debug("[%d] %s %s | %s", status, method, path, ip)
		} else {
			Warn("[%d] %s %s | %s | %s", status, method, path, ip, detail)
		}
	}
}
