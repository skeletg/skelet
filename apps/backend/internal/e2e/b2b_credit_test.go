package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/b2b"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// onAccountStore is a store with purchase-order enabled, plus the placement option that selects it
func (h *harness) onAccountStore(t *testing.T, stock float64, priceCents int64) (*store, func(*order.PlaceOrderInput)) {
	t.Helper()
	f := h.newStore(t, stock, priceCents)
	if err := h.d.Payment.Sync(h.ctx); err != nil {
		t.Fatalf("payment sync: %v", err)
	}
	if _, err := h.d.Payment.SetEnabled(h.ctx, f.Channel.ID, "purchase-order", true); err != nil {
		t.Fatalf("enable purchase-order: %v", err)
	}
	po := "purchase-order"
	return f, func(in *order.PlaceOrderInput) { in.PaymentProviderSlug = &po }
}

// newCompanyBuyer registers a customer into a fresh company - orderLimit 0 means no per-member cap
func (h *harness) newCompanyBuyer(t *testing.T, f *store, creditLimit, orderLimit int64) (b2b.Company, uuid.UUID, string) {
	t.Helper()
	email := "b2b-" + uuid.NewString()[:8] + "@e2e.test"
	c, err := h.d.Customer.Register(h.ctx, f.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register buyer: %v", err)
	}
	co, err := h.d.B2B.CreateCompany(h.ctx, f.Channel.ID, "E2E Co "+uuid.NewString()[:8], creditLimit, 30)
	if err != nil {
		t.Fatalf("create company: %v", err)
	}
	if err := h.d.B2B.SetMember(h.ctx, co.ID, c.ID, "buyer", orderLimit); err != nil {
		t.Fatalf("set member: %v", err)
	}
	return co, c.ID, email
}

func (h *harness) creditUsed(t *testing.T, companyID uuid.UUID) int64 {
	t.Helper()
	co, err := h.d.B2B.GetCompany(h.ctx, companyID)
	if err != nil {
		t.Fatalf("get company: %v", err)
	}
	return co.CreditUsedCents
}

func (h *harness) placeOnAccount(t *testing.T, f *store, customerID uuid.UUID, email string, qty float64, onAccount func(*order.PlaceOrderInput)) (order.Order, error) {
	t.Helper()
	return h.tryPlace(t, f.Channel, h.newCustomerCart(t, f, customerID, f.Product.SKU, qty), func(in *order.PlaceOrderInput) {
		onAccount(in)
		in.CustomerID = &customerID
		in.Email = email
	})
}

// A per-member cap is the "this buyer may spend up to X per order" control, and it's checked BEFORE the company's own headroom - a junior buyer with a generous company line still can't place big
func TestB2BMemberOrderLimitRefusesBeforeTheCompanyLine(t *testing.T) {
	h := newHarness(t)
	f, onAccount := h.onAccountStore(t, 20, 2000)
	// a generous company line with a member cap below even one unit, so the refusal can only be the member cap - the limits are checked against the order TOTAL, tax and shipping included, so the bounds here stay clear of both
	co, buyer, email := h.newCompanyBuyer(t, f, 500_000, 1000)

	_, err := h.placeOnAccount(t, f, buyer, email, 1, onAccount)
	if err == nil {
		t.Fatal("a member placed past their per-order limit")
	}
	if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "B2B_MEMBER_ORDER_LIMIT" {
		t.Errorf("error = %v, want code B2B_MEMBER_ORDER_LIMIT rather than the company line's error", err)
	}
	if got := h.creditUsed(t, co.ID); got != 0 {
		t.Errorf("company credit used = %d after the refusal, want 0", got)
	}

	// the same buyer with a roomy cap places fine, so the refusal above was the cap and nothing else
	if err := h.d.B2B.SetMember(h.ctx, co.ID, buyer, "buyer", 100_000); err != nil {
		t.Fatalf("raise member limit: %v", err)
	}
	if _, err := h.placeOnAccount(t, f, buyer, email, 1, onAccount); err != nil {
		t.Fatalf("a member order within the raised cap: %v", err)
	}
	h.settle(t)
	if got := h.creditUsed(t, co.ID); got == 0 {
		t.Error("company credit used = 0 after an accepted on-account order, want the hold recorded")
	}
}

// A zero member limit means "no per-member cap" - reading it as "may spend nothing" would lock every buyer out, since that's the default SetMember writes
func TestB2BZeroMemberLimitMeansNoCap(t *testing.T) {
	h := newHarness(t)
	f, onAccount := h.onAccountStore(t, 20, 2000)
	co, buyer, email := h.newCompanyBuyer(t, f, 500_000, 0)

	if _, err := h.placeOnAccount(t, f, buyer, email, 5, onAccount); err != nil {
		t.Fatalf("a 10000-cent order with no member cap: %v", err)
	}
	h.settle(t)
	if got := h.creditUsed(t, co.ID); got < 10_000 {
		t.Errorf("company credit used = %d, want at least the 10000 of merchandise held", got)
	}
}

// The company line is the hard ceiling - a second order that would breach it must refuse even though each order alone fits, and the refusal must not consume headroom
func TestB2BCreditLimitAccumulatesAcrossOrders(t *testing.T) {
	h := newHarness(t)
	f, onAccount := h.onAccountStore(t, 50, 2000)
	co, buyer, email := h.newCompanyBuyer(t, f, 9000, 0) // room for 4 units, not 5

	if _, err := h.placeOnAccount(t, f, buyer, email, 4, onAccount); err != nil {
		t.Fatalf("first on-account order: %v", err)
	}
	h.settle(t)
	used := h.creditUsed(t, co.ID)
	if used < 8000 {
		t.Fatalf("credit used after the first order = %d, want at least 8000", used)
	}

	_, err := h.placeOnAccount(t, f, buyer, email, 4, onAccount)
	if err == nil {
		t.Fatal("a second order breached the company credit limit")
	}
	if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "B2B_CREDIT_EXCEEDED" {
		t.Errorf("error = %v, want code B2B_CREDIT_EXCEEDED", err)
	}
	if got := h.creditUsed(t, co.ID); got != used {
		t.Errorf("credit used = %d after the refusal, want the unchanged %d", got, used)
	}
	h.settle(t)
}

