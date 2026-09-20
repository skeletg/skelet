package actions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/internal/money"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// fixedOffMatching applies a fixed cents-per-unit discount to lines matching a nested condition. max_qty caps the number of discounted
// units - Discounts are clamped per unit to the unit price so a $10/unit discount on a $5 widget never goes negative
type fixedOffMatching struct{}

type fixedOffMatchingParams struct {
	CentsPerUnit int64         `json:"cents_per_unit"`
	Condition    matchingChild `json:"condition"`
	MaxQty       *float64      `json:"max_qty,omitempty"`
}

func (fixedOffMatching) Slug() string  { return "core.fixed_off_matching_items" }
func (fixedOffMatching) Label() string { return "Fixed off matching items" }
func (fixedOffMatching) Description() string {
	return "Fixed cents off each unit of lines matching a nested condition; optionally capped."
}

func (fixedOffMatching) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["cents_per_unit", "condition"],
  "properties": {
    "cents_per_unit": {"type": "integer", "minimum": 1, "title": "Discount per unit (cents)"},
    "condition": {"type": "object", "x-widget": "nested-condition", "title": "Matching condition"},
    "max_qty": {"type": "integer", "minimum": 1, "title": "Max discounted units"}
  },
  "additionalProperties": false
}`)
}

func (fixedOffMatching) ValidateParams(raw json.RawMessage) error {
	var p fixedOffMatchingParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.CentsPerUnit <= 0 {
		return fmt.Errorf("cents_per_unit must be > 0")
	}
	if p.Condition.Type == "" {
		return fmt.Errorf("condition.type is required")
	}
	impl, ok := promotion.DefaultRegistry.LookupCondition(p.Condition.Type)
	if !ok {
		return fmt.Errorf("condition: unknown type %q", p.Condition.Type)
	}
	if err := impl.ValidateParams(p.Condition.Params); err != nil {
		return fmt.Errorf("condition: %w", err)
	}
	if p.MaxQty != nil && *p.MaxQty <= 0 {
		return fmt.Errorf("max_qty must be > 0 when set")
	}
	return nil
}

func (fixedOffMatching) Apply(ctx context.Context, env promotion.Environment, raw json.RawMessage, ledger promotion.Ledger, _ promotion.AppliedRuleContext) error {
	var p fixedOffMatchingParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	matching, err := applyToMatchingLines(ctx, env, p.Condition, p.MaxQty, func(line promotion.CartLineView, qty float64) int64 {
		// Clamp per-unit discount so it never exceeds the unit price
		per := min(p.CentsPerUnit, line.UnitPriceCents)
		return money.LineTotal(per, qty)
	})
	if err != nil {
		return err
	}
	for lineID, discount := range matching {
		ledger.AddLineAdjustment(lineID, discount)
	}
	return nil
}

func init() {
	add(fixedOffMatching{})
}
