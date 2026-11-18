package order

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// lineShares is what each line of an order came to beyond its price, in the lines' own order: its part of the order's
// discount and its tax. They are written onto the order line, so a refund, a report and a later edit read what the
// line was charged rather than work it out again from the order's totals
type lineShares struct {
	discount, tax []int64
}

// splitDiscount shares an order's discount over its lines. What already sits on a line - the promotion engine's own
// allocation at checkout, the stored share on an edit - stays there, and the rest is spread by what each line has
// left, so no line goes below zero. A discount smaller than what the lines already carry takes from each in proportion
func splitDiscount(lineSubs, onLine []int64, total int64) []int64 {
	out := make([]int64, len(lineSubs))
	var placed int64
	for i, sub := range lineSubs {
		if i < len(onLine) {
			out[i] = min(max(onLine[i], 0), max(sub, 0))
		}
		placed += out[i]
	}
	if placed >= total {
		return money.Spread(out, total)
	}
	room := make([]int64, len(lineSubs))
	for i, sub := range lineSubs {
		room[i] = sub - out[i]
	}
	for i, more := range money.Spread(room, total-placed) {
		out[i] += more
	}
	return out
}

func lineSubtotals(lines []cart.CartLine) []int64 {
	out := make([]int64, len(lines))
	for i, l := range lines {
		out[i] = money.LineTotal(l.UnitPrice, l.Quantity)
	}
	return out
}

func (cc *checkoutContext) discountShares(total int64) []int64 {
	var onLine []int64
	if len(cc.discountOnLine) > 0 {
		onLine = make([]int64, len(cc.cart.Lines))
		for i, l := range cc.cart.Lines {
			onLine[i] = cc.discountOnLine[l.ID]
		}
	}
	return splitDiscount(lineSubtotals(cc.cart.Lines), onLine, total)
}

// shares settles each line's part of a priced order. Core's tax is worked out line by line and kept as it came. A
// figure from elsewhere - a tax plugin, a remote provider, an order nothing re-taxed - is one number, shared by what
// each line came to after its discount. goodsTax is the order's tax less shipping's and the fees'
func (cc *checkoutContext) shares(discount, goodsTax int64) lineShares {
	out := lineShares{discount: cc.discountShares(discount), tax: cc.lineTax}
	if len(out.tax) != len(cc.cart.Lines) {
		net := lineSubtotals(cc.cart.Lines)
		for i := range net {
			net[i] -= out.discount[i]
		}
		out.tax = money.Spread(net, goodsTax)
	}
	return out
}

// Writes the lines' shares after an edit re-priced the order - lines in cc's cart order, each already a row
func writeLineShares(ctx context.Context, r Repository, orderID uuid.UUID, lines []cart.CartLine, s lineShares) error {
	ids := make([]uuid.UUID, len(lines))
	for i, l := range lines {
		ids[i] = l.ID
	}
	return r.SetOrderLineMoney(ctx, orderID, ids, s.discount, s.tax)
}

// The stored shares of an order's lines by line id, each scaled to the quantity the line is being edited to
func storedDiscounts(lines []OrderLine, newQty map[uuid.UUID]float64) map[uuid.UUID]int64 {
	out := make(map[uuid.UUID]int64, len(lines))
	for _, l := range lines {
		share := l.DiscountCents
		if q, ok := newQty[l.ID]; ok && l.Quantity > 0 {
			share = money.ScaleFraction(share, q/l.Quantity)
		}
		out[l.ID] = share
	}
	return out
}
