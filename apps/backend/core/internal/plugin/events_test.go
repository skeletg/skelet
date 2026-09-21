package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The simplest events.Event for testing - JSON marshals to a single field so assertions are easy
type fakeEvent struct {
	Value string
}

func (fakeEvent) Type() string { return "test.event" }

// TestSubscribeAsync_QueueAndReplay: subscriptions made before SetEventBus get attached when the bus is wired - This is
// the load-bearing path - plugin init() runs before main.go constructs the bus
func TestSubscribeAsync_QueueAndReplay(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()

	received := make(chan json.RawMessage, 1)
	sdk.NewPluginForTest("test-plugin").SubscribeAsync("test.event", func(_ context.Context, payload json.RawMessage) error {
		received <- payload
		return nil
	})

	bus := events.NewBus()
	SetEventBus(bus)
	bus.Publish(context.Background(), &fakeEvent{Value: "hello"})

	select {
	case got := <-received:
		var e fakeEvent
		if err := json.Unmarshal(got, &e); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if e.Value != "hello" {
			t.Errorf("Value: got %q, want hello", e.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not receive event within 1s")
	}
}

// Subscriptions made AFTER the bus is wired attach directly without queuing
func TestSubscribeAsync_PostBusAttach(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()
	bus := events.NewBus()
	SetEventBus(bus)

	received := make(chan json.RawMessage, 1)
	sdk.NewPluginForTest("test-plugin").SubscribeAsync("test.event", func(_ context.Context, payload json.RawMessage) error {
		received <- payload
		return nil
	})

	bus.Publish(context.Background(), &fakeEvent{Value: "post"})

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("handler did not receive event within 1s")
	}
}

func TestSubscribeAsync_MultipleSubscribers(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()

	var calls atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3)
	for range 3 {
		sdk.NewPluginForTest("test-plugin").SubscribeAsync("test.event", func(_ context.Context, _ json.RawMessage) error {
			calls.Add(1)
			wg.Done()
			return nil
		})
	}

	bus := events.NewBus()
	SetEventBus(bus)
	bus.Publish(context.Background(), &fakeEvent{Value: "fan-out"})

	waitWG(t, &wg, time.Second)
	if calls.Load() != 3 {
		t.Errorf("calls: got %d, want 3", calls.Load())
	}
}

// TestSubscribeAsync_HandlerErrorDoesNotPropagate: a returned error from one handler is logged but does not affect
// other handlers or the originating publish
func TestSubscribeAsync_HandlerErrorDoesNotPropagate(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()

	var wg sync.WaitGroup
	wg.Add(2)
	sentinel := errors.New("handler failed")
	otherRan := false

	sdk.NewPluginForTest("test-plugin").SubscribeAsync("test.event", func(_ context.Context, _ json.RawMessage) error {
		defer wg.Done()
		return sentinel
	})
	sdk.NewPluginForTest("test-plugin").SubscribeAsync("test.event", func(_ context.Context, _ json.RawMessage) error {
		defer wg.Done()
		otherRan = true
		return nil
	})

	bus := events.NewBus()
	SetEventBus(bus)
	bus.Publish(context.Background(), &fakeEvent{})

	waitWG(t, &wg, time.Second)
	if !otherRan {
		t.Error("second handler did not run after first returned error")
	}
}

// mutableEvent is the simplest mutable-payload events.Event: a pointer field plus an "OK" gate so handlers can refuse -
// It models the cart-subtotal scenario without dragging in the cart domain
type mutableEvent struct {
	Subtotal *int64
	Reject   bool
}

func (*mutableEvent) Type() string { return "test.mutable" }

// Sync subscriptions made before SetEventBus attach at bus-wire time, same as async
func TestSubscribeSync_QueueAndReplay(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()

	called := false
	sdk.NewPluginForTest("test-plugin").SubscribeSync("test.mutable", 100, func(_ context.Context, _ any) error {
		called = true
		return nil
	})

	bus := events.NewBus()
	SetEventBus(bus)

	subtotal := int64(1000)
	if err := bus.PublishSync(context.Background(), &mutableEvent{Subtotal: &subtotal}); err != nil {
		t.Fatalf("PublishSync: %v", err)
	}
	if !called {
		t.Fatal("queued sync handler did not run after SetEventBus")
	}
}

