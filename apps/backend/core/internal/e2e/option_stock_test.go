package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

type linkedFixture struct {
	f   *store
	src catalog.Product
	opt catalog.ProductOption
	val catalog.ProductOptionValue
}

// A configured product whose one value takes perUnit of a stocked source product per line unit
func (h *harness) newLinkedStock(t *testing.T, srcStock, perUnit float64) linkedFixture {
	t.Helper()
	f := h.NewStore(t, 10, 2000)
	src := h.AddProduct(t, f, 700, true, srcStock)
	opt, err := h.D.Catalog.AddProductOption(h.Ctx, catalog.AddProductOptionInput{
		ProductID: f.Product.ID, InputType: catalog.OptionInputDropdown, Locale: "en", Title: "Add-on",
	})
	if err != nil {
		t.Fatalf("add option: %v", err)
	}
	v, err := h.D.Catalog.AddProductOptionValue(h.Ctx, catalog.AddProductOptionValueInput{
		OptionID: opt.ID, Locale: "en", Title: "Linked", PriceType: catalog.OptionPriceFixed,
		SourceProductID: &src.ID, Quantity: perUnit,
	})
	if err != nil {
		t.Fatalf("add linked value: %v", err)
	}
	return linkedFixture{f: f, src: src, opt: opt, val: v}
}

func (h *harness) linkedCart(t *testing.T, lf linkedFixture, qty float64) (uuid.UUID, error) {
	t.Helper()
	c, err := h.D.Cart.CreateGuestCart(h.Ctx, lf.f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	_, err = h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: lf.f.Product.SKU, Quantity: qty,
		Options: []cart.CartLineOptionInput{{OptionID: lf.opt.ID, ValueIDs: []uuid.UUID{lf.val.ID}}},
	})
	return c.ID, err
}

func (h *harness) placeLinked(t *testing.T, lf linkedFixture, qty float64) order.Order {
	t.Helper()
	cartID, err := h.linkedCart(t, lf, qty)
	if err != nil {
		t.Fatalf("add configured: %v", err)
	}
	o, err := h.TryPlace(t, lf.f.Channel, cartID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)
	return o
}

func (h *harness) onHand(t *testing.T, productID uuid.UUID) float64 {
	t.Helper()
	item, err := h.D.Inventory.EnsureItem(h.Ctx, productID, true)
	if err != nil {
		t.Fatalf("ensure item: %v", err)
	}
	levels, err := h.D.Queries.ListInventoryLevelsByItem(h.Ctx, item.ID)
	if err != nil {
		t.Fatalf("levels: %v", err)
	}
	var q float64
	for _, l := range levels {
		q += l.Quantity
	}
	return q
}

// Scenario: a linked value holds its source per line unit, an edit moves the hold once, and shipping takes both products at their own ratios
func TestLinkedSourceReservesEditsAndShips(t *testing.T) {
	h := newHarness(t)
	lf := h.newLinkedStock(t, 10, 2)
	o := h.placeLinked(t, lf, 1)
	if got := h.reservedForProduct(t, lf.src.ID); got != 2 {
		t.Fatalf("source reserved after placement = %v, want 2", got)
	}
	if got := h.ReservedFor(t, lf.f); got != 1 {
		t.Fatalf("line product reserved = %v, want 1", got)
	}

	line := h.lineBySKU(t, o.ID, lf.f.Product.SKU)
	if _, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID: o.ID, UpdateQty: []order.EditLineQty{{OrderLineID: line, Quantity: 2}},
	}); err != nil {
		t.Fatalf("EditOrder: %v", err)
	}
	h.Settle(t)
	if got := h.reservedForProduct(t, lf.src.ID); got != 4 {
		t.Fatalf("source reserved after edit = %v, want 4", got)
	}

	for _, status := range []string{"confirmed", "processing"} {
		if _, err := h.D.Order.UpdateStatus(h.Ctx, o.ID, status); err != nil {
			t.Fatalf("advance to %s: %v", status, err)
		}
	}
	if _, err := h.D.Order.CreateFulfillment(h.Ctx, order.CreateFulfillmentInput{
		OrderID: o.ID, Lines: []order.FulfillmentLineInput{{OrderLineID: line, Quantity: 2}},
		Carrier: "ups", TrackingNumber: "1Z-linked",
	}); err != nil {
		t.Fatalf("fulfill: %v", err)
	}
	h.Settle(t)
	if got := h.reservedForProduct(t, lf.src.ID); got != 0 {
		t.Errorf("source still reserved after shipping = %v", got)
	}
	if got := h.onHand(t, lf.src.ID); got != 6 {
		t.Errorf("source on hand after shipping = %v, want 10 - 4", got)
	}
	if got := h.onHand(t, lf.f.Product.ID); got != 8 {
		t.Errorf("line product on hand after shipping = %v, want 10 - 2", got)
	}

	ret, err := h.D.Order.RequestReturn(h.Ctx, order.RequestReturnInput{
		OrderID: o.ID, Lines: []order.ReturnLineInput{{OrderLineID: line, Quantity: 1, Reason: "damaged"}}, Restock: true,
	})
	if err != nil {
		t.Fatalf("request return: %v", err)
	}
	if _, err := h.D.Order.ApproveReturn(h.Ctx, ret.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := h.D.Order.ReceiveReturn(h.Ctx, ret.ID); err != nil {
		t.Fatalf("receive: %v", err)
	}
	h.Settle(t)
	if got := h.onHand(t, lf.src.ID); got != 8 {
		t.Errorf("source on hand after returning one unit = %v, want 6 + 2", got)
	}
	if got := h.onHand(t, lf.f.Product.ID); got != 9 {
		t.Errorf("line product on hand after returning one unit = %v, want 8 + 1", got)
	}
}

