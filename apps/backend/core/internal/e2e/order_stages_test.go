package e2e

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

var (
	stagesOnce   sync.Once
	stagesPlugin = sdk.NewPluginForTest("e2e-stages")
)

// Scenario: a subscriber holds a staged order's shipment back until the plugin moves it on, and shipping clears the stage
func TestAStageHoldsAShipmentBackUntilThePluginMovesItOn(t *testing.T) {
	h := newHarness(t)
	stagesOnce.Do(func() {
		stagesPlugin.RegisterOrderStage(sdk.OrderStage{Key: "e2e_awaiting", Label: "Awaiting stock", Within: []string{sdk.OrderStatusConfirmed, sdk.OrderStatusProcessing}})
		stagesPlugin.RegisterOrderStage(sdk.OrderStage{Key: "e2e_packed", Label: "Packed", Within: []string{sdk.OrderStatusProcessing}})
	})
	ctx := context.Background()
	placed := h.PlaceGuestCOD(t, h.NewStore(t, 10, 2000), 1)
	h.D.Bus.SubscribeSync(order.EventTransitioning, 100, func(_ context.Context, e events.Event) error {
		if m := e.(*order.OrderTransitioning); m.OrderID == placed.ID && m.FromStage == stagesPlugin.Key("e2e_awaiting") && m.ToStatus == order.OrderStatusFulfilled {
			return errors.New("still awaiting stock")
		}
		return nil
	})

	if _, err := h.D.Order.UpdateStatus(ctx, placed.ID, order.OrderStatusConfirmed); err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.Order.SetStage(ctx, placed.ID, stagesPlugin.Key("e2e_awaiting"), ""); err != nil {
		t.Fatal(err)
	}
	o, err := h.D.Order.UpdateStatus(ctx, placed.ID, order.OrderStatusProcessing)
	if err != nil || o.Stage != stagesPlugin.Key("e2e_awaiting") {
		t.Fatalf("processing: %v, stage %q", err, o.Stage)
	}
	if _, err := h.D.Order.FulfillRemaining(ctx, placed.ID, "", "", 0); err == nil {
		t.Fatal("shipped while a subscriber held it back")
	}

	if err := stagesPlugin.SetOrderStage(ctx, placed.ID, "e2e_packed"); err != nil {
		t.Fatalf("the plugin moving its own order on: %v", err)
	}
	if o, err = h.D.Order.FulfillRemaining(ctx, placed.ID, "", "", 0); err != nil {
		t.Fatal(err)
	}
	if o.Status != order.OrderStatusFulfilled || o.Stage != "" {
		t.Errorf("shipped: %s with stage %q, want fulfilled with none", o.Status, o.Stage)
	}
}
