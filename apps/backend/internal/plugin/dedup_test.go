package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

type memDedup struct {
	mu      sync.Mutex
	claimed map[string]bool
}

func (m *memDedup) Claim(_ context.Context, slug, event, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := slug + "|" + event + "|" + key
	if m.claimed[k] {
		return false, nil
	}
	m.claimed[k] = true
	return true, nil
}

func (m *memDedup) Release(_ context.Context, slug, event, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.claimed, slug+"|"+event+"|"+key)
	return nil
}

// The three-way contract in one flow: a handler error releases the claim (replay retries), a
// processed key is skipped, a fresh key runs
func TestSubscribeAsyncOnce_SkipsReplaysAndRetriesAfterErrors(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()
	SetEventDedup(&memDedup{claimed: map[string]bool{}})

	var runs int
	failFirst := true
	sdk.NewPluginForTest("dedup-plugin").SubscribeAsyncOnce("test.event",
		func(payload json.RawMessage) (string, error) {
			var e fakeEvent
			if err := json.Unmarshal(payload, &e); err != nil {
				return "", err
			}
			return e.Value, nil
		},
		func(context.Context, json.RawMessage) error {
			runs++
			if failFirst {
				failFirst = false
				return errors.New("transient")
			}
			return nil
		})

	bus := events.NewBus()
	SetEventBus(bus)

	ctx := context.Background()
	_ = bus.DispatchAsyncNow(ctx, &fakeEvent{Value: "k1"}) // fails - the claim releases
	_ = bus.DispatchAsyncNow(ctx, &fakeEvent{Value: "k1"}) // retry succeeds
	_ = bus.DispatchAsyncNow(ctx, &fakeEvent{Value: "k1"}) // replay of a processed key - skipped
	_ = bus.DispatchAsyncNow(ctx, &fakeEvent{Value: "k2"}) // fresh key runs

	if runs != 3 {
		t.Fatalf("want 3 handler runs (fail, retry, fresh key), got %d", runs)
	}
}
