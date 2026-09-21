package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

// Scenario: a gift card is a TENDER, not a discount - the total stands, the card drains inside
// the placement tx, and cancellation restores it exactly once even when the durable event replays
func TestGiftCardTenderAndCancelRestore(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 3000)
	card, err := h.D.Giftcard.Issue(h.Ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 1000})
	if err != nil {
		t.Fatalf("issue gift card: %v", err)
	}

	cartID := h.NewGuestCart(t, f, 1)
	o, err := h.TryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.GiftCardCodes = []string{card.Code}
	})
	if err != nil {
		t.Fatalf("place with gift card: %v", err)
	}
	if o.GiftCardAmount != 1000 {
		t.Errorf("gift_card_amount = %d, want 1000", o.GiftCardAmount)
	}
	if bal, _ := h.D.Giftcard.Balance(h.Ctx, card.Code); bal != 0 {
		t.Errorf("card balance after redeem = %d, want 0", bal)
	}
	h.Settle(t)

	h.Cancel(t, o.ID)
	h.Settle(t)
	if bal, _ := h.D.Giftcard.Balance(h.Ctx, card.Code); bal != 1000 {
		t.Errorf("card balance after cancel = %d, want 1000 restored", bal)
	}

	// The exactly-once proof: a second full delivery must see "already restored" and add nothing
	h.RearmOutbox(t, "order.cancelled", o.ID.String(), 1)
	h.Settle(t)
	if bal, _ := h.D.Giftcard.Balance(h.Ctx, card.Code); bal != 1000 {
		t.Errorf("card balance after replayed cancel = %d, want still 1000", bal)
	}
}

// Scenario: a card covering the whole total places the order PAID with no payment provider at all
func TestGiftCardFullCoverPlacesPaid(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2000)
	card, err := h.D.Giftcard.Issue(h.Ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 5000})
	if err != nil {
		t.Fatalf("issue gift card: %v", err)
	}

	cartID := h.NewGuestCart(t, f, 1)
	o, err := h.TryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.GiftCardCodes = []string{card.Code}
		in.PaymentProviderSlug = nil // fully tendered - the PAYMENT_REQUIRED guard must not fire
	})
	if err != nil {
		t.Fatalf("place fully covered: %v", err)
	}
	if o.PaymentStatus != "paid" || o.GiftCardAmount != 2000 {
		t.Errorf("payment/gift = %q/%d, want paid/2000", o.PaymentStatus, o.GiftCardAmount)
	}
	if bal, _ := h.D.Giftcard.Balance(h.Ctx, card.Code); bal != 3000 {
		t.Errorf("card balance = %d, want 3000 left", bal)
	}
	h.Settle(t)
}

// Scenario: a REFUND reverses rewards, never tenders - restoring the card here would pay the
// customer twice (the memo already returns the money)
func TestRefundDoesNotRestoreTenders(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 3000)
	card, err := h.D.Giftcard.Issue(h.Ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 1000})
	if err != nil {
		t.Fatalf("issue gift card: %v", err)
	}
	cartID := h.NewGuestCart(t, f, 1)
	o, err := h.TryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.GiftCardCodes = []string{card.Code}
	})
	if err != nil {
		t.Fatalf("place with gift card: %v", err)
	}
	h.Settle(t)

	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID, Method: "cod", Reference: "e2e-cash",
	}); err != nil {
		t.Fatalf("collect balance: %v", err)
	}
	intent, err := h.D.Order.RecordCreditMemoIntent(h.Ctx, order.IssueCreditMemoInput{
		OrderID: o.ID, AdjustmentRefundCents: 999_999, // clamps to the 2000 captured
	})
	if err != nil {
		t.Fatalf("memo intent: %v", err)
	}
	if _, _, err := h.D.Order.FinalizeCreditMemo(h.Ctx, intent.Txn.ID, true, "manual"); err != nil {
		t.Fatalf("finalize memo: %v", err)
	}
	h.Settle(t)

	if bal, _ := h.D.Giftcard.Balance(h.Ctx, card.Code); bal != 0 {
		t.Errorf("card balance after refund = %d, want 0 - refunds must not restore tenders", bal)
	}
}

// Scenario: buying gift-card products mints one card per unit, exactly once under replay
func TestGiftCardPurchaseMintsOncePerUnit(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 1000)
	gc := h.AddGiftcardProduct(t, f, 1500)

	cartID := h.NewGuestCartSKU(t, f, gc.SKU, 2)
	o, err := h.TryPlace(t, f.Channel, cartID)
	if err != nil {
		t.Fatalf("place gift card purchase: %v", err)
	}
	h.Settle(t)

	minted, err := h.D.Queries.ListGiftCardsByIssuedOrder(h.Ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
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

	h.RearmOutbox(t, "order.placed", o.ID.String(), 1)
	h.Settle(t)
	after, err := h.D.Queries.ListGiftCardsByIssuedOrder(h.Ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
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
	f := h.NewStore(t, 5, 3000)
	email := "cust-" + uuid.NewString()[:8] + "@e2e.test"
	c, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, email, "password123", "E2E", "Customer", nil)
	if err != nil {
		t.Fatalf("register customer: %v", err)
	}
	if _, err := h.D.StoreCredit.Adjust(h.Ctx, c.ID, 800, "USD", "e2e grant", nil); err != nil {
		t.Fatalf("grant credit: %v", err)
	}

	cartID := h.NewCustomerCart(t, f, c.ID, f.Product.SKU, 1)
	o, err := h.TryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
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
	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, c.ID, "USD"); bal != 0 {
		t.Errorf("credit balance after placement = %d, want 0", bal)
	}
	h.Settle(t)

	h.Cancel(t, o.ID)
	h.Settle(t)
	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, c.ID, "USD"); bal != 800 {
		t.Errorf("credit balance after cancel = %d, want 800 restored", bal)
	}

	h.RearmOutbox(t, "order.cancelled", o.ID.String(), 1)
	h.Settle(t)
	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, c.ID, "USD"); bal != 800 {
		t.Errorf("credit balance after replayed cancel = %d, want still 800", bal)
	}
}
