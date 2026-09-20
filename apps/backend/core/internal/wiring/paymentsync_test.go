package wiring

import (
	"testing"

	paymentpkg "github.com/skeletg/skelet/apps/backend/core/internal/payment"
)

// The sweep must never settle on ambiguity - only the four definite gateway states move an order
func TestPaymentSyncActionFor(t *testing.T) {
	for name, tc := range map[string]struct {
		state paymentpkg.PaymentState
		want  paymentSyncAction
	}{
		"authorized records the hold": {paymentpkg.PaymentStateAuthorized, syncRecordAuthorized},
		"captured settles paid":       {paymentpkg.PaymentStateCaptured, syncSettleCaptured},
		"refunded settles refund":     {paymentpkg.PaymentStateRefunded, syncSettleRefunded},
		"voided settles void":         {paymentpkg.PaymentStateVoided, syncSettleVoided},
		"pending stays parked":        {paymentpkg.PaymentStatePending, syncNothing},
		"failed stays retryable":      {paymentpkg.PaymentStateFailed, syncNothing},
		"unknown is a no-op":          {paymentpkg.PaymentState("gibberish"), syncNothing},
	} {
		t.Run(name, func(t *testing.T) {
			if got := paymentSyncActionFor(tc.state); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
