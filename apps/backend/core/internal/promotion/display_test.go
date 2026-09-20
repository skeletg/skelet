package promotion_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

func TestMatchesProduct_CategoryIn(t *testing.T) {
	catA := uuid.New()
	catB := uuid.New()
	rule := promotion.Rule{
		Conditions: []promotion.Condition{{
			Type:   "core.product_in_category",
			Params: json.RawMessage(`{"category_ids":["` + catA.String() + `"]}`),
		}},
	}
	if !promotion.MatchesProduct(rule, promotion.ProductCandidate{CategoryIDs: []uuid.UUID{catA}}) {
		t.Error("expected match when product is in category A")
	}
	if promotion.MatchesProduct(rule, promotion.ProductCandidate{CategoryIDs: []uuid.UUID{catB}}) {
		t.Error("expected no match when product is in different category")
	}
	if promotion.MatchesProduct(rule, promotion.ProductCandidate{}) {
		t.Error("expected no match when product has no categories")
	}
}

func TestMatchesProduct_KindAndSKU(t *testing.T) {
	rule := promotion.Rule{
		Conditions: []promotion.Condition{{
			Type:   "core.product_kind",
			Params: json.RawMessage(`{"kinds":["simple"]}`),
		}},
	}
	if !promotion.MatchesProduct(rule, promotion.ProductCandidate{Kind: "simple"}) {
		t.Error("expected simple to match")
	}
	if promotion.MatchesProduct(rule, promotion.ProductCandidate{Kind: "configurable"}) {
		t.Error("configurable should not match")
	}

	skuRule := promotion.Rule{
		Conditions: []promotion.Condition{{
			Type:   "core.cart_contains_sku",
			Params: json.RawMessage(`{"skus":["TEE-CLS-001"]}`),
		}},
	}
	if !promotion.MatchesProduct(skuRule, promotion.ProductCandidate{ProductSKU: "TEE-CLS-001"}) {
		t.Error("expected SKU match")
	}
	if promotion.MatchesProduct(skuRule, promotion.ProductCandidate{ProductSKU: "OTHER"}) {
		t.Error("expected non-match for different SKU")
	}
}

func TestMatchesProduct_CartLevelConditionsIgnored(t *testing.T) {
	// Rule with only cart-level conditions must match every product -
	// display is informational, the engine still enforces at cart time
	rule := promotion.Rule{
		Conditions: []promotion.Condition{
			{Type: "core.cart_subtotal_range", Params: json.RawMessage(`{"min_cents":5000}`)},
			{Type: "core.customer_group_in", Params: json.RawMessage(`{"group_slugs":["vip"]}`)},
		},
	}
	if !promotion.MatchesProduct(rule, promotion.ProductCandidate{ProductSKU: "anything"}) {
		t.Error("cart-level conditions should not filter out display matches")
	}
}

func TestMatchesProduct_AnyOf(t *testing.T) {
	catA := uuid.New()
	rule := promotion.Rule{
		Conditions: []promotion.Condition{{
			Type: "core.any_of",
			Params: json.RawMessage(`{"conditions":[
				{"type":"core.product_in_category","params":{"category_ids":["` + catA.String() + `"]}},
				{"type":"core.product_kind","params":{"kinds":["bundle"]}}
			]}`),
		}},
	}
	// Match via category
	if !promotion.MatchesProduct(rule, promotion.ProductCandidate{CategoryIDs: []uuid.UUID{catA}}) {
		t.Error("expected match via category")
	}
	// Match via kind
	if !promotion.MatchesProduct(rule, promotion.ProductCandidate{Kind: "bundle"}) {
		t.Error("expected match via kind")
	}
	// No match
	if promotion.MatchesProduct(rule, promotion.ProductCandidate{Kind: "simple"}) {
		t.Error("expected no match for non-bundle, non-category-A product")
	}
}

func TestMatchesProduct_NoConditionsMatchesAll(t *testing.T) {
	rule := promotion.Rule{}
	if !promotion.MatchesProduct(rule, promotion.ProductCandidate{ProductSKU: "ANY"}) {
		t.Error("a rule with no conditions should advertise on every product")
	}
}
