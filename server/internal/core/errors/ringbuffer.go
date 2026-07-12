package errors

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// LogEntry 单条日志记录
// Timestamp 存 RFC3339 UTC 字符串（如 "2026-07-12T09:00:00.000Z"），
// 前端按用户时区（User.TimeZone）用 Intl.DateTimeFormat 格式化显示。
// RFC3339 字符串定长可排序，可直接用字符串比较做增量过滤。
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	TraceID   string `json:"trace_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

const defaultRingSize = 500

var (
	ringMu      sync.RWMutex
	ringEntries []LogEntry
	ringEnabled bool
	ringSize    int = defaultRingSize
)

// EnableRingBuffer 启用内存环形日志缓冲区
func EnableRingBuffer(size int) {
	ringMu.Lock()
	defer ringMu.Unlock()
	ringEnabled = true
	if size > 0 {
		ringSize = size
	}
	ringEntries = make([]LogEntry, 0, ringSize)
}

// GetRingBufferEntries 获取最近 limit 条日志（默认50）
func GetRingBufferEntries(limit int) []LogEntry {
	ringMu.RLock()
	defer ringMu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	if len(ringEntries) <= limit {
		result := make([]LogEntry, len(ringEntries))
		copy(result, ringEntries)
		return result
	}
	start := len(ringEntries) - limit
	result := make([]LogEntry, limit)
	copy(result, ringEntries[start:])
	return result
}

// GetRingBufferEntriesSince 获取指定时间戳之后的新增日志（用于增量轮询）
func GetRingBufferEntriesSince(since string) []LogEntry {
	ringMu.RLock()
	defer ringMu.RUnlock()
	if since == "" {
		return GetRingBufferEntries(50)
	}
	result := make([]LogEntry, 0)
	for _, e := range ringEntries {
		if e.Timestamp > since {
			result = append(result, e)
		}
	}
	return result
}

// ringBufferHandler 包装 slog.Handler，将日志同时写入环形缓冲区
type ringBufferHandler struct {
	next slog.Handler
}

func (h *ringBufferHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *ringBufferHandler) Handle(ctx context.Context, r slog.Record) error {
	// 推入环形缓冲区
	appendToRing(r)
	// 传递给下游 handler
	return h.next.Handle(ctx, r)
}

func (h *ringBufferHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ringBufferHandler{next: h.next.WithAttrs(attrs)}
}

func (h *ringBufferHandler) WithGroup(name string) slog.Handler {
	return &ringBufferHandler{next: h.next.WithGroup(name)}
}

func appendToRing(r slog.Record) {
	ringMu.Lock()
	defer ringMu.Unlock()

	if !ringEnabled {
		return
	}

	entry := LogEntry{
		Timestamp: r.Time.UTC().Format(time.RFC3339Nano),
		Level:     r.Level.String(),
		Message:   r.Message,
	}

	// 提取额外属性
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "trace_id" {
			entry.TraceID = a.Value.String()
		}
		return true
	})

	if len(ringEntries) >= ringSize {
		// 环形覆盖：移除最旧条目
		ringEntries = ringEntries[1:]
	}
	ringEntries = append(ringEntries, entry)
}

// pushRingEntry 供外部直接写入日志到环形缓冲区（非 slog 路径）
func PushRingEntry(level, message, traceID string) {
	ringMu.Lock()
	defer ringMu.Unlock()

	if !ringEnabled {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level,
		Message:   message,
		TraceID:   traceID,
	}

	if len(ringEntries) >= ringSize {
		ringEntries = ringEntries[1:]
	}
	ringEntries = append(ringEntries, entry)
}
