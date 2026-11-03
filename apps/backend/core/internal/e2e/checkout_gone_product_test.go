package e2e

import (
	"errors"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

// Scenario: a product leaves the store - deleted, or its listing unpublished - while it sits in a cart. Checkout must refuse the line rather than sell what the store no longer offers
func TestASimpleProductThatLeftTheStoreIsRefusedAtCheckout(t *testing.T) {
	for _, tc := range []struct {
		name  string
		leave func(h *harness, f *store, p catalog.Product) error
	}{
		{"deleted", func(h *harness, _ *store, p catalog.Product) error { return h.D.Catalog.DeleteProduct(h.Ctx, p.ID) }},
		{"unpublished", func(h *harness, f *store, p catalog.Product) error {
			price := int64(500)
			_, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{ProductID: p.ID, ChannelID: f.Channel.ID, Price: &price, IsPublished: false})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			f := h.NewStore(t, 10, 1000)
			p := h.AddProduct(t, f, 500, true, 10)
			cartID := h.NewGuestCartSKU(t, f, p.SKU, 1)
			if err := tc.leave(h, f, p); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if _, err := h.TryPlace(t, f.Channel, cartID); !errors.Is(err, cart.ErrProductNotPublished) {
				t.Fatalf("place after %s = %v, want ErrProductNotPublished", tc.name, err)
			}
			h.Settle(t)
		})
	}
}
