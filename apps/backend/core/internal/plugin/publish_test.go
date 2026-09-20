package plugin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The Emits contract both ways: a declared type reaches subscribers with the exact payload, an
// undeclared one is refused - always, since boot-time Consumes resolution depends on the list
func TestPublishAsync_EnforcesEmits(t *testing.T) {
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})

	Install()
	p := sdk.RegisterPlugin(sdk.Manifest{
		Slug:     "emits-plugin",
		Name:     "Emits Plugin",
		Version:  "0.1.0",
		SDKRange: ">=1.0.0",
		Emits:    []string{"emits-plugin.thing_happened"},
	})

	received := make(chan json.RawMessage, 1)
	p.SubscribeAsync("emits-plugin.thing_happened", func(_ context.Context, payload json.RawMessage) error {
		received <- payload
		return nil
	})
	SetEventBus(events.NewBus())

	ctx := context.Background()
	if err := p.PublishAsync(ctx, "emits-plugin.thing_happened", map[string]string{"ref": "pub-1"}); err != nil {
		t.Fatalf("declared publish refused: %v", err)
	}
	select {
	case payload := <-received:
		var e struct{ Ref string }
		if err := json.Unmarshal(payload, &e); err != nil || e.Ref != "pub-1" {
			t.Errorf("payload did not survive the envelope: %s (%v)", payload, err)
		}
	case <-time.After(time.Second):
		t.Fatal("published event never reached the subscriber")
	}

	if err := p.PublishAsync(ctx, "emits-plugin.undeclared", struct{}{}); err == nil {
		t.Fatal("undeclared event type published without error - Emits is a claim again")
	}
}
