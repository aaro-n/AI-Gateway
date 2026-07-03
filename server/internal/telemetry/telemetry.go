// Package telemetry 提供 OpenTelemetry 初始化与生命周期管理。
//
// 使用方式：
//
//	otelProviders, err := telemetry.InitOpenTelemetry(ctx, cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer telemetry.Shutdown(ctx, otelProviders)
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config OpenTelemetry 配置
type Config struct {
	Enabled     bool
	Endpoint    string // OTLP collector 地址，如 http://localhost:4318
	ServiceName string
}

// ProviderBundle 持有 TracerProvider 和 MeterProvider，用于优雅关闭
type ProviderBundle struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
}

// InitOpenTelemetry 配置全局 OpenTelemetry providers。
// 当 Config.Enabled 为 false 时返回 nil，不执行任何操作。
func InitOpenTelemetry(ctx context.Context, cfg Config) (*ProviderBundle, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("otel endpoint is required when enabled")
	}

	res, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	// Trace exporter
	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.Endpoint),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
	)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Metric exporter
	metricExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(cfg.Endpoint),
		otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression),
	)
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(metricExporter,
				sdkmetric.WithInterval(60*time.Second),
			),
		),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	return &ProviderBundle{
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
	}, nil
}

// Shutdown 优雅关闭 OpenTelemetry providers
func Shutdown(ctx context.Context, bundle *ProviderBundle) error {
	if bundle == nil {
		return nil
	}
	var errs []error
	if bundle.TracerProvider != nil {
		if err := bundle.TracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown tracer: %w", err))
		}
	}
	if bundle.MeterProvider != nil {
		if err := bundle.MeterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown meter: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}
