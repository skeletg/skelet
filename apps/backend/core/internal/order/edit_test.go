package order

import (
	"testing"

	"github.com/google/uuid"
)

func TestEditableStatuses(t *testing.T) {
	// Drafts and strictly pre-fulfillment statuses are editable
	editable := []string{OrderStatusDraft, OrderStatusPending, OrderStatusConfirmed, OrderStatusProcessing}
	for _, s := range editable {
		if !editableStatuses[s] {
			t.Errorf("status %q should be editable", s)
		}
	}
	for _, s := range []string{OrderStatusPartiallyFulfilled, OrderStatusFulfilled, OrderStatusCancelled} {
		if editableStatuses[s] {
			t.Errorf("status %q must not be editable", s)
		}
	}
}

func TestSyntheticCart(t *testing.T) {
	lineA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	lineB := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	compProduct := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	chID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	o := Order{ID: uuid.New(), ChannelID: chID}
	lines := []OrderLine{
		{ID: lineA, ProductSku: "SIMPLE-1", ProductName: "Simple", UnitPrice: 1000, Quantity: 2},
		{ID: lineB, ProductSku: "BUNDLE-1", ProductName: "Bundle", UnitPrice: 5000, Quantity: 1, Components: []OrderLineComponent{
			{BundleOptionID: uuid.New(), ProductID: compProduct, ProductSku: "COMP-1", ProductName: "Comp", UnitPrice: 2500, Quantity: 2},
		}},
		// A kit's items are for show and return - its stock parts are what it takes
		{ID: uuid.New(), ProductSku: "KIT-1", ProductName: "Kit", UnitPrice: 900, Quantity: 1,
			Components: []OrderLineComponent{{ProductID: compProduct, ProductSku: "COMP-1", Quantity: 1}},
			StockParts: []OrderLineStockPart{{ProductID: compProduct, Quantity: 1, Source: "kit"}}},
	}

	c := syntheticCart(o, lines)

	if c.ChannelID != chID {
		t.Errorf("channel id = %v, want %v", c.ChannelID, chID)
	}
	if len(c.Lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(c.Lines))
	}
	if kit := c.Lines[2]; len(kit.Components) != 0 || len(kit.StockParts) != 1 {
		t.Errorf("kit line = %d components, %d stock parts - want none and its one part", len(kit.Components), len(kit.StockParts))
	}
	// Line ID must carry through so reservations link to the order line
	if c.Lines[0].ID != lineA {
		t.Errorf("line[0].ID = %v, want %v", c.Lines[0].ID, lineA)
	}
	if c.Lines[0].ProductSku != "SIMPLE-1" || c.Lines[0].Quantity != 2 || c.Lines[0].UnitPrice != 1000 {
		t.Errorf("line[0] sku/qty/price mismatch: %+v", c.Lines[0])
	}
	// Bundle components must be carried so inventory reserves per-component
	if len(c.Lines[1].Components) != 1 {
		t.Fatalf("bundle components = %d, want 1", len(c.Lines[1].Components))
	}
	comp := c.Lines[1].Components[0]
	if comp.ProductID != compProduct || comp.Quantity != 2 || comp.ProductSku != "COMP-1" {
		t.Errorf("component mismatch: %+v", comp)
	}
}
