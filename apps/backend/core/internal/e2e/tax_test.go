package e2e

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func (h *harness) newTaxRule(t *testing.T, channelID uuid.UUID, bps int, conditions ...tax.Condition) {
	t.Helper()
	if _, err := h.D.Tax.CreateRule(h.Ctx, tax.CreateRuleInput{
		Rule:       tax.Rule{ChannelID: channelID, Name: "E2E tax", IsActive: true},
		Conditions: conditions,
		Effects: []tax.Effect{{Type: "core.percentage",
			Params: json.RawMessage(fmt.Sprintf(`{"bps":%d}`, bps))}},
	}); err != nil {
		t.Fatalf("create tax rule: %v", err)
	}
}

// Scenario: tax-exclusive pricing adds the computed tax on top - 8.75% on a US order lands on
// TaxAmount, in the total, and as a snapshot tax line carrying the rate
func TestTaxExclusiveAddsOnTop(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.newTaxRule(t, f.Channel.ID, 875, tax.Condition{
		Type: "core.jurisdiction", Params: json.RawMessage(`{"country_code":"US"}`),
	})

	o := h.PlaceGuestCOD(t, f, 1)
	if o.TaxAmount != 175 { // round(2000 * 875 / 10000)
		t.Errorf("tax = %d, want 175", o.TaxAmount)
	}
	if o.TotalAmount != 2175 {
		t.Errorf("total = %d, want subtotal 2000 + tax 175", o.TotalAmount)
	}
	taxLines, err := h.D.Order.TaxLines(h.Ctx, o.ID)
	if err != nil || len(taxLines) != 1 || taxLines[0].RateBPS != 875 || taxLines[0].AmountCents != 175 {
		t.Errorf("tax lines = %+v (%v), want one snapshot at 875bps/175", taxLines, err)
	}
	h.Settle(t)
}

// Scenario: a tax-inclusive channel keeps the shelf price fixed and carves the tax OUT of it -
// the total equals the subtotal and TaxAmount reports the extracted share
func TestTaxInclusiveCarvesOutOfTotal(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.newTaxRule(t, f.Channel.ID, 875)

	// No Go setter exists for the flag - flip the column and re-read (GetByID is uncached)
	if _, err := h.D.DB.ExecContext(h.Ctx,
		`UPDATE channels SET tax_inclusive_pricing = TRUE WHERE id = $1`, f.Channel.ID); err != nil {
		t.Fatalf("mark channel inclusive: %v", err)
	}
	ch, err := h.D.Channel.GetByID(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("re-read channel: %v", err)
	}

	o, err := h.TryPlace(t, ch, h.NewGuestCart(t, f, 1))
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if o.TotalAmount != 2000 {
		t.Errorf("total = %d, want the fixed shelf 2000 - inclusive tax must not add", o.TotalAmount)
	}
	if o.TaxAmount != 161 { // round(2000 * 875 / 10875) - the extraction formula
		t.Errorf("carved-out tax = %d, want 161", o.TaxAmount)
	}
	h.Settle(t)
}

// Scenario: tax-class scoping - a reduced-class rule ignores standard products and fires once the
// product is reclassified
func TestTaxClassScopesRules(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	h.newTaxRule(t, f.Channel.ID, 500, tax.Condition{
		Type: "core.product_tax_class", Params: json.RawMessage(`{"classes":["reduced"]}`),
	})

	standard := h.PlaceGuestCOD(t, f, 1)
	if standard.TaxAmount != 0 {
		t.Errorf("standard-class product taxed %d by a reduced-class rule, want 0", standard.TaxAmount)
	}

	if _, err := h.D.Catalog.SetProductTaxClass(h.Ctx, f.Product.ID, "reduced"); err != nil {
		t.Fatalf("reclassify: %v", err)
	}
	reduced := h.PlaceGuestCOD(t, f, 1)
	if reduced.TaxAmount != 50 {
		t.Errorf("reduced-class product taxed %d, want 50", reduced.TaxAmount)
	}
	h.Settle(t)
}

func (h *harness) setChannelConfig(t *testing.T, f *store, path, val string) {
	t.Helper()
	if _, err := h.D.Config.Set(h.Ctx, path, val, config.Scope{Kind: config.ScopeChannel, ChannelID: &f.Channel.ID}); err != nil {
		t.Fatalf("set %s: %v", path, err)
	}
}

// Scenario: shipping is taxed by default, alongside the goods, and the channel can turn it off
func TestTaxAppliesToShipping(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	h.newTaxRule(t, f.Channel.ID, 1000)

	o := h.PlaceGuestCOD(t, f, 1)
	if o.TaxAmount != 250 {
		t.Errorf("tax = %d, want 10%% of goods 2000 + shipping 500 = 250", o.TaxAmount)
	}
	if o.TotalAmount != 2750 {
		t.Errorf("total = %d, want 2000 + 500 + 250", o.TotalAmount)
	}
	if o.ShippingTaxAmount != 50 {
		t.Errorf("shipping tax = %d, want 50 of the 250", o.ShippingTaxAmount)
	}
	taxLines, err := h.D.Order.TaxLines(h.Ctx, o.ID)
	if err != nil || len(taxLines) != 1 || taxLines[0].AmountCents != 250 {
		t.Errorf("tax lines = %+v (%v), want one line of 250", taxLines, err)
	}

	h.setChannelConfig(t, f, "sales/checkout/shipping_taxable", "false")
	exempt := h.PlaceGuestCOD(t, f, 1)
	if exempt.TaxAmount != 200 || exempt.TotalAmount != 2700 || exempt.ShippingTaxAmount != 0 {
		t.Errorf("shipping-exempt tax/total/shipping tax = %d/%d/%d, want 200/2700/0", exempt.TaxAmount, exempt.TotalAmount, exempt.ShippingTaxAmount)
	}
	h.Settle(t)
}

