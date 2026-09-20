package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	slogsentry "github.com/samber/slog-sentry/v2"
)

const sentryDSNEnv = "SENTRY_DSN"

const flushTimeout = 2 * time.Second

// With SENTRY_DSN empty every Sentry piece is a no-op, so the boot path stays unconditional - the reporter is nil when disabled, the shutdown flushes buffered events, and an error means genuinely broken configuration
func initSentry(env string) (*sentryReporter, func(context.Context) error, error) {
	dsn := os.Getenv(sentryDSNEnv)
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
		return nil, noopShutdown, fmt.Errorf("observability: sentry init: %w", err)
	}
	slog.Info("observability: sentry initialised", "environment", env, "release", release)
	return &sentryReporter{}, sentryShutdown, nil
}

// Only Error-level records go to Sentry, lower levels stay in stdout - safe whether or not Init ran, since an uninitialised SDK drops to a no-op hub
func sentrySlogHandler() slog.Handler {
	return slogsentry.Option{
		Level:     slog.LevelError,
		AddSource: true,
	}.NewSentryHandler()
}

func sentryShutdown(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	timeout := flushTimeout
	if ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	if ok := sentry.Flush(timeout); !ok {
		slog.Warn("observability: sentry flush did not complete before deadline", "timeout", timeout)
	}
	return nil
}

func noopShutdown(context.Context) error { return nil }
