package e2e

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
)

// poBoxStore is a store whose built-in rates price door delivery at 5 every 5 items, free above 100, with fridges at 30 each that never ship free, and PO boxes at 3 - with one PO box listed and a PO box option switched on
func (h *harness) poBoxStore(t *testing.T) (*store, uuid.UUID) {
	t.Helper()
	f := h.NewStore(t, 10, 2000)
	fridgeType, err := h.D.Catalog.CreateProductType(h.Ctx, catalog.CreateProductTypeInput{
		Slug: "e2e-fridge-" + uuid.NewString()[:8], Name: "Fridge", IsShippingRequired: true, TracksInventory: true,
	})
	if err != nil {
		t.Fatalf("fridge type: %v", err)
	}
	if err := h.D.Queries.SetProductTypeDeliveryKinds(h.Ctx, db.SetProductTypeDeliveryKindsParams{
		ID: fridgeType.ID, DeliveryKinds: []string{delivery.KindShipToAddress, delivery.KindStorePickup},
	}); err != nil {
		t.Fatalf("fridge kinds: %v", err)
	}

	repo := h.D.Shipping.Repo()
	z, err := repo.CreateZone(h.Ctx, shipping.Zone{ChannelID: f.Channel.ID, Code: "e2e-us-" + uuid.NewString()[:8], Name: "US", IsActive: true})
	if err != nil {
		t.Fatalf("zone: %v", err)
	}
	if err := repo.AddCountryToZone(h.Ctx, z.ID, "US"); err != nil {
		t.Fatalf("zone country: %v", err)
	}
	door, err := repo.CreateMethod(h.Ctx, shipping.Method{
		ZoneID: z.ID, Code: "standard", Name: "Standard", CalcType: shipping.CalcFreeAbove, FlatCents: 500, FreeAboveCents: 10000,
		UnitsPerCharge: 5, IsActive: true, CountryMode: "all", DeliveryKind: "ship_to_address",
	})
	if err != nil {
		t.Fatalf("door method: %v", err)
	}
	if err := h.D.Shipping.ReplaceMethodTypeRates(h.Ctx, door.ID, []shipping.TypeRate{{ProductTypeID: fridgeType.ID, PriceCents: 3000, UnitsPerCharge: 1}}); err != nil {
		t.Fatalf("fridge rate: %v", err)
	}
	if _, err := repo.CreateMethod(h.Ctx, shipping.Method{
		ZoneID: z.ID, Code: "pobox", Name: "PO box", CalcType: shipping.CalcFlat, FlatCents: 300,
		IsActive: true, CountryMode: "all", DeliveryKind: "parcel_locker",
	}); err != nil {
		t.Fatalf("PO box method: %v", err)
	}

	if _, err := h.D.Delivery.SavePoints(h.Ctx, f.Channel.ID, []delivery.ManualPoint{{
		Name: "Main post office", Active: true,
		Address: delivery.Address{Street1: "1 Post St", City: "Boxton", PostalCode: "12345", CountryCode: "US"},
	}}); err != nil {
		t.Fatalf("PO box: %v", err)
	}
	if err := h.D.Delivery.AdoptDefaults(h.Ctx, f.Channel.ID); err != nil {
		t.Fatalf("adopt defaults: %v", err)
	}
	if _, err := h.D.Delivery.CreateOption(h.Ctx, f.Channel.ID, delivery.SaveOptionInput{Kind: delivery.KindParcelLocker, Name: "PO box", Enabled: true}); err != nil {
		t.Fatalf("PO box option: %v", err)
	}
	return f, fridgeType.ID
}

