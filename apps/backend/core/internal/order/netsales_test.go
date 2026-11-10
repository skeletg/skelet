package order

import (
	"testing"

	"github.com/google/uuid"
)

// The worked example in docs/design/boards-cases.md: 2 × €20 and 1 × €10 with a 10% coupon
func TestLineNetTakesEachLinesShareOfTheDiscount(t *testing.T) {
	shirt, mug := uuid.New(), uuid.New()
	exclusive := Order{
		Lines:          []OrderLine{{ID: shirt, UnitPrice: 2000, Quantity: 2}, {ID: mug, UnitPrice: 1000, Quantity: 1}},
		SubtotalAmount: 5000, DiscountAmount: 500, TaxAmount: 900, ShippingAmount: 490,
	}
	exclusive.TotalAmount = 5000 - 500 + 900 + 490 // tax added on top
	net := LineNet(exclusive)
	if net[shirt] != 3600 || net[mug] != 900 {
		t.Fatalf("net = shirt %d mug %d, want 3600 and 900 (tax added on top isn't revenue, and isn't in the price)", net[shirt], net[mug])
	}
	if got := LineNetFor(exclusive, shirt, 1); got != 1800 {
		t.Fatalf("one returned shirt takes back %d, want 1800", got)
	}

	// The same order priced with tax included: the €45 after discount holds €7.50 of 20% VAT, which comes out of each line
	inclusive := exclusive
	inclusive.TaxAmount = 750
	inclusive.TotalAmount = 5000 - 500 + 490
	net = LineNet(inclusive)
	if net[shirt]+net[mug] != 4500-750 || net[shirt] != 3000 {
		t.Fatalf("tax-inclusive net = shirt %d mug %d, want 3000 and 750 - the tax share taken out", net[shirt], net[mug])
	}
}
