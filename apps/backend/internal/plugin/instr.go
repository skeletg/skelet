package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	stdsync "sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	otrace "go.opentelemetry.io/otel/trace"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/applog"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Plugin instrumentation - every hook (payment methods, promotion hooks, plugin resolvers) runs through Instrument for uniform panic isolation, per-plugin timeout, kill switch, OTel span and metrics, and error attribution
// Adapters `defer finish(&err)`, and finish closes the span, records metrics and recovers panics

// All three wrapped in *PluginError for errors.Is/As
var ErrPluginDisabled = errors.New("plugin: disabled")
var ErrPluginPanicked = errors.New("plugin: panicked")
var ErrPluginTimeout = errors.New("plugin: hook timed out")

// isGateError - the kill switch or per-minute allowance refused the hook before it ran - the promotion
// adapters translate these to a neutral outcome instead of failing checkout, everything else propagates
func isGateError(err error) bool {
	return errors.Is(err, ErrPluginDisabled) || errors.Is(err, ErrPluginRateLimited)
}

// PluginError tags an error with its plugin slug + hook, pulled out via errors.As
type PluginError struct {
	Slug string
	Hook string
	Err  error
}

func (e *PluginError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("plugin %s: %s: %v", e.Slug, e.Hook, e.Err)
}

func (e *PluginError) Unwrap() error { return e.Err }

// finishFunc closes one Instrument call; takes *error so `defer finish(&err)` sees the named return
type finishFunc func(*error)

const defaultHookTimeout = 5 * time.Second // when no plugin/<slug>/hook_timeout_ms override is set

// bounds on operator timeout overrides so a fat-fingered value can't wedge a request lane
const (
	hookTimeoutMin = 50 * time.Millisecond
	hookTimeoutMax = 10 * time.Minute
)

// lazy so otelx.Init can set the global MeterProvider first
var instrMeter = stdsync.OnceValue(func() metric.Meter {
	return otel.Meter("github.com/skeletg/skelet/apps/backend/internal/plugin")
})

// singletons - the SDK pre-aggregates by attribute set, so per-call instruments would multiply cost
var hookInstruments = stdsync.OnceValue(func() *instruments {
	m := instrMeter()
	calls, err := m.Int64Counter(
		"plugin.hook.calls",
		metric.WithDescription("Total plugin hook invocations"),
	)
	if err != nil {
		// no-op meter never errors; a real one only on a bad name - log + fallback, don't crash boot
		slog.Error("plugin: failed to create hook calls counter", "err", err)
		calls, _ = m.Int64Counter("plugin.hook.calls.fallback")
	}
	errs, err := m.Int64Counter(
		"plugin.hook.errors",
		metric.WithDescription("Total plugin hook errors, including panics and timeouts"),
	)
	if err != nil {
		slog.Error("plugin: failed to create hook errors counter", "err", err)
		errs, _ = m.Int64Counter("plugin.hook.errors.fallback")
	}
	durs, err := m.Float64Histogram(
		"plugin.hook.duration",
		metric.WithDescription("Plugin hook latency in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		slog.Error("plugin: failed to create hook duration histogram", "err", err)
		durs, _ = m.Float64Histogram("plugin.hook.duration.fallback")
	}
	return &instruments{calls: calls, errs: errs, durs: durs}
})

type instruments struct {
	calls metric.Int64Counter
	errs  metric.Int64Counter
	durs  metric.Float64Histogram
}

// Wraps every plugin hook - returns the enriched ctx with timeout, tagged logger and span, a finish closure to defer, and a non-nil gate error the adapter must check and short-circuit on
// Two things close the gate - the kill switch (plugin/<slug>/enabled) and the per-minute allowance (plugin/<slug>/rate_limit_per_min, 0 = unlimited) - channelID is the request's channel, uuid.Nil when unscoped, and slug/hook become bounded metric labels
// The timeout is cooperative - it cancels ctx, so a hook ignoring ctx still runs to completion
func Instrument(ctx context.Context, slug, hook string, channelID uuid.UUID) (context.Context, finishFunc, error) {
	start := time.Now()

	// kill switch first so a disabled plugin never enters the span/timeout, and never burns a rate-limit slot
	enabled := isEnabledFromContext(ctx, slug, channelID)
	var limitErr error
	if enabled {
		limitErr = checkRateLimitFn(ctx, slug)
	}

	timeout := hookTimeoutFromContext(ctx, slug)
	ctx, cancel := context.WithTimeout(ctx, timeout)

	// tag the logger so log lines inside the hook carry slug+hook
	tagged := applog.From(ctx).With(
		slog.String("plugin", slug),
		slog.String("hook", hook),
	)
	ctx = applog.Into(ctx, tagged)

	// span for tracing; attrs carry slug+hook for filtering
	tracer := otel.Tracer("github.com/skeletg/skelet/apps/backend/internal/plugin")
	ctx, span := tracer.Start(ctx, "plugin."+slug+"."+hook,
		otrace.WithAttributes(
			attribute.String("plugin.slug", slug),
			attribute.String("plugin.hook", hook),
		),
	)

	inst := hookInstruments()
	commonAttrs := metric.WithAttributes(
		attribute.String("slug", slug),
		attribute.String("hook", hook),
	)
	inst.calls.Add(ctx, 1, commonAttrs)

	var preErr error
	switch {
	case !enabled:
		// still emit span+metrics so operators see disabled-but-attempted hooks
		span.SetStatus(codes.Error, "disabled")
		span.SetAttributes(attribute.String("plugin.disabled", "true"))
		preErr = &PluginError{Slug: slug, Hook: hook, Err: ErrPluginDisabled}
	case limitErr != nil:
		span.SetStatus(codes.Error, "rate limited")
		span.SetAttributes(attribute.String("plugin.rate_limited", "true"))
		preErr = &PluginError{Slug: slug, Hook: hook, Err: limitErr}
	}

	finish := func(errPtr *error) {
		defer cancel()
		defer span.End()

		// recover the hook's panic -> error, log with stack, record on span
		if rec := recover(); rec != nil {
			recErr := errFromPanic(rec)
			wrapped := &PluginError{Slug: slug, Hook: hook, Err: fmt.Errorf("%w: %w", ErrPluginPanicked, recErr)}
			if errPtr != nil {
				*errPtr = wrapped
			}
			span.RecordError(wrapped)
			span.SetStatus(codes.Error, "panic")
			inst.errs.Add(ctx, 1, metric.WithAttributes(
				attribute.String("slug", slug),
				attribute.String("hook", hook),
				attribute.String("kind", "panic"),
			))
			applog.From(ctx).Error("plugin hook panicked",
				"err", recErr.Error(),
				"stack", string(debug.Stack()),
			)
			return
		}

		// kill switch was off - surface the disabled error as the outcome
		if !enabled {
			if errPtr != nil && *errPtr == nil {
				*errPtr = &PluginError{Slug: slug, Hook: hook, Err: ErrPluginDisabled}
			}
			inst.errs.Add(ctx, 1, metric.WithAttributes(
				attribute.String("slug", slug),
				attribute.String("hook", hook),
				attribute.String("kind", "disabled"),
			))
			inst.durs.Record(ctx, time.Since(start).Seconds(), commonAttrs)
			return
		}

		// over the per-minute allowance - the hook never ran
		if limitErr != nil {
			if errPtr != nil && *errPtr == nil {
				*errPtr = &PluginError{Slug: slug, Hook: hook, Err: limitErr}
			}
			inst.errs.Add(ctx, 1, metric.WithAttributes(
				attribute.String("slug", slug),
				attribute.String("hook", hook),
				attribute.String("kind", "rate_limited"),
			))
			inst.durs.Record(ctx, time.Since(start).Seconds(), commonAttrs)
			return
		}

		err := error(nil)
		if errPtr != nil {
			err = *errPtr
		}

		// distinguish hook error from timeout; wrap with slug attribution either way
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				err = fmt.Errorf("%w: %w", ErrPluginTimeout, err)
			}
			wrapped := &PluginError{Slug: slug, Hook: hook, Err: err}
			if errPtr != nil && !isUserFacing(err) { // a user error is already the sentence the caller reads - don't prefix "plugin X: hook Y:" onto it
				*errPtr = wrapped
			}
			span.RecordError(wrapped)
			span.SetStatus(codes.Error, "hook error")
			kind := "error"
			if errors.Is(err, ErrPluginTimeout) {
				kind = "timeout"
			}
			inst.errs.Add(ctx, 1, metric.WithAttributes(
				attribute.String("slug", slug),
				attribute.String("hook", hook),
				attribute.String("kind", kind),
			))
		}

		inst.durs.Record(ctx, time.Since(start).Seconds(), commonAttrs)
	}

	return ctx, finish, preErr
}

