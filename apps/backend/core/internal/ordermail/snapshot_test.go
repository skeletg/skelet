package ordermail

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

func snapshotOrder() order.Order {
	lineA, lineB := uuid.New(), uuid.New()
	return order.Order{
		ID: uuid.New(), Number: "ORD-1042", CurrencyCode: "EUR",
		SubtotalAmount: 24000, DiscountAmount: 1000, ShippingAmount: 500, TaxAmount: 4370, TotalAmount: 27870,
		ShippingAddress: order.OrderAddress{FirstName: "Ada", LastName: "Lovelace", Street1: "1 Analytical Way", PostalCode: "EC1A", City: "London", CountryCode: "GB"},
		Lines: []order.OrderLine{
			{ID: lineA, ProductName: "Winter Tire 205/55", ProductSku: "TIRE-205", Quantity: 2, UnitPrice: 10000, TotalPrice: 20000},
			{ID: lineB, ProductName: "Valve Stem", ProductSku: "VALVE-1", Quantity: 4, UnitPrice: 1000, TotalPrice: 4000},
		},
	}
}

// The engine renders from this and nothing else, so anything the four order emails print has to be
// on it - a missing total is an email that says the customer paid nothing
func TestTheSnapshotCarriesEveryFieldAnOrderEmailPrints(t *testing.T) {
	o := snapshotOrder()
	snap := Snapshot(o)

	if snap.Number != "ORD-1042" || snap.Currency != "EUR" {
		t.Errorf("identity = %q/%q, want ORD-1042/EUR", snap.Number, snap.Currency)
	}
	if snap.FirstName != "Ada" {
		t.Errorf("first name = %q, want the ship-to name", snap.FirstName)
	}
	for _, c := range []struct {
		field     string
		got, want int64
	}{
		{"subtotal", snap.Subtotal, 24000},
		{"discount", snap.Discount, 1000},
		{"shipping", snap.Shipping, 500},
		{"tax", snap.Tax, 4370},
		{"total", snap.Total, 27870},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.field, c.got, c.want)
		}
	}
	if len(snap.Lines) != 2 || snap.Lines[0].SKU != "TIRE-205" || snap.Lines[0].LineTotal != 20000 {
		t.Errorf("lines = %+v, want both order lines with their totals", snap.Lines)
	}
	if len(snap.ShipTo) == 0 {
		t.Error("no ship-to lines, so the confirmation prints no address")
	}
}

// A shipment email shows what went in THIS box and how to track it - the order's other lines are
// still on their way, and printing them tells the customer their parcel holds more than it does
func TestAShipmentSnapshotCarriesOnlyWhatShipped(t *testing.T) {
	o := snapshotOrder()
	ful := order.Fulfillment{
		ID: uuid.New(), Carrier: "DHL", TrackingNumber: "JD014600003",
		Lines: []order.FulfillmentLine{{OrderLineID: o.Lines[0].ID, Quantity: 2}},
	}

	snap := ShipmentSnapshot(o, ful)

	if snap.Carrier != "DHL" || snap.TrackingNumber != "JD014600003" {
		t.Errorf("tracking = %q/%q, want DHL/JD014600003 - without it the email has nothing to link to", snap.Carrier, snap.TrackingNumber)
	}
	if len(snap.Lines) != 1 {
		t.Fatalf("lines = %d, want only the line this fulfillment carried", len(snap.Lines))
	}
	if snap.Lines[0].SKU != "TIRE-205" {
		t.Errorf("shipped line = %q, want the fulfilled one", snap.Lines[0].SKU)
	}
	if snap.Number != "ORD-1042" {
		t.Errorf("order number = %q, want it kept alongside the narrowed lines", snap.Number)
	}
}
