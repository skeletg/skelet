package events

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/goleak"
)

// WaitIdle must not return while a handler runs, and unlike Drain must leave the bus usable after
func TestWaitIdleWaitsAndBusSurvives(t *testing.T) {
	defer goleak.VerifyNone(t) // outside the bubble - inside, it sees the bubble's own goroutine

	synctest.Test(t, func(t *testing.T) {
		bus := NewBus()
		first, second := make(chan struct{}), make(chan struct{})
		bus.Subscribe("test.waitidle", func(context.Context, Event) error {
			time.Sleep(30 * time.Millisecond) // slow enough that a WaitIdle racing past would return early
			select {
			case <-first:
				close(second)
			default:
				close(first)
			}
			return nil
		})

		bus.Publish(context.Background(), waitIdleEvent{})
		if err := bus.WaitIdle(context.Background()); err != nil {
			t.Fatalf("WaitIdle: %v", err)
		}
		select {
		case <-first:
		default:
			t.Fatal("WaitIdle returned while the handler was still in flight")
		}

		// The stop channel stays open, so a later publish still dispatches - the property Drain gives up
		bus.Publish(context.Background(), waitIdleEvent{})
		if err := bus.WaitIdle(context.Background()); err != nil {
			t.Fatalf("second WaitIdle: %v", err)
		}
		select {
		case <-second:
		case <-time.After(time.Second):
			t.Fatal("bus stopped dispatching after WaitIdle")
		}
	})
}

func TestWaitIdleHonorsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {

		bus := NewBus()
		release := make(chan struct{})
		bus.Subscribe("test.waitidle", func(context.Context, Event) error { <-release; return nil })
		bus.Publish(context.Background(), waitIdleEvent{})

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if err := bus.WaitIdle(ctx); err == nil {
			t.Fatal("WaitIdle returned nil while a handler was parked")
		}
		close(release)
		_ = bus.WaitIdle(context.Background()) // settle before goleak-free tests elsewhere in the package run
	})
}

type waitIdleEvent struct{}

func (waitIdleEvent) Type() string { return "test.waitidle" }
