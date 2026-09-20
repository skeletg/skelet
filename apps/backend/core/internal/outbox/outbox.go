// events.Bus.Publish is fire-and-forget from a detached goroutine, so a crash between commit and the handlers finishing loses the event permanently - webhooks, search indexing and order notifications all hang off it Emitters call Enqueue INSIDE the state change's transaction, so the row commits or rolls back atomically with it The relay claims due rows under a short lease, re-materializes the concrete type, dispatches to the bus's async handlers and marks the row delivered - the claim is one UPDATE…RETURNING, so no transaction is held while subscribers run and a slow webhook can't pin a connection A crash mid-dispatch leaves the row undelivered until the lease expires, making delivery at-least-once - subscribers must tolerate replays
package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"log"
	"runtime/debug"
	"sync"
	"time"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

const (
	defaultPollInterval = time.Second
	claimBatchSize      = 50
	// How long a claimed row stays invisible to other claimers - must comfortably exceed the slowest dispatch path (a webhook retry chain is roughly 6 min plus HTTP timeouts)
	leaseSeconds = 600
	// Bounds concurrent dispatches so one slow subscriber can't head-of-line-block the queue, and a burst can't spawn unbounded goroutines
	dispatchWorkers = 8
	// Rows that can't be dispatched (unknown type, broken payload) are dropped loudly instead of poisoning the queue - transient handler failures don't count, handlers manage their own retries
	maxAttempts        = 10
	deliveredRetention = 24 * time.Hour
	cleanupEvery       = time.Hour
)

type Outbox struct {
	queries *db.Queries
	bus     *events.Bus

	mu     sync.RWMutex
	decode map[string]func() events.Event

	pollInterval time.Duration

	// invoked when a row exhausts maxAttempts and is dropped - the last chance for an operator to
	// learn a committed event's side effects never ran; nil degrades to the log line alone
	dropAlert func(ctx context.Context, eventType string, rowID uuid.UUID, reason string)
}

// SetDropAlert wires the exhaustion callback - typically the admin notification service
func (o *Outbox) SetDropAlert(fn func(ctx context.Context, eventType string, rowID uuid.UUID, reason string)) {
	o.dropAlert = fn
}

func New(queries *db.Queries, bus *events.Bus) *Outbox {
	return &Outbox{
		queries:      queries,
		bus:          bus,
		decode:       make(map[string]func() events.Event),
		pollInterval: defaultPollInterval,
	}
}

func (o *Outbox) SetPollInterval(d time.Duration) {
	if d > 0 {
		o.pollInterval = d
	}
}

// Maps a serialized type to a constructor, so the relay hands subscribers the same pointer type a direct Publish would
// Every type passed to Enqueue must be registered at boot - Enqueue refuses unknown ones so the failure surfaces at write time, in the emitter's tx, rather than as a poisoned row at relay time
func (o *Outbox) RegisterType(eventType string, fn func() events.Event) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.decode[eventType] = fn
}

func (o *Outbox) newEvent(eventType string) (events.Event, bool) {
	o.mu.RLock()
	fn, ok := o.decode[eventType]
	o.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return fn(), true
}

// Writes through the caller's transaction - the row only becomes visible to the relay when that commits, and a rollback discards it with everything else
func (o *Outbox) Enqueue(ctx context.Context, sqlTx *sql.Tx, e events.Event) error {
	if _, ok := o.newEvent(e.Type()); !ok {
		return fmt.Errorf("outbox: event type %q not registered", e.Type())
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("outbox: marshal %s: %w", e.Type(), err)
	}
	_, err = o.queries.WithTx(sqlTx).InsertOutboxEvent(ctx, db.InsertOutboxEventParams{
		EventType: e.Type(),
		Payload:   payload,
	})
	if err != nil {
		return fmt.Errorf("outbox: insert %s: %w", e.Type(), err)
	}
	return nil
}

// Polls until ctx is cancelled, then waits for in-flight dispatches to wind down on the same ctx
func (o *Outbox) Run(ctx context.Context) {
	var wg sync.WaitGroup
	defer wg.Wait()
	sem := make(chan struct{}, dispatchWorkers)

	poll := time.NewTicker(o.pollInterval)
	defer poll.Stop()
	cleanup := time.NewTicker(cleanupEvery)
	defer cleanup.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			o.drainDue(ctx, &wg, sem)
		case <-cleanup.C:
			cutoff := time.Now().Add(-deliveredRetention)
			if n, err := o.queries.DeleteDeliveredOutboxBefore(ctx, cutoff); err != nil {
				log.Printf("[outbox] cleanup: %v", err)
			} else if n > 0 {
				log.Printf("[outbox] swept %d delivered rows", n)
			}
		}
	}
}

