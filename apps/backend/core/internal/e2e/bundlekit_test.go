package e2e

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Scenario: a bundle prices as the sum of its components' CHANNEL prices (the 2026-08 billing
// fix), materializes component rows, and reserves each component's stock - never its own
func TestBundlePricesFromComponentsAndReservesThem(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	c1 := h.AddProduct(t, f, 500, true, 10)
	c2 := h.AddProduct(t, f, 250, true, 10)

	b, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
		ProductTypeID: f.Product.ProductTypeID, Locale: "en",
		Name: "E2E Bundle " + uuid.NewString()[:8], SKU: "e2e-bundle-" + uuid.NewString()[:8],
		Kind: catalog.KindBundle, DefaultCurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}
	h.ListOn(t, f.Channel, b.ID, 0)
	opt, err := h.D.Catalog.AddBundleOption(h.Ctx, catalog.AddBundleOptionInput{
		ProductID: b.ID, InputType: catalog.BundleInputCheckbox,
		IsRequired: true, MinSelections: 1, Locale: "en", Title: "Pick parts",
	})
	if err != nil {
		t.Fatalf("bundle option: %v", err)
	}
	s1, err := h.D.Catalog.AddBundleSelection(h.Ctx, catalog.AddBundleSelectionInput{
		OptionID: opt.ID, ProductID: c1.ID, Quantity: 2,
	})
	if err != nil {
		t.Fatalf("selection 1: %v", err)
	}
	s2, err := h.D.Catalog.AddBundleSelection(h.Ctx, catalog.AddBundleSelectionInput{
		OptionID: opt.ID, ProductID: c2.ID, Quantity: 1, SortOrder: 1,
	})
	if err != nil {
		t.Fatalf("selection 2: %v", err)
	}

	// The plain cart door is closed to bundles
	c0, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddToCart(h.Ctx, c0.ID, nil, b.SKU, 1); !errors.Is(err, cart.ErrProductNotPurchasable) {
		t.Errorf("direct bundle add = %v, want ErrProductNotPurchasable", err)
	}

	got, err := h.D.Cart.AddBundleToCart(h.Ctx, c0.ID, nil, cart.AddBundleToCartInput{
		ProductSku: b.SKU, Quantity: 2,
		Selections: []cart.AddBundleSelectionInput{{OptionID: opt.ID, SelectionIDs: []uuid.UUID{s1.ID, s2.ID}}},
	})
	if err != nil {
		t.Fatalf("add bundle: %v", err)
	}
	if len(got.Lines) != 1 || got.Lines[0].UnitPrice != 1250 || got.Lines[0].Quantity != 2 {
		t.Fatalf("bundle line = %+v, want unit 2*500+1*250=1250 at qty 2", got.Lines)
	}
	if len(got.Lines[0].Components) != 2 {
		t.Fatalf("component rows = %d, want 2", len(got.Lines[0].Components))
	}

	o, err := h.TryPlace(t, f.Channel, c0.ID)
	if err != nil {
		t.Fatalf("place bundle: %v", err)
	}
	lines := h.OrderLines(t, o.ID)
	if len(lines) != 1 || lines[0].UnitPrice != 1250 {
		t.Errorf("order lines = %+v, want the one 1250 bundle line", lines)
	}
	var compRows int
	if err := h.D.DB.QueryRowContext(h.Ctx,
		`SELECT count(*) FROM order_line_components WHERE order_line_id = $1`, lines[0].ID).Scan(&compRows); err != nil {
		t.Fatalf("order components: %v", err)
	}
	if compRows != 2 {
		t.Errorf("order_line_components = %d, want 2", compRows)
	}

	// Reservations hit the COMPONENT items: qty-per-bundle x line qty
	wantDeltas := map[uuid.UUID]float64{}
	for pid, want := range map[uuid.UUID]float64{c1.ID: -4, c2.ID: -2} {
		item, err := h.D.Queries.GetInventoryItemByProductID(h.Ctx, pid)
		if err != nil {
			t.Fatalf("component item: %v", err)
		}
		wantDeltas[item.ID] = want
	}
	res, err := h.D.Queries.ListReservationsForOrder(h.Ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil || len(res) != 2 {
		t.Fatalf("reservations = %d (%v), want one per component", len(res), err)
	}
	for _, r := range res {
		if want, ok := wantDeltas[r.InventoryItemID]; !ok || r.QuantityDelta != want {
			t.Errorf("reservation %v delta %v, want %v", r.InventoryItemID, r.QuantityDelta, wantDeltas[r.InventoryItemID])
		}
	}
	h.Settle(t)
}

