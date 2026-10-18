// Package observability is the single entry point for telemetry wiring - Sentry exception capture and OpenTelemetry tracing/metrics
// It hides how optional each integration is: unset env vars leave a no-op facade, set ones initialise the component - Init never errors, so a misconfigured DSN logs a warning and stays disabled rather than crashing boot
package observability

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	slogmulti "github.com/samber/slog-multi"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
)

// Forwards errors and panics to the aggregator - nil when none is configured, and graphqlx's presenter consumes it structurally so it needs no import of this package
type Reporter interface {
	CaptureAppError(ctx context.Context, ae *apperr.Error)
	CapturePanic(ctx context.Context, rec any)
}

// Drains buffered telemetry - safe even after a failed Init, always non-nil, and bounded by the passed deadline - Component errors join but never abort the drain
type Shutdown func(context.Context) error

type Telemetry struct {
	Reporter Reporter
	Shutdown Shutdown
}

// Wires the optional telemetry layers from env vars, with Shutdown draining them LIFO - SENTRY_DSN gives the sentry-go SDK plus slog bridge and presenter reporter, OTEL_EXPORTER_OTLP_* gives the TracerProvider and MeterProvider
func Init(ctx context.Context, env string) Telemetry {
	shutdowns := []Shutdown{}

	// OTel first so the global TracerProvider exists before any middleware needs it - the LIFO drain then shuts OTel down after Sentry, so events fired during otel shutdown still reach Sentry
	otelShutdown, err := otelx.Init(ctx, env)
	if err != nil {
		slog.Warn("observability: otel init failed; continuing without",
			"err", err)
	}
	shutdowns = append(shutdowns, otelShutdown)

	reporter, sentryDown, err := initSentry(env)
	if err != nil {
		slog.Warn("observability: sentry init failed; continuing without",
			"err", err)
	}
	shutdowns = append(shutdowns, sentryDown)

	if reporter != nil {
		base := slog.Default().Handler()
		multi := slog.New(slogmulti.Fanout(base, sentrySlogHandler()))
		slog.SetDefault(multi)
	}

	return Telemetry{
		Reporter: nilOrReporter(reporter),
		Shutdown: chain(shutdowns),
	}
}

// Without this dance a typed-nil *Reporter would survive inside the interface and break `if reporter != nil` guards downstream
func nilOrReporter(r *sentryReporter) Reporter {
	if r == nil {
		return nil
	}
	return r
}

// Runs each in reverse order - LIFO, last initialised first drained - and joins the errors - A nil entry is a no-op
func chain(fns []Shutdown) Shutdown {
	if len(fns) == 0 {
		return func(context.Context) error { return nil }
	}
	return func(ctx context.Context) error {
		var errs []error
		for _, v := range slices.Backward(fns) {
			if v == nil {
				continue
			}
			if err := v(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
}
