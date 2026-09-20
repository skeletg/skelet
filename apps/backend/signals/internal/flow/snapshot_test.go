package flow

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	emailpkg "github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
)

// orderContext is the payload core enrolls an order trigger with, built here the way core builds it
// so the round trip below is the one the wire makes
func orderContext(orderID, fulfillmentID uuid.UUID, snap emailpkg.OrderSnapshot) map[string]any {
	c := map[string]any{signalsapi.PayloadOrderID: orderID.String(), signalsapi.PayloadOrderNumber: snap.Number, signalsapi.PayloadOrder: snap}
	if fulfillmentID != uuid.Nil {
		c[signalsapi.PayloadFulfillmentID] = fulfillmentID.String()
	}
	return c
}

func runWith(t *testing.T, c map[string]any) FlowRun {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal context: %v", err)
	}
	return FlowRun{Context: raw}
}

// The engine advances a run on its own tick, and after the split it cannot read the order back - so
// everything the four order emails render has to survive the round trip through the run's context
func TestTheSnapshotSurvivesTheRunContext(t *testing.T) {
	want := emailpkg.OrderSnapshot{
		Number: "ORD-1042", FirstName: "Ada", Currency: "EUR",
		Lines:    []emailpkg.OrderEmailLine{{Name: "Winter Tire 205/55", Quantity: 2, LineTotal: 24000}},
		Subtotal: 24000, Discount: 1000, Shipping: 500, Tax: 4370, Total: 27870,
		ShipTo:  []string{"Ada Lovelace", "1 Analytical Way"},
		Carrier: "DHL", TrackingNumber: "JD014600", RefundedAmount: 500,
	}

	got, ok := orderSnapshotFromRun(runWith(t, orderContext(uuid.New(), uuid.New(), want)))
	if !ok {
		t.Fatal("the run carried no snapshot back")
	}
	if got.Number != want.Number || got.FirstName != want.FirstName || got.Currency != want.Currency {
		t.Errorf("identity = %q/%q/%q, want %q/%q/%q", got.Number, got.FirstName, got.Currency, want.Number, want.FirstName, want.Currency)
	}
	if len(got.Lines) != 1 || got.Lines[0].Name != want.Lines[0].Name || got.Lines[0].LineTotal != want.Lines[0].LineTotal {
		t.Errorf("lines = %+v, want the order's own", got.Lines)
	}
	for _, c := range []struct {
		field     string
		got, want int64
	}{
		{"subtotal", got.Subtotal, want.Subtotal},
		{"discount", got.Discount, want.Discount},
		{"shipping", got.Shipping, want.Shipping},
		{"tax", got.Tax, want.Tax},
		{"total", got.Total, want.Total},
		{"refunded", got.RefundedAmount, want.RefundedAmount},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.field, c.got, c.want)
		}
	}
	if got.Carrier != want.Carrier || got.TrackingNumber != want.TrackingNumber {
		t.Errorf("tracking = %q/%q, want %q/%q", got.Carrier, got.TrackingNumber, want.Carrier, want.TrackingNumber)
	}
	if len(got.ShipTo) != 2 {
		t.Errorf("ship-to = %v, want both address lines", got.ShipTo)
	}
}

// Rendering an order email from nothing prints a receipt with no order on it, which is worse for the
// customer than no receipt - so a run without a snapshot must take a different branch
func TestARunWithoutASnapshotIsNotTreatedAsAnOrderEmail(t *testing.T) {
	for _, c := range []struct {
		name string
		run  FlowRun
	}{
		{"no context at all", FlowRun{}},
		{"context but no order", runWith(t, map[string]any{"cart_id": uuid.NewString()})},
		{"order id but no snapshot", runWith(t, map[string]any{"order_id": uuid.NewString(), "order_number": "ORD-9"})},
	} {
		t.Run(c.name, func(t *testing.T) {
			if hasOrderSnapshot(c.run) {
				t.Error("a run with nothing to render from was taken for an order email")
			}
		})
	}
}

// The ids stay alongside the snapshot - the send-once record and the admin's run detail key on them
func TestTheContextStillCarriesTheIdsItKeysOn(t *testing.T) {
	orderID, fulfillmentID := uuid.New(), uuid.New()
	run := runWith(t, orderContext(orderID, fulfillmentID, emailpkg.OrderSnapshot{Number: "ORD-7"}))

	if got := orderIDFromRun(run); got != orderID {
		t.Errorf("order id = %s, want %s", got, orderID)
	}
}