// TestSubscribeSync_MutatesPayload: the load-bearing test - A plugin handler writing through a pointer field must show up in the publisher's local variable after PublishSync returns - This is the seam plugins use to alter core behaviour (e.g. cart subtotal surcharge for store pickup)
func TestSubscribeSync_MutatesPayload(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()
	bus := events.NewBus()
	SetEventBus(bus)

	sdk.NewPluginForTest("test-plugin").SubscribeSync("test.mutable", 100, func(_ context.Context, e any) error {
		ev, ok := e.(*mutableEvent)
		if !ok {
			t.Fatalf("type assertion failed: got %T", e)
		}
		*ev.Subtotal += 500
		return nil
	})

	subtotal := int64(1000)
	if err := bus.PublishSync(context.Background(), &mutableEvent{Subtotal: &subtotal}); err != nil {
		t.Fatalf("PublishSync: %v", err)
	}
	if subtotal != 1500 {
		t.Errorf("plugin mutation did not flow back to publisher: subtotal=%d, want 1500", subtotal)
	}
}

// TestSubscribeSync_PriorityOrder: lower priority runs first - The later (higher-priority value) handler sees the
// earlier handler's mutations - this is how plugins compose
func TestSubscribeSync_PriorityOrder(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()
	bus := events.NewBus()
	SetEventBus(bus)

	var calls []int
	sdk.NewPluginForTest("test-plugin").SubscribeSync("test.mutable", 300, func(_ context.Context, _ any) error {
		calls = append(calls, 300)
		return nil
	})
	sdk.NewPluginForTest("test-plugin").SubscribeSync("test.mutable", 100, func(_ context.Context, _ any) error {
		calls = append(calls, 100)
		return nil
	})
	sdk.NewPluginForTest("test-plugin").SubscribeSync("test.mutable", 200, func(_ context.Context, _ any) error {
		calls = append(calls, 200)
		return nil
	})

	subtotal := int64(0)
	if err := bus.PublishSync(context.Background(), &mutableEvent{Subtotal: &subtotal}); err != nil {
		t.Fatalf("PublishSync: %v", err)
	}
	want := []int{100, 200, 300}
	if len(calls) != len(want) {
		t.Fatalf("len(calls)=%d, want %d", len(calls), len(want))
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call[%d]=%d, want %d (full order: %v)", i, calls[i], want[i], calls)
		}
	}
}

// TestSubscribeSync_ErrorAborts: a handler returning error stops the chain and propagates to the publisher - Later handlers are not
// invoked, mutations from earlier handlers still apply (they already ran), but the publisher should treat the operation as failed
func TestSubscribeSync_ErrorAborts(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()
	bus := events.NewBus()
	SetEventBus(bus)

	sentinel := errors.New("plugin says no")
	laterRan := false

	sdk.NewPluginForTest("test-plugin").SubscribeSync("test.mutable", 100, func(_ context.Context, _ any) error {
		return sentinel
	})
	sdk.NewPluginForTest("test-plugin").SubscribeSync("test.mutable", 200, func(_ context.Context, _ any) error {
		laterRan = true
		return nil
	})

	subtotal := int64(0)
	err := bus.PublishSync(context.Background(), &mutableEvent{Subtotal: &subtotal})
	if !errors.Is(err, sentinel) {
		t.Errorf("PublishSync error: got %v, want sentinel", err)
	}
	if laterRan {
		t.Error("second handler ran after first returned error")
	}
}

// Sync subscriptions made after the bus is wired attach directly
func TestSubscribeSync_PostBusAttach(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()
	bus := events.NewBus()
	SetEventBus(bus)

	called := false
	sdk.NewPluginForTest("test-plugin").SubscribeSync("test.mutable", 100, func(_ context.Context, _ any) error {
		called = true
		return nil
	})

	subtotal := int64(0)
	if err := bus.PublishSync(context.Background(), &mutableEvent{Subtotal: &subtotal}); err != nil {
		t.Fatalf("PublishSync: %v", err)
	}
	if !called {
		t.Fatal("post-bus-attach sync handler did not run")
	}
}

func TestSubscribeAsync_DoubleSetEventBusPanics(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()
	SetEventBus(events.NewBus())

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("second SetEventBus did not panic")
		}
	}()
	SetEventBus(events.NewBus())
}

// resetHostForTest clears the installed host so tests can call Install + SetEventBus from a clean slate -
// Package-private - production code never resets
func resetHostForTest() {
	installedHost.Store(nil)
}

// Fails the test when wg doesn't complete within timeout
func waitWG(t *testing.T, wg *sync.WaitGroup, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("WaitGroup did not complete in time")
	}
}
