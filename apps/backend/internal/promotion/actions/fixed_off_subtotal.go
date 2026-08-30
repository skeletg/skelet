package actions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/internal/promotion"
)

// fixedOffSubtotal applies a fixed cents discount to the cart subtotal - The discount is clamped to the subtotal so it
// can never make the cart go negative
type fixedOffSubtotal struct{}

type fixedOffSubtotalParams struct {
	Cents int64 `json:"cents"`
}

func (fixedOffSubtotal) Slug() string  { return "core.fixed_off_subtotal" }
func (fixedOffSubtotal) Label() string { return "Fixed off subtotal" }
func (fixedOffSubtotal) Description() string {
	return "Fixed cents off cart subtotal (clamped to subtotal)."
}

func (fixedOffSubtotal) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["cents"],
  "properties": {
    "cents": {"type": "integer", "minimum": 1, "title": "Discount amount (cents)"}
  },
  "additionalProperties": false
}`)
}

func (fixedOffSubtotal) ValidateParams(raw json.RawMessage) error {
	var p fixedOffSubtotalParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.Cents <= 0 {
		return fmt.Errorf("cents must be > 0")
	}
	return nil
}

func (fixedOffSubtotal) Apply(ctx context.Context, env promotion.Environment, raw json.RawMessage, ledger promotion.Ledger, _ promotion.AppliedRuleContext) error {
	var p fixedOffSubtotalParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	discount := min(p.Cents, env.SubtotalCents())
	ledger.AddSubtotalDiscount(discount)
	return nil
}

func init() {
	add(fixedOffSubtotal{})
}
