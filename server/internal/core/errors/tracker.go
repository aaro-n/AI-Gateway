package errors

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// Level 日志级别（参考 slog）
type Level int32

const (
	DEBUGLevel Level = iota - 4 // -4
	INFOLevel                   // 0
	WARNLevel                   // 4
	ERRORLevel                  // 8
	FATALLevel                  // 12
)

// String 返回级别名称
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

// ParseLevel 从字符串解析级别
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

// ── 全局最低输出级别 ──

var gLevel = new(atomic.Int32)

func init() {
	gLevel.Store(int32(INFOLevel)) // 默认 INFO 级别
}

// SetLevel 设置全局最低输出级别
// 部署测试时可通过环境变量控制：AG_LOG_LEVEL=debug
func SetLevel(l Level) {
	gLevel.Store(int32(l))
}

// GetLevel 获取当前最低输出级别
func GetLevel() Level {
	return Level(gLevel.Load())
}

// ── 终端日志输出（所有级别都输出，受 SetLevel 控制）──

var (
	stdoutLog = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	stderrLog = log.New(os.Stderr, "", log.Ldate|log.Ltime|log.Lmicroseconds)
)

func logf(level Level, format string, args ...interface{}) {
	if level < GetLevel() {
		return // 低于最低级别，不输出
	}

	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] %s", level.String(), msg)

	if level >= WARNLevel {
		stderrLog.Println(line)
	} else {
		stdoutLog.Println(line)
	}
}

// ── 公开 API ──

// Debug 调试信息
// 示例: errors.Debug("incoming request: %s %s", method, path)
func Debug(format string, args ...interface{}) {
	logf(DEBUGLevel, format, args...)
}

// Info 常规信息
// 示例: errors.Info("provider %s synced %d models", name, count)
func Info(format string, args ...interface{}) {
	logf(INFOLevel, format, args...)
}

// Warn 警告（不影响服务但需关注）
// 示例: errors.Warn("rate limited by %s, cooldown until %s", provider, until)
func Warn(format string, args ...interface{}) {
	logf(WARNLevel, format, args...)
}

// Error 错误（服务受影响）
// 示例: errors.Error("upstream %s unreachable: %v", provider, err)
func Error(format string, args ...interface{}) {
	logf(ERRORLevel, format, args...)
}

// Fatal 致命错误并退出（仅用于启动阶段不可恢复错误）
// 示例: errors.Fatal("cannot connect to database: %v", err)
func Fatal(format string, args ...interface{}) {
	logf(FATALLevel, format, args...)
	os.Exit(1)
}

// ── Panic 恢复 ──

// Recover 必须在 defer 中调用，捕获 panic 并记录堆栈
// 用法: defer errors.Recover()
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

		logf(FATALLevel, "PANIC RECOVERED: %s\n%s", errMsg, stack)
	}
}

// ── 带上下文的日志 ──

// LogEntry 带请求上下文的日志条目
type LogEntry struct {
	Level    string    `json:"level"`
	Time     time.Time `json:"time"`
	Message  string    `json:"message"`
	Path     string    `json:"path,omitempty"`
	Method   string    `json:"method,omitempty"`
	ClientIP string    `json:"client_ip,omitempty"`
}

// RequestLog 记录一条带请求上下文的日志
func RequestLog(level Level, path, method, clientIP, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if level < GetLevel() {
		return
	}
	line := fmt.Sprintf("[%s] %s | %s %s | %s", level.String(), msg, method, path, clientIP)
	logf(level, "%s", line)
}
