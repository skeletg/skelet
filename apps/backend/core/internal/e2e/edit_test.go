package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// reservedForProduct is reservedFor for a product that isn't the store's own - added lines reserve against their own inventory item
func (h *harness) reservedForProduct(t *testing.T, productID uuid.UUID) float64 {
	t.Helper()
	item, err := h.D.Inventory.EnsureItem(h.Ctx, productID, true)
	if err != nil {
		t.Fatalf("ensure item: %v", err)
	}
	levels, err := h.D.Queries.ListInventoryLevelsByItem(h.Ctx, item.ID)
	if err != nil {
		t.Fatalf("levels: %v", err)
	}
	var reserved float64
	for _, l := range levels {
		reserved += l.ReservedQuantity
	}
	return reserved
}

func (h *harness) eventTypes(t *testing.T, orderID uuid.UUID) map[string]int {
	t.Helper()
	rows, err := h.D.Queries.ListOrderEventsByOrder(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("order events: %v", err)
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.Type]++
	}
	return out
}

func (h *harness) lineBySKU(t *testing.T, orderID uuid.UUID, sku string) uuid.UUID {
	t.Helper()
	for _, l := range h.OrderLines(t, orderID) {
		if l.ProductSku == sku {
			return l.ID
		}
	}
	t.Fatalf("no line with sku %q on order %s", sku, orderID)
	return uuid.Nil
}

// Scenario: raising a line's quantity re-prices the order from the new line set AND moves the reserved counter with it - a re-price that skipped the re-reservation would oversell
func TestEditRaisesQuantityRepricesAndReReserves(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 2500)
	o := h.PlaceGuestCOD(t, f, 2)
	h.Settle(t)

	if got := h.ReservedFor(t, f); got != 2 {
		t.Fatalf("reserved after placement = %v, want 2", got)
	}
	lineID := h.OrderLines(t, o.ID)[0].ID

	edited, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID:   o.ID,
		UpdateQty: []order.EditLineQty{{OrderLineID: lineID, Quantity: 5}},
	})
	if err != nil {
		t.Fatalf("EditOrder: %v", err)
	}

	if edited.SubtotalAmount != 5*2500 {
		t.Errorf("subtotal = %d, want %d", edited.SubtotalAmount, 5*2500)
	}
	if edited.TotalAmount != edited.SubtotalAmount-edited.DiscountAmount+edited.TaxAmount+edited.ShippingAmount {
		t.Errorf("total %d does not equal its own breakdown %+v", edited.TotalAmount, edited)
	}
	if got := h.ReservedFor(t, f); got != 5 {
		t.Errorf("reserved after raising to 5 = %v, want 5", got)
	}
	if got := h.eventTypes(t, o.ID)["order_edited"]; got != 1 {
		t.Errorf("%d order_edited events, want 1 on the timeline", got)
	}
	h.Settle(t)
}

// Lowering the quantity has to RELEASE the difference, or stock stays held for units nobody bought
func TestEditLowersQuantityAndReleasesReservation(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 2500)
	o := h.PlaceGuestCOD(t, f, 6)
	h.Settle(t)
	lineID := h.OrderLines(t, o.ID)[0].ID

	edited, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID:   o.ID,
		UpdateQty: []order.EditLineQty{{OrderLineID: lineID, Quantity: 2}},
	})
	if err != nil {
		t.Fatalf("EditOrder: %v", err)
	}
	if edited.SubtotalAmount != 2*2500 {
		t.Errorf("subtotal = %d, want %d", edited.SubtotalAmount, 2*2500)
	}
	if got := h.ReservedFor(t, f); got != 2 {
		t.Errorf("reserved after lowering to 2 = %v, want 2", got)
	}
	h.Settle(t)
}

