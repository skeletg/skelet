package e2e

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Scenario: an order keeps the seller lines it was placed under, so moving offices never rewrites an old invoice
func TestAnOrderKeepsTheSellerItWasPlacedUnder(t *testing.T) {
	h := newHarness(t)
	s := h.NewStore(t, 10, 2000)
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &s.Channel.ID}
	set := func(path, v string) {
		t.Helper()
		if _, err := h.D.Config.Set(h.Ctx, path, v, scope); err != nil {
			t.Fatal(err)
		}
	}
	set("sales/documents/seller_details", "Apparel GmbH\nOld Street 1, 10115 Berlin")
	set("sales/documents/invoice_footer", "Pay to the old account")
	placed := h.PlaceGuestCOD(t, s, 1)
	set("sales/documents/seller_details", "Apparel GmbH\nNew Street 2, 10115 Berlin")
	set("sales/documents/invoice_footer", "Pay to the new account")

	o, err := h.D.Order.GetByID(h.Ctx, placed.ID)
	if err != nil || o.Seller == nil || len(o.Seller.Lines) != 2 || o.Seller.Lines[1] != "Old Street 1, 10115 Berlin" {
		t.Fatalf("seller kept on the order: %+v, %v", o.Seller, err)
	}
	in, err := h.D.Documents.Input(h.Ctx, "invoice", o, nil)
	if err != nil || in.Store.Lines[1] != "Old Street 1, 10115 Berlin" || in.Footer != "Pay to the old account" {
		t.Errorf("the invoice prints %v under %q, %v", in.Store.Lines, in.Footer, err)
	}
}