// fridgeProduct lists a fridge under the fridge type
func (h *harness) fridgeProduct(t *testing.T, f *store, typeID uuid.UUID) string {
	t.Helper()
	sku := "e2e-fridge-sku-" + uuid.NewString()[:8]
	p, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
		ProductTypeID: typeID, Locale: "en", Name: "Fridge", SKU: sku, DefaultCurrencyCode: "USD", DefaultPrice: 50000,
	})
	if err != nil {
		t.Fatalf("fridge: %v", err)
	}
	price := int64(50000)
	if _, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{ProductID: p.ID, ChannelID: f.Channel.ID, Price: &price, IsPublished: true}); err != nil {
		t.Fatalf("fridge listing: %v", err)
	}
	item, err := h.D.Inventory.EnsureItem(h.Ctx, p.ID, true)
	if err != nil {
		t.Fatalf("fridge item: %v", err)
	}
	if _, err := h.D.Inventory.AdjustLevel(h.Ctx, inventory.AdjustLevelInput{InventoryItemID: item.ID, LocationID: f.Location.ID, NewQuantity: 5}); err != nil {
		t.Fatalf("fridge stock: %v", err)
	}
	return sku
}

func choiceOfKind(t *testing.T, g map[string]any, kind string) (map[string]any, bool) {
	t.Helper()
	for _, c := range g["choices"].([]any) {
		if ch := c.(map[string]any); ch["kind"] == kind {
			return ch, true
		}
	}
	return nil, false
}

// Scenario: a fridge can't go to a PO box, so a cart with a fridge and a shirt is offered only door delivery - priced as the fridge's own 30 plus the shirts' 5, and past the free-shipping threshold the shirts ship free while the fridge still costs 30
func TestCheckoutFridgeIsNeverOfferedAPOBoxAndKeepsItsRate(t *testing.T) {
	h := newHarness(t)
	f, fridgeType := h.poBoxStore(t)
	fridge := h.fridgeProduct(t, f, fridgeType)

	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	h.addSKU(t, ts, f, cartID, fridge)
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))

	groups := h.gqlCartGroups(t, ts, f, cartID)
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want the shirt and the fridge together", len(groups))
	}
	if _, ok := choiceOfKind(t, groups[0], delivery.KindParcelLocker); ok {
		t.Fatalf("a fridge was offered a PO box: %v", groups[0]["choices"])
	}
	door, ok := choiceOfKind(t, groups[0], delivery.KindShipToAddress)
	if !ok {
		t.Fatalf("no door delivery offered: %v", groups[0]["choices"])
	}
	// 2000 shirt + 50000 fridge is past the 10000 threshold - the shirt's 5 is waived, the fridge's 30 is not
	if got := door["priceCents"].(float64); got != 3000 {
		t.Fatalf("door delivery = %v, want only the fridge's 3000", got)
	}
}

// Scenario: a shirt alone is offered a PO box, the only listed one is picked for the shopper, no shipping address is asked, and the order ships to the PO box's address
func TestCheckoutShipsToAListedPOBox(t *testing.T) {
	h := newHarness(t)
	f, _ := h.poBoxStore(t)

	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))
	groups := h.gqlCartGroups(t, ts, f, cartID)
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want one", len(groups))
	}
	box, ok := choiceOfKind(t, groups[0], delivery.KindParcelLocker)
	if !ok {
		t.Fatalf("no PO box offered: %v", groups[0]["choices"])
	}
	if got := box["priceCents"].(float64); got != 300 {
		t.Fatalf("PO box = %v, want 300", got)
	}
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDelivery(cartId: %q, groupKey: %q, optionId: %q, carrierSlug: %q, methodCode: %q)`,
		cartID, groups[0]["key"], box["optionId"], box["carrierSlug"], box["methodCode"]))
	sel := h.gqlCartGroups(t, ts, f, cartID)[0]["selection"].(map[string]any)
	if p, _ := sel["point"].(map[string]any); p == nil || p["city"] != "Boxton" {
		t.Fatalf("selection point = %v, want the only listed PO box picked", sel["point"])
	}

	email := "pobox-" + uuid.NewString()[:8] + "@e2e.test"
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, phone: "+12025550123", billingSameAsShipping: false, billingAddress: %s`, email, usAddress)))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))
	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	orderID := uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string))
	row, err := h.D.Queries.GetOrderByID(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("order: %v", err)
	}
	if row.ShippingAmount != 300 || row.ShippingCity != "Boxton" {
		t.Errorf("order shipping=%d city=%s, want 300 to the PO box's city", row.ShippingAmount, row.ShippingCity)
	}
	h.Settle(t)
}
