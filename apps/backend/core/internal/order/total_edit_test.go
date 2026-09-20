package order

import (
	"testing"

	"github.com/google/uuid"
)

// Inclusive prices ALREADY contain the tax - adding it again overcharges on every edit and placement
func TestComputeOrderTotal(t *testing.T) {
	cases := []struct {
		name                              string
		subtotal, discount, tax, shipping int64
		taxInclusive                      bool
		want                              int64
	}{
		{"exclusive adds tax on top", 10_000, 0, 800, 500, false, 11_300},
		{"inclusive leaves tax inside the lines", 10_000, 0, 800, 500, true, 10_500},
		{"discount comes off the subtotal", 10_000, 1_500, 0, 500, false, 9_000},
		{"discount and tax together", 10_000, 1_000, 720, 500, false, 10_220},
		{"zero everything", 0, 0, 0, 0, false, 0},

		// A discount above the goods must never charge a negative amount
		{"discount above subtotal floors at zero", 1_000, 5_000, 0, 0, false, 0},
		{"discount above subtotal with shipping floors at zero", 1_000, 5_000, 0, 300, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := computeOrderTotal(c.subtotal, c.discount, c.tax, c.shipping, c.taxInclusive, nil)
			if got != c.want {
				t.Errorf("computeOrderTotal(sub=%d, disc=%d, tax=%d, ship=%d, inclusive=%v) = %d, want %d",
					c.subtotal, c.discount, c.tax, c.shipping, c.taxInclusive, got, c.want)
			}
		})
	}
}

// A draft's operator-set total replaces the arithmetic entirely, but still can't go negative
func TestComputeOrderTotal_Override(t *testing.T) {
	override := int64(4_200)
	if got := computeOrderTotal(10_000, 0, 800, 500, false, &override); got != 4_200 {
		t.Errorf("total = %d, want the override 4200", got)
	}
	negative := int64(-100)
	if got := computeOrderTotal(10_000, 0, 0, 0, false, &negative); got != 0 {
		t.Errorf("total = %d, want 0 - an override must not make the order owe the customer", got)
	}
}

// An edit holds the discount constant but clamps it - removing lines until the goods cost less would persist a discount the order never earned
func TestEditDiscountClampedToSubtotal(t *testing.T) {
	clamp := func(discount, subtotal int64) int64 {
		if discount > subtotal {
			return subtotal
		}
		return discount
	}
	cases := []struct{ discount, subtotal, want int64 }{
		{1_500, 10_000, 1_500}, // untouched when it fits
		{1_500, 1_000, 1_000},  // clamped to the shrunken subtotal
		{1_500, 0, 0},          // every line removed
	}
	for _, c := range cases {
		if got := clamp(c.discount, c.subtotal); got != c.want {
			t.Errorf("clamp(discount=%d, subtotal=%d) = %d, want %d", c.discount, c.subtotal, got, c.want)
		}
		if got := clamp(c.discount, c.subtotal); got > c.subtotal {
			t.Errorf("discount %d exceeds subtotal %d after clamping", got, c.subtotal)
		}
	}
}

// The admin surface floors AmountDue at zero, so an edit dropping below the tenders hides the overpayment - EditOrder writes payment_overtendered for it
func TestAmountDue_GoesNegativeWhenAnEditDropsBelowTheTenders(t *testing.T) {
	o := Order{TotalAmount: 10_000, GiftCardAmount: 4_000, StoreCreditAmount: 1_000}
	if got := o.AmountDue(); got != 5_000 {
		t.Fatalf("AmountDue = %d, want 5000", got)
	}

	edited := Order{TotalAmount: 3_000, GiftCardAmount: 4_000, StoreCreditAmount: 1_000}
	if got := edited.AmountDue(); got >= 0 {
		t.Errorf("AmountDue = %d, want negative - the customer overpaid and it must be visible", got)
	}
	if tendered := edited.GiftCardAmount + edited.StoreCreditAmount; tendered <= edited.TotalAmount {
		t.Error("this fixture is meant to be over-tendered")
	}
}

// A zero-total order owes nothing - derivePaymentStatus has to read that as paid, not unpaid
func TestZeroTotalOrderOwesNothing(t *testing.T) {
	o := Order{TotalAmount: 0}
	if got := o.AmountDue(); got != 0 {
		t.Errorf("AmountDue = %d, want 0", got)
	}
	if got := derivePaymentStatus(PaymentTotals{}, o.AmountDue(), o.TotalAmount); got != PaymentPaid {
		t.Errorf("status = %q, want %q", got, PaymentPaid)
	}
}

// Strictly pre-fulfillment - a shipped line can't be un-ordered, so returns and memos are the path
func TestEditableStatusesExcludeAnythingShipped(t *testing.T) {
	for _, s := range []string{OrderStatusPartiallyFulfilled, OrderStatusFulfilled} {
		if editableStatuses[s] {
			t.Errorf("status %q must not be editable - use a return or a credit memo", s)
		}
	}
	if editableStatuses[OrderStatusCancelled] {
		t.Error("a cancelled order must not be editable")
	}
	if editableStatuses["on_hold"] {
		t.Error("an on-hold order must not be editable - release it first")
	}
}

// The edit hands this to the tax and reservation code - the line id has to survive or reservations detach
func TestSyntheticCartPreservesLineIdentity(t *testing.T) {
	lineA := uuid.New()
	o := Order{ID: uuid.New(), ChannelID: uuid.New()}
	c := syntheticCart(o, []OrderLine{{ID: lineA, ProductSku: "SKU", UnitPrice: 100, Quantity: 3}})
	if len(c.Lines) != 1 || c.Lines[0].ID != lineA {
		t.Fatalf("line identity lost: %+v", c.Lines)
	}
	if c.Lines[0].Quantity != 3 || c.Lines[0].UnitPrice != 100 {
		t.Errorf("line money/qty mismatch: %+v", c.Lines[0])
	}
}
