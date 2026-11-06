package webhook

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/leasequeue"
)

// Delivers queued async webhooks - the dispatcher enqueues one webhook_queue row per matching webhook, and this worker claims due rows under a short lease with SKIP LOCKED so replicas partition the work, attempts one delivery each, reschedules failures on the retry ladder and parks exhausted rows as dead letters Rows are claimed by pushing next_attempt_at forward, so a crash mid-delivery replays the row after the lease - delivery is at-least-once and receivers must dedupe Unlike an in-memory retry chain this survives restarts between attempts, exposes a dead-letter queue, and makes redelivery possible

const (
	queuePollInterval = 2 * time.Second
	queueBatchSize    = 50
	// The lease must exceed the max per-attempt timeout of 30s, with headroom
	queueLeaseSeconds     = 90
	queueWorkers          = 8
	enqueueDedupRetention = 48 * time.Hour
)

// The first attempt plus one per retry delay
var maxQueueAttempts = int32(1 + len(retryDelays))

type QueueWorker struct {
	svc        *Service
	dispatcher *Dispatcher
	runner     leasequeue.Runner[QueueItem]
}

func NewQueueWorker(svc *Service, dispatcher *Dispatcher) *QueueWorker {
	w := &QueueWorker{svc: svc, dispatcher: dispatcher}
	w.runner = leasequeue.Runner[QueueItem]{
		Name: "webhook", Poll: queuePollInterval, Batch: queueBatchSize, Lease: queueLeaseSeconds, Workers: queueWorkers,
		Claim:   svc.repo.ClaimDueQueue,
		Prepare: w.prepare,
		Hourly: func(ctx context.Context) {
			if _, err := svc.repo.PruneEnqueueDedup(ctx, time.Now().Add(-enqueueDedupRetention)); err != nil {
				log.Printf("[webhook] prune enqueue dedup: %v", err)
			}
		},
	}
	return w
}

// Overrides the tick - useful for tests
func (w *QueueWorker) SetPollInterval(d time.Duration) {
	if d > 0 {
		w.runner.Poll = d
	}
}

// Polls for due rows until ctx is cancelled - blocks, so run it from a tracked background goroutine
func (w *QueueWorker) Run(ctx context.Context) { w.runner.Run(ctx) }

// Drains everything currently due and returns how many rows it processed
func (w *QueueWorker) RunOnce(ctx context.Context) int { return w.runner.RunOnce(ctx) }

// Loads each webhook a batch names once - many rows usually share one
func (w *QueueWorker) prepare(ctx context.Context, items []QueueItem) func(context.Context, QueueItem) {
	webhooks := map[uuid.UUID]Webhook{}
	for _, item := range items {
		if _, ok := webhooks[item.WebhookID]; ok {
			continue
		}
		if wh, err := w.svc.GetByID(ctx, item.WebhookID); err == nil {
			webhooks[item.WebhookID] = wh
		}
	}
	return func(ctx context.Context, item QueueItem) {
		wh, ok := webhooks[item.WebhookID]
		if !ok {
			// The webhook row is gone - the FK cascade should have removed the queue row, but be defensive and drop the item
			log.Printf("[webhook] queue item %s: webhook %s missing, dropping", item.ID, item.WebhookID)
			_ = w.svc.repo.DeleteQueueItem(ctx, item.ID)
			return
		}
		w.deliverItem(ctx, item, wh)
	}
}

func (w *QueueWorker) deliverItem(ctx context.Context, item QueueItem, wh Webhook) {
	if !wh.IsActive {
		// Don't spin on a paused webhook - park the row, and re-activating then retrying the dead letter delivers it
		if err := w.svc.repo.MarkQueueItemDead(ctx, item.ID, "webhook inactive"); err != nil {
			log.Printf("[webhook] park inactive queue item %s: %v", item.ID, err)
		}
		return
	}

	secret, err := w.svc.GetSigningSecret(ctx, wh.ID)
	if err != nil {
		log.Printf("[webhook] get secret for %s: %v", wh.ID, err)
		w.failItem(ctx, item, "signing secret unavailable")
		return
	}

	delivery, _ := w.dispatcher.attemptDelivery(ctx, wh, secret, []byte(item.Payload), item.EventType, item.Attempts, effectiveTimeout(wh, asyncHTTPTimeout))
	if _, err := w.svc.RecordDelivery(ctx, delivery); err != nil {
		log.Printf("[webhook] record delivery for %s: %v", wh.ID, err)
	}

	if delivery.Success {
		if err := w.svc.repo.DeleteQueueItem(ctx, item.ID); err != nil {
			log.Printf("[webhook] complete queue item %s: %v (will replay after lease)", item.ID, err)
		}
		return
	}

	reason := "delivery failed"
	if delivery.ErrorMessage != nil {
		reason = *delivery.ErrorMessage
	}
	w.failItem(ctx, item, reason)
}

// Schedules the next retry, or parks the row as a dead letter once the ladder is exhausted
func (w *QueueWorker) failItem(ctx context.Context, item QueueItem, reason string) {
	delay, retry := leasequeue.Retry(item.Attempts, retryDelays)
	if !retry {
		if err := w.svc.repo.MarkQueueItemDead(ctx, item.ID, reason); err != nil {
			log.Printf("[webhook] mark queue item %s dead: %v", item.ID, err)
		}
		log.Printf("[webhook] queue item %s (%s) dead after %d attempts: %s", item.ID, item.EventType, item.Attempts, reason)
		return
	}
	if err := w.svc.repo.RescheduleQueueItem(ctx, item.ID, time.Now().Add(delay), reason); err != nil {
		log.Printf("[webhook] reschedule queue item %s: %v", item.ID, err)
	}
}
