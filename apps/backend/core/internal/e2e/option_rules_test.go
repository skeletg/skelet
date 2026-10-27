package e2e

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

// Scenario: an option rule prices a combination and fixes its quantity - the discount lands on the line as its own
// labelled row, survives placement into the order total, and a quantity edit can't undo the lock
func TestAnOptionRulePricesTheLineAndLocksItsQuantity(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2000)
	opt, values := h.addSelectOption(t, f.Product.ID, "Pack", true, 0, 500)
	pair := values[1]
	two := 2.0
	if _, err := h.D.Catalog.SetProductOptionRules(h.Ctx, f.Product.ID, []catalog.ProductOptionRuleInput{{
		Labels:    map[string]string{"en": "Pair deal"},
		Condition: catalog.OptionConditionGroup{Rules: []catalog.OptionRule{{OptionID: opt.ID, Op: catalog.OptionRuleAnyOf, ValueIDs: []uuid.UUID{pair.ID}}}},
		Actions: []catalog.ProductOptionRuleAction{
			{Type: catalog.RuleActionSetQuantity, Quantity: &two},
			{Type: catalog.RuleActionAdjustPrice, PriceType: catalog.OptionPriceFixed, PriceValue: -300},
		},
	}}); err != nil {
		t.Fatalf("save rule: %v", err)
	}

	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	got, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, nil, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 1,
		Options: []cart.CartLineOptionInput{{OptionID: opt.ID, ValueIDs: []uuid.UUID{pair.ID}}},
	})
	if err != nil {
		t.Fatalf("add configured: %v", err)
	}
	line := got.Lines[0]
	if line.Quantity != 2 {
		t.Errorf("line quantity = %v, want the rule's 2", line.Quantity)
	}
	if line.UnitPrice != 2000+500-300 {
		t.Errorf("unit price = %d, want 2200 - the base, the choice and the rule", line.UnitPrice)
	}
	if _, err := h.D.Cart.UpdateCartLine(h.Ctx, c.ID, line.ID, nil, 3); !errors.Is(err, cart.ErrProductOptionQtyForced) {
		t.Errorf("quantity edit = %v, want ErrProductOptionQtyForced - the rule's lock outlives the add", err)
	}

	o, err := h.TryPlace(t, f.Channel, c.ID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)
	if o.SubtotalAmount != 2*2200 {
		t.Errorf("order subtotal = %d, want %d", o.SubtotalAmount, 2*2200)
	}
	reread, err := h.D.Order.GetByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	var ruleRows int
	for _, row := range reread.Lines[0].Options {
		if row.InputType == cart.OptionRuleInputType {
			ruleRows++
			if row.OptionTitle != "Pair deal" || row.PriceDelta != -300 {
				t.Errorf("rule row = %+v, want Pair deal at -300", row)
			}
		}
	}
	if ruleRows != 1 {
		t.Errorf("%d rule rows on the order line, want 1", ruleRows)
	}
}
