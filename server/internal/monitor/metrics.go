// Package monitor 提供可观测性接口与指标记录基础设施。
//
// 核心设计：
//   - MetricsRecorder 接口定义所有可记录的指标操作
//   - GlobalRecorder 是全局单例，默认指向 NoOpRecorder
//   - InitMonitoring() 根据配置选择 Prometheus / OpenTelemetry / MultiRecorder
package monitor

import "time"

// MetricsRecorder 定义所有可记录指标的操作接口。
// 所有方法必须线程安全。
type MetricsRecorder interface {
	// ── HTTP 指标 ──

	// RecordHTTPRequest 记录一次 HTTP 请求的延迟与状态
	RecordHTTPRequest(startTime time.Time, path, method, statusCode string)
	// RecordHTTPActiveRequest 记录当前活跃 HTTP 请求数（delta 为正 = 增加, 负 = 减少）
	RecordHTTPActiveRequest(path, method string, delta float64)

	// ── 网关 Relay 指标 ──

	// RecordRelayRequest 记录一次 API 网关转发请求
	// model 为请求的目标模型别名，providerModel 为实际调用的上游模型ID
	// protocol 为入口协议 (openai/anthropic/gemini/deepseek/openrouter)
	// upstreamProtocol 为实际上游协议
	// success 为是否成功
	RecordRelayRequest(startTime time.Time, model, providerModel, protocol, upstreamProtocol string, success bool, promptTokens, completionTokens, cachedTokens int)

	// ── Provider 指标 ──

	// UpdateProviderMetrics 更新 Provider 的健康状态指标
	UpdateProviderMetrics(providerID uint, providerName, protocol string, status int, responseTimeMs int64)

	// ── 错误指标 ──

	// RecordError 记录一个错误
	RecordError(errorType, component string)
}

// GlobalRecorder 全局指标记录器，默认 NoOp。
// 在 InitMonitoring() 中被替换为实际实现。
var GlobalRecorder MetricsRecorder = &NoOpRecorder{}

// NoOpRecorder 不记录任何指标的空实现
type NoOpRecorder struct{}

func (n *NoOpRecorder) RecordHTTPRequest(startTime time.Time, path, method, statusCode string) {}
func (n *NoOpRecorder) RecordHTTPActiveRequest(path, method string, delta float64)             {}
func (n *NoOpRecorder) RecordRelayRequest(startTime time.Time, model, providerModel, protocol, upstreamProtocol string, success bool, promptTokens, completionTokens, cachedTokens int) {
}
func (n *NoOpRecorder) UpdateProviderMetrics(providerID uint, providerName, protocol string, status int, responseTimeMs int64) {
}
func (n *NoOpRecorder) RecordError(errorType, component string) {}

// MultiRecorder 将指标同时广播到多个 Recorder
type MultiRecorder struct {
	Recorders []MetricsRecorder
}

func (m *MultiRecorder) RecordHTTPRequest(startTime time.Time, path, method, statusCode string) {
	for _, r := range m.Recorders {
		r.RecordHTTPRequest(startTime, path, method, statusCode)
	}
}
func (m *MultiRecorder) RecordHTTPActiveRequest(path, method string, delta float64) {
	for _, r := range m.Recorders {
		r.RecordHTTPActiveRequest(path, method, delta)
	}
}
func (m *MultiRecorder) RecordRelayRequest(startTime time.Time, model, providerModel, protocol, upstreamProtocol string, success bool, promptTokens, completionTokens, cachedTokens int) {
	for _, r := range m.Recorders {
		r.RecordRelayRequest(startTime, model, providerModel, protocol, upstreamProtocol, success, promptTokens, completionTokens, cachedTokens)
	}
}
func (m *MultiRecorder) UpdateProviderMetrics(providerID uint, providerName, protocol string, status int, responseTimeMs int64) {
	for _, r := range m.Recorders {
		r.UpdateProviderMetrics(providerID, providerName, protocol, status, responseTimeMs)
	}
}
func (m *MultiRecorder) RecordError(errorType, component string) {
	for _, r := range m.Recorders {
		r.RecordError(errorType, component)
	}
}