// Scenario: a kit prices off its OWN listing but reserves its components - and placement fails
// closed when any component's stock can't cover the expansion
func TestKitReservesComponentsAndFailsClosed(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	k1 := h.AddProduct(t, f, 400, true, 10)
	k2 := h.AddProduct(t, f, 100, true, 4)

	price := int64(900)
	k, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
		ProductTypeID: f.Product.ProductTypeID, Locale: "en",
		Name: "E2E Kit " + uuid.NewString()[:8], SKU: "e2e-kit-" + uuid.NewString()[:8],
		Kind: catalog.KindKit, DefaultCurrencyCode: "USD", DefaultPrice: price,
	})
	if err != nil {
		t.Fatalf("create kit: %v", err)
	}
	if _, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{
		ProductID: k.ID, ChannelID: f.Channel.ID, Price: &price, IsPublished: true,
	}); err != nil {
		t.Fatalf("list kit: %v", err)
	}
	for i, comp := range []struct {
		id  uuid.UUID
		qty float64
	}{{k1.ID, 1}, {k2.ID, 2}} {
		if _, err := h.D.Catalog.AddKitComponent(h.Ctx, catalog.AddKitComponentInput{
			ProductID: k.ID, ComponentProductID: comp.id, Quantity: comp.qty, SortOrder: int32(i),
		}); err != nil {
			t.Fatalf("kit component: %v", err)
		}
	}

	o, err := h.TryPlace(t, f.Channel, h.NewGuestCartSKU(t, f, k.SKU, 1))
	if err != nil {
		t.Fatalf("place kit: %v", err)
	}
	lines := h.OrderLines(t, o.ID)
	if len(lines) != 1 || lines[0].UnitPrice != 900 {
		t.Errorf("kit line = %+v, want the kit's own 900 listing price", lines)
	}
	res, err := h.D.Queries.ListReservationsForOrder(h.Ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil || len(res) != 2 {
		t.Errorf("kit reservations = %d (%v), want one per component", len(res), err)
	}
	h.Settle(t)

	// k2 has 2 left; qty 2 needs 4 - the expansion must fail the placement, holding nothing
	if _, err := h.TryPlace(t, f.Channel, h.NewGuestCartSKU(t, f, k.SKU, 2)); err == nil {
		t.Fatal("placed a kit its components can't cover")
	} else if !errors.Is(err, inventory.ErrInsufficientStock) {
		t.Errorf("short-kit error = %v, want ErrInsufficientStock", err)
	}
	item2, err := h.D.Queries.GetInventoryItemByProductID(h.Ctx, k2.ID)
	if err != nil {
		t.Fatalf("k2 item: %v", err)
	}
	levels, err := h.D.Queries.ListInventoryLevelsByItem(h.Ctx, item2.ID)
	if err != nil || len(levels) != 1 {
		t.Fatalf("k2 levels: %v", err)
	}
	if levels[0].ReservedQuantity != 2 {
		t.Errorf("k2 reserved = %v after the failed placement, want the first order's 2 only", levels[0].ReservedQuantity)
	}
	h.Settle(t)
}

// One required checkbox option over the given components, listed on the store at 0 - returns the bundle and its option
func (h *harness) newBundle(t *testing.T, f *store, comps ...catalog.Product) (catalog.Product, catalog.BundleOption) {
	t.Helper()
	b, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
		ProductTypeID: f.Product.ProductTypeID, Locale: "en",
		Name: "E2E Bundle " + uuid.NewString()[:8], SKU: "e2e-bundle-" + uuid.NewString()[:8],
		Kind: catalog.KindBundle, DefaultCurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}
	h.ListOn(t, f.Channel, b.ID, 0)
	opt, err := h.D.Catalog.AddBundleOption(h.Ctx, catalog.AddBundleOptionInput{
		ProductID: b.ID, InputType: catalog.BundleInputCheckbox,
		IsRequired: true, MinSelections: 1, Locale: "en", Title: "Pick parts",
	})
	if err != nil {
		t.Fatalf("bundle option: %v", err)
	}
	for i, c := range comps {
		if _, err := h.D.Catalog.AddBundleSelection(h.Ctx, catalog.AddBundleSelectionInput{
			OptionID: opt.ID, ProductID: c.ID, Quantity: 1, SortOrder: int32(i), IsDefault: true,
		}); err != nil {
			t.Fatalf("selection %d: %v", i, err)
		}
	}
	return b, opt
}

