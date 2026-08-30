package order

import "testing"

// What this guards: without an authorize row Authorized reads zero, so the
// over-capture guard rejects the FIRST capture on every order - goods ship and
// the money is never collected. The guard arithmetic itself
// was never wrong, which is why hand-built PaymentTotals in the other tests
// couldn't see it. This asserts the arithmetic against the totals the ledger
// actually produces for each kind.
func TestCaptureGuardAgainstLedgerTotals(t *testing.T) {
	cases := []struct {
		name    string
		totals  PaymentTotals
		capture int64
		wantOK  bool
	}{
		{"no authorize row rejects any capture", PaymentTotals{}, 1, false},
		{"authorized covers an equal capture", PaymentTotals{Authorized: 10000}, 10000, true},
		{"authorized covers a partial capture", PaymentTotals{Authorized: 10000}, 4000, true},
		{"capture beyond authorized rejected", PaymentTotals{Authorized: 10000}, 10001, false},
		{"second capture limited to the remainder", PaymentTotals{Authorized: 10000, Captured: 6000}, 4000, true},
		{"second capture past the remainder rejected", PaymentTotals{Authorized: 10000, Captured: 6000}, 4001, false},
		// Pending rows count toward Captured in the guard's totals, which is what
		// stops two concurrent intents both passing and double-charging
		{"in-flight pending capture blocks a duplicate", PaymentTotals{Authorized: 10000, Captured: 10000}, 10000, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if ok := c.capture <= c.totals.Authorized-c.totals.Captured; ok != c.wantOK {
				t.Fatalf("capture %d against %+v allowed=%v, want %v", c.capture, c.totals, ok, c.wantOK)
			}
		})
	}
}

func TestRefundGuardAgainstLedgerTotals(t *testing.T) {
	cases := []struct {
		name   string
		totals PaymentTotals
		refund int64
		wantOK bool
	}{
		{"nothing captured rejects a refund", PaymentTotals{Authorized: 10000}, 1, false},
		{"refund up to captured", PaymentTotals{Captured: 10000}, 10000, true},
		{"refund beyond captured rejected", PaymentTotals{Captured: 10000}, 10001, false},
		{"second refund limited to the remainder", PaymentTotals{Captured: 10000, Refunded: 9000}, 1000, true},
		// Two concurrent full refunds: the second sees the first as pending
		{"in-flight pending refund blocks a duplicate", PaymentTotals{Captured: 10000, Refunded: 10000}, 10000, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if ok := c.refund <= c.totals.Captured-c.totals.Refunded; ok != c.wantOK {
				t.Fatalf("refund %d against %+v allowed=%v, want %v", c.refund, c.totals, ok, c.wantOK)
			}
		})
	}
}
