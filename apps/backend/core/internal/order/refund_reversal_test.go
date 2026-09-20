package order

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// The two lookups the fraction needs - exact line credits, plus the memos so line-less ones can be attributed
type creditedRepo struct {
	Repository
	credited map[uuid.UUID]float64
	memos    []CreditMemo
}

func (c *creditedRepo) CreditedByOrderLine(context.Context, uuid.UUID) (map[uuid.UUID]float64, error) {
	return c.credited, nil
}

func (c *creditedRepo) ListCreditMemos(context.Context, uuid.UUID) ([]CreditMemo, error) {
	return c.memos, nil
}

// Rewards are earned on (subtotal - discount), so the fraction tracks GOODS returned, never shipping, tax or a goodwill adjustment
func TestRefundedMerchandiseFraction(t *testing.T) {
	lineA, lineB := uuid.New(), uuid.New()
	// 2 x 1000 + 4 x 500 = 4000 merchandise, 400 tax on top, 200 shipping
	o := Order{
		ID: uuid.New(),
		Lines: []OrderLine{
			{ID: lineA, UnitPrice: 1_000, Quantity: 2},
			{ID: lineB, UnitPrice: 500, Quantity: 4},
		},
		SubtotalAmount: 4_000,
		TaxAmount:      400,
		ShippingAmount: 200,
		TotalAmount:    4_600, // tax-exclusive: subtotal + tax + shipping
	}

	cases := []struct {
		name     string
		credited map[uuid.UUID]float64
		want     float64
	}{
		{"nothing credited", nil, 0},
		{"half of line A", map[uuid.UUID]float64{lineA: 1}, 0.25},    // 1000 / 4000
		{"all of line A", map[uuid.UUID]float64{lineA: 2}, 0.5},      // 2000 / 4000
		{"all of line B", map[uuid.UUID]float64{lineB: 4}, 0.5},      // 2000 / 4000
		{"everything", map[uuid.UUID]float64{lineA: 2, lineB: 4}, 1}, // 4000 / 4000
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := refundedMerchandiseFraction(t.Context(), &creditedRepo{credited: c.credited}, o)
			if err != nil {
				t.Fatalf("refundedMerchandiseFraction: %v", err)
			}
			if got != c.want {
				t.Errorf("fraction = %v, want %v", got, c.want)
			}
		})
	}

	// 600 cents of real money, but no lines credited - so no reward may be revoked
	t.Run("shipping and tax alone revoke nothing", func(t *testing.T) {
		got, err := refundedMerchandiseFraction(t.Context(), &creditedRepo{credited: map[uuid.UUID]float64{}}, o)
		if err != nil {
			t.Fatalf("refundedMerchandiseFraction: %v", err)
		}
		if got != 0 {
			t.Errorf("fraction = %v, want 0 - no goods came back", got)
		}
	})
}

// The plain Refund button names no lines - attributing nothing would let it refund the whole order and revoke zero points
func TestRefundedMerchandiseFraction_LinelessMemosAreAttributedByMoney(t *testing.T) {
	// 4000 merchandise, 400 tax, 200 shipping, 4600 total
	o := Order{
		ID:             uuid.New(),
		Lines:          []OrderLine{{ID: uuid.New(), UnitPrice: 1_000, Quantity: 4}},
		SubtotalAmount: 4_000,
		TaxAmount:      400,
		ShippingAmount: 200,
		TotalAmount:    4_600,
	}

	t.Run("a full flat refund revokes everything", func(t *testing.T) {
		repo := &creditedRepo{memos: []CreditMemo{{TotalCents: 4_600}}}
		got, err := refundedMerchandiseFraction(t.Context(), repo, o)
		if err != nil {
			t.Fatalf("refundedMerchandiseFraction: %v", err)
		}
		if got != 1 {
			t.Errorf("fraction = %v, want 1 - the whole order was handed back", got)
		}
	})

	t.Run("half a flat refund revokes about half", func(t *testing.T) {
		repo := &creditedRepo{memos: []CreditMemo{{TotalCents: 2_300}}}
		got, err := refundedMerchandiseFraction(t.Context(), repo, o)
		if err != nil {
			t.Fatalf("refundedMerchandiseFraction: %v", err)
		}
		if got < 0.45 || got > 0.6 {
			t.Errorf("fraction = %v, want roughly half", got)
		}
	})

	t.Run("a shipping-only memo revokes nothing", func(t *testing.T) {
		repo := &creditedRepo{memos: []CreditMemo{{TotalCents: 200, ShippingRefundCents: 200}}}
		got, err := refundedMerchandiseFraction(t.Context(), repo, o)
		if err != nil {
			t.Fatalf("refundedMerchandiseFraction: %v", err)
		}
		if got != 0 {
			t.Errorf("fraction = %v, want 0 - no goods came back", got)
		}
	})

	// Line memos are already exact through CreditedByOrderLine - attributing again would double-count
	t.Run("line memos are not counted twice", func(t *testing.T) {
		lineID := o.Lines[0].ID
		repo := &creditedRepo{
			credited: map[uuid.UUID]float64{lineID: 2},
			memos:    []CreditMemo{{TotalCents: 2_300, Lines: []CreditMemoLine{{OrderLineID: lineID, Quantity: 2}}}},
		}
		got, err := refundedMerchandiseFraction(t.Context(), repo, o)
		if err != nil {
			t.Fatalf("refundedMerchandiseFraction: %v", err)
		}
		if got != 0.5 {
			t.Errorf("fraction = %v, want exactly 0.5 from the lines alone", got)
		}
	})
}

