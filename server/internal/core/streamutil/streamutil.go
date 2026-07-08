// Package streamutil 提供 SSE 流处理的共享工具函数。
// 所有协议 Provider 应使用本包的常量与函数，而非各自手写重复代码。
package streamutil

import (
	"context"
	"strings"

	"ai-gateway/internal/core/unified"
)

// BufferSize 是流事件 channel 的默认缓冲区大小。
const BufferSize = 32

// SendEvent 向 ch 发送一个事件，同时尊重 ctx 取消（用于背压控制）。
// 返回 true 表示发送成功，false 表示 ctx 已取消。
func SendEvent(ctx context.Context, ch chan<- unified.StreamEvent, event unified.StreamEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// TrimSSELine 修剪 SSE 行并检查 "data:" 前缀。
// 返回 data 内容和是否有效。
func TrimSSELine(line string) (data string, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	return data, true
}

// Truncate 截断字符串用于安全日志输出。
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
