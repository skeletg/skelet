package wiring

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
)

// A $10 partial refund once marked a $500 order refunded - the two money-moving names must route to the amount-aware path and nothing else may
func TestProviderWebhookTxnKind(t *testing.T) {
	for name, tc := range map[string]struct {
		event       string
		wantKind    string
		amountAware bool
	}{
		"captured settles by amount": {payment.EventCaptured, order.PaymentTxnCapture, true},
		"refunded settles by amount": {payment.EventRefunded, order.PaymentTxnRefund, true},
		"authorized keeps name path": {payment.EventAuthorized, "", false},
		"voided keeps name path":     {payment.EventVoided, "", false},
		"provider-specific names":    {"payment.paypal.capture_completed", "", false},
		"empty":                      {"", "", false},
	} {
		t.Run(name, func(t *testing.T) {
			kind, aware := providerWebhookTxnKind(tc.event)
			if kind != tc.wantKind || aware != tc.amountAware {
				t.Errorf("got (%q, %v), want (%q, %v)", kind, aware, tc.wantKind, tc.amountAware)
			}
		})
	}
}
