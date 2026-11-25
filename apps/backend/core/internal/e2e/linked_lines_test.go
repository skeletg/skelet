package e2e

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
)

// Scenario: a jacket with a protection plan offered as one of its options, sold as a line of its own. The plan lands
// in the cart under the jacket with its own SKU and price, is taxed by its own tax class, becomes its own order line
// tied to the jacket's, follows the jacket through an edit, and can be refunded without the jacket
func TestAnAddOnIsItsOwnLineFromCartToRefund(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 10_000)
	plan := h.AddProduct(t, f, 1_500, true, 50)
	if _, err := h.D.Catalog.SetProductTaxClass(h.Ctx, plan.ID, "services"); err != nil {
		t.Fatalf("classify the plan: %v", err)
	}
	h.newTaxRule(t, f.Channel.ID, 1000, tax.Condition{Type: "core.product_tax_class", Params: json.RawMessage(`{"classes":["services"]}`)})

	opt, err := h.D.Catalog.AddProductOption(h.Ctx, catalog.AddProductOptionInput{
		ProductID: f.Product.ID, InputType: catalog.OptionInputDropdown, Locale: "en", Title: "Protection",
	})
	if err != nil {
		t.Fatalf("add option: %v", err)
	}
	if _, err := h.D.Catalog.AddProductOptionValue(h.Ctx, catalog.AddProductOptionValueInput{
		OptionID: opt.ID, Locale: "en", Title: "No product", PriceType: catalog.OptionPriceFixed, Quantity: 1, SoldAsLine: true,
	}); err == nil {
		t.Fatal("a choice with no product behind it was set to be sold as a line")
	}
	val, err := h.D.Catalog.AddProductOptionValue(h.Ctx, catalog.AddProductOptionValueInput{
		OptionID: opt.ID, Locale: "en", Title: "Two years", PriceType: catalog.OptionPriceFixed, SourceProductID: &plan.ID, Quantity: 1, SoldAsLine: true,
	})
	if err != nil {
		t.Fatalf("add the plan as a choice: %v", err)
	}

	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`{ product(id: %q) { options { values { id soldAsLine } } } }`, f.Product.ID))
	if len(errs) > 0 {
		t.Fatalf("product options: %v", errs)
	}
	if v := data["product"].(map[string]any)["options"].([]any)[0].(map[string]any)["values"].([]any)[0].(map[string]any); v["soldAsLine"] != true {
		t.Errorf("the storefront reads the choice as %v, want it marked as sold as its own line", v)
	}

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 2, Options: []cart.CartLineOptionInput{{OptionID: opt.ID, ValueIDs: []uuid.UUID{val.ID}}},
	}); err != nil {
		t.Fatalf("add the jacket with its plan: %v", err)
	}
	data, errs = h.gqlPost(t, ts, chID, fmt.Sprintf(`{ cart(id: %q) { subtotal lines { id sku unitPrice quantity parentLineId promotionsApply } } }`, c.ID))
	if len(errs) > 0 {
		t.Fatalf("the cart: %v", errs)
	}
	shown := data["cart"].(map[string]any)
	lines := shown["lines"].([]any)
	if len(lines) != 2 || shown["subtotal"] != float64(23_000) {
		t.Fatalf("the cart shows %v, want the jacket and its plan, 2 x 10000 + 2 x 1500", shown)
	}
	jacketLine, planLine := lines[0].(map[string]any), lines[1].(map[string]any)
	if jacketLine["sku"] != f.Product.SKU || jacketLine["unitPrice"] != float64(10_000) || jacketLine["parentLineId"] != nil {
		t.Errorf("the jacket line %v, want its own price and no parent", jacketLine)
	}
	if planLine["sku"] != plan.SKU || planLine["unitPrice"] != float64(1_500) || planLine["quantity"] != float64(2) || planLine["parentLineId"] != jacketLine["id"] || planLine["promotionsApply"] != false {
		t.Errorf("the plan line %v, want 2 at 1500 under the jacket, left alone by promotions", planLine)
	}
	if _, refused := h.gqlPostErrors(t, ts, chID, fmt.Sprintf(`mutation { updateCartLine(cartId: %q, lineId: %q, quantity: 5) { id } }`, c.ID, planLine["id"])); len(refused) != 1 || refused[0].Extensions["code"] != "CART_LINE_FOLLOWS_PARENT" {
		t.Errorf("changing the plan's own quantity: %+v, want CART_LINE_FOLLOWS_PARENT", refused)
	}

	o, err := h.TryPlace(t, f.Channel, c.ID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	// Only the plan is in a taxed class: 10% of its 3000, none of the jackets'
	if o.SubtotalAmount != 23_000 || o.TaxAmount-o.ShippingTaxAmount != 300 {
		t.Fatalf("subtotal %d, tax on the goods %d - want 23000 and 300, the plan taxed by its own class", o.SubtotalAmount, o.TaxAmount-o.ShippingTaxAmount)
	}
	ol := h.linesAddUp(t, o)
	if len(ol) != 2 || ol[0].ProductSku != f.Product.SKU || ol[1].ProductSku != plan.SKU {
		t.Fatalf("order lines %+v, want the jacket then its plan", ol)
	}
	if !ol[1].ParentLineID.Valid || ol[1].ParentLineID.UUID != ol[0].ID || ol[0].ParentLineID.Valid || ol[1].TaxCents != 300 || ol[0].TaxCents != 0 {
		t.Errorf("order lines %+v, want the plan tied to the jacket and carrying all the tax", ol)
	}
	if reserved := h.reservedForProduct(t, plan.ID); reserved != 2 {
		t.Errorf("%v of the plan reserved, want its own line's 2 and nothing through the jacket", reserved)
	}
	h.Settle(t)

	// The plan follows the jacket through an edit, and can't be re-quantified on its own
	if _, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{OrderID: o.ID, UpdateQty: []order.EditLineQty{{OrderLineID: ol[1].ID, Quantity: 1}}}); err == nil {
		t.Error("the plan's own quantity was edited on the order")
	}
	edited, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{OrderID: o.ID, UpdateQty: []order.EditLineQty{{OrderLineID: ol[0].ID, Quantity: 3}}})
	if err != nil {
		t.Fatalf("edit the jacket's quantity: %v", err)
	}
	ol = h.linesAddUp(t, edited)
	if edited.SubtotalAmount != 34_500 || ol[1].Quantity != 3 {
		t.Fatalf("after 3 jackets: subtotal %d and %v plans, want 34500 and 3", edited.SubtotalAmount, ol[1].Quantity)
	}

	// Refunded without the jacket: one plan and its tax
	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{OrderID: o.ID, Method: "cod", Reference: "e2e-cash"}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	memo, err := h.D.Order.RecordCreditMemoIntent(h.Ctx, order.IssueCreditMemoInput{OrderID: o.ID, Lines: []order.CreditMemoLineInput{{OrderLineID: ol[1].ID, Quantity: 1}}})
	if err != nil {
		t.Fatalf("refund one plan: %v", err)
	}
	if memo.AmountCents != 1_650 {
		t.Errorf("one plan refunds %d, want its 1500 and its 150 of tax", memo.AmountCents)
	}

	// Removing the jacket from the order takes its plan with it
	extra := h.AddProduct(t, f, 2_000, true, 20)
	o2cart, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.Cart.AddConfiguredToCart(h.Ctx, o2cart.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 1, Options: []cart.CartLineOptionInput{{OptionID: opt.ID, ValueIDs: []uuid.UUID{val.ID}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.Cart.AddToCart(h.Ctx, o2cart.ID, nil, extra.SKU, 1); err != nil {
		t.Fatal(err)
	}
	o2, err := h.TryPlace(t, f.Channel, o2cart.ID)
	if err != nil {
		t.Fatalf("place the second order: %v", err)
	}
	h.Settle(t)
	removed, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{OrderID: o2.ID, RemoveLine: []uuid.UUID{h.lineBySKU(t, o2.ID, f.Product.SKU)}})
	if err != nil {
		t.Fatalf("remove the jacket: %v", err)
	}
	if left := h.OrderLines(t, o2.ID); len(left) != 1 || left[0].ProductSku != extra.SKU || removed.SubtotalAmount != 2_000 {
		t.Errorf("after removing the jacket: %d lines, subtotal %d - want only the other product's 2000", len(left), removed.SubtotalAmount)
	}
	h.Settle(t)
}

// Scenario: the jacket ships, the protection plan bought with it is a service - nothing to ship. Shipping the jacket
// alone fulfils the order. The plan's line says it doesn't ship, so it never held the order at partially fulfilled
func TestAServiceAddOnDoesNotHoldTheOrderBackFromFulfilled(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 10_000)
	planSKU := h.splitProduct(t, f, true, nil, 1_500)
	var planID uuid.UUID
	if err := h.D.DB.QueryRowContext(h.Ctx, `SELECT id FROM catalog_products WHERE sku = $1`, planSKU).Scan(&planID); err != nil {
		t.Fatalf("find the plan: %v", err)
	}
	opt, err := h.D.Catalog.AddProductOption(h.Ctx, catalog.AddProductOptionInput{
		ProductID: f.Product.ID, InputType: catalog.OptionInputDropdown, Locale: "en", Title: "Protection",
	})
	if err != nil {
		t.Fatalf("add option: %v", err)
	}
	val, err := h.D.Catalog.AddProductOptionValue(h.Ctx, catalog.AddProductOptionValueInput{
		OptionID: opt.ID, Locale: "en", Title: "Two years", PriceType: catalog.OptionPriceFixed, SourceProductID: &planID, Quantity: 1, SoldAsLine: true,
	})
	if err != nil {
		t.Fatalf("add the plan as a choice: %v", err)
	}
	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 1, Options: []cart.CartLineOptionInput{{OptionID: opt.ID, ValueIDs: []uuid.UUID{val.ID}}},
	}); err != nil {
		t.Fatalf("add the jacket with its plan: %v", err)
	}
	o, err := h.TryPlace(t, f.Channel, c.ID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)
	lines := h.OrderLines(t, o.ID)
	if len(lines) != 2 || !lines[0].RequiresShipping || lines[1].RequiresShipping {
		t.Fatalf("order lines %+v, want the jacket to ship and the plan not to", lines)
	}
	for _, status := range []string{"confirmed", "processing"} {
		if _, err := h.D.Order.UpdateStatus(h.Ctx, o.ID, status); err != nil {
			t.Fatalf("advance to %s: %v", status, err)
		}
	}
	shipped, err := h.D.Order.CreateFulfillment(h.Ctx, order.CreateFulfillmentInput{
		OrderID: o.ID, Lines: []order.FulfillmentLineInput{{OrderLineID: lines[0].ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("ship the jacket: %v", err)
	}
	if shipped.Status != order.OrderStatusFulfilled {
		t.Errorf("after shipping the jacket the order is %q, want fulfilled - the plan is nothing to ship", shipped.Status)
	}
	h.Settle(t)
}
