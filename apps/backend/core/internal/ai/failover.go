package ai

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	FallbackModelPath = "ai/llm/fallback_model"

	breakerTrips  = 3
	breakerRest   = 30 * time.Second
	retryPause    = 300 * time.Millisecond
	maxRetryAfter = time.Second
	fallbackNeeds = 2 * time.Second // a fallback with less time than this left in the caller's deadline isn't started
	// not a standard status - some providers answer it when they're overloaded
	statusOverloaded = 529
)

type providerError struct {
	host       string
	status     int
	retryAfter time.Duration
	snippet    string
}

func (e *providerError) Error() string {
	return fmt.Sprintf("ai: %s HTTP %d: %s", e.host, e.status, e.snippet)
}

func newProviderError(host string, resp *http.Response, snippet string) *providerError {
	e := &providerError{host: host, status: resp.StatusCode, snippet: snippet}
	if secs, err := strconv.Atoi(resp.Header.Get("retry-after")); err == nil && secs > 0 {
		e.retryAfter = time.Duration(secs) * time.Second
	}
	return e
}

// transient is overload, rate limits, the provider's own errors or a dropped connection - never a request the provider
// turned down as asked, nor one the caller gave up on
func transient(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	if pe, ok := errors.AsType[*providerError](err); ok {
		switch pe.status {
		case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests, http.StatusInternalServerError,
			http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, statusOverloaded:
			return true
		}
		return false
	}
	_, ok := errors.AsType[*url.Error](err)
	return ok
}

type breaker struct {
	mu    sync.Mutex
	trips map[string]*trip
}

type trip struct {
	fails int
	until time.Time
}

func (b *breaker) resting(key string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.trips[key]
	return t != nil && now.Before(t.until)
}

func (b *breaker) failed(key string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.trips == nil {
		b.trips = map[string]*trip{}
	}
	t := b.trips[key]
	if t == nil {
		t = &trip{}
		b.trips[key] = t
	}
	t.fails++
	if t.fails < breakerTrips {
		return false
	}
	t.fails, t.until = 0, now.Add(breakerRest)
	return true
}

func (b *breaker) ok(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.trips, key)
}

var failovers = func() metric.Int64Counter {
	c, _ := otel.Meter("github.com/skeletg/skelet/apps/backend/core/internal/ai").Int64Counter("ai.model.failovers",
		metric.WithDescription("Model calls answered on the fallback model"))
	return c
}()

// failover gives a transient failure one more try after a pause, then the fallback; a model that keeps failing rests,
// its calls going straight to the fallback
func (s *Service) failover(ctx context.Context, pc providerCfg, model string, call func(model string) error) (string, error) {
	fallback := s.fallbackFor(ctx, model)
	key := pc.baseURL + "|" + model
	if fallback != "" && s.trips.resting(key, time.Now()) {
		return fallback, call(fallback)
	}
	err := call(model)
	if err == nil {
		s.trips.ok(key)
		return model, nil
	}
	if !transient(ctx, err) {
		return model, err
	}
	if pause := retryPauseFor(err); room(ctx, pause) {
		if !sleep(ctx, pause) {
			return model, err
		}
		if err = call(model); err == nil {
			s.trips.ok(key)
			return model, nil
		}
		if !transient(ctx, err) {
			return model, err
		}
	}
	if s.trips.failed(key, time.Now()) && fallback != "" {
		slog.WarnContext(ctx, "ai: a model keeps failing - its calls go to the fallback for a while", "model", model, "fallback", fallback, "rest", breakerRest)
	}
	if fallback == "" || !room(ctx, 0) {
		return model, err
	}
	slog.WarnContext(ctx, "ai: a model call failed, answering on the fallback", "model", model, "fallback", fallback, "err", err)
	failovers.Add(ctx, 1, metric.WithAttributes(attribute.String("model", model), attribute.String("fallback", fallback)))
	return fallback, call(fallback)
}

// The setting's model, else for short writing on the quick model the main one - never the model that just failed
func (s *Service) fallbackFor(ctx context.Context, model string) string {
	fallback := s.get(ctx, FallbackModelPath)
	if quick, _ := ctx.Value(quickKey{}).(bool); quick && fallback == "" {
		fallback = cmp.Or(s.get(ctx, cfgModel), defaultModel)
	}
	if fallback == model {
		return ""
	}
	return fallback
}

func retryPauseFor(err error) time.Duration {
	if pe, ok := errors.AsType[*providerError](err); ok && pe.retryAfter > 0 {
		return min(pe.retryAfter, maxRetryAfter)
	}
	return retryPause
}

func room(ctx context.Context, wait time.Duration) bool {
	deadline, ok := ctx.Deadline()
	return !ok || time.Until(deadline) > wait+fallbackNeeds
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
