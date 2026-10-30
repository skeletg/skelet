package order

import (
	"testing"

	"github.com/google/uuid"
)

// What this pins: a component's refund is its price share of what the bundle actually cost, so a discounted bundle's
// components refund to the discounted total rather than their list prices
func TestComponentRefundUnitIsItsShareOfTheBundle(t *testing.T) {
	l := OrderLine{ID: uuid.New(), Quantity: 2, Components: []OrderLineComponent{
		{UnitPrice: 600, Quantity: 2}, {UnitPrice: 300, Quantity: 2},
	}}
	// The bundle sold at 10% off: 810 per bundle against 900 of parts
	if got := componentRefundUnit(l, l.Components[0], 810); got != 540 {
		t.Errorf("mug refund = %d, want 540 (two thirds of 810)", got)
	}
	if got := componentRefundUnit(l, l.Components[1], 810); got != 270 {
		t.Errorf("tea refund = %d, want 270", got)
	}
}
