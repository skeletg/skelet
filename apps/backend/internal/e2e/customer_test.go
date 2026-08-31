package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// Scenario: guest orders attach to an account only on EMAIL VERIFICATION - registering proves
// nothing about owning the address, the emailed link is the proof
func TestVerifiedEmailClaimsGuestOrders(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 1200)
	email := "claim-" + uuid.NewString()[:8] + "@e2e.test" // lowercase - the claim matches verbatim

	cartID := h.newGuestCart(t, f, 1)
	o, err := h.tryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) { in.Email = email })
	if err != nil {
		t.Fatalf("guest placement: %v", err)
	}
	h.settle(t)

	c, err := h.d.Customer.Register(h.ctx, f.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// Registration alone must claim nothing
	if row, _ := h.d.Queries.GetOrderByID(h.ctx, o.ID); row.CustomerID.Valid {
		t.Fatal("guest order claimed at registration - claiming requires the verified link")
	}

	token, err := h.d.Verify.Create(h.ctx, c.ID)
	if err != nil {
		t.Fatalf("mint verification token: %v", err)
	}
	if err := h.d.Customer.VerifyEmail(h.ctx, token); err != nil {
		t.Fatalf("verify email: %v", err)
	}
	h.settle(t) // the claim is a plain async subscriber - WaitIdle covers it

	row, err := h.d.Queries.GetOrderByID(h.ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	if !row.CustomerID.Valid || row.CustomerID.UUID != c.ID {
		t.Errorf("order customer = %v, want claimed by %s", row.CustomerID, c.ID)
	}
}

// Scenario: the full loyalty cycle - accrual on placement for a registered customer, idempotent
// under replay, revoked in full on cancellation, idempotent again
func TestLoyaltyAccrualAndRevocationCycle(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 990)
	if _, err := h.d.Config.Set(h.ctx, "sales/loyalty/enabled", "true",
		config.Scope{Kind: config.ScopeChannel, ChannelID: &f.Channel.ID}); err != nil {
		t.Fatalf("enable loyalty: %v", err)
	}

	email := "loyal-" + uuid.NewString()[:8] + "@e2e.test"
	c, err := h.d.Customer.Register(h.ctx, f.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	cartID := h.newCustomerCart(t, f, c.ID, f.Product.SKU, 1)
	o, err := h.tryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.CustomerID = &c.ID
		in.Email = email
	})
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.settle(t)

	// 990 cents of merchandise at 1 point per unit-of-100 = 9 points (integer division)
	if bal, _ := h.d.Loyalty.Balance(h.ctx, c.ID); bal != 9 {
		t.Errorf("points after placement = %d, want 9", bal)
	}

	h.rearmOutbox(t, "order.placed", o.ID.String(), 1)
	h.settle(t)
	if bal, _ := h.d.Loyalty.Balance(h.ctx, c.ID); bal != 9 {
		t.Errorf("points after replayed placement = %d, want still 9", bal)
	}

	h.cancel(t, o.ID)
	h.settle(t)
	if bal, _ := h.d.Loyalty.Balance(h.ctx, c.ID); bal != 0 {
		t.Errorf("points after cancel = %d, want 0 - full revocation at fraction 1.0", bal)
	}

	h.rearmOutbox(t, "order.cancelled", o.ID.String(), 1)
	h.settle(t)
	if bal, _ := h.d.Loyalty.Balance(h.ctx, c.ID); bal != 0 {
		t.Errorf("points after replayed cancel = %d, want still 0 - revoke targets, never stacks", bal)
	}
}