// Scenario: flat mode deducts the source at placement, moves it once on edit and gives it back on cancel
func TestFlatLinkedSourceEditAndCancel(t *testing.T) {
	h := newHarness(t)
	lf := h.newLinkedStock(t, 20, 2)
	h.setFlatQty(t, lf.src.ID, 20)
	h.setFlatQty(t, lf.f.Product.ID, 20)
	h.flatMode(t)
	src, own := lf.src.ID, lf.f.Product.ID

	o := h.placeLinked(t, lf, 1)
	h.wantFlat(t, "after placement", map[uuid.UUID]float64{src: 18, own: 19})

	if _, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID: o.ID, UpdateQty: []order.EditLineQty{{OrderLineID: h.lineBySKU(t, o.ID, lf.f.Product.SKU), Quantity: 3}},
	}); err != nil {
		t.Fatalf("EditOrder: %v", err)
	}
	h.Settle(t)
	h.wantFlat(t, "after edit", map[uuid.UUID]float64{src: 14, own: 17})

	h.Cancel(t, o.ID)
	h.Settle(t)
	h.wantFlat(t, "after cancel", map[uuid.UUID]float64{src: 20, own: 20})
}

// Scenario: the add-time guard counts what the linked source has to cover
func TestLinkedSourceShortageBlocksTheAdd(t *testing.T) {
	h := newHarness(t)
	lf := h.newLinkedStock(t, 3, 2)
	_, err := h.linkedCart(t, lf, 2)
	if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "CART_INSUFFICIENT_STOCK" {
		t.Fatalf("adding 2 x 2 of a 3-stock source = %v, want CART_INSUFFICIENT_STOCK", err)
	}
	if _, err := h.linkedCart(t, lf, 1); err != nil {
		t.Fatalf("adding 1 x 2 of a 3-stock source: %v", err)
	}
}

// Scenario: an untracked source is never gated or held, like any untracked product
func TestUntrackedLinkedSourceIsIgnored(t *testing.T) {
	h := newHarness(t)
	lf := h.newLinkedStock(t, 0, 2)
	if _, err := h.D.Inventory.SyncProductTracking(h.Ctx, lf.src.ID, false); err != nil {
		t.Fatalf("untrack source: %v", err)
	}
	o := h.placeLinked(t, lf, 3)
	if got := h.reservedForProduct(t, lf.src.ID); got != 0 {
		t.Errorf("untracked source reserved = %v, want 0", got)
	}
	var parts int
	if err := h.D.DB.QueryRow(`SELECT count(*) FROM order_line_stock_parts p JOIN order_lines l ON l.id = p.order_line_id WHERE l.order_id = $1 AND p.source = 'option'`, o.ID).Scan(&parts); err != nil || parts != 1 {
		t.Fatalf("option stock parts = %d (%v), want 1", parts, err)
	}
}
