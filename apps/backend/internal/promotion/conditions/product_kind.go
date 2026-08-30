package conditions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/internal/promotion"
)

// productKind matches when any cart line is of one of the listed product kinds ("simple", "configurable", "bundle",
// "grouped") Used by rules like "10% off any bundle" or "free shipping on simple products only"
type productKind struct{}

type productKindParams struct {
	Kinds []string `json:"kinds"`
}

func (productKind) Slug() string  { return "core.product_kind" }
func (productKind) Label() string { return "Cart contains product of kind" }
func (productKind) Description() string {
	return "Any cart line is of one of the listed product kinds (simple, configurable, bundle, grouped)."
}

func (productKind) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["kinds"],
  "properties": {
    "kinds": {
      "type": "array",
      "items": {"type": "string", "enum": ["simple", "configurable", "bundle", "grouped"]},
      "minItems": 1,
      "title": "Product kinds"
    }
  },
  "additionalProperties": false
}`)
}

func (productKind) ValidateParams(raw json.RawMessage) error {
	var p productKindParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if len(p.Kinds) == 0 {
		return fmt.Errorf("kinds is required and must have at least one entry")
	}
	allowed := map[string]struct{}{"simple": {}, "configurable": {}, "bundle": {}, "grouped": {}}
	for _, k := range p.Kinds {
		if _, ok := allowed[k]; !ok {
			return fmt.Errorf("unknown product kind %q", k)
		}
	}
	return nil
}

func (productKind) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p productKindParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	want := make(map[string]struct{}, len(p.Kinds))
	for _, k := range p.Kinds {
		want[k] = struct{}{}
	}
	for _, line := range env.Lines() {
		if _, hit := want[line.ProductKind]; hit {
			return true, nil
		}
	}
	return false, nil
}

func init() {
	add(productKind{})
}
