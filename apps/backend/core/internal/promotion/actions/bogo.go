package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/skeletg/skelet/apps/backend/core/internal/money"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// bogo (buy N, get M) discounts every Mth unit of matching lines, pooling eligible units across every line matching target_condition before computing clusters
// Per cluster the pool covers, get_qty units are discounted by get_kind - free is 100% off, percent is get_value bps, fixed is get_value cents clamped to unit price
type bogo struct{}

type bogoParams struct {
	BuyQty          float64       `json:"buy_qty"`
	GetQty          float64       `json:"get_qty"`
	TargetCondition matchingChild `json:"target_condition"`
	GetKind         string        `json:"get_kind"` // "free" | "percent" | "fixed"
	Value           int32         `json:"value,omitempty"`
}

func (bogo) Slug() string  { return "core.bogo" }
func (bogo) Label() string { return "Buy N, get M" }
func (bogo) Description() string {
	return "Buy N, get M at free / percent off / fixed off, on lines matching a nested condition."
}

func (bogo) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["buy_qty", "get_qty", "target_condition", "get_kind"],
  "properties": {
    "buy_qty": {"type": "integer", "minimum": 1, "title": "Buy quantity"},
    "get_qty": {"type": "integer", "minimum": 1, "title": "Get quantity"},
    "target_condition": {"type": "object", "x-widget": "nested-condition", "title": "Matching condition"},
    "get_kind": {"type": "string", "enum": ["free", "percent", "fixed"], "title": "Discount on the get-units"},
    "value": {"type": "integer", "minimum": 0, "title": "Discount value (bps for percent, cents for fixed)"}
  },
  "additionalProperties": false
}`)
}

func (bogo) ValidateParams(raw json.RawMessage) error {
	var p bogoParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.BuyQty <= 0 || p.GetQty <= 0 {
		return fmt.Errorf("buy_qty and get_qty must be > 0")
	}
	if p.TargetCondition.Type == "" {
		return fmt.Errorf("target_condition.type is required")
	}
	impl, ok := promotion.DefaultRegistry.LookupCondition(p.TargetCondition.Type)
	if !ok {
		return fmt.Errorf("target_condition: unknown type %q", p.TargetCondition.Type)
	}
	if err := impl.ValidateParams(p.TargetCondition.Params); err != nil {
		return fmt.Errorf("target_condition: %w", err)
	}
	switch p.GetKind {
	case "free":
	case "percent":
		if p.Value <= 0 || p.Value > 10000 {
			return fmt.Errorf("value must be in 1..10000 for percent")
		}
	case "fixed":
		if p.Value <= 0 {
			return fmt.Errorf("value must be > 0 for fixed")
		}
	default:
		return fmt.Errorf("get_kind must be one of: free, percent, fixed")
	}
	return nil
}

func (bogo) Apply(ctx context.Context, env promotion.Environment, raw json.RawMessage, ledger promotion.Ledger, _ promotion.AppliedRuleContext) error {
	var p bogoParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	impl, ok := promotion.RegistryFrom(ctx).LookupCondition(p.TargetCondition.Type)
	if !ok {
		return nil
	}
	// Collect every matching line FIRST, so clusters are computed against the pooled eligible quantity - one qty-1 widget-A line plus one qty-1 widget-B line is two eligible units and earns a buy-1-get-1 discount, same as a single qty-2 line Deciding per line instead silently gives nothing whenever the units were spread thin, and left the cross-line carry-over unreachable
	var matching []promotion.CartLineView
	var eligibleUnits float64
	for _, line := range env.Lines() {
		singleLineEnv := promotion.ProjectLines(env, []promotion.CartLineView{line})
		matches, err := impl.Evaluate(ctx, singleLineEnv, p.TargetCondition.Params)
		if err != nil {
			return err
		}
		if !matches {
			continue
		}
		matching = append(matching, line)
		eligibleUnits += line.Quantity
	}
	if len(matching) == 0 {
		return nil
	}
	// How many units the pool earns: one full cluster per (buy+get) units bought, plus a
	// partial trailing cluster where the buy leg is satisfied but the get leg isn't full
	clusterSize := p.BuyQty + p.GetQty
	clusters := math.Floor(eligibleUnits / clusterSize)
	discountable := clusters * p.GetQty
	if tail := eligibleUnits - clusters*clusterSize; tail > p.BuyQty {
		discountable += tail - p.BuyQty
	}
	if discountable <= 0 {
		return nil
	}
	// Spend the earned units in cart order - cheapest-first would be kinder to the merchant, but cart order keeps the
	// customer's mental model simple, so pooling doesn't change who gets discounted
	for _, line := range matching {
		if discountable <= 0 {
			break
		}
		units := line.Quantity
		if units > discountable {
			units = discountable
		}
		discountable -= units
		var per int64
		switch p.GetKind {
		case "free":
			per = line.UnitPriceCents
		case "percent":
			per = (line.UnitPriceCents * int64(p.Value)) / 10000
		case "fixed":
			per = min(int64(p.Value), line.UnitPriceCents)
		}
		if discount := money.LineTotal(per, units); discount > 0 {
			ledger.AddLineAdjustment(line.LineID, discount)
		}
	}
	return nil
}

func init() {
	add(bogo{})
}
