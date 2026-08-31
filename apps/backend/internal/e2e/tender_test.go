package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// Scenario: a gift card is a TENDER, not a discount - the total stands, the card drains inside
// the placement tx, and cancellation restores it exactly once even when the durable event replays
func TestGiftCardTenderAndCancelRestore(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 3000)
	card, err := h.d.Giftcard.Issue(h.ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 1000})
	if err != nil {
		t.Fatalf("issue gift card: %v", err)
	}

	cartID := h.newGuestCart(t, f, 1)
	o, err := h.tryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.GiftCardCodes = []string{card.Code}
	})
	if err != nil {
		t.Fatalf("place with gift card: %v", err)
	}
	if o.GiftCardAmount != 1000 {
		t.Errorf("gift_card_amount = %d, want 1000", o.GiftCardAmount)
	}
	if bal, _ := h.d.Giftcard.Balance(h.ctx, card.Code); bal != 0 {
		t.Errorf("card balance after redeem = %d, want 0", bal)
	}
	h.settle(t)

	h.cancel(t, o.ID)
	h.settle(t)
	if bal, _ := h.d.Giftcard.Balance(h.ctx, card.Code); bal != 1000 {
		t.Errorf("card balance after cancel = %d, want 1000 restored", bal)
	}

	// The exactly-once proof: a second full delivery must see "already restored" and add nothing
	h.rearmOutbox(t, "order.cancelled", o.ID.String(), 1)
	h.settle(t)
	if bal, _ := h.d.Giftcard.Balance(h.ctx, card.Code); bal != 1000 {
		t.Errorf("card balance after replayed cancel = %d, want still 1000", bal)
	}
}

// Scenario: a card covering the whole total places the order PAID with no payment provider at all
func TestGiftCardFullCoverPlacesPaid(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 2000)
	card, err := h.d.Giftcard.Issue(h.ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 5000})
	if err != nil {
		t.Fatalf("issue gift card: %v", err)
	}

	cartID := h.newGuestCart(t, f, 1)
	o, err := h.tryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.GiftCardCodes = []string{card.Code}
		in.PaymentProviderSlug = nil // fully tendered - the PAYMENT_REQUIRED guard must not fire
	})
	if err != nil {
		t.Fatalf("place fully covered: %v", err)
	}
	if o.PaymentStatus != "paid" || o.GiftCardAmount != 2000 {
		t.Errorf("payment/gift = %q/%d, want paid/2000", o.PaymentStatus, o.GiftCardAmount)
	}
	if bal, _ := h.d.Giftcard.Balance(h.ctx, card.Code); bal != 3000 {
		t.Errorf("card balance = %d, want 3000 left", bal)
	}
	h.settle(t)
}

// Scenario: a REFUND reverses rewards, never tenders - restoring the card here would pay the
// customer twice (the memo already returns the money)
func TestRefundDoesNotRestoreTenders(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 3000)
	card, err := h.d.Giftcard.Issue(h.ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 1000})
	if err != nil {
		t.Fatalf("issue gift card: %v", err)
	}
	cartID := h.newGuestCart(t, f, 1)
	o, err := h.tryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.GiftCardCodes = []string{card.Code}
	})
	if err != nil {
		t.Fatalf("place with gift card: %v", err)
	}
	h.settle(t)

	if _, err := h.d.Order.RecordOfflinePayment(h.ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID, Method: "cod", Reference: "e2e-cash",
	}); err != nil {
		t.Fatalf("collect balance: %v", err)
	}
	intent, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID: o.ID, AdjustmentRefundCents: 999_999, // clamps to the 2000 captured
	})
	if err != nil {
		t.Fatalf("memo intent: %v", err)
	}
	if _, _, err := h.d.Order.FinalizeCreditMemo(h.ctx, intent.Txn.ID, true, "manual"); err != nil {
		t.Fatalf("finalize memo: %v", err)
	}
	h.settle(t)

	if bal, _ := h.d.Giftcard.Balance(h.ctx, card.Code); bal != 0 {
		t.Errorf("card balance after refund = %d, want 0 - refunds must not restore tenders", bal)
	}
}

// Scenario: buying gift-card products mints one card per unit, exactly once under replay
func TestGiftCardPurchaseMintsOncePerUnit(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 1000)
	gc := h.addGiftcardProduct(t, f, 1500)

	cartID := h.newGuestCartSKU(t, f, gc.SKU, 2)
	o, err := h.tryPlace(t, f.Channel, cartID)
	if err != nil {
		t.Fatalf("place gift card purchase: %v", err)
	}
	h.settle(t)

	minted, err := h.d.Queries.ListGiftCardsByIssuedOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil {
		t.Fatalf("minted cards: %v", err)
	}
	if len(minted) != 2 {
		t.Fatalf("minted %d cards for qty 2, want one per unit", len(minted))
	}
	for _, c := range minted {
		if c.Balance != 1500 {
			t.Errorf("minted card balance = %d, want the 1500 unit price", c.Balance)
		}
	}

	h.rearmOutbox(t, "order.placed", o.ID.String(), 1)
	h.settle(t)
	after, err := h.d.Queries.ListGiftCardsByIssuedOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil {
		t.Fatalf("minted cards after replay: %v", err)
	}
	if len(after) != 2 {
		t.Errorf("replay minted extra cards: %d, want still 2", len(after))
	}
}

// Scenario: store credit as a tender for a registered customer, restored exactly once on cancel
func TestStoreCreditTenderAndCancelRestore(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 3000)
	email := "cust-" + uuid.NewString()[:8] + "@e2e.test"
	c, err := h.d.Customer.Register(h.ctx, f.Channel.ID, email, "password123", "E2E", "Customer", nil)
	if err != nil {
		t.Fatalf("register customer: %v", err)
	}
	if _, err := h.d.StoreCredit.Adjust(h.ctx, c.ID, 800, "USD", "e2e grant", nil); err != nil {
		t.Fatalf("grant credit: %v", err)
	}

	cartID := h.newCustomerCart(t, f, c.ID, f.Product.SKU, 1)
	o, err := h.tryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.CustomerID = &c.ID
		in.Email = email
		in.UseStoreCredit = true
	})
	if err != nil {
		t.Fatalf("place with store credit: %v", err)
	}
	if o.StoreCreditAmount != 800 {
		t.Errorf("store_credit_amount = %d, want 800", o.StoreCreditAmount)
	}
	if bal, _ := h.d.StoreCredit.Balance(h.ctx, c.ID, "USD"); bal != 0 {
		t.Errorf("credit balance after placement = %d, want 0", bal)
	}
	h.settle(t)

	h.cancel(t, o.ID)
	h.settle(t)
	if bal, _ := h.d.StoreCredit.Balance(h.ctx, c.ID, "USD"); bal != 800 {
		t.Errorf("credit balance after cancel = %d, want 800 restored", bal)
	}

	h.rearmOutbox(t, "order.cancelled", o.ID.String(), 1)
	h.settle(t)
	if bal, _ := h.d.StoreCredit.Balance(h.ctx, c.ID, "USD"); bal != 800 {
		t.Errorf("credit balance after replayed cancel = %d, want still 800", bal)
	}
}