// Scenario: the merchant deletes a bundle while it sits in a cart - checkout must refuse the line, not place an order for a product that no longer exists
func TestADeletedBundleIsRefusedAtCheckout(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	b, _ := h.newBundle(t, f, h.AddProduct(t, f, 500, true, 10))

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddBundleToCart(h.Ctx, c.ID, nil, cart.AddBundleToCartInput{ProductSku: b.SKU, Quantity: 1}); err != nil {
		t.Fatalf("add bundle: %v", err)
	}
	if err := h.D.Catalog.DeleteProduct(h.Ctx, b.ID); err != nil {
		t.Fatalf("delete bundle: %v", err)
	}
	if _, err := h.TryPlace(t, f.Channel, c.ID); !errors.Is(err, cart.ErrProductNotPublished) {
		t.Fatalf("place after delete = %v, want ErrProductNotPublished", err)
	}
	h.Settle(t)
}

// Scenario: the merchant removes a selection from a bundle while a cart holds it - checkout must refuse the line, since the bundle no longer offers what the customer picked
func TestABundleLineWhosePickWasRemovedIsRefusedAtCheckout(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	b, _ := h.newBundle(t, f, h.AddProduct(t, f, 500, true, 10), h.AddProduct(t, f, 250, true, 10))

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	got, err := h.D.Cart.AddBundleToCart(h.Ctx, c.ID, nil, cart.AddBundleToCartInput{ProductSku: b.SKU, Quantity: 1})
	if err != nil || len(got.Lines) != 1 || len(got.Lines[0].Components) != 2 {
		t.Fatalf("add bundle with both defaults: %+v %v", got.Lines, err)
	}
	if err := h.D.Catalog.DeleteBundleSelection(h.Ctx, got.Lines[0].Components[1].BundleSelectionID); err != nil {
		t.Fatalf("remove selection: %v", err)
	}
	if _, err := h.TryPlace(t, f.Channel, c.ID); !errors.Is(err, cart.ErrBundleChanged) {
		t.Fatalf("place after the pick was removed = %v, want ErrBundleChanged", err)
	}
	h.Settle(t)
}

// Scenario: a component's price changes while its bundle sits in a cart - the order must charge today's sum, the same as a simple line would, and its component rows must carry the new price
func TestABundleLineIsRepricedAtCheckout(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	mug := h.AddProduct(t, f, 500, true, 10)
	b, _ := h.newBundle(t, f, mug)

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddBundleToCart(h.Ctx, c.ID, nil, cart.AddBundleToCartInput{ProductSku: b.SKU, Quantity: 2}); err != nil {
		t.Fatalf("add bundle: %v", err)
	}
	price := int64(800)
	if _, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{ProductID: mug.ID, ChannelID: f.Channel.ID, Price: &price, IsPublished: true}); err != nil {
		t.Fatalf("raise component price: %v", err)
	}
	o, err := h.TryPlace(t, f.Channel, c.ID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	lines := h.OrderLines(t, o.ID)
	if len(lines) != 1 || lines[0].UnitPrice != 800 {
		t.Fatalf("order lines = %+v, want the bundle at today's 800, not the added-at 500", lines)
	}
	var compPrice int64
	if err := h.D.DB.QueryRowContext(h.Ctx, `SELECT unit_price FROM order_line_components WHERE order_line_id = $1`, lines[0].ID).Scan(&compPrice); err != nil || compPrice != 800 {
		t.Fatalf("order component price = %d (%v), want 800", compPrice, err)
	}
	h.Settle(t)
}

