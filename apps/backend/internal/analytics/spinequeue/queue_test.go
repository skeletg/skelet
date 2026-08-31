package spinequeue

import (
	"context"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
)

type fakeSpine struct {
	analytics.SpineStore // only AppendEvent matters to the consumer - this nil interface panics if anything else is ever touched
	mu                   sync.Mutex
	events               []analytics.SpineEvent
}

func (f *fakeSpine) AppendEvent(_ context.Context, e analytics.SpineEvent) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return true, nil
}

func (f *fakeSpine) AppendEvents(_ context.Context, events []analytics.SpineEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, events...)
	return nil
}

func (f *fakeSpine) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

// End to end against an embedded JetStream server - a produced event reaches the engine exactly once even when the producer retries it (msg-id dedupe), and the wrap passes the durability bar before AppendEvent returns
func TestQueueRoundTripAndDedupe(t *testing.T) {
	srv, err := natsserver.NewServer(&natsserver.Options{Port: -1, JetStream: true, StoreDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	defer srv.Shutdown()
	if !srv.ReadyForConnections(10 * time.Second) {
		t.Fatal("embedded nats not ready")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	q, err := New(ctx, srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()

	inner := &fakeSpine{}
	consumerCtx, stopConsumer := context.WithCancel(ctx)
	defer stopConsumer()
	done := make(chan struct{})
	go func() { defer close(done); _ = q.Consume(consumerCtx, inner) }()

	wrapped := q.WrapSpine(inner)
	ev := analytics.SpineEvent{EventName: "page_view", OccurredAt: time.Now().UTC(), AnonID: "a1", Source: "web", Props: map[string]any{"k": "v"}, DedupeKey: "qtest:1"}
	for i := range 3 { // a sendBeacon retransmit publishes the same key thrice
		if ok, err := wrapped.AppendEvent(ctx, ev); err != nil || !ok {
			t.Fatalf("publish %d: ok=%v err=%v", i, ok, err)
		}
	}
	ev2 := ev
	ev2.DedupeKey = "qtest:2"
	if _, err := wrapped.AppendEvent(ctx, ev2); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for inner.count() < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	stopConsumer()
	<-done
	if got := inner.count(); got != 2 {
		t.Fatalf("engine got %d events, want 2 (three publishes of one key + one distinct)", got)
	}
	if inner.events[0].EventName != "page_view" || inner.events[0].Props["k"] != "v" {
		t.Fatalf("payload mangled in transit: %+v", inner.events[0])
	}
}
