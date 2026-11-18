package order

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
)

// priced gives an order built in a test the line shares pricing would have written: the discount by line subtotal, and
// the goods' tax by what each line came to after it
func priced(o Order) Order {
	lines := make([]cart.CartLine, len(o.Lines))
	for i, l := range o.Lines {
		lines[i] = cart.CartLine{ID: l.ID, UnitPrice: l.UnitPrice, Quantity: l.Quantity}
	}
	cc := &checkoutContext{cart: cart.Cart{Lines: lines}}
	shares := cc.shares(o.DiscountAmount, o.TaxAmount-o.ShippingTaxAmount-o.FeeTaxAmount())
	o.Lines = slices.Clone(o.Lines)
	for i := range o.Lines {
		o.Lines[i].DiscountCents, o.Lines[i].TaxCents = shares.discount[i], shares.tax[i]
	}
	return o
}

func TestSplitDiscount(t *testing.T) {
	cases := []struct {
		name   string
		subs   []int64
		onLine []int64
		total  int64
		want   []int64
	}{
		{"nothing known spreads by line subtotal", []int64{1_000, 3_000}, nil, 400, []int64{100, 300}},
		{"the cent left over goes to the last line", []int64{333, 333, 334}, nil, 100, []int64{33, 33, 34}},
		{"a free line takes nothing", []int64{1_000, 0}, nil, 101, []int64{101, 0}},
		{"what the engine put on a line stays there", []int64{10_000, 5_000}, []int64{2_000, 0}, 2_000, []int64{2_000, 0}},
		{"the rest goes by what each line has left", []int64{10_000, 5_000}, []int64{2_000, 0}, 3_300, []int64{2_800, 500}},
		{"a share is held to what its line is worth", []int64{1_000, 5_000}, []int64{4_000, 0}, 4_000, []int64{1_000, 3_000}},
		{"a smaller discount takes from each in proportion", []int64{10_000, 5_000}, []int64{3_000, 1_000}, 2_000, []int64{1_500, 500}},
		{"no line goes below zero", []int64{300, 200}, nil, 900, []int64{300, 200}},
		{"no discount", []int64{1_000, 500}, []int64{100, 0}, 0, []int64{0, 0}},
		{"a negative share counts as none", []int64{1_000, 1_000}, []int64{-50, 0}, 200, []int64{100, 100}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := splitDiscount(c.subs, c.onLine, c.total); !slices.Equal(got, c.want) {
				t.Fatalf("splitDiscount(%v, %v, %d) = %v, want %v", c.subs, c.onLine, c.total, got, c.want)
			}
		})
	}
}

func TestCheckoutShares(t *testing.T) {
	shirt, book := uuid.New(), uuid.New()
	lines := []cart.CartLine{
		{ID: shirt, UnitPrice: 5_000, Quantity: 2},
		{ID: book, UnitPrice: 5_000, Quantity: 1},
	}

	// Core taxed the lines itself - its per-line figures are kept, whatever a pro-rata share would have been
	cc := &checkoutContext{cart: cart.Cart{Lines: lines}, discountOnLine: map[uuid.UUID]int64{shirt: 2_000}, lineTax: []int64{1_520, 350}}
	got := cc.shares(2_000, 9_999)
	if !slices.Equal(got.discount, []int64{2_000, 0}) || !slices.Equal(got.tax, []int64{1_520, 350}) {
		t.Fatalf("core-taxed shares = %+v", got)
	}

	// A plugin's tax is one figure: shared by what each line came to after its discount (8000 and 5000)
	cc.lineTax = nil
	got = cc.shares(2_000, 1_300)
	if !slices.Equal(got.tax, []int64{800, 500}) {
		t.Fatalf("spread tax = %v, want [800 500]", got.tax)
	}

	// Lines with no id yet - a draft's - share by subtotal alone
	draft := &checkoutContext{cart: cart.Cart{Lines: []cart.CartLine{{UnitPrice: 1_000, Quantity: 1}, {UnitPrice: 3_000, Quantity: 1}}}}
	if got := draft.shares(400, 0); !slices.Equal(got.discount, []int64{100, 300}) || !slices.Equal(got.tax, []int64{0, 0}) {
		t.Fatalf("draft shares = %+v", got)
	}
}

func TestStoredDiscounts(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	lines := []OrderLine{
		{ID: a, Quantity: 4, DiscountCents: 1_000},
		{ID: b, Quantity: 1, DiscountCents: 300},
		{ID: c, Quantity: 2, DiscountCents: 500},
	}
	got := storedDiscounts(lines, map[uuid.UUID]float64{a: 1, c: 6})
	if got[a] != 250 {
		t.Errorf("a line cut from 4 to 1 keeps %d of its 1000, want 250", got[a])
	}
	if got[b] != 300 {
		t.Errorf("an untouched line keeps %d, want 300", got[b])
	}
	if got[c] != 500 {
		t.Errorf("a line that grew keeps %d, want its 500 and no more", got[c])
	}
}
