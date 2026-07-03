// Package prometheus 提供基于 Prometheus client_golang 的指标记录实现。
package prometheus

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// PrometheusRecorder 实现 monitor.MetricsRecorder，使用 Prometheus 指标
type PrometheusRecorder struct{}

// ── HTTP 指标 ──

var (
	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_gateway_http_request_duration_seconds",
		Help:    "HTTP 请求延迟（秒）",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"path", "method", "status_code"})

	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_gateway_http_requests_total",
		Help: "HTTP 请求总数",
	}, []string{"path", "method", "status_code"})

	httpActiveRequests = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ai_gateway_http_active_requests",
		Help: "当前活跃 HTTP 请求数",
	}, []string{"path", "method"})
)

// ── Relay 指标 ──

var (
	relayRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_gateway_relay_request_duration_seconds",
		Help:    "网关转发请求延迟（秒）",
		Buckets: []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120},
	}, []string{"model", "provider_model", "protocol", "upstream_protocol", "success"})

	relayRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_gateway_relay_requests_total",
		Help: "网关转发请求总数",
	}, []string{"model", "provider_model", "protocol", "upstream_protocol", "success"})

	relayTokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_gateway_relay_tokens_total",
		Help: "网关转发的 Token 总数",
	}, []string{"model", "provider_model", "token_type"})

	relayQuotaUsed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_gateway_relay_quota_used_total",
		Help: "网关转发消耗的配额总数",
	}, []string{"model", "provider_model"})
)

// ── Provider 指标 ──

var (
	providerStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ai_gateway_provider_status",
		Help: "Provider 状态 (1=启用, 0=禁用)",
	}, []string{"provider_id", "provider_name", "protocol"})

	providerResponseTime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ai_gateway_provider_response_time_ms",
		Help: "Provider 响应时间（毫秒）",
	}, []string{"provider_id", "provider_name", "protocol"})
)

// ── 错误指标 ──

var (
	errorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_gateway_errors_total",
		Help: "错误总数",
	}, []string{"error_type", "component"})
)

// ── 方法实现 ──

func (p *PrometheusRecorder) RecordHTTPRequest(startTime time.Time, path, method, statusCode string) {
	duration := time.Since(startTime).Seconds()
	httpRequestDuration.WithLabelValues(path, method, statusCode).Observe(duration)
	httpRequestsTotal.WithLabelValues(path, method, statusCode).Inc()
}

func (p *PrometheusRecorder) RecordHTTPActiveRequest(path, method string, delta float64) {
	httpActiveRequests.WithLabelValues(path, method).Add(delta)
}

func (p *PrometheusRecorder) RecordRelayRequest(startTime time.Time, model, providerModel, protocol, upstreamProtocol string, success bool, promptTokens, completionTokens, cachedTokens int) {
	duration := time.Since(startTime).Seconds()
	successStr := strconv.FormatBool(success)

	relayRequestDuration.WithLabelValues(model, providerModel, protocol, upstreamProtocol, successStr).Observe(duration)
	relayRequestsTotal.WithLabelValues(model, providerModel, protocol, upstreamProtocol, successStr).Inc()

	if promptTokens > 0 {
		relayTokensTotal.WithLabelValues(model, providerModel, "prompt").Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		relayTokensTotal.WithLabelValues(model, providerModel, "completion").Add(float64(completionTokens))
	}
	if cachedTokens > 0 {
		relayTokensTotal.WithLabelValues(model, providerModel, "cached").Add(float64(cachedTokens))
	}

	totalTokens := float64(promptTokens + completionTokens + cachedTokens)
	if totalTokens > 0 {
		relayQuotaUsed.WithLabelValues(model, providerModel).Add(totalTokens)
	}
}

func (p *PrometheusRecorder) UpdateProviderMetrics(providerID uint, providerName, protocol string, status int, responseTimeMs int64) {
	pid := strconv.FormatUint(uint64(providerID), 10)
	providerStatus.WithLabelValues(pid, providerName, protocol).Set(float64(status))
	providerResponseTime.WithLabelValues(pid, providerName, protocol).Set(float64(responseTimeMs))
}

func (p *PrometheusRecorder) RecordError(errorType, component string) {
	errorsTotal.WithLabelValues(errorType, component).Inc()
}
