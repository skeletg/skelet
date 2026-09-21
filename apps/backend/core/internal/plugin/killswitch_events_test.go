package plugin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// A disabled plugin's async event subscriber must not fire - the host
// enforces the kill switch (attachAsyncHandler), not the plugin
func TestSubscribeAsync_KillSwitchSkips(t *testing.T) {
	bus := events.NewBus()
	sdk.ResetForTest()
	resetHostForTest()
	orig := isEnabledFromContext
	// Publish detaches, so a handler can still be reading isEnabledFromContext when the test ends -
	// drain BEFORE restoring it, or the restore races the goroutine
	t.Cleanup(func() {
		drainBus(t, bus)
		sdk.ResetForTest()
		resetHostForTest()
		isEnabledFromContext = orig
	})
	isEnabledFromContext = func(_ context.Context, slug string, _ uuid.UUID) bool {
		return slug != "off-plugin" // off-plugin is disabled
	}

	Install()
	fired := make(chan struct{}, 1)
	sdk.NewPluginForTest("off-plugin").SubscribeAsync("test.event", func(_ context.Context, _ json.RawMessage) error {
		fired <- struct{}{}
		return nil
	})
	SetEventBus(bus)
	bus.Publish(context.Background(), &fakeEvent{Value: "x"})

	select {
	case <-fired:
		t.Fatal("disabled plugin's async handler fired")
	case <-time.After(200 * time.Millisecond):
		// success - handler skipped
	}
}

// A disabled plugin's sync subscriber is skipped (returns nil) - it must
// NOT abort the emitting operation
func TestSubscribeSync_KillSwitchSkipsWithoutAborting(t *testing.T) {
	bus := events.NewBus()
	sdk.ResetForTest()
	resetHostForTest()
	orig := isEnabledFromContext
	t.Cleanup(func() {
		drainBus(t, bus)
		sdk.ResetForTest()
		resetHostForTest()
		isEnabledFromContext = orig
	})
	isEnabledFromContext = func(_ context.Context, slug string, _ uuid.UUID) bool {
		return slug != "off-plugin"
	}

	Install()
	sdk.NewPluginForTest("off-plugin").SubscribeSync("test.event", 100, func(_ context.Context, _ any) error {
		return context.Canceled // would abort IF it ran
	})
	SetEventBus(bus)

	// A disabled sync handler is skipped, so PublishSync succeeds
	if err := bus.PublishSync(context.Background(), &fakeEvent{Value: "x"}); err != nil {
		t.Fatalf("disabled sync handler must not abort the chain, got %v", err)
	}
}

// Waits out the detached async handlers so a test's Cleanup can safely restore package globals the
// handlers read - without it the restore races whatever is still in flight
func drainBus(t *testing.T, bus *events.Bus) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := bus.Drain(ctx); err != nil {
		t.Logf("drain bus: %v", err)
	}
}
