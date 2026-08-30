package order

import "testing"

func TestNextPaymentStatus(t *testing.T) {
	cases := []struct {
		name        string
		current     string
		event       string
		wantStatus  string
		wantChanged bool
	}{
		// Forward lifecycle
		{"unpaid→authorized", PaymentUnpaid, PaymentAuthorized, PaymentAuthorized, true},
		{"authorized→paid", PaymentAuthorized, PaymentPaid, PaymentPaid, true},
		{"paid→refunded", PaymentPaid, PaymentRefunded, PaymentRefunded, true},
		{"unpaid→paid (immediate sale)", PaymentUnpaid, PaymentPaid, PaymentPaid, true},
		{"unpaid→voided", PaymentUnpaid, PaymentVoided, PaymentVoided, true},
		{"authorized→voided", PaymentAuthorized, PaymentVoided, PaymentVoided, true},
		// Empty current is treated as unpaid
		{"empty→authorized", "", PaymentAuthorized, PaymentAuthorized, true},

		// Idempotency: same event = no change
		{"paid→paid", PaymentPaid, PaymentPaid, PaymentPaid, false},
		{"authorized→authorized", PaymentAuthorized, PaymentAuthorized, PaymentAuthorized, false},
		{"refunded→refunded", PaymentRefunded, PaymentRefunded, PaymentRefunded, false},

		// No downgrade / out-of-order webhook protection
		{"paid→authorized blocked", PaymentPaid, PaymentAuthorized, PaymentPaid, false},
		{"paid→voided blocked (use refund)", PaymentPaid, PaymentVoided, PaymentPaid, false},
		{"refunded→paid blocked (terminal)", PaymentRefunded, PaymentPaid, PaymentRefunded, false},
		{"voided→authorized blocked (terminal)", PaymentVoided, PaymentAuthorized, PaymentVoided, false},
		{"refunded from unpaid blocked", PaymentUnpaid, PaymentRefunded, PaymentUnpaid, false},

		// Unknown / empty events are no-ops
		{"unknown event", PaymentAuthorized, "garbage", PaymentAuthorized, false},
		{"empty event", PaymentPaid, "", PaymentPaid, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed := nextPaymentStatus(c.current, c.event)
			if got != c.wantStatus || changed != c.wantChanged {
				t.Fatalf("nextPaymentStatus(%q, %q) = (%q, %v), want (%q, %v)",
					c.current, c.event, got, changed, c.wantStatus, c.wantChanged)
			}
		})
	}
}

func TestDerivePaymentStatus(t *testing.T) {
	cases := []struct {
		name   string
		totals PaymentTotals
		owed   int64
		total  int64
		want   string
	}{
		{"empty ledger → unpaid", PaymentTotals{}, 1000, 1000, PaymentUnpaid},
		{"authorized only", PaymentTotals{Authorized: 1000}, 1000, 1000, PaymentAuthorized},
		{"fully captured → paid", PaymentTotals{Authorized: 1000, Captured: 1000}, 1000, 1000, PaymentPaid},
		{"partial capture", PaymentTotals{Authorized: 1000, Captured: 400}, 1000, 1000, PaymentPartiallyCaptured},
		{"partial refund", PaymentTotals{Authorized: 1000, Captured: 1000, Refunded: 300}, 1000, 1000, PaymentPartiallyRefunded},
		{"full refund", PaymentTotals{Authorized: 1000, Captured: 1000, Refunded: 1000}, 1000, 1000, PaymentRefunded},
		{"voided before capture", PaymentTotals{Authorized: 1000, Voided: 1000}, 1000, 1000, PaymentVoided},
		{"capture covers gift-card-reduced owed", PaymentTotals{Authorized: 600, Captured: 600}, 600, 1000, PaymentPaid},
		{"refund exceeding capture still refunded", PaymentTotals{Captured: 1000, Refunded: 1200}, 1000, 1000, PaymentRefunded},
		// The ledger only records money that moved through a payment instrument - an order fully covered by gift card or
		// store credit owes nothing and must not recompute back to unpaid
		{"fully tendered, empty ledger → paid", PaymentTotals{}, 0, 1000, PaymentPaid},
		{"zero-total order → paid", PaymentTotals{}, 0, 0, PaymentPaid},
		{"still owed with empty ledger → unpaid", PaymentTotals{}, 500, 500, PaymentUnpaid},

		// A gift-card-paid order refunded to a new gift card has no capture behind it - without the Captured==0 && Refunded>0
		// branch these read as "paid" and the refund vanished from the order
		{"gift-card order partially refunded", PaymentTotals{Refunded: 400}, 0, 1000, PaymentPartiallyRefunded},
		{"gift-card order fully refunded", PaymentTotals{Refunded: 1000}, 0, 1000, PaymentRefunded},
		{"gift-card refund over the total", PaymentTotals{Refunded: 1200}, 0, 1000, PaymentRefunded},

		// Offline payments write a capture row now, so COD behaves exactly like a card
		{"offline capture → paid", PaymentTotals{Captured: 1000}, 1000, 1000, PaymentPaid},
		{"offline capture partially refunded", PaymentTotals{Captured: 1000, Refunded: 250}, 1000, 1000, PaymentPartiallyRefunded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := derivePaymentStatus(c.totals, c.owed, c.total); got != c.want {
				t.Fatalf("derivePaymentStatus(%+v, owed=%d, total=%d) = %q, want %q", c.totals, c.owed, c.total, got, c.want)
			}
		})
	}
}
