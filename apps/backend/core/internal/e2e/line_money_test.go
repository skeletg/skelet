package e2e

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
)

// The lines of an order account for all of its discount and all the tax on its goods
func (h *harness) linesAddUp(t *testing.T, o order.Order) []db.OrderLine {
	t.Helper()
	lines := h.OrderLines(t, o.ID)
	var discount, taxed int64
	for _, l := range lines {
		discount += l.DiscountCents
		taxed += l.TaxCents
	}
	if discount != o.DiscountAmount {
		t.Errorf("the lines carry %d of discount, the order %d", discount, o.DiscountAmount)
	}
	if goods := o.TaxAmount - o.ShippingTaxAmount - o.FeeTaxAmount(); taxed != goods {
		t.Errorf("the lines carry %d of tax, the order's goods %d", taxed, goods)
	}
	return lines
}

// Scenario: a fifth off one product, and a tax that only the other product is charged. Each order line carries the
// discount the promotion put on it and its own tax, the quote taxed the checkout the same way, and a refund of a
// line gives back what that line was charged - not a share of the other's discount at the other's rate
func TestOrderLinesCarryTheirOwnDiscountAndTax(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 10_000)
	book := h.AddProduct(t, f, 5_000, true, 50)
	if _, err := h.D.Catalog.SetProductTaxClass(h.Ctx, book.ID, "reduced"); err != nil {
		t.Fatalf("classify: %v", err)
	}
	h.newTaxRule(t, f.Channel.ID, 500, tax.Condition{
		Type: "core.product_tax_class", Params: json.RawMessage(`{"classes":["reduced"]}`),
	})
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E a fifth off one product",
			ChannelID: &f.Channel.ID, IsActive: true,
		},
		Actions: []promotion.Action{{Type: "core.percent_off_matching_items", Params: json.RawMessage(fmt.Sprintf(
			`{"bps":2000,"condition":{"type":"core.cart_contains_sku","params":{"skus":[%q]}}}`, f.Product.SKU))}},
	})
	newCart := func() uuid.UUID {
		id := h.NewGuestCart(t, f, 1)
		if _, err := h.D.Cart.AddToCart(h.Ctx, id, nil, book.SKU, 2); err != nil {
			t.Fatalf("add the second product: %v", err)
		}
		return id
	}

	quoted, cod := newCart(), "cod"
	quote, err := h.D.Order.QuoteCheckout(channel.WithChannel(h.Ctx, &f.Channel), order.PlaceOrderInput{
		CartID: &quoted, BillingAddress: e2eAddress(), ShippingAddress: e2eAddress(), PaymentProviderSlug: &cod,
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	o, err := h.TryPlace(t, f.Channel, newCart())
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if o.DiscountAmount != 2_000 {
		t.Fatalf("discount = %d, want the 2000 off the first product", o.DiscountAmount)
	}
	// The second product was never discounted, so all 10000 of it is taxed - a share of the discount by subtotal
	// would have taxed 9000
	if goods := o.TaxAmount - o.ShippingTaxAmount; goods != 500 {
		t.Errorf("tax on the goods = %d, want 5%% of the undiscounted 10000", goods)
	}
	if quote.TaxCents != o.TaxAmount || quote.TotalCents != o.TotalAmount {
		t.Errorf("quoted tax %d total %d, placed %d and %d", quote.TaxCents, quote.TotalCents, o.TaxAmount, o.TotalAmount)
	}

	lines := h.linesAddUp(t, o)
	if len(lines) != 2 || lines[0].ProductSku != f.Product.SKU || lines[1].ProductSku != book.SKU {
		t.Fatalf("lines = %+v, want the two products in the order they were added", lines)
	}
	if lines[0].DiscountCents != 2_000 || lines[0].TaxCents != 0 {
		t.Errorf("discounted line carries discount %d tax %d, want 2000 and 0", lines[0].DiscountCents, lines[0].TaxCents)
	}
	if lines[1].DiscountCents != 0 || lines[1].TaxCents != 500 {
		t.Errorf("taxed line carries discount %d tax %d, want 0 and 500", lines[1].DiscountCents, lines[1].TaxCents)
	}

	h.Settle(t)
	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{OrderID: o.ID, Method: "cod", Reference: "e2e-cash"}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	memo, err := h.D.Order.RecordCreditMemoIntent(h.Ctx, order.IssueCreditMemoInput{
		OrderID: o.ID, Lines: []order.CreditMemoLineInput{{OrderLineID: lines[1].ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("refund one unit: %v", err)
	}
	if memo.AmountCents != 5_250 {
		t.Errorf("one unit of the taxed line refunds %d, want its 5000 and its 250 of tax", memo.AmountCents)
	}
	h.Settle(t)
}

// Scenario: an edit prices the order again. A line keeps the discount it carried, an added line takes none of it,
// and what a removed line carried moves to the lines that are left - the lines always adding up to the order
func TestAnEditKeepsEachLinesDiscount(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 10_000)
	book := h.AddProduct(t, f, 5_000, true, 50)
	extra := h.AddProduct(t, f, 3_000, true, 50)
	h.newTaxRule(t, f.Channel.ID, 1000)
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E a fifth off one product",
			ChannelID: &f.Channel.ID, IsActive: true,
		},
		Actions: []promotion.Action{{Type: "core.percent_off_matching_items", Params: json.RawMessage(fmt.Sprintf(
			`{"bps":2000,"condition":{"type":"core.cart_contains_sku","params":{"skus":[%q]}}}`, f.Product.SKU))}},
	})
	cartID := h.NewGuestCart(t, f, 2)
	if _, err := h.D.Cart.AddToCart(h.Ctx, cartID, nil, book.SKU, 1); err != nil {
		t.Fatalf("add the second product: %v", err)
	}
	o, err := h.TryPlace(t, f.Channel, cartID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)
	if lines := h.linesAddUp(t, o); lines[0].DiscountCents != 4_000 || lines[1].DiscountCents != 0 {
		t.Fatalf("placed: discounts %d and %d, want 4000 on the discounted product alone", lines[0].DiscountCents, lines[1].DiscountCents)
	}

	added, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{OrderID: o.ID, AddLine: []order.EditAddLine{{ProductSku: extra.SKU, Quantity: 1}}})
	if err != nil {
		t.Fatalf("add a line: %v", err)
	}
	lines := h.linesAddUp(t, added)
	if len(lines) != 3 || lines[2].ProductSku != extra.SKU {
		t.Fatalf("after adding: %d lines, last %q - want the added product last", len(lines), lines[len(lines)-1].ProductSku)
	}
	if lines[0].DiscountCents != 4_000 || lines[1].DiscountCents != 0 || lines[2].DiscountCents != 0 {
		t.Errorf("after adding: discounts %d, %d, %d - want the 4000 where it was", lines[0].DiscountCents, lines[1].DiscountCents, lines[2].DiscountCents)
	}
	if lines[0].TaxCents != 1_600 || lines[1].TaxCents != 500 || lines[2].TaxCents != 300 {
		t.Errorf("after adding: taxes %d, %d, %d - want 10%% of 16000, 5000 and 3000", lines[0].TaxCents, lines[1].TaxCents, lines[2].TaxCents)
	}

	// One of the two discounted units goes: the line keeps half its discount, and the half it gave up is shared by
	// what every line has left (8000, 5000 and 3000), since the order keeps the discount it was given
	halved, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{OrderID: o.ID, UpdateQty: []order.EditLineQty{{OrderLineID: lines[0].ID, Quantity: 1}}})
	if err != nil {
		t.Fatalf("lower a quantity: %v", err)
	}
	lines = h.linesAddUp(t, halved)
	if halved.DiscountAmount != 4_000 || lines[0].DiscountCents != 3_000 || lines[1].DiscountCents != 625 || lines[2].DiscountCents != 375 {
		t.Errorf("after halving: order discount %d, lines %d, %d, %d - want 4000 as 3000, 625 and 375",
			halved.DiscountAmount, lines[0].DiscountCents, lines[1].DiscountCents, lines[2].DiscountCents)
	}
	if lines[0].ProductSku != f.Product.SKU {
		t.Errorf("the edited line moved: first is now %q", lines[0].ProductSku)
	}
	h.Settle(t)
}

