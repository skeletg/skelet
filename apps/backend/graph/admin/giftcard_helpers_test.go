package admin

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/giftcard"
)

func TestToGiftCardModelMapsBalancesAndNullables(t *testing.T) { // the admin refund screen reads Balance off this model, so a swap with InitialBalance would show spent cards as full
	order := uuid.New()
	exp := time.Now().Add(24 * time.Hour)
	g := giftcard.GiftCard{
		ID: uuid.New(), Code: "GIFT-XYZ", CurrencyCode: "USD",
		InitialBalance: 5000, Balance: 1200, Status: "active",
		RecipientEmail: "ana@shop.test", IssuedOrderID: &order, ExpiresAt: &exp,
		CreatedAt: time.Now(),
	}

	m := toGiftCardModel(g, nil)
	if m.InitialBalance != 5000 || m.Balance != 1200 {
		t.Errorf("initial=%d balance=%d, want 5000 and 1200 kept distinct", m.InitialBalance, m.Balance)
	}
	if m.IssuedOrderID == nil || *m.IssuedOrderID != order.String() {
		t.Errorf("IssuedOrderID = %v, want %s", m.IssuedOrderID, order)
	}
	if m.ExpiresAt == nil || !m.ExpiresAt.Equal(exp) {
		t.Errorf("ExpiresAt = %v, want %s", m.ExpiresAt, exp)
	}
	if m.Transactions == nil || len(m.Transactions) != 0 {
		t.Errorf("Transactions = %v for a list view, want an empty non-nil list", m.Transactions)
	}
}

func TestToGiftCardModelWithoutTheOptionals(t *testing.T) { // a manually-issued card has no order and no expiry
	m := toGiftCardModel(giftcard.GiftCard{ID: uuid.New(), Code: "GIFT-A"}, nil)
	if m.IssuedOrderID != nil {
		t.Errorf("IssuedOrderID = %v, want nil", m.IssuedOrderID)
	}
	if m.ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil", m.ExpiresAt)
	}
}

func TestToGiftCardModelMapsTheLedger(t *testing.T) { // redemptions are negative and refund top-ups positive - flipping a sign here misreads the whole ledger
	order := uuid.New()
	txns := []giftcard.Transaction{
		{ID: uuid.New(), Amount: -3800, Reason: "redemption", OrderID: &order},
		{ID: uuid.New(), Amount: 1000, Reason: "refund", Note: "partial return"},
	}

	m := toGiftCardModel(giftcard.GiftCard{ID: uuid.New()}, txns)
	if len(m.Transactions) != 2 {
		t.Fatalf("%d transactions, want 2", len(m.Transactions))
	}
	spend, topup := m.Transactions[0], m.Transactions[1]
	if spend.Amount != -3800 || spend.Reason != "redemption" {
		t.Errorf("spend = %+v, want -3800 redemption", spend)
	}
	if spend.OrderID == nil || *spend.OrderID != order.String() {
		t.Errorf("spend.OrderID = %v, want %s", spend.OrderID, order)
	}
	if topup.Amount != 1000 || topup.OrderID != nil || topup.Note != "partial return" {
		t.Errorf("topup = %+v, want 1000 with no order and the note kept", topup)
	}
}
