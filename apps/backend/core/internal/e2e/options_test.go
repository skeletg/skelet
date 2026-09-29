package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

// addSelectOption attaches a select option whose values carry their own price deltas - the deltas are on the VALUES for select-family types, since the option itself rejects a price
func (h *harness) addSelectOption(t *testing.T, productID uuid.UUID, title string, required bool, deltas ...int32) (catalog.ProductOption, []catalog.ProductOptionValue) {
	t.Helper()
	opt, err := h.D.Catalog.AddProductOption(h.Ctx, catalog.AddProductOptionInput{
		ProductID: productID, InputType: catalog.OptionInputDropdown,
		IsRequired: required, Locale: "en", Title: title,
	})
	if err != nil {
		t.Fatalf("add select option: %v", err)
	}
	values := make([]catalog.ProductOptionValue, 0, len(deltas))
	for i, d := range deltas {
		v, err := h.D.Catalog.AddProductOptionValue(h.Ctx, catalog.AddProductOptionValueInput{
			OptionID: opt.ID, Locale: "en", Title: title + " choice", SortOrder: int32(i),
			PriceValue: d, PriceType: catalog.OptionPriceFixed,
		})
		if err != nil {
			t.Fatalf("add option value: %v", err)
		}
		values = append(values, v)
	}
	return opt, values
}

// Scenario: a picked option's price delta is folded into the line's unit price at add time and rides through placement into the order total - a delta that stops at the cart is revenue never charged
func TestOptionPriceDeltaReachesTheOrderTotal(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2000)
	opt, values := h.addSelectOption(t, f.Product.ID, "Mounting", true, 500)

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	got, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 2,
		Options: []cart.CartLineOptionInput{{OptionID: opt.ID, ValueIDs: []uuid.UUID{values[0].ID}}},
	})
	if err != nil {
		t.Fatalf("add configured: %v", err)
	}
	if len(got.Lines) != 1 {
		t.Fatalf("%d cart lines, want 1", len(got.Lines))
	}
	if got.Lines[0].UnitPrice != 2500 {
		t.Errorf("cart unit price = %d, want the base 2000 plus the 500 delta", got.Lines[0].UnitPrice)
	}

	o, err := h.TryPlace(t, f.Channel, c.ID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)

	if o.SubtotalAmount != 2*2500 {
		t.Errorf("order subtotal = %d, want %d - the delta has to survive placement", o.SubtotalAmount, 2*2500)
	}
	lines := h.OrderLines(t, o.ID)
	if len(lines) != 1 || lines[0].UnitPrice != 2500 {
		t.Errorf("order line unit price = %d, want 2500", lines[0].UnitPrice)
	}
	if o.TotalAmount != o.SubtotalAmount-o.DiscountAmount+o.TaxAmount+o.ShippingAmount {
		t.Errorf("total %d does not equal its own breakdown", o.TotalAmount)
	}
}

// The order keeps a SNAPSHOT of what was picked, so an admin editing the option later can't rewrite what the customer bought - and the picked value is what the packing slip prints
func TestOptionPicksAreSnapshottedOntoTheOrder(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2000)
	opt, values := h.addSelectOption(t, f.Product.ID, "Engraving", true, 300)

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 1,
		Options: []cart.CartLineOptionInput{{OptionID: opt.ID, ValueIDs: []uuid.UUID{values[0].ID}}},
	}); err != nil {
		t.Fatalf("add configured: %v", err)
	}
	o, err := h.TryPlace(t, f.Channel, c.ID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)

	reread, err := h.D.Order.GetByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	if len(reread.Lines) != 1 {
		t.Fatalf("%d lines, want 1", len(reread.Lines))
	}
	opts := reread.Lines[0].Options
	if len(opts) != 1 {
		t.Fatalf("%d option snapshots on the line, want 1", len(opts))
	}
	if opts[0].OptionTitle != "Engraving" {
		t.Errorf("snapshot title = %q, want %q", opts[0].OptionTitle, "Engraving")
	}
	if opts[0].PriceDelta != 300 {
		t.Errorf("snapshot price delta = %d, want 300", opts[0].PriceDelta)
	}
	if opts[0].ProductOptionID != opt.ID {
		t.Errorf("snapshot option id = %v, want %v", opts[0].ProductOptionID, opt.ID)
	}

	// deleting the catalog option must not change what the order says was bought
	if err := h.D.Catalog.DeleteProductOption(h.Ctx, opt.ID); err != nil {
		t.Fatalf("delete option: %v", err)
	}
	after, err := h.D.Order.GetByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read after delete: %v", err)
	}
	if len(after.Lines[0].Options) != 1 || after.Lines[0].Options[0].OptionTitle != "Engraving" {
		t.Errorf("option snapshot after deleting the catalog row = %+v, want it intact", after.Lines[0].Options)
	}
	if after.SubtotalAmount != o.SubtotalAmount {
		t.Errorf("subtotal moved to %d after deleting the option, want the snapshot price %d", after.SubtotalAmount, o.SubtotalAmount)
	}
}

