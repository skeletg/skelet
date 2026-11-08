package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

// Scenario: an order keeps the storefront language it was placed in, so its emails go out in it, and a draft an admin
// builds for a customer takes the language they last signed in with - the dashboard's own says nothing about them
func TestOrdersKeepTheShoppersLanguage(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	background := h.Ctx
	h.Ctx = locale.WithLocale(background, "de")
	placed := h.PlaceGuestCOD(t, f, 1)
	h.Ctx = background
	if placed.Locale != "de" {
		t.Errorf("an order placed in de stored %q", placed.Locale)
	}

	cust, err := h.D.Customer.Register(locale.WithLocale(h.Ctx, "fr"), f.Channel.ID, "lang-"+uuid.NewString()[:8]+"@e2e.test", "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	h.Ctx = locale.WithLocale(background, "en") // the admin's dashboard language
	d := h.newDraft(t, f, []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 1}}, func(in *order.CreateDraftOrderInput) {
		in.CustomerID, in.Email = &cust.ID, cust.Email
	})
	h.Ctx = background
	if d.Locale != "fr" {
		t.Errorf("a draft for a customer who signed in in fr stored %q", d.Locale)
	}
	h.Settle(t)
}
