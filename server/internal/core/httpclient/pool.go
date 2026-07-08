// Package httpclient 提供全局共享的 HTTP 客户端（连接池复用）。
// 所有协议 Provider 应通过此包获取 *http.Client，避免每次请求创建新客户端。
package httpclient

import (
	"net/http"
	"sync"
	"time"
)

var (
	pool     *http.Client
	poolOnce sync.Once
)

// Pool 返回全局共享的 *http.Client，配置了连接池参数。
// 适用于所有协议的流式请求（无固定超时）和同步请求（请配合 context.WithTimeout）。
func Pool() *http.Client {
	poolOnce.Do(func() {
		pool = &http.Client{
			Timeout: 10 * time.Minute, // 流式可能很长时间，用 context 控制
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				MaxConnsPerHost:     20,
			},
		}
	})
	return pool
}
