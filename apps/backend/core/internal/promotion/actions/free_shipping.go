package actions

import (
	"context"
	"encoding/json"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// freeShipping zeros out the shipping line - Optional method_slugs restrict the free-shipping benefit to specific
// shipping methods (e.g. "free standard shipping but not express") - Empty list means all methods
type freeShipping struct{}

type freeShippingParams struct {
	MethodSlugs []string `json:"method_slugs,omitempty"`
}

func (freeShipping) Slug() string  { return "core.free_shipping" }
func (freeShipping) Label() string { return "Free shipping" }
func (freeShipping) Description() string {
	return "Zero shipping cost on the cart; optionally restricted to specific shipping methods."
}

func (freeShipping) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "method_slugs": {
      "type": "array",
      "items": {"type": "string"},
      "title": "Eligible shipping method slugs (empty = all)"
    }
  },
  "additionalProperties": false
}`)
}

func (freeShipping) ValidateParams(raw json.RawMessage) error {
	var p freeShippingParams
	return unmarshalParams(raw, &p)
}

func (freeShipping) Apply(ctx context.Context, env promotion.Environment, raw json.RawMessage, ledger promotion.Ledger, _ promotion.AppliedRuleContext) error {
	var p freeShippingParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	ledger.SetFreeShipping(p.MethodSlugs)
	return nil
}

func init() {
	add(freeShipping{})
}
