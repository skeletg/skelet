package actions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/internal/promotion"
)

// Clamps the cart's aggregate subtotal discount to a ceiling - designed to run last in a stacking chain, so admins put it on a high-priority no-op rule to guarantee the total discount never exceeds a set amount however many promotions stack
// Caveat: actions run in the engine's iteration order, so it only clamps the discounts seen UP TO the point it fires - put the hosting rule LAST in priority, or use exclusivity, so every earlier rule has stamped its share onto the ledger first
type capTotalDiscount struct{}

type capTotalDiscountParams struct {
	MaxCents int64 `json:"max_cents"`
}

func (capTotalDiscount) Slug() string  { return "core.cap_total_discount" }
func (capTotalDiscount) Label() string { return "Cap total discount" }
func (capTotalDiscount) Description() string {
	return "Clamps the running subtotal discount to max_cents. Place on a high-priority rule that fires after every other stackable rule."
}

func (capTotalDiscount) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["max_cents"],
  "properties": {
    "max_cents": {"type": "integer", "minimum": 1, "title": "Maximum aggregate discount (cents)"}
  },
  "additionalProperties": false
}`)
}

func (capTotalDiscount) ValidateParams(raw json.RawMessage) error {
	var p capTotalDiscountParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.MaxCents <= 0 {
		return fmt.Errorf("max_cents must be > 0")
	}
	return nil
}

func (capTotalDiscount) Apply(ctx context.Context, env promotion.Environment, raw json.RawMessage, ledger promotion.Ledger, _ promotion.AppliedRuleContext) error {
	var p capTotalDiscountParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	ledger.CapSubtotalDiscount(p.MaxCents)
	return nil
}

func init() {
	add(capTotalDiscount{})
}
