package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter 基于滑动窗口的 API Key 级别速率限制器。
type RateLimiter struct {
	mu       sync.Mutex
	windows  map[uint]*slidingWindow // key_id → window
	maxReq   int                     // 窗口内最大请求数
	interval time.Duration           // 窗口时间长度
}

type slidingWindow struct {
	timestamps []time.Time
}

// NewRateLimiter 创建一个速率限制器。
// maxReq: 时间窗口内允许的最大请求数。
// interval: 时间窗口长度。
func NewRateLimiter(maxReq int, interval time.Duration) *RateLimiter {
	return &RateLimiter{
		windows:  make(map[uint]*slidingWindow),
		maxReq:   maxReq,
		interval: interval,
	}
}

// Middleware 返回 Gin 中间件，从 gin.Context 中读取 key_id 进行限流。
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		keyIDVal, exists := c.Get("key_id")
		if !exists {
			c.Next()
			return
		}
		keyID, ok := keyIDVal.(uint)
		if !ok {
			c.Next()
			return
		}
		if !rl.Allow(keyID) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// Allow 检查指定 key_id 是否允许通过。
func (rl *RateLimiter) Allow(keyID uint) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.maxReq <= 0 {
		return false
	}

	now := time.Now()
	window, exists := rl.windows[keyID]
	if !exists {
		rl.windows[keyID] = &slidingWindow{timestamps: []time.Time{now}}
		return true
	}

	// 清理窗口外的旧时间戳
	cutoff := now.Add(-rl.interval)
	valid := window.timestamps[:0]
	for _, ts := range window.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	window.timestamps = valid

	if len(window.timestamps) >= rl.maxReq {
		return false
	}

	window.timestamps = append(window.timestamps, now)
	return true
}

// 全局速率限制器（默认未启用）
var globalRateLimiter *RateLimiter

// SetGlobalRateLimiter 设置全局速率限制器。
// maxReq=0 表示禁用限流。
func SetGlobalRateLimiter(maxReq int, interval time.Duration) {
	if maxReq <= 0 {
		globalRateLimiter = nil
		return
	}
	globalRateLimiter = NewRateLimiter(maxReq, interval)
}

// GlobalRateLimiter 返回全局速率限制器中间件，如果未启用则返回空操作中间件。
func GlobalRateLimiter() gin.HandlerFunc {
	if globalRateLimiter == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return globalRateLimiter.Middleware()
}
