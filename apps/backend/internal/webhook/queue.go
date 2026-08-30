package webhook

import (
	"context"
	"log"
	"runtime/debug"
	"sync"
	"time"
)

// Delivers queued async webhooks - the dispatcher enqueues one webhook_queue row per matching webhook, and this worker claims due rows under a short lease with SKIP LOCKED so replicas partition the work, attempts one delivery each, reschedules failures on the retry ladder and parks exhausted rows as dead letters
// Rows are claimed by pushing next_attempt_at forward, so a crash mid-delivery replays the row after the lease - delivery is at-least-once and receivers must dedupe
// Unlike an in-memory retry chain this survives restarts between attempts, exposes a dead-letter queue, and makes redelivery possible

const (
	queuePollInterval = 2 * time.Second
	queueBatchSize    = 50
	// The lease must exceed the max per-attempt timeout of 30s, with headroom
	queueLeaseSeconds = 90
	queueWorkers      = 8
)

// The first attempt plus one per retry delay
var maxQueueAttempts = int32(1 + len(retryDelays))

type QueueWorker struct {
	svc        *Service
	dispatcher *Dispatcher

	pollInterval time.Duration
}

func NewQueueWorker(svc *Service, dispatcher *Dispatcher) *QueueWorker {
	return &QueueWorker{svc: svc, dispatcher: dispatcher, pollInterval: queuePollInterval}
}

// Overrides the tick - useful for tests
func (w *QueueWorker) SetPollInterval(d time.Duration) {
	if d > 0 {
		w.pollInterval = d
	}
}

// Polls for due rows until ctx is cancelled - blocks, so run it from a tracked background goroutine
func (w *QueueWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// Drains everything currently due and returns how many rows it processed - batches keep claiming until the due set is empty, so a burst doesn't wait pollInterval per batch
func (w *QueueWorker) RunOnce(ctx context.Context) int {
	processed := 0
	for ctx.Err() == nil {
		items, err := w.svc.repo.ClaimDueQueue(ctx, queueBatchSize, queueLeaseSeconds)
		if err != nil {
			log.Printf("[webhook] claim due queue: %v", err)
			return processed
		}
		if len(items) == 0 {
			return processed
		}
		processed += len(items)

		// Per-batch webhook cache - many rows usually share a webhook
		webhooks := map[string]*Webhook{}
		var wg sync.WaitGroup
		sem := make(chan struct{}, queueWorkers)
		for _, item := range items {
			wh, ok := webhooks[item.WebhookID.String()]
			if !ok {
				loaded, err := w.svc.GetByID(ctx, item.WebhookID)
				if err != nil {
					// The webhook row is gone - the FK cascade should have removed the queue row, but be defensive and drop the item
					log.Printf("[webhook] queue item %s: webhook %s missing, dropping", item.ID, item.WebhookID)
					_ = w.svc.repo.DeleteQueueItem(ctx, item.ID)
					continue
				}
				wh = &loaded
				webhooks[item.WebhookID.String()] = wh
			}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				wg.Wait()
				return processed
			}
			wg.Add(1)
			go func(item QueueItem, wh Webhook) {
				defer wg.Done()
				defer func() { <-sem }()
				// a panic here ran response handling for an untrusted endpoint with no recover and took
				// the process down - the claim lease expires and the item redelivers, so log is enough
				defer func() {
					if rec := recover(); rec != nil {
						log.Printf("[webhook] delivery of %s panicked: %v\n%s", item.ID, rec, debug.Stack())
					}
				}()
				w.deliverItem(ctx, item, wh)
			}(item, *wh)
		}
		wg.Wait()
		if len(items) < queueBatchSize {
			return processed
		}
	}
	return processed
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
	if item.Attempts >= maxQueueAttempts {
		if err := w.svc.repo.MarkQueueItemDead(ctx, item.ID, reason); err != nil {
			log.Printf("[webhook] mark queue item %s dead: %v", item.ID, err)
		}
		log.Printf("[webhook] queue item %s (%s) dead after %d attempts: %s", item.ID, item.EventType, item.Attempts, reason)
		return
	}
	delay := retryDelays[len(retryDelays)-1]
	if idx := int(item.Attempts) - 1; idx >= 0 && idx < len(retryDelays) {
		delay = retryDelays[idx]
	}
	if err := w.svc.repo.RescheduleQueueItem(ctx, item.ID, time.Now().Add(delay), reason); err != nil {
		log.Printf("[webhook] reschedule queue item %s: %v", item.ID, err)
	}
}
