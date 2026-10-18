package events // "-ing" events (OrderPlacing) fire sync pre-commit via PublishSync and can abort or enrich (priority order, mutable pointer payload); "-ed" events (OrderPlaced) fire async post-commit via Publish and can't abort

import (
	"context"
	"time"

	"github.com/google/uuid"
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

type outboxIDKey struct{}

// WithOutboxID names the outbox row being dispatched, which its replays share
func WithOutboxID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, outboxIDKey{}, id)
}

func OutboxID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(outboxIDKey{}).(uuid.UUID)
	return id, ok && id != uuid.Nil
}

// An event a handler publishes is its own, not the outbox row that woke the handler
func withoutOutboxID(ctx context.Context) context.Context {
	if _, ok := OutboxID(ctx); !ok {
		return ctx
	}
	return context.WithValue(ctx, outboxIDKey{}, uuid.Nil)
}
