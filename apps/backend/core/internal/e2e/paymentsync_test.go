package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/wiring"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The sweep only asks about orders parked longer than a checkout takes - age one past that window
func (h *harness) ageOrder(t *testing.T, orderID uuid.UUID) {
	t.Helper()
	if _, err := h.D.DB.ExecContext(h.Ctx, `UPDATE orders SET updated_at = NOW() - interval '1 hour' WHERE id = $1`, orderID); err != nil {
		t.Fatalf("age order: %v", err)
	}
}

func (h *harness) sweepPayments(t *testing.T) {
	t.Helper()
	if err := wiring.SweepUnsettledPayments(h.Ctx, h.D.Order, h.D.Payment); err != nil {
		t.Fatalf("payment sync sweep: %v", err)
	}
	h.Settle(t)
}

func (h *harness) paymentStatus(t *testing.T, orderID uuid.UUID) string {
	t.Helper()
	row, err := h.D.Queries.GetOrderByID(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	return row.PaymentStatus
}

// Scenario: a shopper closes the hosted checkout, so no webhook ever arrives - the sweep asks the gateway, waits out a
// pending answer, and settles the capture it eventually reports, once, onto the ledger
func TestTheSyncSweepSettlesAPaymentNoWebhookReported(t *testing.T) {
	h := newHarness(t)
	f := h.gatewayStore(t, 5, 2600)
	fakeGW.parkAuthorizations(t)
	o := h.placeWithGateway(t, f)
	h.Settle(t)
	if got := h.paymentStatus(t, o.ID); got != order.PaymentUnpaid {
		t.Fatalf("a parked redirect left payment_status %q, want unpaid until the gateway says otherwise", got)
	}

	h.ageOrder(t, o.ID)
	fakeGW.answerSync(sdk.SyncPaymentResult{State: sdk.PaymentStatePending})
	h.sweepPayments(t)
	if got := h.paymentStatus(t, o.ID); got != order.PaymentUnpaid {
		t.Fatalf("a pending answer moved payment_status to %q", got)
	}

	h.ageOrder(t, o.ID)
	fakeGW.answerSync(sdk.SyncPaymentResult{State: sdk.PaymentStateCaptured, CapturedCents: 2600})
	h.sweepPayments(t)
	if got := h.paymentStatus(t, o.ID); got != order.PaymentPaid {
		t.Fatalf("payment_status = %q after the gateway reported the capture, want paid", got)
	}
	if got := h.LedgerByKind(t, o.ID)[order.PaymentTxnCapture]; got != 2600 {
		t.Fatalf("capture ledger = %d, want the 2600 the gateway reported", got)
	}

	// A paid order leaves the worklist, so a later pass can't book the capture twice
	h.ageOrder(t, o.ID)
	h.sweepPayments(t)
	if got := h.LedgerByKind(t, o.ID)[order.PaymentTxnCapture]; got != 2600 {
		t.Fatalf("capture ledger = %d after a second sweep, want it booked once", got)
	}
}

// A shopper who abandoned the redirect leaves a failed payment - the order stays unpaid so they can retry, not voided under them
func TestTheSyncSweepLeavesAFailedPaymentRetryable(t *testing.T) {
	h := newHarness(t)
	f := h.gatewayStore(t, 5, 2600)
	fakeGW.parkAuthorizations(t)
	o := h.placeWithGateway(t, f)
	h.Settle(t)
	h.ageOrder(t, o.ID)
	fakeGW.answerSync(sdk.SyncPaymentResult{State: sdk.PaymentStateFailed})
	h.sweepPayments(t)
	if got := h.paymentStatus(t, o.ID); got != order.PaymentUnpaid {
		t.Fatalf("a failed payment moved payment_status to %q, want unpaid", got)
	}
}
