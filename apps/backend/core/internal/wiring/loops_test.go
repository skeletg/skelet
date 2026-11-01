package wiring

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/fx"
	"go.uber.org/goleak"
)

type recordingLifecycle struct{ hooks []fx.Hook }

func (r *recordingLifecycle) Append(h fx.Hook) { r.hooks = append(r.hooks, h) }

// Guards the ordering the fx migration once broke - fx runs invokes inside fx.New, but the entrypoint keeps registering subscribers long after that, and the outbox relay marks a row delivered whether or not anyone was listening - nil services are the assertion: anything the body touches instead of deferring panics right here
func TestLoopsDeferToOnStart(t *testing.T) {
	for _, tc := range []struct {
		name   string
		invoke func(fx.Lifecycle, RootContext, BackgroundWaitGroup)
	}{
		{"server", func(lc fx.Lifecycle, rc RootContext, bg BackgroundWaitGroup) {
			startServerLoops(lc, rc, bg, nil, nil)
		}},
		{"background", func(lc fx.Lifecycle, rc RootContext, bg BackgroundWaitGroup) {
			startBackgroundLoops(lc, rc, bg, nil, nil, nil, nil, nil, nil, nil, nil, SignalsEnabled(false), nil, nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bg sync.WaitGroup
			lc := &recordingLifecycle{}

			tc.invoke(lc, RootContext{Context: context.Background()}, BackgroundWaitGroup{WaitGroup: &bg})

			if len(lc.hooks) != 1 {
				t.Fatalf("want the loops parked on one lifecycle hook, got %d", len(lc.hooks))
			}
			if lc.hooks[0].OnStart == nil {
				t.Fatal("hook has no OnStart, so the loops never start")
			}
			if lc.hooks[0].OnStop == nil {
				t.Fatal("hook has no OnStop, so nothing stops the loops before the pools close")
			}
		})
	}
}

// With the suite on, the roster prime parks beside the loops rather than running inside the invoke -
// pushing there would reach a channel service and a gRPC client during fx.New, and nil ones say so
func TestTheChannelRosterPrimeDefersToOnStart(t *testing.T) {
	var bg sync.WaitGroup
	lc := &recordingLifecycle{}

	startBackgroundLoops(lc, RootContext{Context: context.Background()}, BackgroundWaitGroup{WaitGroup: &bg},
		nil, nil, nil, nil, nil, nil, nil, nil, SignalsEnabled(true), nil, nil)

	if len(lc.hooks) != 2 {
		t.Fatalf("want the loops and the roster prime on two hooks, got %d", len(lc.hooks))
	}
	if lc.hooks[0].OnStart == nil {
		t.Fatal("the roster prime has no OnStart, so the suite starts with no channels")
	}
}

// The OnStop half - a failed Start or a listener that never bound skips the entrypoint's drain, so fx's unwind is the only thing left
func TestLoopsStopOnLifecycleStop(t *testing.T) {
	defer goleak.VerifyNone(t)

	synctest.Test(t, func(t *testing.T) {
		var bg sync.WaitGroup
		lc := &recordingLifecycle{}
		running, stopped := make(chan struct{}), make(chan struct{})

		parkLoops(lc, RootContext{Context: context.Background()}, BackgroundWaitGroup{WaitGroup: &bg}, "test", // root ctx is never cancelled here, so only the hook's own cancel can end this loop
			func(ctx context.Context) {
				close(running)
				<-ctx.Done()
				time.Sleep(20 * time.Millisecond) // unwinds, but not instantly - a hook that cancelled without waiting would race past
				close(stopped)
			})

		if err := lc.hooks[0].OnStart(context.Background()); err != nil {
			t.Fatalf("OnStart: %v", err)
		}
		<-running

		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := lc.hooks[0].OnStop(sctx); err != nil {
			t.Fatalf("OnStop: %v", err)
		}

		select {
		case <-stopped:
		default:
			t.Fatal("OnStop returned while a loop was still running - the pools would close under it")
		}
		bg.Wait() // the entrypoint's group has to be settled too, or its own drain would return early
	})
}
