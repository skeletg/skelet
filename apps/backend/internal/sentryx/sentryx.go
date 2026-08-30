// Package sentryx wires optional Sentry error aggregation - with SENTRY_DSN empty every piece is a no-op, so the boot path stays unconditional: the SDK, a slog.Handler mirroring Error records, an http.Handler giving each request a Hub tagged with request_id and surface, and a Reporter the graphqlx presenter calls for ClassInternal and ClassUnavailable
// The Reporter fingerprints by apperr.Code, so dedup groups by failure mode rather than by stack
package sentryx

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	slogsentry "github.com/samber/slog-sentry/v2"
)

const DSNEnv = "SENTRY_DSN"

const flushTimeout = 2 * time.Second

// Returns a *Reporter for the graphqlx presenter (nil when disabled), a shutdown func that flushes buffered events (no-op when disabled), and an error only for genuinely catastrophic configuration
func Init(env string) (*Reporter, func(context.Context) error, error) {
	dsn := os.Getenv(DSNEnv)
	if dsn == "" {
		return nil, noopShutdown, nil
	}
	release := os.Getenv("APP_VERSION")
	if release == "" {
		release = "dev"
	}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: env,
		Release:     release,
		// OTel owns tracing, so Sentry stays focused on error aggregation - performance monitoring is opt-in through OTel's span exporter, not Sentry's TracesSampleRate
		EnableTracing: false,
	}); err != nil {
		return nil, noopShutdown, fmt.Errorf("sentryx: init: %w", err)
	}
	slog.Info("sentryx: initialised", "environment", env, "release", release)
	return &Reporter{}, shutdown, nil
}

// Only Error-level records go to Sentry, lower levels stay in stdout - safe whether or not Init ran, since an uninitialised SDK drops to a no-op hub
func SlogHandler() slog.Handler {
	return slogsentry.Option{
		Level:     slog.LevelError,
		AddSource: true,
	}.NewSentryHandler()
}

func shutdown(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	timeout := flushTimeout
	if ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	if ok := sentry.Flush(timeout); !ok {
		slog.Warn("sentryx: flush did not complete before deadline", "timeout", timeout)
	}
	return nil
}

func noopShutdown(context.Context) error { return nil }
