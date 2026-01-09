package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// SetupOTel initializes OpenTelemetry providers.
// Metrics are always enabled. Traces are optional.
func SetupOTel(ctx context.Context, endpoint, serviceName string, disableTraces bool) (func(context.Context) error, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, err
	}

	// Metrics exporter (OTLP HTTP → Collector).
	metricExp, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(3*time.Second))),
	)
	otel.SetMeterProvider(mp)

	// Traces exporter (optional).
	var tp *sdktrace.TracerProvider
	if !disableTraces {
		traceExp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
		if err != nil {
			return nil, err
		}
		tp = sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithBatcher(traceExp),
		)
		otel.SetTracerProvider(tp)
	}

	// Runtime metrics (goroutines, heap, GC, etc.).
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(2 * time.Second)); err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		_ = mp.Shutdown(ctx)
		if tp != nil {
			_ = tp.Shutdown(ctx)
		}
		return nil
	}, nil
}
