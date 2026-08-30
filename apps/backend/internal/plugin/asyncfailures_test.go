package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/sdk"
)

// The drain is what the notification scan reads - failures tally per (plugin, event), a drain
// resets, and a quiet interval yields nothing
func TestAsyncFailuresTallyAndDrain(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})
	TakeAsyncFailures() // earlier tests in this binary may have tallied

	Install()

	boom := errors.New("sink unreachable")
	sdk.NewPluginForTest("fail-plugin").SubscribeAsync("test.event", func(context.Context, json.RawMessage) error {
		return boom
	})

	bus := events.NewBus()
	SetEventBus(bus)
	// inline dispatch, so the failure is recorded before the assertions run
	_ = bus.DispatchAsyncNow(context.Background(), &fakeEvent{Value: "x"})
	_ = bus.DispatchAsyncNow(context.Background(), &fakeEvent{Value: "y"})

	got := TakeAsyncFailures()
	if len(got) != 1 {
		t.Fatalf("want one tally entry, got %d: %+v", len(got), got)
	}
	f := got[0]
	if f.PluginSlug != "fail-plugin" || f.EventType != "test.event" || f.Count != 2 {
		t.Errorf("tally: %+v", f)
	}
	if f.LastError != "sink unreachable" || f.LastAt.IsZero() {
		t.Errorf("last error not captured: %+v", f)
	}

	if again := TakeAsyncFailures(); len(again) != 0 {
		t.Fatalf("drain did not reset: %+v", again)
	}
}
