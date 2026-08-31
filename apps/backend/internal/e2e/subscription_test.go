package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// Scenario: placing a subscription product spawns the subscription, the billing sweep mints one
// invoice per elapsed period exactly once, and collection with no saved payment method walks the
// dunning ladder instead of charging - the invoice parks with a retry, the subscription goes
// past_due, and an immediate re-sweep touches nothing
func TestSubscriptionInvoiceAndDunning(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 1000)
	sub := h.addSubscriptionProduct(t, f, 2000)

	cartID := h.newGuestCartSKU(t, f, sub.SKU, 1)
	o, err := h.tryPlace(t, f.Channel, cartID)
	if err != nil {
		t.Fatalf("place subscription: %v", err)
	}
	h.settle(t) // subscription row spawns off the durable order.placed

	subs, err := h.d.Queries.ListSubscriptionsByOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil {
		t.Fatalf("subscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("got %d subscriptions from placement, want 1", len(subs))
	}
	subID := subs[0].ID

	// A month and change from now the period has elapsed once - exactly one invoice
	future := time.Now().AddDate(0, 1, 2)
	if n, err := h.d.Subscription.ProcessDue(h.ctx, future); err != nil || n != 1 {
		t.Fatalf("ProcessDue = %d, %v - want 1 subscription billed", n, err)
	}
	invoices, err := h.d.Queries.ListSubscriptionInvoices(h.ctx, subID)
	if err != nil {
		t.Fatalf("invoices: %v", err)
	}
	if len(invoices) != 1 || invoices[0].Status != "due" || invoices[0].Amount != 2000 {
		t.Fatalf("invoices = %+v, want one due invoice of 2000", invoices)
	}

	// The period already advanced - a second sweep at the same instant mints nothing
	if _, err := h.d.Subscription.ProcessDue(h.ctx, future); err != nil {
		t.Fatalf("second ProcessDue: %v", err)
	}
	if again, _ := h.d.Queries.ListSubscriptionInvoices(h.ctx, subID); len(again) != 1 {
		t.Errorf("second sweep minted extra invoices: %d, want still 1", len(again))
	}

	// Guest order, no vaulted method - collection must dun, not charge. The return counts
	// successful charges, so a dunning walk correctly collects zero
	if n, err := h.d.Subscription.CollectDue(h.ctx, future); err != nil || n != 0 {
		t.Fatalf("CollectDue = %d, %v - want 0 collected, the charge must fail into dunning", n, err)
	}
	invoices, err = h.d.Queries.ListSubscriptionInvoices(h.ctx, subID)
	if err != nil {
		t.Fatalf("invoices after dunning: %v", err)
	}
	inv := invoices[0]
	if inv.Status != "due" || inv.AttemptCount != 1 || !inv.NextAttemptAt.Valid {
		t.Errorf("invoice after failed charge = %+v, want due/attempts 1/retry scheduled", inv)
	}
	after, err := h.d.Queries.ListSubscriptionsByOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil {
		t.Fatalf("subscription after dunning: %v", err)
	}
	if after[0].Status != "past_due" {
		t.Errorf("subscription status = %q, want past_due", after[0].Status)
	}

	// The retry is parked in the future, so an immediate re-collection selects nothing - the
	// attempt counter is the proof, since the collected count is 0 either way
	if n, err := h.d.Subscription.CollectDue(h.ctx, future); err != nil || n != 0 {
		t.Errorf("immediate re-collection = %d, %v - want 0, the ladder waits", n, err)
	}
	final, err := h.d.Queries.ListSubscriptionInvoices(h.ctx, subID)
	if err != nil {
		t.Fatalf("invoices after re-collection: %v", err)
	}
	if final[0].AttemptCount != 1 {
		t.Errorf("attempt_count after parked re-collection = %d, want still 1", final[0].AttemptCount)
	}
}

// Scenario: the success half of renewals - a vaulted method charges off-session through the fake
// gateway, the invoice marks paid with a reference, and the subscription never leaves active
func TestSubscriptionVaultedRenewalCharges(t *testing.T) {
	h := newHarness(t)
	f := h.gatewayStore(t, 5, 1000)
	sub := h.addSubscriptionProduct(t, f, 3000)
	email := "vault-" + uuid.NewString()[:8] + "@e2e.test"
	c, err := h.d.Customer.Register(h.ctx, f.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	cartID := h.newCustomerCart(t, f, c.ID, sub.SKU, 1)
	o, err := h.tryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.CustomerID = &c.ID
		in.Email = email
	})
	if err != nil {
		t.Fatalf("place subscription: %v", err)
	}
	h.settle(t)

	method, err := h.d.PaymentVault.Save(h.ctx, c.ID, email, gwSlug, "tok-e2e", f.Channel.ID)
	if err != nil {
		t.Fatalf("vault method: %v", err)
	}
	subs, err := h.d.Queries.ListSubscriptionsByOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil || len(subs) != 1 {
		t.Fatalf("subscriptions: %d (%v)", len(subs), err)
	}
	if _, err := h.d.Subscription.SetPaymentMethodForCustomer(h.ctx, c.ID, subs[0].ID, &method.ID); err != nil {
		t.Fatalf("attach method: %v", err)
	}

	future := time.Now().AddDate(0, 1, 2)
	if n, err := h.d.Subscription.ProcessDue(h.ctx, future); err != nil || n != 1 {
		t.Fatalf("ProcessDue = %d, %v", n, err)
	}
	if n, err := h.d.Subscription.CollectDue(h.ctx, future); err != nil || n != 1 {
		t.Fatalf("CollectDue = %d, %v - want the vaulted charge to succeed", n, err)
	}
	invoices, err := h.d.Queries.ListSubscriptionInvoices(h.ctx, subs[0].ID)
	if err != nil || len(invoices) != 1 {
		t.Fatalf("invoices: %d (%v)", len(invoices), err)
	}
	if invoices[0].Status != "paid" || invoices[0].PaymentReference == "" {
		t.Errorf("invoice = %s/%q, want paid with a gateway reference", invoices[0].Status, invoices[0].PaymentReference)
	}
	// the renewal must charge the STORED method - the vaulted-token slot once had no producer, so
	// renewals reached the gateway with nothing identifying what to charge and nothing noticed
	if got := fakeGW.lastAuth().VaultedToken; got != "vault-tok-e2e" {
		t.Errorf("renewal charged with VaultedToken %q, want vault-tok-e2e (the stored method's provider token)", got)
	}
	after, _ := h.d.Queries.ListSubscriptionsByOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if after[0].Status != "active" {
		t.Errorf("subscription = %q after a successful renewal, want active", after[0].Status)
	}
	h.settle(t)
}
