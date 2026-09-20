// Package otelx wires optional OpenTelemetry tracing and metrics, driven entirely by env vars - with none set, Init returns a no-op shutdown and the global TracerProvider stays the SDK's built-in no-op, so every downstream tracer.Start is free The autoexport selection reads OTEL_* env, so switching from Jaeger to Tempo or Datadog is configuration-only and nothing in app code names a specific exporter Resource attributes come from APP_VERSION and the env arg
package otelx

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// The value populated as semconv.ServiceName on every emitted span and metric - constant, so dashboards built against the fixed name don't break across deployments
const ServiceName = "skelet-backend"

// OTEL_SERVICE_NAME when set, so sibling processes like cmd/signals report a distinct identity, else the historical default
func serviceName() string {
	if v := os.Getenv("OTEL_SERVICE_NAME"); v != "" {
		return v
	}
	return ServiceName
}

// Wires the global TracerProvider and MeterProvider and returns a shutdown that drains exporters - with no OTEL_* vars set it short-circuits to a no-op
// env is forwarded as semconv.DeploymentEnvironment so backends can segregate dev, staging and prod, and failures log a warning rather than returning an error
func Init(ctx context.Context, env string) (func(context.Context) error, error) {
	if !enabled() {
		slog.Debug("otelx: not enabled (OTEL_EXPORTER_OTLP_ENDPOINT / OTEL_TRACES_EXPORTER unset)")
		return noopShutdown, nil
	}

	res, err := buildResource(ctx, env)
	if err != nil {
		return noopShutdown, fmt.Errorf("otelx: build resource: %w", err)
	}

	traceShutdown, err := initTracer(ctx, res)
	if err != nil {
		return noopShutdown, fmt.Errorf("otelx: init tracer: %w", err)
	}

	meterShutdown, err := initMeter(ctx, res)
	if err != nil {
		// The trace exporter is up, so proceed without metrics - log the error but don't abort
		slog.Warn("otelx: init meter failed; continuing with tracing only", "err", err)
		meterShutdown = noopShutdown
	}

	// Global propagators, so an incoming traceparent is read by otelhttp and an outgoing one is written by clients using otelhttp.NewTransport
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("otelx: initialised", "service", serviceName(), "environment", env)
	return chain(traceShutdown, meterShutdown), nil
}

// True when at least one OTLP env var hints the operator wants telemetry exported - without any, autoexport would default to a no-op exporter, but short-circuiting keeps the global TracerProvider the SDK no-op, which is cheaper than batch-spanning into /dev/null
func enabled() bool {
	return firstSet(
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_TRACES_EXPORTER",
		"OTEL_METRICS_EXPORTER",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
	) != ""
}

func firstSet(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func buildResource(ctx context.Context, env string) (*resource.Resource, error) {
	version := os.Getenv("APP_VERSION")
	if version == "" {
		version = "dev"
	}
	return resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName()),
			semconv.ServiceVersion(version),
			semconv.DeploymentEnvironmentName(env),
		),
	)
}

func initTracer(ctx context.Context, res *resource.Resource) (func(context.Context) error, error) {
	exporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return noopShutdown, err
	}
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter,
			trace.WithBatchTimeout(5*time.Second),
			trace.WithMaxExportBatchSize(512),
		),
		trace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func initMeter(ctx context.Context, res *resource.Resource) (func(context.Context) error, error) {
	reader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return noopShutdown, err
	}
	mp := metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(mp)
	return mp.Shutdown, nil
}

func chain(fns ...func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		for _, v := range slices.Backward(fns) {
			if v == nil {
				continue
			}
			if err := v(ctx); err != nil {
				slog.Warn("otelx: shutdown component returned error", "err", err)
			}
		}
		return nil
	}
}

func noopShutdown(context.Context) error { return nil }
