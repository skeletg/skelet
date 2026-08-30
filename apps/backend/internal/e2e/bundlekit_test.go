package e2e

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/cart"
	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/inventory"
)

// Scenario: a bundle prices as the sum of its components' CHANNEL prices (the 2026-08 billing
// fix), materializes component rows, and reserves each component's stock - never its own
func TestBundlePricesFromComponentsAndReservesThem(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 1000)
	c1 := h.addProduct(t, f, 500, true, 10)
	c2 := h.addProduct(t, f, 250, true, 10)

	b, err := h.d.Catalog.CreateProduct(h.ctx, catalog.CreateProductInput{
		ProductTypeID: f.Product.ProductTypeID, Locale: "en",
		Name: "E2E Bundle " + uuid.NewString()[:8], SKU: "e2e-bundle-" + uuid.NewString()[:8],
		Kind: catalog.KindBundle, DefaultCurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}
	opt, err := h.d.Catalog.AddBundleOption(h.ctx, catalog.AddBundleOptionInput{
		ProductID: b.ID, InputType: catalog.BundleInputCheckbox,
		IsRequired: true, MinSelections: 1, Locale: "en", Title: "Pick parts",
	})
	if err != nil {
		t.Fatalf("bundle option: %v", err)
	}
	s1, err := h.d.Catalog.AddBundleSelection(h.ctx, catalog.AddBundleSelectionInput{
		OptionID: opt.ID, ProductID: c1.ID, Quantity: 2,
	})
	if err != nil {
		t.Fatalf("selection 1: %v", err)
	}
	s2, err := h.d.Catalog.AddBundleSelection(h.ctx, catalog.AddBundleSelectionInput{
		OptionID: opt.ID, ProductID: c2.ID, Quantity: 1, SortOrder: 1,
	})
	if err != nil {
		t.Fatalf("selection 2: %v", err)
	}

	// The plain cart door is closed to bundles
	c0, err := h.d.Cart.CreateGuestCart(h.ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.d.Cart.AddToCart(h.ctx, c0.ID, nil, cart.AddCartLineInput{ProductSku: b.SKU, Quantity: 1}); !errors.Is(err, cart.ErrProductNotPurchasable) {
		t.Errorf("direct bundle add = %v, want ErrProductNotPurchasable", err)
	}

	got, err := h.d.Cart.AddBundleToCart(h.ctx, c0.ID, nil, cart.AddBundleToCartInput{
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

	o, err := h.tryPlace(t, f.Channel, c0.ID)
	if err != nil {
		t.Fatalf("place bundle: %v", err)
	}
	lines := h.orderLines(t, o.ID)
	if len(lines) != 1 || lines[0].UnitPrice != 1250 {
		t.Errorf("order lines = %+v, want the one 1250 bundle line", lines)
	}
	var compRows int
	if err := h.d.DB.QueryRowContext(h.ctx,
		`SELECT count(*) FROM order_line_components WHERE order_line_id = $1`, lines[0].ID).Scan(&compRows); err != nil {
		t.Fatalf("order components: %v", err)
	}
	if compRows != 2 {
		t.Errorf("order_line_components = %d, want 2", compRows)
	}

	// Reservations hit the COMPONENT items: qty-per-bundle x line qty
	wantDeltas := map[uuid.UUID]float64{}
	for pid, want := range map[uuid.UUID]float64{c1.ID: -4, c2.ID: -2} {
		item, err := h.d.Queries.GetInventoryItemByProductID(h.ctx, pid)
		if err != nil {
			t.Fatalf("component item: %v", err)
		}
		wantDeltas[item.ID] = want
	}
	res, err := h.d.Queries.ListReservationsForOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil || len(res) != 2 {
		t.Fatalf("reservations = %d (%v), want one per component", len(res), err)
	}
	for _, r := range res {
		if want, ok := wantDeltas[r.InventoryItemID]; !ok || r.QuantityDelta != want {
			t.Errorf("reservation %v delta %v, want %v", r.InventoryItemID, r.QuantityDelta, wantDeltas[r.InventoryItemID])
		}
	}
	h.settle(t)
}

// Scenario: a kit prices off its OWN listing but reserves its components - and placement fails
// closed when any component's stock can't cover the expansion
func TestKitReservesComponentsAndFailsClosed(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 1000)
	k1 := h.addProduct(t, f, 400, true, 10)
	k2 := h.addProduct(t, f, 100, true, 4)

	price := int64(900)
	k, err := h.d.Catalog.CreateProduct(h.ctx, catalog.CreateProductInput{
		ProductTypeID: f.Product.ProductTypeID, Locale: "en",
		Name: "E2E Kit " + uuid.NewString()[:8], SKU: "e2e-kit-" + uuid.NewString()[:8],
		Kind: catalog.KindKit, DefaultCurrencyCode: "USD", DefaultPrice: price,
	})
	if err != nil {
		t.Fatalf("create kit: %v", err)
	}
	if _, err := h.d.Catalog.SetProductListing(h.ctx, catalog.ProductChannelListingInput{
		ProductID: k.ID, ChannelID: f.Channel.ID, Price: &price, IsPublished: true,
	}); err != nil {
		t.Fatalf("list kit: %v", err)
	}
	for i, comp := range []struct {
		id  uuid.UUID
		qty float64
	}{{k1.ID, 1}, {k2.ID, 2}} {
		if _, err := h.d.Catalog.AddKitComponent(h.ctx, catalog.AddKitComponentInput{
			ProductID: k.ID, ComponentProductID: comp.id, Quantity: comp.qty, SortOrder: int32(i),
		}); err != nil {
			t.Fatalf("kit component: %v", err)
		}
	}

	o, err := h.tryPlace(t, f.Channel, h.newGuestCartSKU(t, f, k.SKU, 1))
	if err != nil {
		t.Fatalf("place kit: %v", err)
	}
	lines := h.orderLines(t, o.ID)
	if len(lines) != 1 || lines[0].UnitPrice != 900 {
		t.Errorf("kit line = %+v, want the kit's own 900 listing price", lines)
	}
	res, err := h.d.Queries.ListReservationsForOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil || len(res) != 2 {
		t.Errorf("kit reservations = %d (%v), want one per component", len(res), err)
	}
	h.settle(t)

	// k2 has 2 left; qty 2 needs 4 - the expansion must fail the placement, holding nothing
	if _, err := h.tryPlace(t, f.Channel, h.newGuestCartSKU(t, f, k.SKU, 2)); err == nil {
		t.Fatal("placed a kit its components can't cover")
	} else if !errors.Is(err, inventory.ErrInsufficientStock) {
		t.Errorf("short-kit error = %v, want ErrInsufficientStock", err)
	}
	item2, err := h.d.Queries.GetInventoryItemByProductID(h.ctx, k2.ID)
	if err != nil {
		t.Fatalf("k2 item: %v", err)
	}
	levels, err := h.d.Queries.ListInventoryLevelsByItem(h.ctx, item2.ID)
	if err != nil || len(levels) != 1 {
		t.Fatalf("k2 levels: %v", err)
	}
	if levels[0].ReservedQuantity != 2 {
		t.Errorf("k2 reserved = %v after the failed placement, want the first order's 2 only", levels[0].ReservedQuantity)
	}
	h.settle(t)
}
