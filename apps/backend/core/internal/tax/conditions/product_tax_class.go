package conditions

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
)

// productTaxClass matches when the line's product tax class is one of the configured classes - An empty list matches
// any class (so a rule without this condition - or with an empty one - taxes every line)
type productTaxClass struct{}

type productTaxClassParams struct {
	Classes []string `json:"classes"`
}

func (productTaxClass) Slug() string  { return "core.product_tax_class" }
func (productTaxClass) Label() string { return "Product tax class" }
func (productTaxClass) Description() string {
	return "Matches when the line's product tax class is one of the listed classes (e.g. reduced, zero, exempt). Empty = any."
}

func (productTaxClass) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "classes": {
      "type": "array",
      "title": "Tax classes (empty = any)",
      "items": {"type": "string"}
    }
  },
  "additionalProperties": false
}`)
}

func (productTaxClass) ValidateParams(raw json.RawMessage) error {
	var p productTaxClassParams
	return unmarshalParams(raw, &p)
}

func (productTaxClass) Evaluate(_ context.Context, env tax.Environment, raw json.RawMessage) (bool, error) {
	var p productTaxClassParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	if len(p.Classes) == 0 {
		return true, nil
	}
	got := env.ProductTaxClass()
	if slices.Contains(p.Classes, got) {
		return true, nil
	}
	return false, nil
}

func init() { add(productTaxClass{}) }