// Cancelling before anything ships returns the WHOLE hold - the company owes nothing on goods that never left
func TestB2BFullCancelReleasesTheEntireHold(t *testing.T) {
	h := newHarness(t)
	f, onAccount := h.onAccountStore(t, 20, 2000)
	co, buyer, email := h.newCompanyBuyer(t, f, 500_000, 0)

	o, err := h.placeOnAccount(t, f, buyer, email, 3, onAccount)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.settle(t)
	if h.creditUsed(t, co.ID) == 0 {
		t.Fatal("no credit held after placement")
	}

	h.cancel(t, o.ID)
	h.settle(t)
	if got := h.creditUsed(t, co.ID); got != 0 {
		t.Errorf("credit used after a full cancel = %d, want the whole hold released", got)
	}
}

// The release is PRORATED to the unshipped share - the shipped part is still owed, so a company that received half the goods keeps half the hold
func TestB2BPartialShipCancelReleasesOnlyTheUnshippedShare(t *testing.T) {
	h := newHarness(t)
	f, onAccount := h.onAccountStore(t, 20, 2000)
	co, buyer, email := h.newCompanyBuyer(t, f, 500_000, 0)

	o, err := h.placeOnAccount(t, f, buyer, email, 4, onAccount) // 8000 of merchandise
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.settle(t)
	held := h.creditUsed(t, co.ID)
	if held == 0 {
		t.Fatal("no credit held after placement")
	}

	// ship half, then cancel the rest
	lineID := h.orderLines(t, o.ID)[0].ID
	for _, st := range []string{order.OrderStatusConfirmed, order.OrderStatusProcessing} {
		if _, err := h.d.Order.UpdateStatus(h.ctx, o.ID, st); err != nil {
			t.Fatalf("advance to %s: %v", st, err)
		}
	}
	if _, err := h.d.Order.CreateFulfillment(h.ctx, order.CreateFulfillmentInput{
		OrderID: o.ID,
		Lines:   []order.FulfillmentLineInput{{OrderLineID: lineID, Quantity: 2}},
	}); err != nil {
		t.Fatalf("ship half: %v", err)
	}
	h.settle(t)

	h.cancel(t, o.ID)
	h.settle(t)

	after := h.creditUsed(t, co.ID)
	if after == 0 {
		t.Error("the whole hold was released, want the shipped half still owed")
	}
	if after >= held {
		t.Errorf("credit used = %d after cancelling the unshipped half, want it below the original %d", after, held)
	}
	// half the value shipped, so roughly half the hold should remain
	if after*2 < held || after*2 > held*3/2 {
		t.Errorf("credit used = %d of an original %d, want roughly half held back", after, held)
	}
}

// The release is at-least-once through the outbox, so a redelivery must not return the hold twice and hand the company free headroom
func TestB2BCreditReleaseIsIdempotent(t *testing.T) {
	h := newHarness(t)
	f, onAccount := h.onAccountStore(t, 20, 2000)
	co, buyer, email := h.newCompanyBuyer(t, f, 500_000, 0)

	o, err := h.placeOnAccount(t, f, buyer, email, 2, onAccount)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.settle(t)
	h.cancel(t, o.ID)
	h.settle(t)
	if got := h.creditUsed(t, co.ID); got != 0 {
		t.Fatalf("credit used after cancel = %d, want 0", got)
	}

	// re-arm the cancellation so the release subscriber runs again
	h.rearmOutbox(t, "order.cancelled", o.ID.String(), 1)
	h.settle(t)

	if got := h.creditUsed(t, co.ID); got != 0 {
		t.Errorf("credit used after a replayed release = %d, want it still 0 rather than negative headroom", got)
	}
	// and the ledger records the release once, not twice
	entries, err := h.d.B2B.CreditEntries(h.ctx, co.ID, 50)
	if err != nil {
		t.Fatalf("credit entries: %v", err)
	}
	var releases int
	for _, e := range entries {
		if e.AmountCents < 0 {
			releases++
		}
	}
	if releases != 1 {
		t.Errorf("%d release entries in the ledger, want exactly 1", releases)
	}
}

// A manual release is the "we got paid, give the headroom back" button, and it's floored at zero so an over-release can't invent credit
func TestB2BManualReleaseIsFlooredAtZero(t *testing.T) {
	h := newHarness(t)
	f, onAccount := h.onAccountStore(t, 20, 2000)
	co, buyer, email := h.newCompanyBuyer(t, f, 500_000, 0)

	if _, err := h.placeOnAccount(t, f, buyer, email, 2, onAccount); err != nil {
		t.Fatalf("place: %v", err)
	}
	h.settle(t)
	held := h.creditUsed(t, co.ID)
	if held == 0 {
		t.Fatal("no credit held")
	}

	// release far more than is outstanding
	if err := h.d.B2B.ReleaseCredit(h.ctx, co.ID, held*10, nil, "e2e over-release"); err != nil {
		t.Fatalf("ReleaseCredit: %v", err)
	}
	if got := h.creditUsed(t, co.ID); got != 0 {
		t.Errorf("credit used after an over-release = %d, want it floored at 0 rather than negative", got)
	}
	h.settle(t)
}
