package e2e

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Scenario: the checkout fields' answers ride the placement onto the order with their labels, and a plugin on order.placing reads them through the SDK and records its own metadata beside them
func TestCheckoutFieldsAndPluginMetadataLandOnTheOrder(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	cartID := h.NewGuestCart(t, f, 1)
	h.D.Bus.SubscribeSync(order.EventPlacing, 0, func(_ context.Context, e events.Event) error {
		p := e.(*order.OrderPlacing)
		if p.Cart.ID != cartID {
			return nil
		}
		msg, ok := sdk.CheckoutFieldAnswers(p).CheckoutFieldValue("gift_message")
		if !ok {
			t.Error("order.placing didn't carry the gift message")
		}
		return sdk.OrderPlacingMetadata(p).SetPlacingMetadata("acme.message_length", strconv.Itoa(len(msg)))
	})

	email := "fields@e2e.test"
	answers := []order.CheckoutFieldAnswer{{Key: "gift_message", Label: "Gift message", Value: "Happy birthday"}}
	placed, err := h.TryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.Email, in.CheckoutFields = email, answers
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	got, err := h.D.Order.GetGuestOrder(h.Ctx, placed.ID, email)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.CheckoutFields, answers) {
		t.Errorf("checkout fields = %+v, want %+v", got.CheckoutFields, answers)
	}
	if want := map[string]string{"acme.message_length": "14"}; !maps.Equal(got.Metadata, want) {
		t.Errorf("metadata = %v, want %v", got.Metadata, want)
	}
}
