package shipping

import (
	"reflect"
	"testing"
)

// What this pins: a bundle line ships as its components - their SKUs carry the weight, size and product type - and a
// partial shipment of the line takes the same share of each component
func TestLineItemsShipABundleAsItsComponents(t *testing.T) {
	line := RateItem{SKU: "box", Quantity: 2, UnitPriceCents: 1650}
	if got := LineItems(line, 2, nil); !reflect.DeepEqual(got, []RateItem{line}) {
		t.Fatalf("plain line = %+v, want itself", got)
	}
	parts := []Part{{SKU: "mug", Quantity: 2, UnitPriceCents: 750}, {SKU: "tea", Quantity: 6, UnitPriceCents: 300}}
	if got := LineItems(line, 2, parts); !reflect.DeepEqual(got, []RateItem{{SKU: "mug", Quantity: 2, UnitPriceCents: 750}, {SKU: "tea", Quantity: 6, UnitPriceCents: 300}}) {
		t.Fatalf("bundle line = %+v, want its components", got)
	}
	half := RateItem{SKU: "box", Quantity: 1, UnitPriceCents: 1650}
	if got := LineItems(half, 2, parts); got[0].Quantity != 1 || got[1].Quantity != 3 {
		t.Fatalf("one of two boxes = %+v, want 1 mug and 3 teas", got)
	}
}
