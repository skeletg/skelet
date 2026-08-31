package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// Scenario: the on-account lifecycle - guests refused, company-less customers refused, a member's placement holds credit and opens an
// invoice, credit limits refuse, and cancellation releases the hold and voids the invoice exactly once
func TestB2BOnAccountLifecycle(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 2000)
	if err := h.d.Payment.Sync(h.ctx); err != nil {
		t.Fatalf("payment sync: %v", err)
	}
	if _, err := h.d.Payment.SetEnabled(h.ctx, f.Channel.ID, "purchase-order", true); err != nil {
		t.Fatalf("enable purchase-order: %v", err)
	}
	po := "purchase-order"
	onAccount := func(in *order.PlaceOrderInput) { in.PaymentProviderSlug = &po }

	// Guests can't pay on account
	if _, err := h.tryPlace(t, f.Channel, h.newGuestCart(t, f, 1), onAccount); err == nil {
		t.Fatal("guest placed on account")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "B2B_SIGN_IN" {
		t.Errorf("guest PO error = %v, want B2B_SIGN_IN", err)
	}

	// A customer without a company can't either
	lone, err := h.d.Customer.Register(h.ctx, f.Channel.ID, "lone-"+uuid.NewString()[:8]+"@e2e.test", "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register lone: %v", err)
	}
	if _, err := h.tryPlace(t, f.Channel, h.newCustomerCart(t, f, lone.ID, f.Product.SKU, 1),
		func(in *order.PlaceOrderInput) { in.PaymentProviderSlug = &po; in.CustomerID = &lone.ID }); err == nil {
		t.Fatal("company-less customer placed on account")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "B2B_NO_COMPANY" {
		t.Errorf("company-less PO error = %v, want B2B_NO_COMPANY", err)
	}

	// A member within limits places unpaid, holds credit, opens an invoice
	email := "buyer-" + uuid.NewString()[:8] + "@e2e.test"
	buyer, err := h.d.Customer.Register(h.ctx, f.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register buyer: %v", err)
	}
	co, err := h.d.B2B.CreateCompany(h.ctx, f.Channel.ID, "E2E Co "+uuid.NewString()[:8], 500_000, 30)
	if err != nil {
		t.Fatalf("create company: %v", err)
	}
	if err := h.d.B2B.SetMember(h.ctx, co.ID, buyer.ID, "buyer", 0); err != nil {
		t.Fatalf("set member: %v", err)
	}
	o, err := h.tryPlace(t, f.Channel, h.newCustomerCart(t, f, buyer.ID, f.Product.SKU, 1),
		func(in *order.PlaceOrderInput) {
			in.PaymentProviderSlug = &po
			in.CustomerID = &buyer.ID
			in.Email = email
			in.PONumber = "PO-12345"
		})
	if err != nil {
		t.Fatalf("on-account placement: %v", err)
	}
	if o.PaymentStatus != "unpaid" || o.PONumber != "PO-12345" {
		t.Errorf("status/po = %q/%q, want unpaid/PO-12345", o.PaymentStatus, o.PONumber)
	}
	held, err := h.d.B2B.GetCompany(h.ctx, co.ID)
	if err != nil {
		t.Fatalf("company: %v", err)
	}
	if held.CreditUsedCents != o.TotalAmount {
		t.Errorf("credit used = %d, want the order's %d", held.CreditUsedCents, o.TotalAmount)
	}
	invoices, err := h.d.B2B.InvoicesForCompany(h.ctx, co.ID, 10)
	if err != nil || len(invoices) != 1 || invoices[0].Status != "open" {
		t.Errorf("invoices = %+v (%v), want one open", invoices, err)
	}
	h.settle(t)

	// A tight limit refuses with the credit code
	tight, err := h.d.B2B.CreateCompany(h.ctx, f.Channel.ID, "E2E Tight "+uuid.NewString()[:8], 100, 30)
	if err != nil {
		t.Fatalf("tight company: %v", err)
	}
	broke, err := h.d.Customer.Register(h.ctx, f.Channel.ID, "broke-"+uuid.NewString()[:8]+"@e2e.test", "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register broke: %v", err)
	}
	if err := h.d.B2B.SetMember(h.ctx, tight.ID, broke.ID, "buyer", 0); err != nil {
		t.Fatalf("tight member: %v", err)
	}
	if _, err := h.tryPlace(t, f.Channel, h.newCustomerCart(t, f, broke.ID, f.Product.SKU, 1),
		func(in *order.PlaceOrderInput) { in.PaymentProviderSlug = &po; in.CustomerID = &broke.ID }); err == nil {
		t.Fatal("placement exceeded the credit limit")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "B2B_CREDIT_EXCEEDED" {
		t.Errorf("over-limit error = %v, want B2B_CREDIT_EXCEEDED", err)
	}

	// Cancellation releases the hold and voids the invoice - exactly once under replay
	h.cancel(t, o.ID)
	h.settle(t)
	released, err := h.d.B2B.GetCompany(h.ctx, co.ID)
	if err != nil {
		t.Fatalf("company after cancel: %v", err)
	}
	if released.CreditUsedCents != 0 {
		t.Errorf("credit used after cancel = %d, want 0", released.CreditUsedCents)
	}
	entries, err := h.d.Queries.B2BCreditEntriesForOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil || len(entries) != 2 {
		t.Fatalf("credit entries = %d (%v), want the +hold and the -release", len(entries), err)
	}
	voided, err := h.d.B2B.InvoicesForCompany(h.ctx, co.ID, 10)
	if err != nil || len(voided) != 1 || voided[0].Status != "void" {
		t.Errorf("invoice after cancel = %+v (%v), want void", voided, err)
	}

	h.rearmOutbox(t, "order.cancelled", o.ID.String(), 1)
	h.settle(t)
	again, _ := h.d.B2B.GetCompany(h.ctx, co.ID)
	entriesAgain, _ := h.d.Queries.B2BCreditEntriesForOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if again.CreditUsedCents != 0 || len(entriesAgain) != 2 {
		t.Errorf("replayed cancel changed credit: used %d, %d entries - want 0/2", again.CreditUsedCents, len(entriesAgain))
	}
}
