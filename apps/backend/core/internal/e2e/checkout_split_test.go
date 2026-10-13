package e2e

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
)

// splitProduct lists a second product on the store under its own type - digital, or limited to some delivery kinds
func (h *harness) splitProduct(t *testing.T, f *store, digital bool, kinds []string, price int64) string {
	t.Helper()
	suffix := uuid.NewString()[:8]
	pt, err := h.D.Catalog.CreateProductType(h.Ctx, catalog.CreateProductTypeInput{
		Slug: "e2e-split-" + suffix, Name: "Split " + suffix, IsDigital: digital, IsShippingRequired: !digital, TracksInventory: !digital,
	})
	if err != nil {
		t.Fatalf("product type: %v", err)
	}
	if kinds != nil {
		if err := h.D.Queries.SetProductTypeDeliveryKinds(h.Ctx, db.SetProductTypeDeliveryKindsParams{ID: pt.ID, DeliveryKinds: kinds}); err != nil {
			t.Fatalf("delivery kinds: %v", err)
		}
	}
	sku := "e2e-split-sku-" + suffix
	p, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
		ProductTypeID: pt.ID, Locale: "en", Name: "Split " + suffix, SKU: sku, DefaultCurrencyCode: "USD", DefaultPrice: price,
	})
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	if _, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{ProductID: p.ID, ChannelID: f.Channel.ID, Price: &price, IsPublished: true}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if !digital {
		item, err := h.D.Inventory.EnsureItem(h.Ctx, p.ID, true)
		if err != nil {
			t.Fatalf("item: %v", err)
		}
		if _, err := h.D.Inventory.AdjustLevel(h.Ctx, inventory.AdjustLevelInput{InventoryItemID: item.ID, LocationID: f.Location.ID, NewQuantity: 10}); err != nil {
			t.Fatalf("stock: %v", err)
		}
	}
	return sku
}

func (h *harness) addSKU(t *testing.T, ts *httptest.Server, f *store, cartID, sku string) {
	t.Helper()
	if _, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`mutation { addToCart(cartId: %q, sku: %q, quantity: 1) { id } }`, cartID, sku)); len(errs) > 0 {
		t.Fatalf("addToCart %s: %v", sku, errs)
	}
}

