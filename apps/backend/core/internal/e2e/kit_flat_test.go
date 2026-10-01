package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type kitFixture struct {
	f     *store
	kit   catalog.Product
	parts []catalog.Product
	comps []catalog.KitComponent
}

func (h *harness) flatMode(t *testing.T) {
	t.Helper()
	if _, err := h.D.Config.Set(h.Ctx, "sales/inventory/flat_mode", "true", config.DefaultScope); err != nil {
		t.Fatalf("flat mode on: %v", err)
	}
	t.Cleanup(func() { _, _ = h.D.Config.Set(h.Ctx, "sales/inventory/flat_mode", "false", config.DefaultScope) })
}

func (h *harness) setFlatQty(t *testing.T, productID uuid.UUID, qty float64) {
	t.Helper()
	if _, err := h.D.DB.Exec(`UPDATE inventory_items SET quantity = $2 WHERE product_id = $1`, productID, qty); err != nil {
		t.Fatalf("flat quantity: %v", err)
	}
}

func (h *harness) flatQty(t *testing.T, productID uuid.UUID) float64 {
	t.Helper()
	var q float64
	if err := h.D.DB.QueryRow(`SELECT quantity FROM inventory_items WHERE product_id = $1`, productID).Scan(&q); err != nil {
		t.Fatalf("read flat quantity: %v", err)
	}
	return q
}

func (h *harness) newFlatKit(t *testing.T) kitFixture {
	t.Helper()
	f := h.NewStore(t, 10, 1000)
	kf := kitFixture{f: f}
	price := int64(900)
	k, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
		ProductTypeID: f.Product.ProductTypeID, Locale: "en",
		Name: "E2E Flat Kit " + uuid.NewString()[:8], SKU: "e2e-flat-kit-" + uuid.NewString()[:8],
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
	kf.kit = k
	for i, qty := range []float64{1, 2} {
		p := h.AddProduct(t, f, 100, true, 20)
		h.setFlatQty(t, p.ID, 20)
		c, err := h.D.Catalog.AddKitComponent(h.Ctx, catalog.AddKitComponentInput{
			ProductID: k.ID, ComponentProductID: p.ID, Quantity: qty, SortOrder: int32(i),
		})
		if err != nil {
			t.Fatalf("kit component: %v", err)
		}
		kf.parts = append(kf.parts, p)
		kf.comps = append(kf.comps, c)
	}
	h.flatMode(t)
	return kf
}

func (h *harness) wantFlat(t *testing.T, when string, want map[uuid.UUID]float64) {
	t.Helper()
	for id, w := range want {
		if got := h.flatQty(t, id); got != w {
			t.Errorf("%s: product %s quantity = %v, want %v", when, id, got, w)
		}
	}
}

// Scenario: flat mode takes a kit's components at placement and gives them back on cancel
func TestFlatKitCancelRestoresComponents(t *testing.T) {
	h := newHarness(t)
	kf := h.newFlatKit(t)
	a, b := kf.parts[0].ID, kf.parts[1].ID

	o, err := h.TryPlace(t, kf.f.Channel, h.NewGuestCartSKU(t, kf.f, kf.kit.SKU, 3))
	if err != nil {
		t.Fatalf("place kit: %v", err)
	}
	h.Settle(t)
	h.wantFlat(t, "after placement", map[uuid.UUID]float64{a: 17, b: 14})
	var snap int
	if err := h.D.DB.QueryRow(`SELECT count(*) FROM order_line_stock_parts k JOIN order_lines l ON l.id = k.order_line_id WHERE l.order_id = $1`, o.ID).Scan(&snap); err != nil || snap != 2 {
		t.Fatalf("kit snapshot rows = %d (%v), want 2", snap, err)
	}

	h.Cancel(t, o.ID)
	h.Settle(t)
	h.wantFlat(t, "after cancel", map[uuid.UUID]float64{a: 20, b: 20})
}

// Scenario: editing a flat kit line moves the component deduction to the new quantity, once
func TestFlatKitEditDoesNotDoubleDeduct(t *testing.T) {
	h := newHarness(t)
	kf := h.newFlatKit(t)
	a, b := kf.parts[0].ID, kf.parts[1].ID

	o, err := h.TryPlace(t, kf.f.Channel, h.NewGuestCartSKU(t, kf.f, kf.kit.SKU, 1))
	if err != nil {
		t.Fatalf("place kit: %v", err)
	}
	h.Settle(t)
	h.wantFlat(t, "after placement", map[uuid.UUID]float64{a: 19, b: 18})
	c := h.swapSecondComponent(t, kf)

	if _, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID:   o.ID,
		UpdateQty: []order.EditLineQty{{OrderLineID: h.lineBySKU(t, o.ID, kf.kit.SKU), Quantity: 2}},
	}); err != nil {
		t.Fatalf("EditOrder: %v", err)
	}
	h.Settle(t)
	h.wantFlat(t, "after edit", map[uuid.UUID]float64{a: 18, b: 16, c: 20})

	h.Cancel(t, o.ID)
	h.Settle(t)
	h.wantFlat(t, "after cancel", map[uuid.UUID]float64{a: 20, b: 20, c: 20})
}

// Scenario: a kit recipe changed after the order still gives back what the order actually took
func TestFlatKitChangedAfterOrderRestoresWhatWasTaken(t *testing.T) {
	h := newHarness(t)
	kf := h.newFlatKit(t)
	a, b := kf.parts[0].ID, kf.parts[1].ID

	o, err := h.TryPlace(t, kf.f.Channel, h.NewGuestCartSKU(t, kf.f, kf.kit.SKU, 2))
	if err != nil {
		t.Fatalf("place kit: %v", err)
	}
	h.Settle(t)
	h.wantFlat(t, "after placement", map[uuid.UUID]float64{a: 18, b: 16})

	c := h.swapSecondComponent(t, kf)

	h.Cancel(t, o.ID)
	h.Settle(t)
	h.wantFlat(t, "after cancel", map[uuid.UUID]float64{a: 20, b: 20, c: 20})
}

func (h *harness) swapSecondComponent(t *testing.T, kf kitFixture) uuid.UUID {
	t.Helper()
	if err := h.D.Catalog.DeleteKitComponent(h.Ctx, kf.comps[1].ID); err != nil {
		t.Fatalf("drop component: %v", err)
	}
	c := h.AddProduct(t, kf.f, 100, true, 0)
	h.setFlatQty(t, c.ID, 20)
	if _, err := h.D.Catalog.AddKitComponent(h.Ctx, catalog.AddKitComponentInput{
		ProductID: kf.kit.ID, ComponentProductID: c.ID, Quantity: 5,
	}); err != nil {
		t.Fatalf("new component: %v", err)
	}
	return c.ID
}
