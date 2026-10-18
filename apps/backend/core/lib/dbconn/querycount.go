package dbconn

import (
	"context"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

// Per-request query counting for the dev-mode debug surface - an HTTP middleware plants a counter on the request context and the counting tracer bumps it on every query flowing through the pool with that context

type queryCounterKey struct{}

func WithQueryCounter(ctx context.Context) (context.Context, *atomic.Int64) {
	c := &atomic.Int64{}
	return context.WithValue(ctx, queryCounterKey{}, c), c
}

func queryCounterFrom(ctx context.Context) *atomic.Int64 {
	c, _ := ctx.Value(queryCounterKey{}).(*atomic.Int64)
	return c
}

// Wraps an optional inner tracer with per-context query counting - safe with a nil inner, so counting works standalone in dev environments without an OTel collector
func CountingTracer(inner pgx.QueryTracer) pgx.QueryTracer {
	return &countingTracer{inner: inner}
}

type countingTracer struct {
	inner pgx.QueryTracer
}

func (t *countingTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if c := queryCounterFrom(ctx); c != nil {
		c.Add(1)
	}
	if t.inner != nil {
		return t.inner.TraceQueryStart(ctx, conn, data)
	}
	return ctx
}

func (t *countingTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if t.inner != nil {
		t.inner.TraceQueryEnd(ctx, conn, data)
	}
}
