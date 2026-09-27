package events

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// Runs detached from the emitting request's context (cancellation dropped, values preserved) because "-ed" events describe committed work that mustn't be torn down when the HTTP response completes The error return exists for the OUTBOX relay: a non-nil error keeps the row undelivered so the whole event replays after the lease expires - which is why every handler on an outbox-routed event must be idempotent (they were all written that way; the retry they assumed now exists). On the fire-and-forget Publish path the error is logged and dropped, as before
type AsyncHandler func(ctx context.Context, e Event) error

// A non-nil error aborts the PublishSync chain and propagates back to the emitter, which typically fails the whole operation
type SyncHandler func(ctx context.Context, e Event) error

type syncEntry struct {
	priority int
	handler  SyncHandler
}

type Bus struct {
	mu        sync.RWMutex
	asyncSubs map[string][]AsyncHandler
	syncSubs  map[string][]syncEntry

	// Tracks detached async goroutines so Drain can wait for them at shutdown instead of racing the DB and Valkey closes
	inflight sync.WaitGroup
	// Bounds concurrent async goroutines - unbounded, a stalled subscriber (a slow analytics spine, say) grows goroutines and memory without limit, exactly the failure the Signals split guards against
	// At the cap Publish runs handlers inline instead, applying backpressure to the emitter rather than dropping a committed-state event
	sem chan struct{}
	// Closed by Drain after the grace window - handler contexts watch it so long backoffs like webhook retries cancel instead of pinning shutdown
	stop     chan struct{}
	stopOnce sync.Once
}

const maxAsyncDispatch = 512

func NewBus() *Bus {
	return &Bus{
		asyncSubs: make(map[string][]AsyncHandler),
		syncSubs:  make(map[string][]syncEntry),
		sem:       make(chan struct{}, maxAsyncDispatch),
		stop:      make(chan struct{}),
	}
}

func (b *Bus) Subscribe(eventType string, h AsyncHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.asyncSubs[eventType] = append(b.asyncSubs[eventType], h)
}

// Lower priority runs first, ties keep registration order - use this when the handler's result is part of the emitting operation's correctness
func (b *Bus) SubscribeSync(eventType string, priority int, h SyncHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries := append(b.syncSubs[eventType], syncEntry{priority: priority, handler: h})
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].priority < entries[j].priority
	})
	b.syncSubs[eventType] = entries
}

// Returns immediately, handlers run detached - a panic is recovered per-handler so one bad listener can't stop the others
func (b *Bus) Publish(ctx context.Context, e Event) {
	ctx = withoutOutboxID(ctx)
	b.mu.RLock()
	handlers := append([]AsyncHandler(nil), b.asyncSubs[e.Type()]...)
	b.mu.RUnlock()
	if len(handlers) == 0 {
		return
	}
	// Detach cancellation but keep ctx values (trace ids, logger) - async events are committed state and mustn't abort because the causing request finished
	// The handler context still observes Drain, so one parked in a retry backoff cancels instead of outliving the process's connections
	detached := stopCtx{Context: context.WithoutCancel(ctx), stop: b.stop}
	select {
	case b.sem <- struct{}{}:
		b.inflight.Go(func() {
			defer func() { <-b.sem }()
			for _, h := range handlers {
				if err := runAsyncWithRecover(detached, e, h); err != nil {
					log.Printf("[events] async handler for %q failed (fire-and-forget path, no retry): %v", e.Type(), err)
				}
			}
		})
	default:
		// At the cap - run inline on the caller so a stalled subscriber applies backpressure instead of spawning unbounded goroutines - Committed-state events are never dropped
		for _, h := range handlers {
			if err := runAsyncWithRecover(detached, e, h); err != nil {
				log.Printf("[events] async handler for %q failed (fire-and-forget path, no retry): %v", e.Type(), err)
			}
		}
	}
}

// Phase one is a short grace window; after it the stop channel closes, cancelling every handler context (aborting retry backoffs and in-flight HTTP), and Drain waits until they return or ctx expires
func (b *Bus) Drain(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		b.inflight.Wait()
		close(done)
	}()

	grace := 5 * time.Second
	if d, ok := ctx.Deadline(); ok {
		if half := time.Until(d) / 2; half < grace {
			grace = half
		}
	}
	graceTimer := time.NewTimer(grace)
	defer graceTimer.Stop()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
	case <-graceTimer.C:
	}

	b.stopOnce.Do(func() { close(b.stop) })
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WaitIdle blocks until every detached async handler has returned, leaving the bus alive - unlike Drain it can be called repeatedly, which is what a test needs between act and assert Quiescence is only meaningful once the caller has stopped emitting - a concurrent Publish can re-raise the count the moment this returns
func (b *Bus) WaitIdle(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		b.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Carries its embedded context's values but is done when the bus stops - detached from the request, attached to the process
type stopCtx struct {
	context.Context
	stop <-chan struct{}
}

func (c stopCtx) Done() <-chan struct{} { return c.stop }

func (c stopCtx) Err() error {
	select {
	case <-c.stop:
		return context.Canceled
	default:
		return nil
	}
}

func (c stopCtx) Deadline() (time.Time, bool) { return time.Time{}, false }

// Runs the async handlers inline on the caller, panics recovered as in Publish - the outbox relay owns its own goroutine and needs to know when handlers returned AND whether they all succeeded, so it can mark the row delivered or leave it for the lease to replay
// Every handler runs even after one fails - they're independent effects, and the replay re-runs the whole set anyway (idempotence is the subscriber contract)
func (b *Bus) DispatchAsyncNow(ctx context.Context, e Event) error {
	b.mu.RLock()
	handlers := append([]AsyncHandler(nil), b.asyncSubs[e.Type()]...)
	b.mu.RUnlock()
	var errs []error
	for _, h := range handlers {
		if err := runAsyncWithRecover(ctx, e, h); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Priority order, returning the first non-nil error and skipping the rest - handlers may enrich e, so the emitter reads mutable fields after a nil return
func (b *Bus) PublishSync(ctx context.Context, e Event) error {
	ctx = withoutOutboxID(ctx)
	b.mu.RLock()
	entries := append([]syncEntry(nil), b.syncSubs[e.Type()]...)
	b.mu.RUnlock()
	for _, entry := range entries {
		if err := callSyncWithRecover(ctx, e, entry.handler); err != nil {
			return err
		}
	}
	return nil
}

// Converts a panic into an error so a misbehaving subscriber aborts the chain rather than crashing the process - sync emits usually sit behind httpx.Recover, but CLI and scheduler callers have no such net
func callSyncWithRecover(ctx context.Context, e Event, h SyncHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[events:panic] sync handler for %q: %v", e.Type(), r)
			err = fmt.Errorf("sync handler for %q panicked: %v", e.Type(), r)
		}
	}()
	return h(ctx, e)
}

func runAsyncWithRecover(ctx context.Context, e Event, h AsyncHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[events:panic] async handler for %q: %v", e.Type(), r)
			err = fmt.Errorf("async handler for %q panicked: %v", e.Type(), r)
		}
	}()
	return h(ctx, e)
}
