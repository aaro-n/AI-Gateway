// Package errors 日志功能（基于标准库 log/slog）。
package errors

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"sync/atomic"
)

type Level int32

const (
	DEBUGLevel Level = -4
	INFOLevel  Level = 0
	WARNLevel  Level = 4
	ERRORLevel Level = 8
	FATALLevel Level = 12
)

func (l Level) String() string {
	switch l {
	case DEBUGLevel:
		return "DEBUG"
	case INFOLevel:
		return "INFO"
	case WARNLevel:
		return "WARN"
	case ERRORLevel:
		return "ERROR"
	case FATALLevel:
		return "FATAL"
	default:
		return fmt.Sprintf("LEVEL(%d)", l)
	}
}

func ParseLevel(s string) Level {
	switch s {
	case "debug":
		return DEBUGLevel
	case "info":
		return INFOLevel
	case "warn":
		return WARNLevel
	case "error":
		return ERRORLevel
	case "fatal":
		return FATALLevel
	default:
		return INFOLevel
	}
}

func toSlogLevel(l Level) slog.Level {
	switch {
	case l <= DEBUGLevel:
		return slog.LevelDebug
	case l <= INFOLevel:
		return slog.LevelInfo
	case l <= WARNLevel:
		return slog.LevelWarn
	case l <= ERRORLevel:
		return slog.LevelError
	default:
		return slog.LevelError
	}
}

var (
	gLogger  atomic.Value
	gHandler atomic.Value
)

type shWrapper struct {
	handler slog.Handler
	level   slog.Level
}

func init() {
	level := slog.LevelInfo
	if os.Getenv("AG_LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	gLogger.Store(slog.New(&ringBufferHandler{next: h}))
	gHandler.Store(&shWrapper{handler: h, level: level})
}

func logger() *slog.Logger { return gLogger.Load().(*slog.Logger) }

func SetLevel(l Level) {
	level := toSlogLevel(l)
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	gLogger.Store(slog.New(&ringBufferHandler{next: h}))
	gHandler.Store(&shWrapper{handler: h, level: level})
}

func GetLevel() Level {
	w := gHandler.Load().(*shWrapper)
	switch {
	case w.level <= slog.LevelDebug:
		return DEBUGLevel
	case w.level <= slog.LevelInfo:
		return INFOLevel
	case w.level <= slog.LevelWarn:
		return WARNLevel
	default:
		return ERRORLevel
	}
}

func SetOutput(w io.Writer) {
	if w == nil {
		return
	}
	sh := gHandler.Load().(*shWrapper)
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: sh.level})
	gLogger.Store(slog.New(&ringBufferHandler{next: h}))
	gHandler.Store(&shWrapper{handler: h, level: sh.level})
}

func kvsToAttrs(kvs ...string) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(kvs)/2+1)
	for i := 0; i+1 < len(kvs); i += 2 {
		attrs = append(attrs, slog.String(kvs[i], kvs[i+1]))
	}
	return attrs
}

func Debug(format string, args ...interface{}) {
	logger().Debug(fmt.Sprintf(format, args...))
}

func DebugKVs(message string, kvs ...string) {
	logger().LogAttrs(context.Background(), slog.LevelDebug, message, kvsToAttrs(kvs...)...)
}

func Info(format string, args ...interface{}) {
	logger().Info(fmt.Sprintf(format, args...))
}

func InfoKVs(message string, kvs ...string) {
	logger().LogAttrs(context.Background(), slog.LevelInfo, message, kvsToAttrs(kvs...)...)
}

func Warn(format string, args ...interface{}) {
	logger().Warn(fmt.Sprintf(format, args...))
}

func Error(format string, args ...interface{}) {
	logger().Error(fmt.Sprintf(format, args...))
}

func Fatal(format string, args ...interface{}) {
	logger().Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

func TraceDebug(traceID, format string, args ...interface{}) {
	logger().Debug(fmt.Sprintf(format, args...), "trace_id", traceID)
}

func TraceDebugKVs(traceID, message string, kvs ...string) {
	kvs = append(kvs, "trace_id", traceID)
	logger().LogAttrs(context.Background(), slog.LevelDebug, message, kvsToAttrs(kvs...)...)
}

func TraceInfo(traceID, format string, args ...interface{}) {
	logger().Info(fmt.Sprintf(format, args...), "trace_id", traceID)
}

func TraceInfoKVs(traceID, message string, kvs ...string) {
	kvs = append(kvs, "trace_id", traceID)
	logger().LogAttrs(context.Background(), slog.LevelInfo, message, kvsToAttrs(kvs...)...)
}

func TraceWarn(traceID, format string, args ...interface{}) {
	logger().Warn(fmt.Sprintf(format, args...), "trace_id", traceID)
}

func TraceError(traceID, format string, args ...interface{}) {
	logger().Error(fmt.Sprintf(format, args...), "trace_id", traceID)
}

func Recover() {
	if r := recover(); r != nil {
		stack := string(debug.Stack())
		var errMsg string
		switch v := r.(type) {
		case error:
			errMsg = v.Error()
		case string:
			errMsg = v
		default:
			errMsg = fmt.Sprintf("%v", v)
		}
		logger().Error("panic recovered", "error", errMsg, "stack", strings.ReplaceAll(stack, "\n", " | "))
	}
}