// Two picked options stack their deltas onto one unit price - taking only the last would undercharge
func TestOptionDeltasStackAcrossOptions(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 1000)
	mount, mountVals := h.addSelectOption(t, f.Product.ID, "Mounting", true, 500)
	wrap, wrapVals := h.addSelectOption(t, f.Product.ID, "Gift wrap", true, 250)

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	got, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 1,
		Options: []cart.CartLineOptionInput{
			{OptionID: mount.ID, ValueIDs: []uuid.UUID{mountVals[0].ID}},
			{OptionID: wrap.ID, ValueIDs: []uuid.UUID{wrapVals[0].ID}},
		},
	})
	if err != nil {
		t.Fatalf("add configured: %v", err)
	}
	if got.Lines[0].UnitPrice != 1750 {
		t.Errorf("unit price = %d, want 1000 + 500 + 250", got.Lines[0].UnitPrice)
	}

	o, err := h.TryPlace(t, f.Channel, c.ID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)
	if o.SubtotalAmount != 1750 {
		t.Errorf("order subtotal = %d, want 1750", o.SubtotalAmount)
	}
	reread, _ := h.D.Order.GetByID(h.Ctx, o.ID)
	if len(reread.Lines[0].Options) != 2 {
		t.Errorf("%d option snapshots, want both picks recorded", len(reread.Lines[0].Options))
	}
}

// A required option with nothing picked must refuse the add - otherwise a personalized product reaches the warehouse with no personalization
func TestRequiredOptionBlocksAPlainAdd(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2000)
	h.addSelectOption(t, f.Product.ID, "Mounting", true, 500)

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	// the plain door skips option validation entirely, so it has to refuse a product that needs picks
	if _, err := h.D.Cart.AddToCart(h.Ctx, c.ID, nil, f.Product.SKU, 1); err == nil {
		t.Error("a product with a required option was added without one")
	}
	// and the configured door refuses an empty pick set too
	if _, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 1,
	}); err == nil {
		t.Error("a required option was satisfied by an empty pick set")
	}
}

// Admin-entered lines carry no picks, so a draft or an edit must refuse a SKU whose options are required rather than placing an unfulfillable order
func TestRequiredOptionSKUIsRefusedForAdminEntry(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2000)
	h.addSelectOption(t, f.Product.ID, "Mounting", true, 500)

	_, err := h.D.Order.CreateDraftOrder(h.Ctx, order.CreateDraftOrderInput{
		ChannelID:       f.Channel.ID,
		Email:           "opt-" + uuid.NewString()[:8] + "@e2e.test",
		BillingAddress:  *e2eAddress(),
		ShippingAddress: *e2eAddress(),
		Lines:           []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 1}},
	})
	if err == nil {
		t.Error("a draft accepted a SKU with a required option and no picks")
	}

	// the same guard on the edit path - build a clean order on a different product, since the store's own product now needs picks the admin paths can't supply
	other := h.AddProduct(t, f, 1500, true, 10)
	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddToCart(h.Ctx, c.ID, nil, other.SKU, 1); err != nil {
		t.Fatalf("add plain line: %v", err)
	}
	clean, err := h.TryPlace(t, f.Channel, c.ID)
	if err != nil {
		t.Fatalf("place clean order: %v", err)
	}
	h.Settle(t)

	if _, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID: clean.ID,
		AddLine: []order.EditAddLine{{ProductSku: f.Product.SKU, Quantity: 1}},
	}); err == nil {
		t.Error("an edit added a SKU with a required option and no picks")
	}
	h.Settle(t)
}

// Re-adding the same configured SKU REPLACES its configuration rather than summing, so a shopper changing their mind doesn't end up with two differently-configured lines
func TestReAddingAConfiguredSKUReplacesTheConfiguration(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 1000)
	opt, values := h.addSelectOption(t, f.Product.ID, "Finish", true, 200, 700)

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 1,
		Options: []cart.CartLineOptionInput{{OptionID: opt.ID, ValueIDs: []uuid.UUID{values[0].ID}}},
	}); err != nil {
		t.Fatalf("first add: %v", err)
	}
	got, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 1,
		Options: []cart.CartLineOptionInput{{OptionID: opt.ID, ValueIDs: []uuid.UUID{values[1].ID}}},
	})
	if err != nil {
		t.Fatalf("second add: %v", err)
	}

	if len(got.Lines) != 1 {
		t.Fatalf("%d lines after re-adding, want the configuration replaced rather than a second line", len(got.Lines))
	}
	if got.Lines[0].UnitPrice != 1700 {
		t.Errorf("unit price = %d, want 1000 + the second value's 700", got.Lines[0].UnitPrice)
	}
}
