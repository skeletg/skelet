package order

import (
	"testing"

	"github.com/google/uuid"
)

// The gateway ceiling is right for money a gateway holds and wrong for everything else - clamping gift-card refunds to it made gift-card, store-credit and COD orders un-refundable
func TestRefundCeiling(t *testing.T) {
	const orderTotal = 10_000

	cases := []struct {
		name   string
		target string
		totals PaymentTotals
		want   int64
	}{
		{"gateway order, nothing refunded yet", RefundTargetOriginal, PaymentTotals{Captured: 10_000}, 10_000},
		{"gateway order, partly refunded", RefundTargetOriginal, PaymentTotals{Captured: 10_000, Refunded: 4_000}, 6_000},
		{"gateway order, fully refunded", RefundTargetOriginal, PaymentTotals{Captured: 10_000, Refunded: 10_000}, 0},

		// Covered entirely by gift card or store credit, so the ledger holds no capture to measure against
		{"gift-card target with an empty ledger", RefundTargetGiftCard, PaymentTotals{}, 10_000},
		{"gift-card target, partly refunded", RefundTargetGiftCard, PaymentTotals{Refunded: 2_500}, 7_500},
		{"gift-card target, fully refunded", RefundTargetGiftCard, PaymentTotals{Refunded: 10_000}, 0},

		// Mixed tender - a gift-card refund reaches the whole total, a gateway one only its share
		{"mixed order via gateway", RefundTargetOriginal, PaymentTotals{Captured: 6_000}, 6_000},
		{"mixed order via gift card", RefundTargetGiftCard, PaymentTotals{Captured: 6_000}, 10_000},

		{"offline capture behaves like a card", RefundTargetOriginal, PaymentTotals{Captured: 10_000}, 10_000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := refundCeiling(c.target, c.totals, orderTotal); got != c.want {
				t.Errorf("refundCeiling(%q, %+v, %d) = %d, want %d", c.target, c.totals, orderTotal, got, c.want)
			}
		})
	}
}

// The exact shape of the bug - gift-card order, refund to a gift card, empty ledger
func TestRefundCeiling_GiftCardOrderCanBeRefundedToAGiftCard(t *testing.T) {
	if got := refundCeiling(RefundTargetGiftCard, PaymentTotals{}, 5_000); got <= 0 {
		t.Fatalf("refundCeiling = %d, want the order total", got)
	}
	if got := refundCeiling(RefundTargetOriginal, PaymentTotals{}, 5_000); got != 0 {
		t.Errorf("gateway ceiling = %d, want 0 - no captured gateway money to return", got)
	}
}

// Returns and direct credit memos refund at this basis, so they have to agree line for line
func TestRefundUnitByLine(t *testing.T) {
	lineA, lineB := uuid.New(), uuid.New()
	mk := func(discount, tax, shipping, total int64) Order {
		return Order{
			Lines: []OrderLine{
				{ID: lineA, UnitPrice: 1_000, Quantity: 2}, // 2000
				{ID: lineB, UnitPrice: 500, Quantity: 4},   // 2000
			},
			SubtotalAmount: 4_000,
			DiscountAmount: discount,
			TaxAmount:      tax,
			ShippingAmount: shipping,
			TotalAmount:    total,
		}
	}

	t.Run("no discount or tax refunds the unit price", func(t *testing.T) {
		got := refundUnitByLine(mk(0, 0, 0, 4_000))
		if got[lineA] != 1_000 || got[lineB] != 500 {
			t.Errorf("got %d / %d, want 1000 / 500", got[lineA], got[lineB])
		}
	})

	t.Run("order discount is spread proportionally", func(t *testing.T) {
		got := refundUnitByLine(mk(400, 0, 0, 3_600))
		if got[lineA] != 900 { // (2000 - 200) / 2
			t.Errorf("line A per-unit = %d, want 900", got[lineA])
		}
		if got[lineB] != 450 { // (2000 - 200) / 4
			t.Errorf("line B per-unit = %d, want 450", got[lineB])
		}
	})

	// total == subtotal - discount + tax + shipping marks the store tax-exclusive
	t.Run("tax-exclusive order adds tax to the basis", func(t *testing.T) {
		if got := refundUnitByLine(mk(0, 400, 0, 4_400))[lineA]; got != 1_100 {
			t.Errorf("line A per-unit = %d, want 1100 - tax-exclusive refunds return the tax", got)
		}
	})

	t.Run("tax-inclusive order does not add tax twice", func(t *testing.T) {
		if got := refundUnitByLine(mk(0, 400, 0, 4_000))[lineA]; got != 1_000 {
			t.Errorf("line A per-unit = %d, want 1000 - inclusive prices already contain the tax", got)
		}
	})
}

func TestOfflineProviderSlug(t *testing.T) {
	// Stored on every offline ledger row and read back to decide whether a gateway call is needed - changing it is a data migration
	if OfflineProviderSlug != "offline" {
		t.Errorf("OfflineProviderSlug = %q, want %q", OfflineProviderSlug, "offline")
	}
}
