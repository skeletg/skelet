package plugin

import (
	"context"
	"errors"
	"testing"
	"time"
)

func resetShutdownForTest(t *testing.T) {
	t.Helper()
	shutdownRegistryInstance.mu.Lock()
	shutdownRegistryInstance.hooks = nil
	shutdownRegistryInstance.mu.Unlock()
	t.Cleanup(func() {
		shutdownRegistryInstance.mu.Lock()
		shutdownRegistryInstance.hooks = nil
		shutdownRegistryInstance.mu.Unlock()
	})
}

// Every hook runs, and one that fails or panics can't stop the others - shutdown has no caller
// left to surface an error to, so the only wrong behaviour is not finishing
func TestShutdownRunsEveryHookDespiteFailures(t *testing.T) {
	resetShutdownForTest(t)
	h := &Host{}
	ran := make(chan string, 3)

	_ = h.RegisterShutdown("ok", func(context.Context) error { ran <- "ok"; return nil })
	_ = h.RegisterShutdown("errs", func(context.Context) error { ran <- "errs"; return errors.New("boom") })
	_ = h.RegisterShutdown("panics", func(context.Context) error { ran <- "panics"; panic("bang") })

	Shutdown(context.Background())
	close(ran)

	seen := map[string]bool{}
	for s := range ran {
		seen[s] = true
	}
	for _, want := range []string{"ok", "errs", "panics"} {
		if !seen[want] {
			t.Errorf("hook %q never ran", want)
		}
	}
}

// A hook ignoring its deadline must not hold the process open - the ctx is cancelled and Shutdown
// returns without it, which is the same cooperative bound Instrument gives hooks
func TestShutdownBoundsAHangingHook(t *testing.T) {
	resetShutdownForTest(t)
	h := &Host{}
	observed := make(chan error, 1)
	_ = h.RegisterShutdown("slow", func(ctx context.Context) error {
		<-ctx.Done()
		observed <- ctx.Err()
		return ctx.Err()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { Shutdown(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown did not return - a hanging hook blocked the exit")
	}
	if err := <-observed; !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("hook ctx was not cancelled: %v", err)
	}
}

// No plugins means no hooks, and shutdown must stay a no-op rather than waiting on nothing
func TestShutdownWithNoHooks(t *testing.T) {
	resetShutdownForTest(t)
	Shutdown(context.Background())
}
