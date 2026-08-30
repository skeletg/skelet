package promotion

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// The Valkey pub/sub channel rule invalidations fan out on - every replica subscribes, so a rule edited on one pod stops firing on all of them at once
const invalidationChannel = "promotion:rules:invalidate"

// Broadcasts "drop your rules cache" across replicas, following internal/notification.Hub's shape
// Without it, Service's Evict only reached the replica that served the mutation and every OTHER replica kept pricing carts from stale rules until its TTL lapsed - an operator switching off a misconfigured discount had to wait it out
// Best-effort, like the notification hub: a dropped message or a Valkey outage falls back to the TTL, which is why the TTL still exists
type CacheBus struct {
	client *redis.Client
}

// client may be nil on tests and CLI, where there's one process and the local evict is already complete
func NewCacheBus(client *redis.Client) *CacheBus {
	if client == nil {
		return nil
	}
	return &CacheBus{client: client}
}

func (b *CacheBus) publish(ctx context.Context) {
	if b == nil || b.client == nil {
		return
	}
	// The payload carries nothing - there's one thing to say and the receiver wipes everything, same as a local EvictAll
	if err := b.client.Publish(ctx, invalidationChannel, "1").Err(); err != nil {
		slog.WarnContext(ctx, "promotion: broadcast cache invalidation", "err", err)
	}
}

// Relays remote invalidations into the engine's cache until ctx is cancelled - go-redis reconnects internally, and the outer loop covers the channel-closed case from connection churn
// Call it once per process in a goroutine, the same way notification.Hub.Run is wired
func (e *Engine) RunCacheInvalidation(ctx context.Context) {
	if e == nil || e.bus == nil || e.bus.client == nil {
		return
	}
	for {
		ps := e.bus.client.Subscribe(ctx, invalidationChannel)
		msgs := ps.Channel()
		open := true
		for open {
			select {
			case <-ctx.Done():
				_ = ps.Close()
				return
			case _, ok := <-msgs:
				if !ok {
					open = false
					continue
				}
				e.cache.EvictAll()
			}
		}
		_ = ps.Close()
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
