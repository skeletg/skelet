package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
)

// Scenario: an edit that takes the order past a table's not-offered ceiling can't re-price its shipping, so it keeps
// what the shopper paid and the timeline says why
func TestAnEditPastATablesCeilingKeepsTheShippingAndSaysSo(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 1000)
	repo := h.D.Shipping.Repo()
	z, err := repo.CreateZone(h.Ctx, shipping.Zone{ChannelID: f.Channel.ID, Code: "e2e-us-" + uuid.NewString()[:8], Name: "US", IsActive: true})
	if err != nil {
		t.Fatalf("create zone: %v", err)
	}
	if err := repo.AddCountryToZone(h.Ctx, z.ID, "US"); err != nil {
		t.Fatalf("zone country: %v", err)
	}
	if _, err := repo.CreateMethod(h.Ctx, shipping.Method{
		ZoneID: z.ID, Code: "standard", Name: "Standard", CalcType: shipping.CalcTable, TableBasis: shipping.BasisQuantity,
		RateTiers: []shipping.RateTier{{From: 0, PriceCents: 500}, {From: 3, PriceCents: 900}, {From: 5, NotOffered: true}},
		IsActive:  true, CountryMode: "all",
	}); err != nil {
		t.Fatalf("create method: %v", err)
	}

	o := h.PlaceGuestCOD(t, f, 2)
	if o.ShippingAmount != 500 {
		t.Fatalf("two items shipped for %d, want the first tier's 500", o.ShippingAmount)
	}
	lineID := h.OrderLines(t, o.ID)[0].ID

	raised, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{OrderID: o.ID, UpdateQty: []order.EditLineQty{{OrderLineID: lineID, Quantity: 3}}})
	if err != nil {
		t.Fatalf("EditOrder to 3: %v", err)
	}
	if raised.ShippingAmount != 900 {
		t.Errorf("three items re-priced to %d, want the 3-item tier's 900", raised.ShippingAmount)
	}

	capped, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{OrderID: o.ID, UpdateQty: []order.EditLineQty{{OrderLineID: lineID, Quantity: 6}}})
	if err != nil {
		t.Fatalf("EditOrder to 6: %v", err)
	}
	if capped.ShippingAmount != 900 {
		t.Errorf("past the ceiling shipping went to %d, want it held at 900", capped.ShippingAmount)
	}
	if got := h.eventTypes(t, o.ID)["shipping_requote_failed"]; got != 1 {
		t.Errorf("%d shipping_requote_failed events, want 1 on the timeline", got)
	}
	h.Settle(t)
}