// A removed line stops being part of the order AND gives its stock back
func TestEditRemovingALineReleasesItsStock(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 2500)
	extra := h.AddProduct(t, f, 1000, true, 20)

	o := h.PlaceGuestCOD(t, f, 2)
	h.Settle(t)
	// add the second product through an edit so the order has two lines to work with
	if _, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID: o.ID,
		AddLine: []order.EditAddLine{{ProductSku: extra.SKU, Quantity: 3}},
	}); err != nil {
		t.Fatalf("add line: %v", err)
	}
	if got := h.reservedForProduct(t, extra.ID); got != 3 {
		t.Fatalf("reserved for the added product = %v, want 3", got)
	}

	extraLine := h.lineBySKU(t, o.ID, extra.SKU)
	edited, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID:    o.ID,
		RemoveLine: []uuid.UUID{extraLine},
	})
	if err != nil {
		t.Fatalf("remove line: %v", err)
	}

	if len(h.OrderLines(t, o.ID)) != 1 {
		t.Errorf("%d active lines, want the removed one gone", len(h.OrderLines(t, o.ID)))
	}
	if edited.SubtotalAmount != 2*2500 {
		t.Errorf("subtotal = %d, want the removed line's money gone (%d)", edited.SubtotalAmount, 2*2500)
	}
	if got := h.reservedForProduct(t, extra.ID); got != 0 {
		t.Errorf("reserved for the removed product = %v, want 0", got)
	}
	if got := h.ReservedFor(t, f); got != 2 {
		t.Errorf("reserved for the kept product = %v, want 2 - a removal must not disturb the other line", got)
	}
	h.Settle(t)
}

// The client names a SKU and a quantity, never a price - an added line is priced from the channel listing, so a caller can't discount itself
func TestEditPricesAnAddedLineServerSide(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 2500)
	extra := h.AddProduct(t, f, 1750, true, 20)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)

	edited, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID: o.ID,
		AddLine: []order.EditAddLine{{ProductSku: extra.SKU, Quantity: 2}},
	})
	if err != nil {
		t.Fatalf("EditOrder: %v", err)
	}

	var added bool
	for _, l := range h.OrderLines(t, o.ID) {
		if l.ProductSku != extra.SKU {
			continue
		}
		added = true
		if l.UnitPrice != 1750 {
			t.Errorf("added line unit price = %d, want the listing's 1750", l.UnitPrice)
		}
		if l.TotalPrice != 3500 {
			t.Errorf("added line total = %d, want 3500", l.TotalPrice)
		}
	}
	if !added {
		t.Fatal("the added line is missing from the order")
	}
	if edited.SubtotalAmount != 2500+3500 {
		t.Errorf("subtotal = %d, want %d", edited.SubtotalAmount, 2500+3500)
	}
	if got := h.reservedForProduct(t, extra.ID); got != 2 {
		t.Errorf("reserved for the added product = %v, want 2", got)
	}
	h.Settle(t)
}

// The edit runs in one transaction, so a shortage on ANY line has to leave the order exactly as it was - a partial apply would leave lines and reservations disagreeing
func TestEditShortageRollsTheWholeEditBack(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 4, 2500)
	o := h.PlaceGuestCOD(t, f, 2)
	h.Settle(t)

	before := h.OrderLines(t, o.ID)
	beforeReserved := h.ReservedFor(t, f)
	lineID := before[0].ID

	// 99 is well past the 4 on hand
	if _, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID:   o.ID,
		UpdateQty: []order.EditLineQty{{OrderLineID: lineID, Quantity: 99}},
	}); err == nil {
		t.Fatal("an edit past available stock succeeded")
	}

	after := h.OrderLines(t, o.ID)
	if len(after) != len(before) {
		t.Errorf("%d lines after the failed edit, want the original %d", len(after), len(before))
	}
	if after[0].Quantity != before[0].Quantity {
		t.Errorf("line quantity = %v after a rolled-back edit, want the original %v", after[0].Quantity, before[0].Quantity)
	}
	if got := h.ReservedFor(t, f); got != beforeReserved {
		t.Errorf("reserved = %v after a rolled-back edit, want the original %v", got, beforeReserved)
	}
	reread, err := h.D.Order.GetByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	if reread.SubtotalAmount != o.SubtotalAmount || reread.TotalAmount != o.TotalAmount {
		t.Errorf("amounts moved to {sub %d total %d} on a failed edit, want {%d %d}",
			reread.SubtotalAmount, reread.TotalAmount, o.SubtotalAmount, o.TotalAmount)
	}
	if got := h.eventTypes(t, o.ID)["order_edited"]; got != 0 {
		t.Errorf("%d order_edited events after a failed edit, want 0", got)
	}
	h.Settle(t)
}

