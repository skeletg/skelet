// Package leasequeue drains a database queue whose rows are claimed under a short lease - the shape the webhook queue
// and the tracking deliveries share. A claim pushes a row's next attempt past the lease, so a crash mid-send replays it
// once the lease lapses (at least once), and SKIP LOCKED in the claim splits the work across replicas. Each queue keeps
// its own SQL and its own idea of success; this is the loop around them
package leasequeue

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

type Runner[T any] struct {
	Name    string // for the logs
	Poll    time.Duration
	Batch   int32
	Lease   int32 // seconds - must exceed the slowest send, or a row is claimed twice
	Workers int
	Claim   func(ctx context.Context, batch, leaseSeconds int32) ([]T, error)
	// Prepare runs once per claimed batch - many rows usually share a parent worth loading once - and returns the
	// function each row is handed to, on up to Workers goroutines. That function records the row's outcome itself
	Prepare func(ctx context.Context, batch []T) func(ctx context.Context, item T)
	Hourly  func(ctx context.Context) // housekeeping, nil for none
}

// Run polls until ctx is cancelled - it blocks, so run it from a tracked background goroutine
func (r Runner[T]) Run(ctx context.Context) {
	tick := time.NewTicker(r.Poll)
	defer tick.Stop()
	hourly := time.NewTicker(time.Hour)
	defer hourly.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			r.RunOnce(ctx)
		case <-hourly.C:
			if r.Hourly != nil {
				r.Hourly(ctx)
			}
		}
	}
}

// RunOnce drains everything due and returns how many rows it took - it keeps claiming while batches come back full, so
// a burst doesn't wait a poll per batch
func (r Runner[T]) RunOnce(ctx context.Context) int {
	done := 0
	for ctx.Err() == nil {
		items, err := r.Claim(ctx, r.Batch, r.Lease)
		if err != nil {
			slog.Warn("claim due rows", "queue", r.Name, "err", err)
			return done
		}
		done += len(items)
		if len(items) > 0 {
			r.send(ctx, items)
		}
		if len(items) < int(r.Batch) {
			return done
		}
	}
	return done
}

func (r Runner[T]) send(ctx context.Context, items []T) {
	deliver := r.Prepare(ctx, items)
	var wg sync.WaitGroup
	sem := make(chan struct{}, r.Workers)
	defer wg.Wait()
	for _, item := range items {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return // the unsent rows replay once their lease lapses
		}
		wg.Go(func() {
			defer func() { <-sem }()
			// A send handles untrusted responses - a panic must not take the process down; the lease replays the row
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("queue send panicked", "queue", r.Name, "panic", rec, "stack", string(debug.Stack()))
				}
			}()
			deliver(ctx, item)
		})
	}
}

// Retry reads a retry ladder: the wait after the 1st, 2nd... failed attempt, and false once the first attempt plus one
// per rung is used up, when the row becomes a dead letter
func Retry(attempts int32, ladder []time.Duration) (time.Duration, bool) {
	if attempts >= int32(1+len(ladder)) {
		return 0, false
	}
	return ladder[max(1, min(int(attempts), len(ladder)))-1], true
}
