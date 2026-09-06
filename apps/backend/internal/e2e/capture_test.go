package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// Scenario 2: money collected outside a gateway lands as real capture rows and the status derives from the ledger - a deposit leaves the
// order partially captured, the remainder settles it, and a third collection attempt refuses because nothing is owed
func TestCODOfflineCaptureSettlesLedger(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 2500)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)

	deposit, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{
		OrderID:     o.ID,
		AmountCents: 1000,
		Method:      "cod",
		Reference:   "e2e-deposit",
	})
	if err != nil {
		t.Fatalf("record deposit: %v", err)
	}
	if deposit.PaymentStatus != "partially_captured" {
		t.Errorf("payment_status after deposit = %q, want partially_captured", deposit.PaymentStatus)
	}

	// Zero amount means the whole remaining balance
	settled, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{
		OrderID:   o.ID,
		Method:    "cod",
		Reference: "e2e-balance",
	})
	if err != nil {
		t.Fatalf("record balance: %v", err)
	}
	if settled.PaymentStatus != "paid" {
		t.Errorf("payment_status after balance = %q, want paid", settled.PaymentStatus)
	}

	assertCaptureTotal(t, h, o.ID, 2500)

	// Over-collection must refuse - a PAID order is not collectable at all, so that guard fires
	// before the nothing-owed check ever runs
	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID,
		Method:  "cod",
	}); err == nil {
		t.Fatal("third collection on a paid order succeeded")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "PAYMENT_NOT_COLLECTABLE" {
		t.Errorf("third collection error = %v, want code PAYMENT_NOT_COLLECTABLE", err)
	}
	assertCaptureTotal(t, h, o.ID, 2500) // and it must not have written anything

	h.Settle(t)
}

func assertCaptureTotal(t *testing.T, h *harness, orderID uuid.UUID, want int64) {
	t.Helper()
	sums, err := h.D.Queries.SumOrderPaymentByKind(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("ledger sums: %v", err)
	}
	if len(sums) != 1 || sums[0].Kind != order.PaymentTxnCapture || sums[0].Total != want {
		t.Errorf("ledger sums = %+v, want exactly one capture kind totaling %d", sums, want)
	}
}
