// Package applog is the slog wiring - one logger built at boot by Setup(env) and stamped onto slog.Default, with per-request loggers living in context, attached by httpx.RequestID and read back through From(ctx)
package applog

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type ctxKey struct{} // unexported so other packages can't smuggle their own logger in by accident - the only way in is Into(ctx, l)

// Builds the root logger - JSON in production for log shippers, text in development for readability in the `bin/logs app` tail, with the level from LOG_LEVEL defaulting to info
func Setup(env string) *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true, // file:line on every line - cheap, the single biggest win for "where did this come from", and it pairs with apperr.Error.Source() at the error site for the full trail
	}
	var handler slog.Handler
	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

func Into(ctx context.Context, l *slog.Logger) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, l)
}

func From(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return slog.Default()
	}
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
