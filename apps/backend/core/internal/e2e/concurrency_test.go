package e2e

import (
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

// Scenario: two checkouts race to spend one gift card - the row lock serializes them, the balance
// can never go negative, and the money applied across every winner never exceeds the face value
func TestGiftCardRedemptionRaceNeverOverspends(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	card, err := h.D.Giftcard.Issue(h.Ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 1500})
	if err != nil {
		t.Fatalf("issue card: %v", err)
	}

	results := []struct {
		id  uuid.UUID
		o   order.Order
		err error
	}{
		{id: h.NewGuestCart(t, f, 1)},
		{id: h.NewGuestCart(t, f, 1)},
	}
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			results[i].o, results[i].err = h.TryPlace(t, f.Channel, results[i].id, func(in *order.PlaceOrderInput) {
				in.GiftCardCodes = []string{card.Code}
			})
		})
	}
	wg.Wait()

	var applied int64
	for _, r := range results {
		if r.err == nil {
			applied += r.o.GiftCardAmount
		}
	}
	bal, err := h.giftcardBalance(card.Code)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal < 0 {
		t.Errorf("card balance went negative: %d", bal)
	}
	if applied+bal != 1500 {
		t.Errorf("applied %d + remaining %d != the 1500 face - money appeared or vanished", applied, bal)
	}
	h.Settle(t)
}

// Scenario: two refund memos race for one remaining balance - the order lock serializes the
// ceiling, so the refunds can never sum past the captures no matter how the race lands
func TestRefundCeilingRaceNeverOverRefunds(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2600)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID, Method: "cod", Reference: "e2e-cash",
	}); err != nil {
		t.Fatalf("collect: %v", err)
	}

	intents := make([]order.CreditMemoIntent, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range intents {
		wg.Go(func() {
			intents[i], errs[i] = h.D.Order.RecordCreditMemoIntent(h.Ctx, order.IssueCreditMemoInput{
				OrderID:               o.ID,
				AdjustmentRefundCents: 999_999, // both ask for everything
			})
		})
	}
	wg.Wait()

	for i := range intents {
		if errs[i] != nil {
			continue // the loser saw a drained ceiling - correct
		}
		if _, _, err := h.D.Order.FinalizeCreditMemo(h.Ctx, intents[i].Txn.ID, true, "manual"); err != nil {
			t.Fatalf("finalize racing memo %d: %v", i, err)
		}
	}
	h.Settle(t)

	sums := h.LedgerByKind(t, o.ID)
	if sums[order.PaymentTxnRefund] != sums[order.PaymentTxnCapture] {
		t.Errorf("ledger = %+v - racing refunds must sum to exactly the captures, never past them", sums)
	}
}
