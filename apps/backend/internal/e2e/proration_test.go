package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/internal/money"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// Scenario: cancelling a HALF-shipped order prorates every reversal by the unshipped value share -
// the gift card comes back at half, loyalty revokes toward half, and only the unshipped unit's
// reservation releases. Every earlier cancel test ran at fraction 1.0; this is the ScaleFraction
// arithmetic's first execution
func TestPartialShipCancelProratesReversals(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 1200) // qty 2 keeps the subtotal under ambient promotion thresholds
	if _, err := h.d.Config.Set(h.ctx, "sales/loyalty/enabled", "true",
		config.Scope{Kind: config.ScopeChannel, ChannelID: &f.Channel.ID}); err != nil {
		t.Fatalf("enable loyalty: %v", err)
	}
	email := "half-" + uuid.NewString()[:8] + "@e2e.test"
	c, err := h.d.Customer.Register(h.ctx, f.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	card, err := h.d.Giftcard.Issue(h.ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 1000})
	if err != nil {
		t.Fatalf("issue card: %v", err)
	}

	cartID := h.newCustomerCart(t, f, c.ID, f.Product.SKU, 2)
	o, err := h.tryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.CustomerID = &c.ID
		in.Email = email
		in.GiftCardCodes = []string{card.Code}
	})
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.settle(t)
	earned, _ := h.d.Loyalty.Balance(h.ctx, c.ID)
	if earned <= 0 {
		t.Fatalf("earned %d points, want accrual before proration can mean anything", earned)
	}
	spent := o.GiftCardAmount

	for _, status := range []string{"confirmed", "processing"} {
		if _, err := h.d.Order.UpdateStatus(h.ctx, o.ID, status); err != nil {
			t.Fatalf("advance to %s: %v", status, err)
		}
	}
	lines := h.orderLines(t, o.ID)
	if _, err := h.d.Order.CreateFulfillment(h.ctx, order.CreateFulfillmentInput{
		OrderID: o.ID,
		Lines:   []order.FulfillmentLineInput{{OrderLineID: lines[0].ID, Quantity: 1}},
	}); err != nil {
		t.Fatalf("ship half: %v", err)
	}
	h.settle(t)

	h.cancel(t, o.ID) // partially_fulfilled -> cancelled, fraction = unshipped value share = 0.5
	h.settle(t)

	wantRestore := money.ScaleFraction(spent, 0.5)
	if bal, _ := h.d.Giftcard.Balance(h.ctx, card.Code); bal != wantRestore {
		t.Errorf("card restored %d, want the prorated %d of %d", bal, wantRestore, spent)
	}
	wantPoints := earned - money.ScaleFraction(earned, 0.5)
	if bal, _ := h.d.Loyalty.Balance(h.ctx, c.ID); bal != wantPoints {
		t.Errorf("points after prorated revoke = %d, want %d of the %d earned", bal, wantPoints, earned)
	}
	if got := h.reservedFor(t, f); got != 0 {
		t.Errorf("reserved = %v after cancel, want 0 - shipped unit settled, unshipped released", got)
	}

	// Proration must also be replay-stable: revoke targets, restores guard
	h.rearmOutbox(t, "order.cancelled", o.ID.String(), 1)
	h.settle(t)
	if bal, _ := h.d.Giftcard.Balance(h.ctx, card.Code); bal != wantRestore {
		t.Errorf("card balance after replayed prorated cancel = %d, want still %d", bal, wantRestore)
	}
	if bal, _ := h.d.Loyalty.Balance(h.ctx, c.ID); bal != wantPoints {
		t.Errorf("points after replayed prorated cancel = %d, want still %d", bal, wantPoints)
	}
}