// Editing is pre-fulfillment only - once units have shipped the snapshot is what the customer was told, and the reservation math no longer describes reality
func TestEditRefusedAfterFulfillment(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 2500)
	o := h.PlaceGuestCOD(t, f, 2)
	h.Settle(t)
	lineID := h.OrderLines(t, o.ID)[0].ID

	// a COD order lands pending, and only a processing order can ship
	for _, st := range []string{order.OrderStatusConfirmed, order.OrderStatusProcessing} {
		if _, err := h.D.Order.UpdateStatus(h.Ctx, o.ID, st); err != nil {
			t.Fatalf("advance to %s: %v", st, err)
		}
	}
	if _, err := h.D.Order.CreateFulfillment(h.Ctx, order.CreateFulfillmentInput{
		OrderID: o.ID,
		Lines:   []order.FulfillmentLineInput{{OrderLineID: lineID, Quantity: 2}},
	}); err != nil {
		t.Fatalf("fulfill: %v", err)
	}
	h.Settle(t)

	_, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID:   o.ID,
		UpdateQty: []order.EditLineQty{{OrderLineID: lineID, Quantity: 1}},
	})
	if err == nil {
		t.Fatal("editing a fulfilled order succeeded")
	}
	if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "ORDER_NOT_EDITABLE" {
		t.Errorf("error = %v, want code ORDER_NOT_EDITABLE", err)
	}
	h.Settle(t)
}

// Every guard refuses BEFORE anything is written, so a malformed edit can't half-apply
func TestEditGuardsRefuseWithoutTouchingTheOrder(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 2500)
	o := h.PlaceGuestCOD(t, f, 2)
	h.Settle(t)
	lineID := h.OrderLines(t, o.ID)[0].ID
	foreign := uuid.New()

	for name, tc := range map[string]struct {
		in   order.EditOrderInput
		code string
	}{
		"no changes at all": {
			order.EditOrderInput{OrderID: o.ID}, "ORDER_EDIT_EMPTY",
		},
		"a line from another order": {
			order.EditOrderInput{OrderID: o.ID, UpdateQty: []order.EditLineQty{{OrderLineID: foreign, Quantity: 1}}},
			"ORDER_EDIT_UNKNOWN_LINE",
		},
		"removing a line that isn't ours": {
			order.EditOrderInput{OrderID: o.ID, RemoveLine: []uuid.UUID{foreign}},
			"ORDER_EDIT_UNKNOWN_LINE",
		},
		"updating and removing the same line": {
			order.EditOrderInput{
				OrderID:    o.ID,
				UpdateQty:  []order.EditLineQty{{OrderLineID: lineID, Quantity: 3}},
				RemoveLine: []uuid.UUID{lineID},
			},
			"ORDER_EDIT_CONFLICT",
		},
		"a zero quantity": {
			order.EditOrderInput{OrderID: o.ID, UpdateQty: []order.EditLineQty{{OrderLineID: lineID, Quantity: 0}}},
			"ORDER_EDIT_INVALID_QTY",
		},
		"a negative quantity": {
			order.EditOrderInput{OrderID: o.ID, UpdateQty: []order.EditLineQty{{OrderLineID: lineID, Quantity: -1}}},
			"ORDER_EDIT_INVALID_QTY",
		},
		"a negative added quantity": {
			order.EditOrderInput{OrderID: o.ID, AddLine: []order.EditAddLine{{ProductSku: f.Product.SKU, Quantity: -2}}},
			"ORDER_EDIT_INVALID_QTY",
		},
		"removing the last line": {
			order.EditOrderInput{OrderID: o.ID, RemoveLine: []uuid.UUID{lineID}},
			"ORDER_EDIT_EMPTY_RESULT",
		},
	} {
		_, err := h.D.Order.EditOrder(h.Ctx, tc.in)
		if err == nil {
			t.Errorf("%s was accepted, want %s", name, tc.code)
			continue
		}
		if ae, ok := apperr.AsAppError(err); !ok || ae.Code != tc.code {
			t.Errorf("%s: error = %v, want code %s", name, err, tc.code)
		}
	}

	// and the order is untouched by any of them
	lines := h.OrderLines(t, o.ID)
	if len(lines) != 1 || lines[0].Quantity != 2 {
		t.Errorf("order lines = %d at qty %v, want the original 1 line at 2", len(lines), lines[0].Quantity)
	}
	if got := h.ReservedFor(t, f); got != 2 {
		t.Errorf("reserved = %v, want the original 2", got)
	}
	if got := h.eventTypes(t, o.ID)["order_edited"]; got != 0 {
		t.Errorf("%d order_edited events, want 0 - every guard fired before the write", got)
	}
	h.Settle(t)
}