// Scenario: one cart, three ways out - an address-only product by courier, a pickup-only product from the store, and an e-book by email. The checkout offers each group only its own kinds, and the order keeps one package per group, shipping summed
func TestCheckoutSplitsDeliveryByProductType(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	if err := h.D.Queries.SetProductTypeDeliveryKinds(h.Ctx, db.SetProductTypeDeliveryKindsParams{ID: f.Product.ProductTypeID, DeliveryKinds: []string{"ship_to_address"}}); err != nil {
		t.Fatalf("delivery kinds: %v", err)
	}
	line1, city, postal, country, active := "9 Depot Rd", "Storeton", "54321", "US", true
	if _, err := h.D.Inventory.UpdateLocation(h.Ctx, inventory.UpdateLocationInput{ID: f.Location.ID, IsActive: &active, Line1: &line1, City: &city, PostalCode: &postal, CountryCode: &country}); err != nil {
		t.Fatalf("location: %v", err)
	}
	if _, err := h.D.Inventory.SetLocationPickup(h.Ctx, f.Location.ID, true, ""); err != nil {
		t.Fatalf("pickup: %v", err)
	}
	if err := h.D.Shipping.SyncCarriers(h.Ctx, []uuid.UUID{f.Channel.ID}); err != nil {
		t.Fatalf("sync carriers: %v", err)
	}
	if _, err := h.D.Shipping.SetCarrierEnabled(h.Ctx, f.Channel.ID, "pickup", true); err != nil {
		t.Fatalf("pickup carrier: %v", err)
	}
	pickupOnly := h.splitProduct(t, f, false, []string{"store_pickup"}, 3000)
	ebook := h.splitProduct(t, f, true, nil, 900)

	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	h.addSKU(t, ts, f, cartID, pickupOnly)
	h.addSKU(t, ts, f, cartID, ebook)
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))

	groups := h.gqlCartGroups(t, ts, f, cartID)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want the courier lines and the pickup lines apart - they share no kind", len(groups))
	}
	var ship, pick map[string]any
	for _, g := range groups {
		kinds := g["kinds"].([]any)
		if len(kinds) != 1 {
			t.Fatalf("group %v offers kinds %v, want exactly its products'", g["key"], kinds)
		}
		if kinds[0] == "ship_to_address" {
			ship = g
		} else {
			pick = g
		}
	}
	choose := func(g map[string]any, kind string) {
		t.Helper()
		for _, c := range g["choices"].([]any) {
			ch := c.(map[string]any)
			if ch["kind"] == kind {
				h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDelivery(cartId: %q, groupKey: %q, optionId: %q, carrierSlug: %q, methodCode: %q)`,
					cartID, g["key"], ch["optionId"], ch["carrierSlug"], ch["methodCode"]))
				return
			}
		}
		t.Fatalf("group %v has no %s choice: %v", g["key"], kind, g["choices"])
	}
	choose(ship, "ship_to_address")
	choose(pick, "store_pickup")

	email := "split-" + uuid.NewString()[:8] + "@e2e.test"
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true, pickupContact: {firstName: "Ana", lastName: "Collector", someoneElse: false}`, email, usAddress)))
	co := h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))
	if got := co["totals"].(map[string]any)["shippingCents"].(float64); got != 500 {
		t.Fatalf("shipping = %v, want the courier's 500 plus a free pickup", got)
	}

	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`mutation { placeOrder(cartId: %q) { id } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	orderID := uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string))
	row, err := h.D.Queries.GetOrderByID(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("order: %v", err)
	}
	if row.FulfillmentNeed != "mixed" || row.ShippingAmount != 500 || row.ShippingCity != "Testville" {
		t.Errorf("order need=%s shipping=%d city=%s, want mixed, 500, the shopper's address", row.FulfillmentNeed, row.ShippingAmount, row.ShippingCity)
	}
	ships, err := h.D.Queries.ListOrderShipments(h.Ctx, orderID)
	if err != nil || len(ships) != 2 {
		t.Fatalf("shipments = %d, %v - want one per group", len(ships), err)
	}
	for _, s := range ships {
		if s.DeliveryKind == "store_pickup" && (!s.Point.Valid || !s.ShipTo.Valid) {
			t.Errorf("the pickup package has no point or ship-to: %+v", s)
		}
	}
	h.Settle(t)
}

// Scenario: an e-book alone asks for no delivery and no shipping address - billing and payment are all the order owes
func TestCheckoutDigitalOnlyAsksBillingAndPayment(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	ebook := h.splitProduct(t, f, true, nil, 900)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), `mutation { createGuestCart { id } }`)
	if len(errs) > 0 {
		t.Fatalf("cart: %v", errs)
	}
	cartID := gqlField(t, data, "createGuestCart", "id")
	h.addSKU(t, ts, f, cartID, ebook)

	if groups := h.gqlCartGroups(t, ts, f, cartID); len(groups) != 0 {
		t.Fatalf("delivery groups = %v, want none for a digital cart", groups)
	}
	if msg := h.gqlCheckoutRefused(t, ts, f, details(cartID, `email: "nobill@e2e.test", billingSameAsShipping: false`)); msg == "" {
		t.Fatal("a digital order without billing must be refused")
	}
	email := "ebook-" + uuid.NewString()[:8] + "@e2e.test"
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, billingSameAsShipping: false, billingAddress: %s`, email, usAddress)))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))
	data, errs = h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`mutation { placeOrder(cartId: %q) { id total } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	row, err := h.D.Queries.GetOrderByID(h.Ctx, uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string)))
	if err != nil {
		t.Fatalf("order: %v", err)
	}
	if row.FulfillmentNeed != "none" || row.ShippingAmount != 0 || strings.TrimSpace(row.ShippingCountry) != "" || row.BillingCity != "Testville" {
		t.Errorf("order need=%s shipping=%d ship-to country=%q billing city=%q, want nothing shipped and the billing kept",
			row.FulfillmentNeed, row.ShippingAmount, row.ShippingCountry, row.BillingCity)
	}
	h.Settle(t)
}
