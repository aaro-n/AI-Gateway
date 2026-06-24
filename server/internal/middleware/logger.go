package middleware

import (
	"bytes"
	"io"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

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

		// Read and restore request body for more detailed log
		var reqBody []byte
		if c.Request.Body != nil {
			reqBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewReader(reqBody))
		}

		// Intercept response body
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()

		trimmedReqBody := stringsTrim(string(reqBody))
		trimmedRespBody := stringsTrim(blw.body.String())

		if status >= 400 {
			log.Printf("[API Detailed Error] %s %s %d %v %s | Req: %s | Resp: %s | Errors: %v",
				method, path, status, latency, clientIP, trimmedReqBody, trimmedRespBody, c.Errors.String())
		} else {
			log.Printf("[API Detailed Info] %s %s %d %v %s | Req: %s | Resp: %s",
				method, path, status, latency, clientIP, trimmedReqBody, trimmedRespBody)
		}
	}
}

func stringsTrim(s string) string {
	s = stringsReplaceAll(s, "\n", " ")
	s = stringsReplaceAll(s, "\r", "")
	s = stringsReplaceAll(s, "\t", " ")
	if len(s) > 1000 {
		return s[:1000] + "... (truncated)"
	}
	return s
}

func stringsReplaceAll(s, old, new string) string {
	return stringsReplace(s, old, new, -1)
}

func stringsReplace(s, old, new string, n int) string {
	return strings.Replace(s, old, new, n)
}