// A draft is priced by the admin building it: the discount they typed is shared by line subtotal
func TestADraftsLinesShareItsDiscount(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 10_000)
	book := h.AddProduct(t, f, 5_000, true, 50)
	h.newTaxRule(t, f.Channel.ID, 1000)
	o, err := h.D.Order.CreateDraftOrder(h.Ctx, order.CreateDraftOrderInput{
		ChannelID: f.Channel.ID, Email: "draft-" + uuid.NewString()[:8] + "@e2e.test",
		BillingAddress: *e2eAddress(), ShippingAddress: *e2eAddress(), ManualDiscountCents: 1_500,
		Lines: []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 1}, {ProductSku: book.SKU, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	lines := h.linesAddUp(t, o)
	if len(lines) != 2 || lines[0].DiscountCents != 1_000 || lines[1].DiscountCents != 500 {
		t.Fatalf("draft lines = %+v, want the 1500 as 1000 and 500", lines)
	}
	if lines[0].TaxCents != 900 || lines[1].TaxCents != 450 {
		t.Errorf("draft taxes %d and %d, want 10%% of 9000 and 4500", lines[0].TaxCents, lines[1].TaxCents)
	}
}

// A line an admin prices by hand on a draft says so on the order: who, and what the catalog asked. The line beside it
// is the catalog's and says nothing
func TestADraftsHandPricedLineKeepsItsListPrice(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 50, 10_000)
	book := h.AddProduct(t, f, 5_000, true, 50)
	admin, hand := uuid.New(), int64(8_000)
	o, err := h.D.Order.CreateDraftOrder(h.Ctx, order.CreateDraftOrderInput{
		ChannelID: f.Channel.ID, Email: "draft-" + uuid.NewString()[:8] + "@e2e.test", CreatedBy: &admin,
		BillingAddress: *e2eAddress(), ShippingAddress: *e2eAddress(),
		Lines: []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 1, UnitPriceCents: &hand}, {ProductSku: book.SKU, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	lines := h.OrderLines(t, o.ID)
	if len(lines) != 2 {
		t.Fatalf("%d lines, want 2", len(lines))
	}
	if l := lines[0]; l.PriceSource != "admin" || l.UnitPrice != 8_000 || !l.ListUnitPrice.Valid || l.ListUnitPrice.Int64 != 10_000 || l.PriceSetBy.UUID != admin {
		t.Errorf("hand-priced line %+v, want the admin's 8000 in place of the catalog's 10000", l)
	}
	if l := lines[1]; l.PriceSource != "catalog" || l.ListUnitPrice.Valid || l.PriceSetBy.Valid {
		t.Errorf("catalog line %+v, want nothing kept about its price", l)
	}
	over := int64(1) << 40
	if _, err := h.D.Order.CreateDraftOrder(h.Ctx, order.CreateDraftOrderInput{
		ChannelID: f.Channel.ID, Email: "draft-" + uuid.NewString()[:8] + "@e2e.test",
		BillingAddress: *e2eAddress(), ShippingAddress: *e2eAddress(),
		Lines: []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 1, UnitPriceCents: &over}},
	}); err == nil {
		t.Error("a draft line priced past what a line can cost was accepted")
	}
}
