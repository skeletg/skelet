package conditions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/internal/promotion"
)

// cartContainsSku matches when the cart contains at least one of N listed SKUs (optionally with at least min_qty units
// across the matching lines) - Used by rules like "spend $100 on widgets to unlock free gadget shipping"
type cartContainsSku struct{}

type cartContainsSkuParams struct {
	SKUs   []string `json:"skus"`
	MinQty *float64 `json:"min_qty,omitempty"`
}

func (cartContainsSku) Slug() string  { return "core.cart_contains_sku" }
func (cartContainsSku) Label() string { return "Cart contains SKU" }
func (cartContainsSku) Description() string {
	return "Cart contains at least one of the listed SKUs (optionally meeting a minimum aggregate quantity)."
}

func (cartContainsSku) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["skus"],
  "properties": {
    "skus": {
      "type": "array",
      "items": {"type": "string"},
      "minItems": 1,
      "title": "SKUs",
      "x-widget": "sku-picker"
    },
    "min_qty": {"type": "integer", "minimum": 1, "title": "Minimum aggregate quantity"}
  },
  "additionalProperties": false
}`)
}

func (cartContainsSku) ValidateParams(raw json.RawMessage) error {
	var p cartContainsSkuParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if len(p.SKUs) == 0 {
		return fmt.Errorf("skus is required and must have at least one entry")
	}
	if p.MinQty != nil && *p.MinQty <= 0 {
		return fmt.Errorf("min_qty must be > 0 when set")
	}
	return nil
}

func (cartContainsSku) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p cartContainsSkuParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	want := make(map[string]struct{}, len(p.SKUs))
	for _, s := range p.SKUs {
		want[s] = struct{}{}
	}
	var matchedQty float64
	for _, line := range env.Lines() {
		if _, hit := want[line.ProductSKU]; !hit {
			continue
		}
		matchedQty += line.Quantity
	}
	if matchedQty == 0 {
		return false, nil
	}
	if p.MinQty != nil && matchedQty < *p.MinQty {
		return false, nil
	}
	return true, nil
}

func init() {
	add(cartContainsSku{})
}
