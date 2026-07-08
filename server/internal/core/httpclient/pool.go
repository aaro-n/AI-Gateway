// Package httpclient 提供全局共享的 HTTP 客户端（连接池复用）。
// 所有协议 Provider 应通过此包获取 *http.Client，避免每次请求创建新客户端。
package httpclient

import (
	"net/http"
	"sync"
	"time"
)

// PoolConfig 可配置的 HTTP 连接池参数。
type PoolConfig struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	RequestTimeout      time.Duration
}

// DefaultPoolConfig 返回合理的默认值。
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     90 * time.Second,
		RequestTimeout:      10 * time.Minute,
	}
}

var (
	pool       *http.Client
	poolConfig *PoolConfig
	poolOnce   sync.Once
	poolMu     sync.RWMutex
)

// ConfigurePool 在服务启动时调用，设置 HTTP 连接池参数。
// 必须在首次使用 Pool() 之前调用。
func ConfigurePool(cfg PoolConfig) {
	poolMu.Lock()
	defer poolMu.Unlock()
	c := cfg
	poolConfig = &c
}

// Pool 返回全局共享的 *http.Client，配置了连接池参数。
// 适用于所有协议的流式请求（无固定超时）和同步请求（请配合 context.WithTimeout）。
func Pool() *http.Client {
	poolOnce.Do(func() {
		cfg := DefaultPoolConfig()
		poolMu.RLock()
		if poolConfig != nil {
			cfg = *poolConfig
		}
		poolMu.RUnlock()

		pool = &http.Client{
			Timeout: cfg.RequestTimeout, // 流式可能很长时间，用 context 控制
			Transport: &http.Transport{
				MaxIdleConns:        cfg.MaxIdleConns,
				MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
				IdleConnTimeout:     cfg.IdleConnTimeout,
				MaxConnsPerHost:     cfg.MaxConnsPerHost,
			},
		}
	})
	return pool
}