// Whether a plugin classified this as the caller's fault with a message meant for them - anything
// unclassified stays a server fault and keeps the slug attribution
// ClassOf not a type assert - it resolves registered sentinels too, which is how auth's ErrUnauthenticated carries a class without being *apperr.Error
func isUserFacing(err error) bool {
	switch apperr.ClassOf(err) {
	case apperr.ClassInternal, apperr.ClassUnknown:
		return false
	default:
		return true
	}
}

// errFromPanic normalises a recovered value (error/string/other) into an error
func errFromPanic(rec any) error {
	switch v := rec.(type) {
	case error:
		return v
	case string:
		return errors.New(v)
	default:
		return fmt.Errorf("%v", v)
	}
}

// Consults the host's IsPluginEnabled, failing open when the host isn't installed during early boot or tests - a var so tests can swap it
var isEnabledFromContext = func(ctx context.Context, slug string, channelID uuid.UUID) bool {
	return sdk.IsEnabled(ctx, slug, channelID)
}

var checkRateLimitFn = CheckRateLimit // test seam, as above

// hookTimeoutFromContext reads + clamps plugin/<slug>/hook_timeout_ms (fallback default) - A var for tests
var hookTimeoutFromContext = func(ctx context.Context, slug string) time.Duration {
	h := installedHost.Load()
	if h == nil {
		return defaultHookTimeout
	}
	svc := h.configSvc.Load()
	if svc == nil {
		return defaultHookTimeout
	}
	ms, err := svc.GetInt(ctx, "plugin/"+slug+"/hook_timeout_ms", config.DefaultScope)
	if err != nil || ms <= 0 {
		return defaultHookTimeout
	}
	d := time.Duration(ms) * time.Millisecond
	if d < hookTimeoutMin {
		return hookTimeoutMin
	}
	if d > hookTimeoutMax {
		return hookTimeoutMax
	}
	return d
}

// IsTransientHookError reports whether a hook failure is retry-worthy rather than a rejection - the
// gate refusing (kill switch, rate limit), the hook timing out or panicking are all conditions a
// later retry can survive, unlike a provider's own "unverified or malformed" verdict
func IsTransientHookError(err error) bool {
	return errors.Is(err, ErrPluginDisabled) ||
		errors.Is(err, ErrPluginRateLimited) ||
		errors.Is(err, ErrPluginTimeout) ||
		errors.Is(err, ErrPluginPanicked) ||
		errors.Is(err, context.DeadlineExceeded)
}
