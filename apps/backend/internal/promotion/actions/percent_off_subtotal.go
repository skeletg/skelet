package actions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/internal/promotion"
)

// percentOffSubtotal applies a flat percent discount to the cart subtotal. bps is the discount in basis points (1000 =
// 10%, 5000 = 50%) - Optional max_cents caps the discount - handy for "10% off, up to $50" rules
type percentOffSubtotal struct{}

type percentOffSubtotalParams struct {
	BPS      int32  `json:"bps"`
	MaxCents *int64 `json:"max_cents,omitempty"`
}

func (percentOffSubtotal) Slug() string  { return "core.percent_off_subtotal" }
func (percentOffSubtotal) Label() string { return "% off subtotal" }
func (percentOffSubtotal) Description() string {
	return "Percent off cart subtotal (basis points); optionally capped at max_cents."
}

func (percentOffSubtotal) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["bps"],
  "properties": {
    "bps": {"type": "integer", "minimum": 1, "maximum": 10000, "title": "Discount (basis points; 1000 = 10%)"},
    "max_cents": {"type": "integer", "minimum": 0, "title": "Maximum discount (cents)"}
  },
  "additionalProperties": false
}`)
}

func (percentOffSubtotal) ValidateParams(raw json.RawMessage) error {
	var p percentOffSubtotalParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.BPS <= 0 || p.BPS > 10000 {
		return fmt.Errorf("bps must be in 1..10000")
	}
	if p.MaxCents != nil && *p.MaxCents < 0 {
		return fmt.Errorf("max_cents must be >= 0")
	}
	return nil
}

func (percentOffSubtotal) Apply(ctx context.Context, env promotion.Environment, raw json.RawMessage, ledger promotion.Ledger, _ promotion.AppliedRuleContext) error {
	var p percentOffSubtotalParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	discount := (env.SubtotalCents() * int64(p.BPS)) / 10000
	if discount <= 0 {
		return nil
	}
	if p.MaxCents != nil && discount > *p.MaxCents {
		discount = *p.MaxCents
	}
	ledger.AddSubtotalDiscount(discount)
	return nil
}

func init() {
	add(percentOffSubtotal{})
}
