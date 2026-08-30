package cart

import (
	"testing"

	"github.com/google/uuid"
)

// Two things the cart hands to code that decides money: the line projection the promotion engine
// scores, and the gate that rejects an add with no picks

func TestPromotionLineViewsCarriesEveryFieldTheEngineScoresOn(t *testing.T) {
	// order placement and storefront hydration each had their own copy of this and drifted into
	// pricing the same cart differently - a field silently dropped here is that bug returning
	id := uuid.New()
	c := Cart{Lines: []CartLine{{
		ID:          id,
		ProductSku:  "BOOT-1",
		ProductName: "Chelsea Boot",
		Quantity:    3,
		UnitPrice:   12995,
	}}}

	views := PromotionLineViews(c)
	if len(views) != 1 {
		t.Fatalf("projected %d views from one line", len(views))
	}
	v := views[0]
	if v.LineID != id {
		t.Errorf("LineID = %v, want %v", v.LineID, id)
	}
	if v.ProductSKU != "BOOT-1" {
		t.Errorf("ProductSKU = %q, want BOOT-1 - conditions match on it", v.ProductSKU)
	}
	if v.ProductName != "Chelsea Boot" {
		t.Errorf("ProductName = %q", v.ProductName)
	}
	if v.Quantity != 3 {
		t.Errorf("Quantity = %v, want 3 - tiered and BOGO actions count on it", v.Quantity)
	}
	if v.UnitPriceCents != 12995 {
		t.Errorf("UnitPriceCents = %d, want 12995", v.UnitPriceCents)
	}
}

func TestPromotionLineViewsKeepsOrderAndHandlesAnEmptyCart(t *testing.T) {
	c := Cart{Lines: []CartLine{{ProductSku: "A"}, {ProductSku: "B"}, {ProductSku: "C"}}}
	got := PromotionLineViews(c)
	for i, want := range []string{"A", "B", "C"} {
		if got[i].ProductSKU != want {
			t.Errorf("view %d = %q, want %q", i, got[i].ProductSKU, want)
		}
	}
	if views := PromotionLineViews(Cart{}); len(views) != 0 {
		t.Errorf("empty cart projected %d views", len(views))
	}
}

func TestHasVisiblyRequiredOptionsSeesAPlainRequiredOption(t *testing.T) {
	res := ProductOptionsResolution{Options: []ProductOptionConfig{
		{ID: uuid.New(), IsRequired: false},
		{ID: uuid.New(), IsRequired: true},
	}}
	if !HasVisiblyRequiredOptions(res) {
		t.Error("a required option with no visibility rule should block a no-picks add")
	}
}

func TestHasVisiblyRequiredOptionsIgnoresOneNobodyCanSeeYet(t *testing.T) {
	gate := uuid.New()
	// required, but only once an earlier option is filled - with no picks at all it is hidden, and
	// buying without it is exactly how a customer legitimately would
	res := ProductOptionsResolution{Options: []ProductOptionConfig{
		{ID: gate, IsRequired: false},
		{
			ID:         uuid.New(),
			IsRequired: true,
			VisibleIf:  &OptionVisibilityGroup{Rules: []OptionVisibilityRule{{OptionID: gate, Op: "filled"}}},
		},
	}}
	if HasVisiblyRequiredOptions(res) {
		t.Error("a required option behind an unsatisfied visible_if blocked the add")
	}
}

func TestHasVisiblyRequiredOptionsSaysNoWhenNothingIsRequired(t *testing.T) {
	res := ProductOptionsResolution{Options: []ProductOptionConfig{{ID: uuid.New()}, {ID: uuid.New()}}}
	if HasVisiblyRequiredOptions(res) {
		t.Error("no option is required, so a bare add must be allowed")
	}
	if HasVisiblyRequiredOptions(ProductOptionsResolution{}) {
		t.Error("a product with no options at all blocked a bare add")
	}
}