// Scenario: a bundle whose component is short - the add and a quantity edit must be refused up front, as a simple product's would, rather than at the payment step
func TestABundleIsRefusedWhenAComponentIsShort(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	b, _ := h.newBundle(t, f, h.AddProduct(t, f, 500, true, 2))
	short := func(what string, err error) {
		t.Helper()
		if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "CART_INSUFFICIENT_STOCK" {
			t.Fatalf("%s = %v, want CART_INSUFFICIENT_STOCK", what, err)
		}
	}

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	_, err = h.D.Cart.AddBundleToCart(h.Ctx, c.ID, nil, cart.AddBundleToCartInput{ProductSku: b.SKU, Quantity: 3})
	short("adding 3 bundles over a 2-stock component", err)

	got, err := h.D.Cart.AddBundleToCart(h.Ctx, c.ID, nil, cart.AddBundleToCartInput{ProductSku: b.SKU, Quantity: 2})
	if err != nil {
		t.Fatalf("adding the 2 in stock: %v", err)
	}
	_, err = h.D.Cart.UpdateCartLine(h.Ctx, c.ID, got.Lines[0].ID, nil, 3)
	short("raising the line to 3", err)
	h.Settle(t)
}

// Scenario: one of an option's defaults isn't sold on this store - a customer who leaves the option alone gets the defaults the store does sell, not a refusal over a product they never saw
func TestAHiddenDefaultIsNotPickedForTheCustomer(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	shown := h.AddProduct(t, f, 500, true, 10)
	b, _ := h.newBundle(t, f, shown, h.AddProduct(t, f, 250, false, 10))

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	got, err := h.D.Cart.AddBundleToCart(h.Ctx, c.ID, nil, cart.AddBundleToCartInput{ProductSku: b.SKU, Quantity: 1})
	if err != nil {
		t.Fatalf("add with defaults: %v", err)
	}
	if comps := got.Lines[0].Components; len(comps) != 1 || comps[0].ProductID != shown.ID || got.Lines[0].UnitPrice != 500 {
		t.Fatalf("components = %+v at %d, want only the listed default at 500", comps, got.Lines[0].UnitPrice)
	}
	h.Settle(t)
}

// Scenario: a bundle sold at 10% off its picks on this store - the cart line, the order line and the tax all come from
// the discounted price, while the component rows keep their own prices
func TestATenPercentOffBundleChargesAndTaxesTheDiscountedPrice(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	h.newTaxRule(t, f.Channel.ID, 875, tax.Condition{Type: "core.jurisdiction", Params: json.RawMessage(`{"country_code":"US"}`)})
	b, _ := h.newBundle(t, f, h.AddProduct(t, f, 500, true, 10), h.AddProduct(t, f, 250, true, 10))
	if _, err := h.D.Catalog.SetBundlePricing(h.Ctx, b.ID, f.Channel.ID, catalog.BundlePricingSumMinusPercent, 10); err != nil {
		t.Fatalf("set pricing: %v", err)
	}

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	got, err := h.D.Cart.AddBundleToCart(h.Ctx, c.ID, nil, cart.AddBundleToCartInput{ProductSku: b.SKU, Quantity: 2})
	if err != nil {
		t.Fatalf("add bundle: %v", err)
	}
	if got.Lines[0].UnitPrice != 675 {
		t.Fatalf("cart line = %d, want 750 less 10%% = 675", got.Lines[0].UnitPrice)
	}
	o, err := h.TryPlace(t, f.Channel, c.ID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if lines := h.OrderLines(t, o.ID); len(lines) != 1 || lines[0].UnitPrice != 675 {
		t.Fatalf("order lines = %+v, want the bundle at 675", lines)
	}
	if o.SubtotalAmount != 1350 || o.TaxAmount != 118 { // round(1350 * 875 / 10000)
		t.Errorf("subtotal %d, tax %d - want 2 x 675 = 1350 and 118 tax on it", o.SubtotalAmount, o.TaxAmount)
	}
	if o.TotalAmount != o.SubtotalAmount-o.DiscountAmount+o.TaxAmount+o.ShippingAmount {
		t.Errorf("total %d doesn't add up from %+v", o.TotalAmount, o)
	}
	var compSum int64
	if err := h.D.DB.QueryRowContext(h.Ctx, `SELECT sum(oc.unit_price) FROM order_line_components oc JOIN order_lines ol ON ol.id = oc.order_line_id WHERE ol.order_id = $1`, o.ID).Scan(&compSum); err != nil || compSum != 750 {
		t.Errorf("component prices sum to %d (%v), want their own 500 + 250", compSum, err)
	}
	h.Settle(t)
}