// Scenario: shipping takes the configured tax class, so a class-scoped rule taxes it and not the goods
func TestShippingTaxClassScopesRules(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	h.newTaxRule(t, f.Channel.ID, 500, tax.Condition{
		Type: "core.product_tax_class", Params: json.RawMessage(`{"classes":["freight"]}`),
	})

	unclassed := h.PlaceGuestCOD(t, f, 1)
	if unclassed.TaxAmount != 0 {
		t.Errorf("tax = %d, want 0 while shipping has no class", unclassed.TaxAmount)
	}
	h.setChannelConfig(t, f, "sales/checkout/shipping_tax_class", "freight")
	freight := h.PlaceGuestCOD(t, f, 1)
	if freight.TaxAmount != 25 || freight.ShippingTaxAmount != 25 {
		t.Errorf("tax = %d, want 5%% of shipping 500 = 25 with the goods untouched", freight.TaxAmount)
	}
	h.Settle(t)
}

// Scenario: an admin draft taxes its shipping like a storefront order
func TestDraftOrderTaxesShipping(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	h.newTaxRule(t, f.Channel.ID, 1000)

	d := h.newDraft(t, f, []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 1}})
	if d.ShippingAmount != 500 || d.TaxAmount != 250 || d.ShippingTaxAmount != 50 || d.TotalAmount != 2750 {
		t.Errorf("draft shipping/tax/shipping tax/total = %d/%d/%d/%d, want 500/250/50/2750", d.ShippingAmount, d.TaxAmount, d.ShippingTaxAmount, d.TotalAmount)
	}
}

// Scenario: both edit recomputes keep taxing the shipping they re-quote
func TestEditsKeepTaxingShipping(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2000)
	h.addFlatShipping(t, f, 500, 0)
	h.newTaxRule(t, f.Channel.ID, 1000)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)

	edited, err := h.D.Order.EditOrder(h.Ctx, order.EditOrderInput{
		OrderID:   o.ID,
		UpdateQty: []order.EditLineQty{{OrderLineID: h.OrderLines(t, o.ID)[0].ID, Quantity: 2}},
	})
	if err != nil {
		t.Fatalf("EditOrder: %v", err)
	}
	if edited.TaxAmount != 450 || edited.ShippingTaxAmount != 50 {
		t.Errorf("tax after edit = %d, want 10%% of goods 4000 + shipping 500 = 450", edited.TaxAmount)
	}

	moved, err := h.D.Order.EditOrderAddress(h.Ctx, order.EditOrderAddressInput{
		OrderID:  o.ID,
		Shipping: editAddr("Ada", "Lovelace", "Boston", "02101"),
	})
	if err != nil {
		t.Fatalf("EditOrderAddress: %v", err)
	}
	if moved.TaxAmount != 450 || moved.ShippingTaxAmount != 50 {
		t.Errorf("tax after address move = %d, want 450", moved.TaxAmount)
	}
	h.Settle(t)
}

// Scenario: line refunds hand back the goods' tax only, and shipping's tax comes back with the shipping refund
func TestRefundsKeepShippingTaxWithShipping(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2000)
	h.addFlatShipping(t, f, 500, 0)
	h.newTaxRule(t, f.Channel.ID, 1000)
	o := h.PlaceGuestCOD(t, f, 2)
	h.Settle(t)
	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID, Method: "cod", Reference: "e2e-cash",
	}); err != nil {
		t.Fatalf("collect in full: %v", err)
	}

	if _, err := h.D.Order.RecordCreditMemoIntent(h.Ctx, order.IssueCreditMemoInput{
		OrderID: o.ID, ShippingRefundCents: 551,
	}); apperr.CodeOf(err) != "CREDIT_MEMO_SHIPPING_EXCEEDED" {
		t.Errorf("shipping refund over 500 + 50 tax: err = %v, want CREDIT_MEMO_SHIPPING_EXCEEDED", err)
	}

	intent, err := h.D.Order.RecordCreditMemoIntent(h.Ctx, order.IssueCreditMemoInput{
		OrderID:             o.ID,
		Lines:               []order.CreditMemoLineInput{{OrderLineID: h.OrderLines(t, o.ID)[0].ID, Quantity: 2}},
		ShippingRefundCents: 550,
	})
	if err != nil {
		t.Fatalf("memo intent: %v", err)
	}
	if intent.Memo.LinesTotalCents != 4400 {
		t.Errorf("lines refund = %d, want 2 x (2000 + 200 tax) = 4400", intent.Memo.LinesTotalCents)
	}
	if intent.AmountCents != o.TotalAmount {
		t.Errorf("full refund = %d, want the order total %d", intent.AmountCents, o.TotalAmount)
	}
	h.Settle(t)
}
