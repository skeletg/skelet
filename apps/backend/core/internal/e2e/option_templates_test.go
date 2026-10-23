package e2e

import (
	"fmt"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/e2ekit"
)

// Scenario: a configurator template's required choice and its rule price a product that uses it - a plain add can't
// skip the template's required option, an edit to the template reaches the next add, and the order keeps what was picked
func TestAConfiguratorTemplatePricesTheProductsUsingIt(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2000)
	tpl, err := h.D.Catalog.CreateOptionTemplate(h.Ctx, catalog.OptionTemplateInput{Name: "Gift wrap " + uuid.NewString()[:6], CurrencyCode: f.Channel.CurrencyCode})
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	wrap, err := h.D.Catalog.AddProductOption(h.Ctx, catalog.AddProductOptionInput{TemplateID: tpl.ID, InputType: catalog.OptionInputRadio, IsRequired: true, Locale: "en", Title: "Wrapping"})
	if err != nil {
		t.Fatalf("template option: %v", err)
	}
	gift, err := h.D.Catalog.AddProductOptionValue(h.Ctx, catalog.AddProductOptionValueInput{OptionID: wrap.ID, Locale: "en", Title: "Gift", PriceType: catalog.OptionPriceFixed, PriceValue: 300, Quantity: 1})
	if err != nil {
		t.Fatalf("template choice: %v", err)
	}
	if _, err := h.D.Catalog.SetOptionTemplateRules(h.Ctx, tpl.ID, []catalog.ProductOptionRuleInput{{
		Labels:    map[string]string{"en": "Wrap promo"},
		Condition: catalog.OptionConditionGroup{Rules: []catalog.OptionRule{{OptionID: wrap.ID, Op: catalog.OptionRuleAnyOf, ValueIDs: []uuid.UUID{gift.ID}}}},
		Actions:   []catalog.ProductOptionRuleAction{{Type: catalog.RuleActionAdjustPrice, PriceType: catalog.OptionPriceFixed, PriceValue: -100}},
	}}); err != nil {
		t.Fatalf("template rule: %v", err)
	}
	if err := h.D.Catalog.AssignOptionTemplate(h.Ctx, tpl.ID, []uuid.UUID{f.Product.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddToCart(h.Ctx, c.ID, nil, f.Product.SKU, 1); err == nil {
		t.Errorf("a plain add went through - the template's required wrapping was skipped")
	}
	pick := []cart.CartLineOptionInput{{OptionID: wrap.ID, ValueIDs: []uuid.UUID{gift.ID}}}
	got, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{ProductSku: f.Product.SKU, Quantity: 1, Options: pick})
	if err != nil {
		t.Fatalf("add configured: %v", err)
	}
	if got.Lines[0].UnitPrice != 2000+300-100 {
		t.Errorf("unit price = %d, want 2200 - the base, the template's choice and its rule", got.Lines[0].UnitPrice)
	}

	charge := int32(450)
	if _, err := h.D.Catalog.UpdateProductOptionValue(h.Ctx, catalog.UpdateProductOptionValueInput{ID: gift.ID, PriceValue: &charge}); err != nil {
		t.Fatalf("edit the template: %v", err)
	}
	c2, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	again, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c2.ID, nil, cart.AddConfiguredToCartInput{ProductSku: f.Product.SKU, Quantity: 1, Options: pick})
	if err != nil {
		t.Fatalf("add after the edit: %v", err)
	}
	if again.Lines[0].UnitPrice != 2000+450-100 {
		t.Errorf("unit price after the template edit = %d, want 2350", again.Lines[0].UnitPrice)
	}

	o, err := h.TryPlace(t, f.Channel, c.ID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)
	if o.SubtotalAmount != 2200 {
		t.Errorf("order subtotal = %d, want 2200 - the cart priced before the edit keeps its price", o.SubtotalAmount)
	}
	reread, err := h.D.Order.GetByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	var picked, promo bool
	for _, row := range reread.Lines[0].Options {
		picked = picked || (row.OptionTitle == "Wrapping" && row.ValueLabel == "Gift")
		promo = promo || (row.InputType == cart.OptionRuleInputType && row.OptionTitle == "Wrap promo")
	}
	if !picked || !promo {
		t.Errorf("order line options = %+v, want the template's Gift pick and its Wrap promo row", reread.Lines[0].Options)
	}
}

// A configurable and its one variant (the store's product) sharing a product type that hands them a required care
// plan - a 300 choice with a 50 rule on it
type typeTemplateFamily struct {
	*e2ekit.Store
	parent catalog.Product
	care   catalog.ProductOption
	yearly catalog.ProductOptionValue
	fee    catalog.ProductOptionRule
}

