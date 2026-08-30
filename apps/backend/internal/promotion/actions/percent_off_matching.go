package actions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/internal/money"
	"github.com/skeletg/skelet/internal/promotion"
)

// Applies a percent discount only to lines whose product matches a nested condition - max_qty caps how many units are discounted across matching lines, which covers "10% off your first 3 widgets"
// The condition's shape mirrors the composite conditions, e.g. {"type": "core.cart_contains_sku", "params": {"skus": ["WIDGET"]}}
// Discounts record as per-line adjustments through Ledger.AddLineAdjustment, so the final ledger carries accurate per-line discounted prices for refund accounting and EU-VAT without AllocateLineDiscounts having to redistribute
type percentOffMatching struct{}

type matchingChild struct {
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params"`
}

type percentOffMatchingParams struct {
	BPS       int32         `json:"bps"`
	Condition matchingChild `json:"condition"`
	MaxQty    *float64      `json:"max_qty,omitempty"`
}

func (percentOffMatching) Slug() string  { return "core.percent_off_matching_items" }
func (percentOffMatching) Label() string { return "% off matching items" }
func (percentOffMatching) Description() string {
	return "Percent off only lines whose product matches a nested condition; optionally capped to N discounted units."
}

func (percentOffMatching) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["bps", "condition"],
  "properties": {
    "bps": {"type": "integer", "minimum": 1, "maximum": 10000, "title": "Discount (basis points)"},
    "condition": {"type": "object", "x-widget": "nested-condition", "title": "Matching condition"},
    "max_qty": {"type": "integer", "minimum": 1, "title": "Max discounted units"}
  },
  "additionalProperties": false
}`)
}

func (percentOffMatching) ValidateParams(raw json.RawMessage) error {
	var p percentOffMatchingParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.BPS <= 0 || p.BPS > 10000 {
		return fmt.Errorf("bps must be in 1..10000")
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

func (percentOffMatching) Apply(ctx context.Context, env promotion.Environment, raw json.RawMessage, ledger promotion.Ledger, _ promotion.AppliedRuleContext) error {
	var p percentOffMatchingParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	matching, err := applyToMatchingLines(ctx, env, p.Condition, p.MaxQty, func(line promotion.CartLineView, qty float64) int64 {
		gross := money.LineTotal(line.UnitPriceCents, qty)
		return (gross * int64(p.BPS)) / 10000
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
	add(percentOffMatching{})
}
