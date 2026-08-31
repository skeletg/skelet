package conditions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/internal/promotion"
)

// cartSubtotalRange matches when the cart's subtotal is within [min_cents, max_cents] - Either bound is optional; with neither set the
// condition is a no-op (always true), which the admin UI should flag - there's no reason to add such a row
type cartSubtotalRange struct{}

type cartSubtotalRangeParams struct {
	MinCents *int64 `json:"min_cents,omitempty"`
	MaxCents *int64 `json:"max_cents,omitempty"`
}

func (cartSubtotalRange) Slug() string  { return "core.cart_subtotal_range" }
func (cartSubtotalRange) Label() string { return "Cart subtotal range" }
func (cartSubtotalRange) Description() string {
	return "Cart subtotal falls within an optional min/max range (in cents)."
}

func (cartSubtotalRange) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "min_cents": {"type": "integer", "minimum": 0, "title": "Minimum subtotal (cents)"},
    "max_cents": {"type": "integer", "minimum": 0, "title": "Maximum subtotal (cents)"}
  },
  "additionalProperties": false
}`)
}

func (c cartSubtotalRange) ValidateParams(raw json.RawMessage) error {
	var p cartSubtotalRangeParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.MinCents != nil && *p.MinCents < 0 {
		return fmt.Errorf("min_cents must be >= 0")
	}
	if p.MaxCents != nil && *p.MaxCents < 0 {
		return fmt.Errorf("max_cents must be >= 0")
	}
	if p.MinCents != nil && p.MaxCents != nil && *p.MinCents > *p.MaxCents {
		return fmt.Errorf("min_cents must be <= max_cents")
	}
	return nil
}

func (c cartSubtotalRange) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p cartSubtotalRangeParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	sub := env.SubtotalCents()
	if p.MinCents != nil && sub < *p.MinCents {
		return false, nil
	}
	if p.MaxCents != nil && sub > *p.MaxCents {
		return false, nil
	}
	return true, nil
}

func init() {
	add(cartSubtotalRange{})
}