func newTypeTemplateFamily(t *testing.T, h *harness) typeTemplateFamily {
	t.Helper()
	f := h.NewStore(t, 20, 2000)
	sfx := uuid.NewString()[:6]
	color, err := h.D.Catalog.CreateAttribute(h.Ctx, catalog.CreateAttributeInput{
		Slug: "e2e-color-" + sfx, Name: "Color", InputType: catalog.InputTypeDropdown, IsPublic: true,
		Values: []catalog.AttributeValueInput{{Slug: "red", Name: "Red"}},
	})
	if err != nil {
		t.Fatalf("attribute: %v", err)
	}
	if _, err := h.D.Catalog.AssignAttribute(h.Ctx, catalog.AssignAttributeInput{ProductTypeID: f.Product.ProductTypeID, AttributeID: color.ID}); err != nil {
		t.Fatalf("assign attribute: %v", err)
	}
	full, err := h.D.Catalog.GetAttribute(h.Ctx, color.ID)
	if err != nil {
		t.Fatalf("attribute values: %v", err)
	}
	parent, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
		ProductTypeID: f.Product.ProductTypeID, Kind: catalog.KindConfigurable, Locale: "en", Name: "E2E Shirt " + sfx,
		SKU: "e2e-shirt-" + sfx, DefaultCurrencyCode: "USD", DefaultPrice: 2000,
	})
	if err != nil {
		t.Fatalf("configurable: %v", err)
	}
	if _, err := h.D.Catalog.SetConfigurableOptions(h.Ctx, parent.ID, []uuid.UUID{color.ID}); err != nil {
		t.Fatalf("configurable axis: %v", err)
	}
	if _, err := h.D.Catalog.SetProductAttributes(h.Ctx, f.Product.ID, []catalog.AttributeAssignment{{AttributeID: color.ID, ValueIDs: []uuid.UUID{full.Values[0].ID}}}); err != nil {
		t.Fatalf("variant colour: %v", err)
	}
	if _, err := h.D.Catalog.AddProductChild(h.Ctx, parent.ID, f.Product.ID, 0); err != nil {
		t.Fatalf("link variant: %v", err)
	}

	tpl, err := h.D.Catalog.CreateOptionTemplate(h.Ctx, catalog.OptionTemplateInput{Name: "Care plan " + sfx, CurrencyCode: f.Channel.CurrencyCode})
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	care, err := h.D.Catalog.AddProductOption(h.Ctx, catalog.AddProductOptionInput{TemplateID: tpl.ID, InputType: catalog.OptionInputRadio, IsRequired: true, Locale: "en", Title: "Care plan"})
	if err != nil {
		t.Fatalf("template option: %v", err)
	}
	yearly, err := h.D.Catalog.AddProductOptionValue(h.Ctx, catalog.AddProductOptionValueInput{OptionID: care.ID, Locale: "en", Title: "Yearly", PriceType: catalog.OptionPriceFixed, PriceValue: 300, Quantity: 1})
	if err != nil {
		t.Fatalf("template choice: %v", err)
	}
	rules, err := h.D.Catalog.SetOptionTemplateRules(h.Ctx, tpl.ID, []catalog.ProductOptionRuleInput{{
		Labels:    map[string]string{"en": "Care fee"},
		Condition: catalog.OptionConditionGroup{Rules: []catalog.OptionRule{{OptionID: care.ID, Op: catalog.OptionRuleAnyOf, ValueIDs: []uuid.UUID{yearly.ID}}}},
		Actions:   []catalog.ProductOptionRuleAction{{Type: catalog.RuleActionAdjustPrice, PriceType: catalog.OptionPriceFixed, PriceValue: 50}},
	}})
	if err != nil {
		t.Fatalf("template rule: %v", err)
	}
	if err := h.D.Catalog.SetProductTypeOptionTemplates(h.Ctx, f.Product.ProductTypeID, []uuid.UUID{tpl.ID}); err != nil {
		t.Fatalf("type templates: %v", err)
	}
	return typeTemplateFamily{Store: f, parent: parent, care: care, yearly: yearly, fee: rules[0]}
}

// Scenario: a product type hands a required, charged template option to a configurable and its variant alike - the
// cart asks and charges it once, not once for the variant and again for the parent, and refuses a plain add without it
func TestAProductTypesTemplateChargesAConfigurableFamilyOnce(t *testing.T) {
	h := newHarness(t)
	f := newTypeTemplateFamily(t, h)
	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddToCart(h.Ctx, c.ID, nil, f.Product.SKU, 1); err == nil {
		t.Errorf("a plain add of the variant went through - the type's required care plan was skipped")
	}
	got, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 1, Options: []cart.CartLineOptionInput{{OptionID: f.care.ID, ValueIDs: []uuid.UUID{f.yearly.ID}}},
	})
	if err != nil {
		t.Fatalf("add configured: %v", err)
	}
	if got.Lines[0].UnitPrice != 2000+300+50 {
		t.Errorf("unit price = %d, want 2350 - the base, the care plan and its fee once each", got.Lines[0].UnitPrice)
	}
}

// Scenario: the variant's product page over the real front API asks the care plan and shows its fee rule once, though
// both the variant and its parent carry them through the type - the PDP would otherwise render the question twice
func TestAProductTypesTemplateShowsOnceOnAVariantsPage(t *testing.T) {
	h := newHarness(t)
	f := newTypeTemplateFamily(t, h)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`{ product(id: %q) { options { id } optionRules { id } } }`, f.Product.ID))
	if len(errs) > 0 {
		t.Fatalf("product: %v", errs)
	}
	product, _ := data["product"].(map[string]any)
	ids := func(field string) []string {
		var out []string
		list, _ := product[field].([]any)
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				out = append(out, fmt.Sprint(m["id"]))
			}
		}
		return out
	}
	if got := ids("options"); !slices.Equal(got, []string{f.care.ID.String()}) {
		t.Errorf("variant options = %v, want the care plan once", got)
	}
	if got := ids("optionRules"); !slices.Equal(got, []string{f.fee.ID.String()}) {
		t.Errorf("variant rules = %v, want the care fee once", got)
	}
}
