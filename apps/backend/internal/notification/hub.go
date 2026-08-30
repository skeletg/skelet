package notification

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// The Valkey pub/sub channel notifications fan out on - every backend replica subscribes, so a notification created on any pod reaches the admins connected to every pod
const hubChannel = "notify:admin"

// Fans freshly created notifications out to live GraphQL subscription listeners, across replicas through Valkey pub/sub
// A LATENCY layer only - delivery is at-most-once, so a dropped message or reconnect loses the push, and correctness always comes from the DB-backed inbox the dashboard still polls as a slow backfill
type Hub struct {
	client *redis.Client

	mu   sync.Mutex
	subs map[uint64]chan Notification
	next uint64
}

// client may be nil on tests and CLI, in which case the hub fans out in-process only
func NewHub(client *redis.Client) *Hub {
	return &Hub{client: client, subs: make(map[uint64]chan Notification)}
}

// Sends a created notification to every replica's listeners, through Valkey when wired and locally otherwise - best-effort by design
func (h *Hub) Publish(ctx context.Context, n Notification) {
	if h == nil {
		return
	}
	if h.client != nil {
		if b, err := json.Marshal(n); err == nil {
			if err := h.client.Publish(ctx, hubChannel, b).Err(); err != nil {
				log.Printf("notification hub: publish: %v", err)
			}
			return // the Run loop delivers locally too - no double fan-out
		}
	}
	h.fanOut(n)
}

// Registers a listener - the returned channel is buffered, and a listener that can't keep up has messages DROPPED rather than blocking producers, since the poll backfill covers the gap - Call the cancel func to unregister
func (h *Hub) Subscribe() (<-chan Notification, func()) {
	ch := make(chan Notification, 16)
	h.mu.Lock()
	id := h.next
	h.next++
	h.subs[id] = ch
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, id)
		h.mu.Unlock()
	}
}

func (h *Hub) fanOut(n Notification) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		select {
		case ch <- n:
		default: // slow listener: drop, the inbox poll backfills
		}
	}
}

// Subscribes to the Valkey channel and relays to local listeners until ctx is cancelled - go-redis PubSub reconnects internally, and the outer loop covers the channel-closed case from connection churn with a short backoff before resubscribing
func (h *Hub) Run(ctx context.Context) {
	if h == nil || h.client == nil {
		return
	}
	for {
		ps := h.client.Subscribe(ctx, hubChannel)
		msgs := ps.Channel()
		open := true
		for open {
			select {
			case <-ctx.Done():
				_ = ps.Close()
				return
			case m, ok := <-msgs:
				if !ok {
					open = false
					continue
				}
				var n Notification
				if err := json.Unmarshal([]byte(m.Payload), &n); err == nil {
					h.fanOut(n)
				}
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
