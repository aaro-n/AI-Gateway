// Package monitor 提供监控初始化与后台指标采集
package monitor

import (
	"log"

	"ai-gateway/internal/monitor/otel"
	"ai-gateway/internal/monitor/prometheus"
)

// Config 监控配置
type Config struct {
	EnablePrometheus bool
	EnableOtel       bool
}

// InitMonitoring 初始化所有监控组件。
// 根据配置选择 Prometheus / OpenTelemetry / 或两者同时启用（MultiRecorder）。
func InitMonitoring(cfg Config) error {
	var recorders []MetricsRecorder

	if cfg.EnablePrometheus {
		recorders = append(recorders, &prometheus.PrometheusRecorder{})
		log.Println("[Monitor] Prometheus metrics recorder enabled")
	}

	if cfg.EnableOtel {
		otelRecorder, err := otel.NewOtelRecorder()
		if err != nil {
			return err
		}
		recorders = append(recorders, otelRecorder)
		log.Println("[Monitor] OpenTelemetry metrics recorder enabled")
	}

	if len(recorders) == 0 {
		GlobalRecorder = &NoOpRecorder{}
		return nil
	}

	if len(recorders) == 1 {
		GlobalRecorder = recorders[0]
	} else {
		GlobalRecorder = &MultiRecorder{Recorders: recorders}
		log.Println("[Monitor] Multi-recorder mode: Prometheus + OpenTelemetry")
	}

	return nil
}
