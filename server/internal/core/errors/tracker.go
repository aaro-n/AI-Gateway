package errors

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"strings"
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

// ── 输出目标 ──

var (
	logWriter  io.Writer = os.Stdout
	stdoutLog  *log.Logger
	stderrLog  *log.Logger
	discardLog = log.New(io.Discard, "", 0)
)

func init() {
	rebuildLoggers()
}

// SetOutput 设置日志输出目标（可传入 os.File 实现文件日志）
func SetOutput(w io.Writer) {
	if w == nil {
		return
	}
	logWriter = w
	rebuildLoggers()
}

func rebuildLoggers() {
	stdoutLog = log.New(logWriter, "", 0)
	stderrLog = log.New(logWriter, "", 0)
}

// ── 核心日志函数 ──

// 内部日志格式（统一前缀）：
//   时间戳 [级别] [traceID] 组件=xxx key=value ... 消息

func logf(level Level, traceID string, format string, args ...interface{}) {
	if level < GetLevel() {
		return
	}

	msg := fmt.Sprintf(format, args...)
	now := time.Now().Format("2006-01-02 15:04:05.000")

	prefix := fmt.Sprintf("%s [%-5s]", now, level.String())
	if traceID != "" {
		prefix += fmt.Sprintf(" [%s]", traceID)
	}

	line := prefix + " " + msg

	if level >= WARNLevel {
		stderrLog.Println(line)
	} else {
		stdoutLog.Println(line)
	}
}

// logKVs 带结构化 key=value 的日志
// 示例: errors.DebugKVs("gateway routing", "model", "gpt-4o", "provider_model", "gpt-4o-2024", "protocol", "openai")
func logKVs(level Level, traceID, message string, kvs ...string) {
	if level < GetLevel() {
		return
	}

	var sb strings.Builder
	sb.WriteString(message)

	for i := 0; i+1 < len(kvs); i += 2 {
		sb.WriteString(" ")
		sb.WriteString(kvs[i])
		sb.WriteString("=")
		sb.WriteString(kvs[i+1])
	}

	// 如果 kvs 数量为奇数，最后一个 key 视为标签
	if len(kvs)%2 == 1 {
		sb.WriteString(" ")
		sb.WriteString(kvs[len(kvs)-1])
	}

	logf(level, traceID, "%s", sb.String())
}

// ── 公开 API ──

// Debug 调试信息
func Debug(format string, args ...interface{}) {
	logf(DEBUGLevel, "", format, args...)
}

// DebugKVs 带 key=value 的结构化调试日志
func DebugKVs(message string, kvs ...string) {
	logKVs(DEBUGLevel, "", message, kvs...)
}

// Info 常规信息
func Info(format string, args ...interface{}) {
	logf(INFOLevel, "", format, args...)
}

// InfoKVs 带 key=value 的结构化信息日志
func InfoKVs(message string, kvs ...string) {
	logKVs(INFOLevel, "", message, kvs...)
}

// Warn 警告（不影响服务但需关注）
func Warn(format string, args ...interface{}) {
	logf(WARNLevel, "", format, args...)
}

// Error 错误（服务受影响）
func Error(format string, args ...interface{}) {
	logf(ERRORLevel, "", format, args...)
}

// Fatal 致命错误并退出（仅用于启动阶段不可恢复错误）
func Fatal(format string, args ...interface{}) {
	logf(FATALLevel, "", format, args...)
	os.Exit(1)
}

// ── 带 Trace ID 的日志 API（用于请求处理链路）──

// TraceDebug 带 trace_id 的调试日志
func TraceDebug(traceID, format string, args ...interface{}) {
	logf(DEBUGLevel, traceID, format, args...)
}

// TraceDebugKVs 带 trace_id + key=value 的结构化调试日志
func TraceDebugKVs(traceID, message string, kvs ...string) {
	logKVs(DEBUGLevel, traceID, message, kvs...)
}

// TraceInfo 带 trace_id 的信息日志
func TraceInfo(traceID, format string, args ...interface{}) {
	logf(INFOLevel, traceID, format, args...)
}

// TraceInfoKVs 带 trace_id + key=value 的结构化信息日志
func TraceInfoKVs(traceID, message string, kvs ...string) {
	logKVs(INFOLevel, traceID, message, kvs...)
}

// TraceWarn 带 trace_id 的告警日志
func TraceWarn(traceID, format string, args ...interface{}) {
	logf(WARNLevel, traceID, format, args...)
}

// TraceError 带 trace_id 的错误日志
func TraceError(traceID, format string, args ...interface{}) {
	logf(ERRORLevel, traceID, format, args...)
}

// ── Panic 恢复 ──

// Recover 必须在 defer 中调用，捕获 panic 并记录堆栈
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

		logf(FATALLevel, "", "PANIC RECOVERED: %s\n%s", errMsg, stack)
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
	logf(level, "", "%s", line)
}
