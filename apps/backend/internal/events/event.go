package events // "-ing" events (OrderPlacing) fire sync pre-commit via PublishSync and can abort or enrich (priority order, mutable pointer payload); "-ed" events (OrderPlaced) fire async post-commit via Publish and can't abort

import (
	"context"
	"time"
)

type Event interface {
	Type() string
}

type emittedAtKey struct{}

// WithEmittedAt marks the dispatch context with when the event was originally enqueued - the outbox relay stamps its row's created_at so a redelivered handler sees the SAME time as the first delivery, keeping time-derived dedupe keys and spine timestamps stable across replays
func WithEmittedAt(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, emittedAtKey{}, t)
}

func EmittedAt(ctx context.Context) (time.Time, bool) {
	t, ok := ctx.Value(emittedAtKey{}).(time.Time)
	return t, ok
}

func EmittedAtOr(ctx context.Context, fallback time.Time) time.Time {
	if t, ok := EmittedAt(ctx); ok {
		return t
	}
	return fallback
}
