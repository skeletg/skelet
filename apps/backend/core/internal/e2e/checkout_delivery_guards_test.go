package e2e

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

// chooseKind picks a group's first choice of a kind
func (h *harness) chooseKind(t *testing.T, ts *httptest.Server, f *store, cartID string, g map[string]any, kind string) map[string]any {
	t.Helper()
	ch, ok := choiceOfKind(t, g, kind)
	if !ok {
		t.Fatalf("group %v has no %s choice: %v", g["key"], kind, g["choices"])
	}
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDelivery(cartId: %q, groupKey: %q, optionId: %q, carrierSlug: %q, methodCode: %q)`,
		cartID, g["key"], ch["optionId"], ch["carrierSlug"], ch["methodCode"]))
	return ch
}

func (h *harness) placeRefused(t *testing.T, ts *httptest.Server, f *store, cartID, want string) {
	t.Helper()
	_, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID))
	if msg := strings.Join(errs, "; "); !strings.Contains(msg, want) {
		t.Fatalf("placeOrder errors = %q, want a refusal saying %q", msg, want)
	}
}

func (h *harness) place(t *testing.T, ts *httptest.Server, f *store, cartID string) uuid.UUID {
	t.Helper()
	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	return uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string))
}

// Scenario: a pickup-only product with nothing to collect it from sits beside a shirt going by courier - the order is refused rather than placed with the pickup lines unpriced and no package to send them in
func TestCheckoutRefusesAGroupNothingQuotes(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	if err := h.D.Queries.SetProductTypeDeliveryKinds(h.Ctx, db.SetProductTypeDeliveryKindsParams{ID: f.Product.ProductTypeID, DeliveryKinds: []string{delivery.KindShipToAddress}}); err != nil {
		t.Fatalf("delivery kinds: %v", err)
	}
	pickupOnly := h.splitProduct(t, f, false, []string{delivery.KindStorePickup}, 3000)

	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	h.addSKU(t, ts, f, cartID, pickupOnly)
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))
	groups := h.gqlCartGroups(t, ts, f, cartID)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want the courier lines and the pickup lines apart", len(groups))
	}
	for _, g := range groups {
		if len(g["choices"].([]any)) > 0 {
			h.chooseKind(t, ts, f, cartID, g, delivery.KindShipToAddress)
		}
	}
	email := "unquoted-" + uuid.NewString()[:8] + "@e2e.test"
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))
	h.placeRefused(t, ts, f, cartID, "can't be delivered here")
	h.Settle(t)
}

// Scenario: one delivery group of 60 split across two warehouses, 30 each, under "free above 50" - both packages ship free, at checkout and on the order. Moving the order's address keeps that shipping and moves the packages with it
func TestSplitPackagesShareTheGroupsFreeShipping(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 3000)
	h.addFlatShipping(t, f, 500, 5000)
	second, err := h.D.Inventory.CreateLocation(h.Ctx, inventory.CreateLocationInput{Slug: "e2e-loc2-" + uuid.NewString()[:8], Name: "Second warehouse"})
	if err != nil {
		t.Fatalf("location: %v", err)
	}
	if err := h.D.Inventory.SetChannelLocations(h.Ctx, f.Channel.ID, []inventory.LocationAssignment{{LocationID: f.Location.ID, Priority: 0}, {LocationID: second.ID, Priority: 1}}); err != nil {
		t.Fatalf("assign locations: %v", err)
	}
	other := h.splitProduct(t, f, false, nil, 3000)
	p, err := h.D.Catalog.GetProductBySKU(h.Ctx, other)
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	item, err := h.D.Inventory.EnsureItem(h.Ctx, p.ID, true)
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	// Only the second warehouse holds it, so it leaves from there
	for loc, qty := range map[uuid.UUID]float64{f.Location.ID: 0, second.ID: 10} {
		if _, err := h.D.Inventory.AdjustLevel(h.Ctx, inventory.AdjustLevelInput{InventoryItemID: item.ID, LocationID: loc, NewQuantity: qty}); err != nil {
			t.Fatalf("stock: %v", err)
		}
	}

	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	h.addSKU(t, ts, f, cartID, other)
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))
	groups := h.gqlCartGroups(t, ts, f, cartID)
	if len(groups) != 1 || len(groups[0]["shipments"].([]any)) != 2 {
		t.Fatalf("groups = %v, want one group leaving from two warehouses", groups)
	}
	if ch := h.chooseKind(t, ts, f, cartID, groups[0], delivery.KindShipToAddress); ch["priceCents"].(float64) != 0 {
		t.Fatalf("delivery = %v, want free - the group is worth 60, past the 50 threshold", ch["priceCents"])
	}
	email := "split-free-" + uuid.NewString()[:8] + "@e2e.test"
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))
	orderID := h.place(t, ts, f, cartID)
	row, err := h.D.Queries.GetOrderByID(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("order: %v", err)
	}
	if row.ShippingAmount != 0 {
		t.Errorf("order shipping = %d, want 0", row.ShippingAmount)
	}

	h.Settle(t)
	if _, err := h.D.Order.EditOrderAddress(h.Ctx, order.EditOrderAddressInput{OrderID: orderID, Shipping: editAddr("Grace", "Hopper", "Arlington", "22202")}); err != nil {
		t.Fatalf("EditOrderAddress: %v", err)
	}
	ships, err := h.D.Order.ListShipments(h.Ctx, orderID)
	if err != nil || len(ships) != 2 {
		t.Fatalf("shipments = %d, %v - want two", len(ships), err)
	}
	var total int64
	for _, s := range ships {
		total += s.AmountCents
		if s.ShipTo == nil || s.ShipTo.City != "Arlington" {
			t.Errorf("package ship-to = %+v, want the new address", s.ShipTo)
		}
	}
	edited, err := h.D.Queries.GetOrderByID(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("order: %v", err)
	}
	if edited.ShippingAmount != 0 || total != edited.ShippingAmount {
		t.Errorf("after the move: order shipping %d, packages %d - want both held at 0", edited.ShippingAmount, total)
	}

	// Dropping the second warehouse's line leaves 30, under the threshold - re-quoting with the first package's method would charge 5, so shipping holds, and the line leaves its package
	h.Settle(t)
	dropped := h.lineBySKU(t, orderID, other)
	if _, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{OrderID: orderID, RemoveLine: []uuid.UUID{dropped}}); err != nil {
		t.Fatalf("EditOrder: %v", err)
	}
	if after, err := h.D.Queries.GetOrderByID(h.Ctx, orderID); err != nil || after.ShippingAmount != 0 {
		t.Fatalf("after the line edit: shipping %d, %v - want it held at 0", after.ShippingAmount, err)
	}
	ships, err = h.D.Order.ListShipments(h.Ctx, orderID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range ships {
		for _, l := range s.Lines {
			if l.CartLineID == dropped {
				t.Errorf("package %d still lists the removed line", s.Position)
			}
		}
	}
	h.Settle(t)
}

// Scenario: the PO box the shopper picked closes before they place the order - placement reads it again and refuses
func TestPlacementRefusesAPointThatClosed(t *testing.T) {
	h := newHarness(t)
	f, _ := h.poBoxStore(t)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))
	h.chooseKind(t, ts, f, cartID, h.gqlCartGroups(t, ts, f, cartID)[0], delivery.KindParcelLocker)
	email := "closed-" + uuid.NewString()[:8] + "@e2e.test"
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, phone: "+12025550123", billingSameAsShipping: false, billingAddress: %s`, email, usAddress)))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))

	points, err := h.D.Delivery.ListPoints(h.Ctx, f.Channel.ID)
	if err != nil || len(points) != 1 {
		t.Fatalf("points = %v, %v", points, err)
	}
	points[0].Active = false
	if _, err := h.D.Delivery.SavePoints(h.Ctx, f.Channel.ID, points); err != nil {
		t.Fatalf("close the PO box: %v", err)
	}
	h.placeRefused(t, ts, f, cartID, "pickup point isn't available")
	h.Settle(t)
}