// A discount reduces what was earned, so it must reduce what is revoked
func TestRefundedMerchandiseFraction_UsesDiscountedMerchandise(t *testing.T) {
	lineA := uuid.New()
	o := Order{
		ID:             uuid.New(),
		Lines:          []OrderLine{{ID: lineA, UnitPrice: 1_000, Quantity: 2}},
		SubtotalAmount: 2_000,
		DiscountAmount: 500,
		TotalAmount:    1_500,
	}
	got, err := refundedMerchandiseFraction(t.Context(), &creditedRepo{credited: map[uuid.UUID]float64{lineA: 2}}, o)
	if err != nil {
		t.Fatalf("refundedMerchandiseFraction: %v", err)
	}
	if got != 1 {
		t.Errorf("fraction = %v, want 1 - the whole discounted merchandise came back", got)
	}
}

// Credited quantity can exceed the ordered merchandise - a fraction above 1 would over-revoke
func TestRefundedMerchandiseFraction_ClampsToOne(t *testing.T) {
	lineA := uuid.New()
	o := Order{
		ID:             uuid.New(),
		Lines:          []OrderLine{{ID: lineA, UnitPrice: 1_000, Quantity: 1}},
		SubtotalAmount: 1_000,
		TotalAmount:    1_000,
	}
	got, err := refundedMerchandiseFraction(t.Context(), &creditedRepo{credited: map[uuid.UUID]float64{lineA: 5}}, o)
	if err != nil {
		t.Fatalf("refundedMerchandiseFraction: %v", err)
	}
	if got != 1 {
		t.Errorf("fraction = %v, want it clamped to 1", got)
	}
}

// A fully-discounted or shipping-only order must not divide by zero
func TestRefundedMerchandiseFraction_ZeroMerchandise(t *testing.T) {
	o := Order{ID: uuid.New(), SubtotalAmount: 1_000, DiscountAmount: 1_000, TotalAmount: 0}
	got, err := refundedMerchandiseFraction(t.Context(), &creditedRepo{}, o)
	if err != nil {
		t.Fatalf("refundedMerchandiseFraction: %v", err)
	}
	if got != 0 {
		t.Errorf("fraction = %v, want 0", got)
	}
}

// Tax rides the refund basis but never the reward basis
func TestMerchandiseUnitByLine_ExcludesTax(t *testing.T) {
	lineA := uuid.New()
	exclusive := Order{
		Lines:          []OrderLine{{ID: lineA, UnitPrice: 1_000, Quantity: 2}},
		SubtotalAmount: 2_000,
		TaxAmount:      400,
		TotalAmount:    2_400,
	}
	if got := merchandiseUnitByLine(exclusive)[lineA]; got != 1_000 {
		t.Errorf("merchandise per-unit = %d, want 1000 - rewards were never earned on tax", got)
	}
	if got := refundUnitByLine(exclusive)[lineA]; got != 1_200 {
		t.Errorf("refund per-unit = %d, want 1200 - the customer paid the tax and gets it back", got)
	}

	inclusive := Order{
		Lines:          []OrderLine{{ID: lineA, UnitPrice: 1_000, Quantity: 2}},
		SubtotalAmount: 2_000,
		TaxAmount:      400,
		TotalAmount:    2_000,
	}
	if got, want := merchandiseUnitByLine(inclusive)[lineA], refundUnitByLine(inclusive)[lineA]; got != want {
		t.Errorf("inclusive pricing: merchandise %d vs refund %d, want them equal", got, want)
	}
}
