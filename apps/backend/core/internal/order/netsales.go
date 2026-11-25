package order

import (
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// LineNet is each line's net sales, the figure product reports add up: the line's subtotal less its share of the
// order's discount, and less its tax where the shop's prices include it. Shipping is never a product's. Both are
// the line's own, stored when the order was priced - the same figures a refund of the line gives back
func LineNet(o Order) map[uuid.UUID]int64 {
	out := make(map[uuid.UUID]int64, len(o.Lines))
	for _, l := range o.Lines {
		net := money.LineTotal(l.UnitPrice, l.Quantity) - l.DiscountCents
		if o.TaxInclusive { // prices include tax, so the line's comes out
			net -= l.TaxCents
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
