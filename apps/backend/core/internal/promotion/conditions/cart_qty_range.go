package conditions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// cartQtyRange matches when the cart's total line quantity falls within [min, max] - Bundle child quantities are
// counted via the parent line's quantity - bundles count as N units, not as the sum of their components
type cartQtyRange struct{}

type cartQtyRangeParams struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

func (cartQtyRange) Slug() string  { return "core.cart_qty_range" }
func (cartQtyRange) Label() string { return "Cart quantity range" }
func (cartQtyRange) Description() string {
	return "Total cart line quantity falls within an optional min/max range."
}

func (cartQtyRange) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "min": {"type": "number", "minimum": 0, "title": "Minimum quantity"},
    "max": {"type": "number", "minimum": 0, "title": "Maximum quantity"}
  },
  "additionalProperties": false
}`)
}

func (cartQtyRange) ValidateParams(raw json.RawMessage) error {
	var p cartQtyRangeParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.Min != nil && *p.Min < 0 {
		return fmt.Errorf("min must be >= 0")
	}
	if p.Max != nil && *p.Max < 0 {
		return fmt.Errorf("max must be >= 0")
	}
	if p.Min != nil && p.Max != nil && *p.Min > *p.Max {
		return fmt.Errorf("min must be <= max")
	}
	return nil
}

func (cartQtyRange) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p cartQtyRangeParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	var total float64
	for _, l := range env.Lines() {
		total += l.Quantity
	}
	if p.Min != nil && total < *p.Min {
		return false, nil
	}
	if p.Max != nil && total > *p.Max {
		return false, nil
	}
	return true, nil
}

func init() {
	add(cartQtyRange{})
}