// A bundle's component quantities are fixed at placement, so changing the wrapper's quantity would silently leave the components describing a different order - remove and re-add instead
func TestEditRefusesABundleLineQuantity(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 2500)
	comp := h.AddProduct(t, f, 1000, true, 30)

	b, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
		ProductTypeID: f.Product.ProductTypeID, Locale: "en",
		Name: "E2E Edit Bundle " + uuid.NewString()[:8], SKU: "e2e-edit-bundle-" + uuid.NewString()[:8],
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
	sel, err := h.D.Catalog.AddBundleSelection(h.Ctx, catalog.AddBundleSelectionInput{
		OptionID: opt.ID, ProductID: comp.ID, Quantity: 2,
	})
	if err != nil {
		t.Fatalf("bundle selection: %v", err)
	}

	c0, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddBundleToCart(h.Ctx, c0.ID, nil, cart.AddBundleToCartInput{
		ProductSku: b.SKU, Quantity: 1,
		Selections: []cart.AddBundleSelectionInput{{OptionID: opt.ID, SelectionIDs: []uuid.UUID{sel.ID}}},
	}); err != nil {
		t.Fatalf("add bundle: %v", err)
	}
	o, err := h.TryPlace(t, f.Channel, c0.ID)
	if err != nil {
		t.Fatalf("place bundle order: %v", err)
	}
	h.Settle(t)

	bundleLine := h.lineBySKU(t, o.ID, b.SKU)
	_, err = h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID:   o.ID,
		UpdateQty: []order.EditLineQty{{OrderLineID: bundleLine, Quantity: 2}},
	})
	if err == nil {
		t.Fatal("editing a bundle line quantity succeeded")
	}
	if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "ORDER_EDIT_BUNDLE_QTY" {
		t.Errorf("error = %v, want code ORDER_EDIT_BUNDLE_QTY", err)
	}
	h.Settle(t)
}

// A re-read order line carries its bundle components, so an edit to an UNRELATED line re-reserves them instead of quietly releasing the stock the order still owes
func TestEditKeepsABundlesComponentReservation(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 2500)
	comp := h.AddProduct(t, f, 1000, true, 30)
	b, opt, sel := h.newBundleProduct(t, f, comp, 2)

	c0, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddBundleToCart(h.Ctx, c0.ID, nil, cart.AddBundleToCartInput{
		ProductSku: b.SKU, Quantity: 1,
		Selections: []cart.AddBundleSelectionInput{{OptionID: opt.ID, SelectionIDs: []uuid.UUID{sel.ID}}},
	}); err != nil {
		t.Fatalf("add bundle: %v", err)
	}
	// a plain line the edit can legally touch
	if _, err := h.D.Cart.AddToCart(h.Ctx, c0.ID, nil, f.Product.SKU, 1); err != nil {
		t.Fatalf("add simple line: %v", err)
	}
	o, err := h.TryPlace(t, f.Channel, c0.ID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)

	if got := h.reservedForProduct(t, comp.ID); got != 2 {
		t.Fatalf("component reserved after placement = %v, want 2", got)
	}
	// the re-read has to carry the components, which is what the reservation and the bundle guard both read
	reread, err := h.D.Order.GetByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	var bundleComponents int
	for _, l := range reread.Lines {
		if l.ProductSku == b.SKU {
			bundleComponents = len(l.Components)
		}
	}
	if bundleComponents != 1 {
		t.Errorf("re-read bundle line carries %d components, want 1 - without them the edit re-reserves nothing for it", bundleComponents)
	}

	simpleLine := h.lineBySKU(t, o.ID, f.Product.SKU)
	if _, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID:   o.ID,
		UpdateQty: []order.EditLineQty{{OrderLineID: simpleLine, Quantity: 3}},
	}); err != nil {
		t.Fatalf("edit the simple line: %v", err)
	}

	if got := h.reservedForProduct(t, comp.ID); got != 2 {
		t.Errorf("component reserved after editing an unrelated line = %v, want it still 2", got)
	}
	if got := h.ReservedFor(t, f); got != 3 {
		t.Errorf("simple line reserved = %v, want the edited 3", got)
	}
	h.Settle(t)
}

// newBundleProduct builds a published one-option bundle over a single component
func (h *harness) newBundleProduct(t *testing.T, f *store, comp catalog.Product, compQty float64) (catalog.Product, catalog.BundleOption, catalog.BundleSelection) {
	t.Helper()
	b, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
		ProductTypeID: f.Product.ProductTypeID, Locale: "en",
		Name: "E2E Edit Bundle " + uuid.NewString()[:8], SKU: "e2e-edit-bundle-" + uuid.NewString()[:8],
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
	sel, err := h.D.Catalog.AddBundleSelection(h.Ctx, catalog.AddBundleSelectionInput{
		OptionID: opt.ID, ProductID: comp.ID, Quantity: compQty,
	})
	if err != nil {
		t.Fatalf("bundle selection: %v", err)
	}
	return b, opt, sel
}
