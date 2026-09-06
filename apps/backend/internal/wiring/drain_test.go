package wiring

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/goleak"

	"github.com/skeletg/skelet/apps/backend/internal/events"
)

// drainBackground must not return until every tracked loop has finished, because the caller closes the pools the moment it does - and in production the failure is quiet, logging "did not stop in time" and carrying on
func TestDrainBackgroundWaitsForTrackedLoops(t *testing.T) {
	defer goleak.VerifyNone(t)

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		bg := BackgroundWaitGroup{WaitGroup: &wg}

		finished := make(chan struct{})
		bg.Go(func() { // unwinds on the signal ctx like a real loop, but not instantly - a drain that merely polled would race past
			<-ctx.Done()
			time.Sleep(50 * time.Millisecond)
			close(finished)
		})

		bus := events.NewBus()
		cancel()
		drainBackground(&wg, bus)

		select {
		case <-finished:
		default:
			t.Fatal("drainBackground returned while a tracked loop was still running - the pools would close under it")
		}
	})
}

func TestDrainBackgroundWaitsForInFlightSubscribers(t *testing.T) { // an in-flight handler has to be waited out too, or an event is lost after its emitter already committed
	defer goleak.VerifyNone(t)

	synctest.Test(t, func(t *testing.T) {
		bus := events.NewBus()
		started := make(chan struct{})
		done := make(chan struct{})
		bus.Subscribe(customerRegisteredForTest, func(context.Context, events.Event) error {
			close(started)
			time.Sleep(50 * time.Millisecond)
			close(done)
			return nil
		})

		bus.Publish(context.Background(), testEvent{})
		<-started

		var wg sync.WaitGroup
		drainBackground(&wg, bus)

		select {
		case <-done:
		default:
			t.Fatal("drainBackground returned with a subscriber still in flight")
		}
	})
}

const customerRegisteredForTest = "test.drain"

type testEvent struct{}

func (testEvent) Type() string { return customerRegisteredForTest }