// Scenario: a store pickup option with no pickup-enabled location has nowhere to collect from, so it doesn't read as ready until one is
func TestStorePickupIsReadyOnlyWithAPickupLocation(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	o, err := h.D.Delivery.CreateOption(h.Ctx, f.Channel.ID, delivery.SaveOptionInput{Kind: delivery.KindStorePickup, Name: "Pickup"})
	if err != nil {
		t.Fatalf("option: %v", err)
	}
	points := func() delivery.Check {
		for _, c := range h.D.Delivery.Readiness(h.Ctx, f.Channel.ID, o) {
			if c.Code == "points" {
				return c
			}
		}
		t.Fatal("no points check for a store pickup")
		return delivery.Check{}
	}
	if c := points(); c.OK {
		t.Fatalf("points check = %+v, want it failing with no pickup-enabled location", c)
	}
	active := true
	if _, err := h.D.Inventory.UpdateLocation(h.Ctx, inventory.UpdateLocationInput{ID: f.Location.ID, IsActive: &active}); err != nil {
		t.Fatalf("location: %v", err)
	}
	h.EnablePickup(t, f.Location.ID, "")
	if c := points(); !c.OK {
		t.Fatalf("points check = %+v, want it passing once a location offers pickup", c)
	}
}

// Shipping supports this product in principle, but the only quoted choice is pickup.
func TestSingleGroupWithOnlyPickupQuoted(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0) // the courier serves US, while this shopper asks from Canada
	if err := h.D.Shipping.SyncCarriers(h.Ctx, []uuid.UUID{f.Channel.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.Shipping.SetCarrierEnabled(h.Ctx, f.Channel.ID, "pickup", true); err != nil {
		t.Fatal(err)
	}
	active := true
	line, city, postal, country := "9 Depot Rd", "Storeton", "54321", "US"
	if _, err := h.D.Inventory.UpdateLocation(h.Ctx, inventory.UpdateLocationInput{ID: f.Location.ID, IsActive: &active, Line1: &line, City: &city, PostalCode: &postal, CountryCode: &country}); err != nil {
		t.Fatal(err)
	}
	h.EnablePickup(t, f.Location.ID, "")
	if _, err := h.D.Delivery.CreateOption(h.Ctx, f.Channel.ID, delivery.SaveOptionInput{Kind: delivery.KindStorePickup, Name: "Collect", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.Delivery.CreateOption(h.Ctx, f.Channel.ID, delivery.SaveOptionInput{Kind: delivery.KindShipToAddress, Name: "Unquoted shipping", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "CA", postalCode: "M5V 1A1")`, cartID))
	groups := h.gqlCartGroups(t, ts, f, cartID)
	if len(groups) != 1 {
		t.Fatalf("groups = %v, want one", groups)
	}
	choices := groups[0]["choices"].([]any)
	if len(choices) == 0 {
		t.Fatal("no pickup quote")
	}
	for _, raw := range choices {
		if raw.(map[string]any)["kind"] != "store_pickup" {
			t.Fatalf("unexpected quote: %v", raw)
		}
	}
	h.chooseKind(t, ts, f, cartID, groups[0], delivery.KindStorePickup)
	email := "only-pickup-" + uuid.NewString() + "@e2e.test"
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, billingSameAsShipping: false, pickupContact: {firstName: "E2E", lastName: "Collector", someoneElse: false}`, email)))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))
	id := h.place(t, ts, f, cartID)
	ships, err := h.D.Order.ListShipments(h.Ctx, id)
	if err != nil || len(ships) != 1 || ships[0].DeliveryKind != delivery.KindStorePickup {
		t.Fatalf("shipments=%+v, %v", ships, err)
	}
	h.Settle(t)
}
