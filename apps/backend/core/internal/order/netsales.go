package order

import (
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// LineNet is each line's net sales, the figure product reports add up: the line's subtotal less its share of the
// order's discount, and less its share of the tax where the shop's prices include it. Shipping is never a product's.
// The shares come from the same allocator placement tax and refunds use, so a product's revenue can't disagree with
// what a customer was charged or refunded for it
func LineNet(o Order) map[uuid.UUID]int64 {
	lineSubs := make([]int64, len(o.Lines))
	var subtotal int64
	for i, l := range o.Lines {
		lineSubs[i] = money.LineTotal(l.UnitPrice, l.Quantity)
		subtotal += lineSubs[i]
	}
	discounts := allocateLineDiscount(lineSubs, subtotal, o.DiscountAmount)
	var taxes []int64
	if !taxAddedOnTop(o) && o.TaxAmount > 0 { // prices include tax, so each line's share comes out
		taxes = allocateLineDiscount(lineSubs, subtotal, max(o.TaxAmount-o.ShippingTaxAmount, 0))
	}
	out := make(map[uuid.UUID]int64, len(o.Lines))
	for i, l := range o.Lines {
		net := lineSubs[i] - discounts[i]
		if taxes != nil {
			net -= taxes[i]
		}
		out[l.ID] = net
	}
	return out
}

// LineNetFor is one line's net sales for a quantity of it - what a refund of part of a line takes back, at the same
// per-unit basis rounding the refund itself used
func LineNetFor(o Order, lineID uuid.UUID, qty float64) int64 {
	for _, l := range o.Lines {
		if l.ID == lineID {
			if qty >= l.Quantity {
				return LineNet(o)[lineID]
			}
			return int64(float64(money.PerUnit(LineNet(o)[lineID], l.Quantity)) * qty)
		}
	}
	return 0
}

// EditedLine is a line's net sales just before an edit, so a subscriber can record what the edit changed
type EditedLine struct {
	LineID     uuid.UUID
	ProductSku string
	Quantity   float64
	NetCents   int64
}

func editedLines(o Order) []EditedLine {
	net := LineNet(o)
	out := make([]EditedLine, 0, len(o.Lines))
	for _, l := range o.Lines {
		out = append(out, EditedLine{LineID: l.ID, ProductSku: l.ProductSku, Quantity: l.Quantity, NetCents: net[l.ID]})
	}
	return out
}
