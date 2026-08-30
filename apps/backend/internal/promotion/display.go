package promotion

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// The slim view the display-matcher uses to ask "would this rule's product-level conditions permit this product?" - built by the storefront resolver from a catalog Product, which keeps promotion free of catalog imports
type ProductCandidate struct {
	ProductSKU   string
	Kind         string // simple / configurable / bundle / grouped
	CategoryIDs  []uuid.UUID
	AttributeKVs map[string]string
}

// Whether the rule's product-level conditions COULD match a hypothetical cart of just this product - the storefront uses it to decide whether to show the rule's label or banner on a listing card or detail page
// Product-level conditions AND together, mirroring the engine's eval loop, and a rule with none at all returns true, so cart-wide promos advertise on every product
// Cart-level conditions - subtotal, qty, customer group, segment, country, store, schedule - ALWAYS return true here, being orthogonal to "does this rule advertise to this product"; the engine still gates the real discount at cart time
// Unknown slugs also return true, so a plugin-supplied condition can't silently hide a rule from display - Composites recurse: any_of ORs, all_of ANDs, not negates
func MatchesProduct(rule Rule, p ProductCandidate) bool {
	for _, c := range rule.Conditions {
		if !matchesProductCondition(c.Type, c.Params, p) {
			return false
		}
	}
	return true
}

// The per-condition decision - true when the condition either permits the product or is cart-level or unknown, and therefore not a display filter
func matchesProductCondition(slug string, raw json.RawMessage, p ProductCandidate) bool {
	switch slug {
	case "core.product_in_category":
		var params struct {
			CategoryIDs []string `json:"category_ids"`
		}
		_ = json.Unmarshal(raw, &params)
		if len(params.CategoryIDs) == 0 {
			return true
		}
		want := make(map[uuid.UUID]struct{}, len(params.CategoryIDs))
		for _, id := range params.CategoryIDs {
			parsed, err := uuid.Parse(id)
			if err != nil {
				continue
			}
			want[parsed] = struct{}{}
		}
		for _, catID := range p.CategoryIDs {
			if _, ok := want[catID]; ok {
				return true
			}
		}
		return false

	case "core.cart_contains_sku":
		var params struct {
			SKUs []string `json:"skus"`
		}
		_ = json.Unmarshal(raw, &params)
		if len(params.SKUs) == 0 {
			return true
		}
		return slices.Contains(params.SKUs, p.ProductSKU)

	case "core.product_kind":
		var params struct {
			Kinds []string `json:"kinds"`
		}
		_ = json.Unmarshal(raw, &params)
		if len(params.Kinds) == 0 {
			return true
		}
		return slices.Contains(params.Kinds, p.Kind)

	case "core.product_has_attribute":
		var params struct {
			Attribute string   `json:"attribute"`
			Values    []string `json:"values"`
		}
		_ = json.Unmarshal(raw, &params)
		if params.Attribute == "" || len(params.Values) == 0 {
			return true
		}
		have, ok := p.AttributeKVs[params.Attribute]
		if !ok {
			return false
		}
		// Split like the condition does, so a badge shows for a product whose
		// multi-valued attribute contains one of the rule's values
		for one := range strings.SplitSeq(have, ",") {
			one = strings.TrimSpace(one)
			if slices.Contains(params.Values, one) {
				return true
			}
		}
		return false

	case "core.all_of":
		var params struct {
			Conditions []struct {
				Type   string          `json:"type"`
				Params json.RawMessage `json:"params"`
			} `json:"conditions"`
		}
		_ = json.Unmarshal(raw, &params)
		for _, child := range params.Conditions {
			if !matchesProductCondition(child.Type, child.Params, p) {
				return false
			}
		}
		return true

	case "core.any_of":
		var params struct {
			Conditions []struct {
				Type   string          `json:"type"`
				Params json.RawMessage `json:"params"`
			} `json:"conditions"`
		}
		_ = json.Unmarshal(raw, &params)
		if len(params.Conditions) == 0 {
			return true
		}
		// If every child is cart-level or unknown they each return true and the OR is true, which is correct - and if any product-level child matches, also true
		for _, child := range params.Conditions {
			if matchesProductCondition(child.Type, child.Params, p) {
				return true
			}
		}
		return false

	case "core.not":
		var params struct {
			Condition struct {
				Type   string          `json:"type"`
				Params json.RawMessage `json:"params"`
			} `json:"condition"`
		}
		_ = json.Unmarshal(raw, &params)
		// Only negate a product-level inner condition - negating a cart-level or unknown one would over-restrict, since "NOT subtotal_range>$50" shouldn't hide the badge from any product
		if isProductLevelSlug(params.Condition.Type) {
			return !matchesProductCondition(params.Condition.Type, params.Condition.Params, p)
		}
		return true

	default:
		// Cart-level conditions plus any unknown or plugin-supplied slug - permissive for display, with the engine still enforcing them at cart time
		return true
	}
}

func isProductLevelSlug(slug string) bool {
	switch slug {
	case "core.product_in_category",
		"core.cart_contains_sku",
		"core.product_kind",
		"core.product_has_attribute",
		"core.all_of",
		"core.any_of",
		"core.not":
		return true
	}
	return false
}
