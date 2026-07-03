// Package otel 提供基于 OpenTelemetry 的指标记录实现。
package otel

import (
	"context"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// bgCtx 用于指标记录——指标不绑定到请求生命周期
var bgCtx = context.Background()

// OtelRecorder 实现 monitor.MetricsRecorder，使用 OpenTelemetry 指标
type OtelRecorder struct {
	meter metric.Meter

	// HTTP
	httpRequestDuration metric.Float64Histogram
	httpRequestsTotal   metric.Int64Counter
	httpActiveRequests  metric.Float64UpDownCounter

	// Relay
	relayRequestDuration metric.Float64Histogram
	relayRequestsTotal   metric.Int64Counter
	relayTokensTotal     metric.Int64Counter
	relayQuotaUsed       metric.Float64Counter

	// Provider
	providerStatus       metric.Int64Gauge
	providerResponseTime metric.Int64Gauge

	// Errors
	errorsTotal metric.Int64Counter
}

// NewOtelRecorder 创建并初始化 OpenTelemetry Recorder
func NewOtelRecorder() (*OtelRecorder, error) {
	meter := otel.Meter("ai-gateway")
	r := &OtelRecorder{meter: meter}

	var err error
	if r.httpRequestDuration, err = meter.Float64Histogram(
		"ai_gateway_http_request_duration_seconds",
		metric.WithDescription("HTTP 请求延迟（秒）"),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}
	if r.httpRequestsTotal, err = meter.Int64Counter(
		"ai_gateway_http_requests_total",
		metric.WithDescription("HTTP 请求总数"),
	); err != nil {
		return nil, err
	}
	if r.httpActiveRequests, err = meter.Float64UpDownCounter(
		"ai_gateway_http_active_requests",
		metric.WithDescription("当前活跃 HTTP 请求数"),
	); err != nil {
		return nil, err
	}

	if r.relayRequestDuration, err = meter.Float64Histogram(
		"ai_gateway_relay_request_duration_seconds",
		metric.WithDescription("网关转发请求延迟（秒）"),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}
	if r.relayRequestsTotal, err = meter.Int64Counter(
		"ai_gateway_relay_requests_total",
		metric.WithDescription("网关转发请求总数"),
	); err != nil {
		return nil, err
	}
	if r.relayTokensTotal, err = meter.Int64Counter(
		"ai_gateway_relay_tokens_total",
		metric.WithDescription("网关转发的 Token 总数"),
	); err != nil {
		return nil, err
	}
	if r.relayQuotaUsed, err = meter.Float64Counter(
		"ai_gateway_relay_quota_used_total",
		metric.WithDescription("网关转发消耗的配额总数"),
	); err != nil {
		return nil, err
	}

	if r.providerStatus, err = meter.Int64Gauge(
		"ai_gateway_provider_status",
		metric.WithDescription("Provider 状态 (1=启用, 0=禁用)"),
	); err != nil {
		return nil, err
	}
	if r.providerResponseTime, err = meter.Int64Gauge(
		"ai_gateway_provider_response_time_ms",
		metric.WithDescription("Provider 响应时间（毫秒）"),
		metric.WithUnit("ms"),
	); err != nil {
		return nil, err
	}

	if r.errorsTotal, err = meter.Int64Counter(
		"ai_gateway_errors_total",
		metric.WithDescription("错误总数"),
	); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *OtelRecorder) RecordHTTPRequest(startTime time.Time, path, method, statusCode string) {
	duration := time.Since(startTime).Seconds()
	attrs := []attribute.KeyValue{
		attribute.String("path", path),
		attribute.String("method", method),
		attribute.String("status_code", statusCode),
	}
	r.httpRequestDuration.Record(bgCtx, duration, metric.WithAttributes(attrs...))
	r.httpRequestsTotal.Add(bgCtx, 1, metric.WithAttributes(attrs...))
}

func (r *OtelRecorder) RecordHTTPActiveRequest(path, method string, delta float64) {
	attrs := []attribute.KeyValue{
		attribute.String("path", path),
		attribute.String("method", method),
	}
	r.httpActiveRequests.Add(bgCtx, delta, metric.WithAttributes(attrs...))
}

func (r *OtelRecorder) RecordRelayRequest(startTime time.Time, model, providerModel, protocol, upstreamProtocol string, success bool, promptTokens, completionTokens, cachedTokens int) {
	duration := time.Since(startTime).Seconds()
	successStr := strconv.FormatBool(success)

	baseAttrs := []attribute.KeyValue{
		attribute.String("model", model),
		attribute.String("provider_model", providerModel),
		attribute.String("protocol", protocol),
		attribute.String("upstream_protocol", upstreamProtocol),
		attribute.String("success", successStr),
	}

	r.relayRequestDuration.Record(bgCtx, duration, metric.WithAttributes(baseAttrs...))
	r.relayRequestsTotal.Add(bgCtx, 1, metric.WithAttributes(baseAttrs...))

	tokenAttrs := []attribute.KeyValue{
		attribute.String("model", model),
		attribute.String("provider_model", providerModel),
	}
	if promptTokens > 0 {
		r.relayTokensTotal.Add(bgCtx, int64(promptTokens), metric.WithAttributes(append(tokenAttrs, attribute.String("token_type", "prompt"))...))
	}
	if completionTokens > 0 {
		r.relayTokensTotal.Add(bgCtx, int64(completionTokens), metric.WithAttributes(append(tokenAttrs, attribute.String("token_type", "completion"))...))
	}
	if cachedTokens > 0 {
		r.relayTokensTotal.Add(bgCtx, int64(cachedTokens), metric.WithAttributes(append(tokenAttrs, attribute.String("token_type", "cached"))...))
	}

	totalTokens := float64(promptTokens + completionTokens + cachedTokens)
	if totalTokens > 0 {
		r.relayQuotaUsed.Add(bgCtx, totalTokens, metric.WithAttributes(tokenAttrs...))
	}
}

func (r *OtelRecorder) UpdateProviderMetrics(providerID uint, providerName, protocol string, status int, responseTimeMs int64) {
	pid := strconv.FormatUint(uint64(providerID), 10)
	attrs := []attribute.KeyValue{
		attribute.String("provider_id", pid),
		attribute.String("provider_name", providerName),
		attribute.String("protocol", protocol),
	}
	r.providerStatus.Record(bgCtx, int64(status), metric.WithAttributes(attrs...))
	r.providerResponseTime.Record(bgCtx, responseTimeMs, metric.WithAttributes(attrs...))
}

func (r *OtelRecorder) RecordError(errorType, component string) {
	attrs := []attribute.KeyValue{
		attribute.String("error_type", errorType),
		attribute.String("component", component),
	}
	r.errorsTotal.Add(bgCtx, 1, metric.WithAttributes(attrs...))
}
