package events

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type testEvent struct {
	name string
	tags []string
}

func (e *testEvent) Type() string { return e.name }

func TestPublishSync_RunsInPriorityOrder(t *testing.T) {
	t.Parallel()
	bus := NewBus()
	var order []int
	bus.SubscribeSync("t", 500, func(_ context.Context, _ Event) error { order = append(order, 500); return nil })
	bus.SubscribeSync("t", 100, func(_ context.Context, _ Event) error { order = append(order, 100); return nil })
	bus.SubscribeSync("t", 900, func(_ context.Context, _ Event) error { order = append(order, 900); return nil })
	bus.SubscribeSync("t", 100, func(_ context.Context, _ Event) error { order = append(order, 101); return nil })

	if err := bus.PublishSync(context.Background(), &testEvent{name: "t"}); err != nil {
		t.Fatalf("PublishSync: %v", err)
	}
	want := []int{100, 101, 500, 900}
	if len(order) != len(want) {
		t.Fatalf("order len %d, want %d: got %v", len(order), len(want), order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order[%d]=%d, want %d: %v", i, order[i], want[i], order)
		}
	}
}

func TestPublishSync_FirstErrorAborts(t *testing.T) {
	t.Parallel()
	bus := NewBus()
	sentinel := errors.New("nope")
	var ran atomic.Int32
	bus.SubscribeSync("t", 100, func(_ context.Context, _ Event) error { ran.Add(1); return nil })
	bus.SubscribeSync("t", 200, func(_ context.Context, _ Event) error { ran.Add(1); return sentinel })
	bus.SubscribeSync("t", 300, func(_ context.Context, _ Event) error { ran.Add(1); return nil })

	err := bus.PublishSync(context.Background(), &testEvent{name: "t"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v, want sentinel", err)
	}
	if got := ran.Load(); got != 2 {
		t.Fatalf("ran=%d, want 2 (third handler must not run after abort)", got)
	}
}

func TestPublishSync_MutatesPayload(t *testing.T) {
	t.Parallel()
	bus := NewBus()
	bus.SubscribeSync("t", 100, func(_ context.Context, e Event) error {
		e.(*testEvent).tags = append(e.(*testEvent).tags, "first")
		return nil
	})
	bus.SubscribeSync("t", 200, func(_ context.Context, e Event) error {
		e.(*testEvent).tags = append(e.(*testEvent).tags, "second")
		return nil
	})

	evt := &testEvent{name: "t"}
	if err := bus.PublishSync(context.Background(), evt); err != nil {
		t.Fatal(err)
	}
	if len(evt.tags) != 2 || evt.tags[0] != "first" || evt.tags[1] != "second" {
		t.Fatalf("tags=%v, want [first second]", evt.tags)
	}
}

func TestPublish_DetachesCancellation(t *testing.T) {
	t.Parallel()
	bus := NewBus()
	done := make(chan struct{})
	bus.Subscribe("t", func(ctx context.Context, _ Event) error {
		// If cancellation leaked through, ctx.Err() would be non-nil immediately because we cancel the parent before the
		// goroutine starts running - The assertion is: handler sees no error
		if ctx.Err() != nil {
			t.Errorf("handler ctx.Err()=%v, want nil", ctx.Err())
		}
		close(done)
		return nil
	})
	parent, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE publish
	bus.Publish(parent, &testEvent{name: "t"})

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("handler never ran")
	}
}

func TestPublish_NoSubscribersIsNoop(t *testing.T) {
	t.Parallel()
	bus := NewBus()
	// Must not panic or block; no observable state to assert beyond return
	bus.Publish(context.Background(), &testEvent{name: "unsubscribed"})
	if err := bus.PublishSync(context.Background(), &testEvent{name: "unsubscribed"}); err != nil {
		t.Fatalf("PublishSync with no subs: %v", err)
	}
}

func TestPublish_HandlerPanicRecovered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := NewBus()
		var ranAfter atomic.Int32
		bus.Subscribe("t", func(_ context.Context, _ Event) error { panic("boom") })
		bus.Subscribe("t", func(_ context.Context, _ Event) error { ranAfter.Add(1); return nil })

		bus.Publish(context.Background(), &testEvent{name: "t"})

		// Waits for the async handlers to finish instead of polling a 500ms deadline
		synctest.Wait()
		if got := ranAfter.Load(); got != 1 {
			t.Fatalf("second async handler did not run after first panicked (ran=%d)", got)
		}
	})
}

// Deliberately NOT synctest: this exists to give the race detector real interleaving to observe, and
// a bubble schedules goroutines deterministically on a fake clock, which is the opposite of the point
func TestBus_ConcurrentSubscribeAndPublish(t *testing.T) {
	t.Parallel()
	bus := NewBus()
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// One goroutine publishes continuously
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				bus.Publish(context.Background(), &testEvent{name: "race"})
			}
		}
	})

	// Another subscribes continuously - Subscribe takes the write lock
	// while Publish takes the read lock - Race detector should find nothing
	wg.Go(func() {
		for range 1000 {
			bus.Subscribe("race", func(_ context.Context, _ Event) error { return nil })
		}
	})

	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestDrainWaitsForInflightHandlers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := NewBus()
		done := make(chan struct{})
		bus.Subscribe("drain.fast", func(_ context.Context, _ Event) error {
			time.Sleep(20 * time.Millisecond) // fake time - the handler is genuinely mid-flight when Drain runs
			close(done)
			return nil
		})
		bus.Publish(context.Background(), &testEvent{name: "drain.fast"})

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := bus.Drain(ctx); err != nil {
			t.Fatalf("Drain: %v", err)
		}
		select {
		case <-done:
		default:
			t.Fatal("Drain returned before the handler finished")
		}
	})
}

func TestDrainCancelsStragglers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := NewBus()
		cancelled := make(chan struct{})
		bus.Subscribe("drain.slow", func(ctx context.Context, _ Event) error {
			// Simulates a webhook retry backoff: parked until the handler
			// context cancels (which Drain does after the grace window)
			select {
			case <-ctx.Done():
				close(cancelled)
			case <-time.After(30 * time.Second):
			}
			return nil
		})
		bus.Publish(context.Background(), &testEvent{name: "drain.slow"})

		// Tight budget → grace window is half of it, then stop closes
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		if err := bus.Drain(ctx); err != nil {
			t.Fatalf("Drain: %v", err)
		}
		select {
		case <-cancelled:
		default:
			t.Fatal("handler context was not cancelled during drain")
		}
	})
}