// DrainPending claims and dispatches every due row once, returning only after the handlers have run - the run-once seam the e2e suite drives between act and assert, where Run's poll timer would make ordering a race Returns how many rows were dispatched, so a caller can loop to quiescence when a handler's own side effects enqueue further rows
func (o *Outbox) DrainPending(ctx context.Context) int {
	var wg sync.WaitGroup
	defer wg.Wait()
	sem := make(chan struct{}, dispatchWorkers)
	return o.drainDue(ctx, &wg, sem)
}

// Claims batches until the due set is empty, so a burst doesn't wait pollInterval per batch
func (o *Outbox) drainDue(ctx context.Context, wg *sync.WaitGroup, sem chan struct{}) int {
	dispatched := 0
	for ctx.Err() == nil {
		rows, err := o.queries.ClaimDueOutboxEvents(ctx, db.ClaimDueOutboxEventsParams{
			BatchSize:    claimBatchSize,
			LeaseSeconds: leaseSeconds,
		})
		if err != nil {
			log.Printf("[outbox] claim: %v", err)
			return dispatched
		}
		for _, row := range rows {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return dispatched
			}
			wg.Add(1)
			dispatched++
			go func(row db.EventOutbox) {
				defer wg.Done()
				defer func() { <-sem }()
				// handler panics are recovered by the bus - this covers dispatchRow's own decode and
				// bookkeeping; the claim lease expires and the row redelivers
				defer func() {
					if rec := recover(); rec != nil {
						log.Printf("[outbox] dispatch of row %s panicked: %v\n%s", row.ID, rec, debug.Stack())
					}
				}()
				o.dispatchRow(ctx, row)
			}(row)
		}
		if len(rows) < claimBatchSize {
			return dispatched
		}
	}
	return dispatched
}

// Undecodable rows retry via lease expiry until maxAttempts and then drop loudly; a failed delivered-mark just leaves the row to be re-claimed, which is the at-least-once part
func (o *Outbox) dispatchRow(ctx context.Context, row db.EventOutbox) {
	e, ok := o.newEvent(row.EventType)
	if ok {
		if err := json.Unmarshal(row.Payload, e); err != nil {
			log.Printf("[outbox] decode %s (row %s): %v", row.EventType, row.ID, err)
			ok = false
		}
	} else {
		log.Printf("[outbox] no decoder registered for %s (row %s)", row.EventType, row.ID)
	}
	if !ok {
		// Let the lease expire for a natural retry - a plugin re-registering its type fixes it - then drop after maxAttempts so it can't poison the queue forever
		if row.Attempts >= maxAttempts {
			o.dropRow(ctx, row, "undecodable payload")
		}
		return
	}

	// Delivered now means "every subscriber succeeded" - a handler error (or panic, converted inside the bus) leaves the row for the lease to replay, which is the retry every compensating handler assumes: without it a failed gift-card restore or stock release is a log line and permanently wrong state - created_at rides along so a redelivery carries the same event time and time-derived dedupe keys stay stable
	if err := o.bus.DispatchAsyncNow(events.WithEmittedAt(ctx, row.CreatedAt), e); err != nil {
		if row.Attempts >= maxAttempts {
			// side effects for a committed event are being abandoned - say so where an operator looks
			o.dropRow(ctx, row, "subscribers kept failing: "+err.Error())
			return
		}
		log.Printf("[outbox] %s (row %s) subscriber failure, will replay (attempt %d/%d): %v", row.EventType, row.ID, row.Attempts, maxAttempts, err)
		return
	}

	// Fresh context because ctx may have been cancelled while the handlers ran - an unmarked-but-dispatched row would replay on restart, and replays are legal but pointless when we can still record completion
	mctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := o.queries.MarkOutboxDelivered(mctx, row.ID); err != nil {
		log.Printf("[outbox] mark delivered %s: %v (will replay after lease)", row.ID, err)
	}
}

// dropRow abandons a row after maxAttempts - marked delivered so it can't poison the queue, alerted
// so the abandonment is an operator event rather than a grep exercise
func (o *Outbox) dropRow(ctx context.Context, row db.EventOutbox, reason string) {
	log.Printf("[outbox] DROPPING row %s (%s) after %d attempts: %s", row.ID, row.EventType, row.Attempts, reason)
	if o.dropAlert != nil {
		o.dropAlert(ctx, row.EventType, row.ID, reason)
	}
	if err := o.queries.MarkOutboxDelivered(ctx, row.ID); err != nil {
		log.Printf("[outbox] drop row %s: %v", row.ID, err)
	}
}
