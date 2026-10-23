package cart

import (
	"github.com/google/uuid"
)

// A product's option rules, resolved for the cart: when the visible picks match a rule's condition its actions apply -
// actions never change picks, so every rule reads the same picks and the order they run in can't matter

// InputType of the line row a price-moving rule adds - the row's ProductOptionID holds the rule's id, so a quantity edit
// re-reads the CURRENT rule, and reorder and "edit options" skip the row as not being a pick
const OptionRuleInputType = "rule"

// Mirrors the catalog's action vocabulary - the cart never imports catalog
const (
	ruleActionRequire     = "require"
	ruleActionOptional    = "optional"
	ruleActionSetQuantity = "set_quantity"
	ruleActionAdjustPrice = "adjust_price"
)

type ProductOptionRuleConfig struct {
	ID        uuid.UUID
	Label     string // names the line row a price change adds, empty for an unnamed rule
	Condition OptionVisibilityGroup
	Actions   []OptionRuleActionConfig
}

type OptionRuleActionConfig struct {
	Type             string // require | optional | set_quantity | adjust_price
	OptionID         uuid.UUID
	Quantity         float64
	PriceType        string // fixed | percent
	PriceValue       int32  // resolved to this channel
	PriceUnavailable bool   // a FIXED amount with nothing in this channel's currency - the rule can't charge here
}

type ruleEffects struct {
	require   map[uuid.UUID]bool
	optional  map[uuid.UUID]bool
	forcedQty *float64
	conflict  bool
	priced    []ProductOptionRuleConfig // fired rules carrying a price action, in authored order
}

func evaluateOptionRules(rules []ProductOptionRuleConfig, picks optionPicks) ruleEffects {
	e := ruleEffects{require: map[uuid.UUID]bool{}, optional: map[uuid.UUID]bool{}}
	for _, r := range rules {
		if !evalVisibilityGroup(r.Condition, picks) {
			continue
		}
		hasPrice := false
		for _, a := range r.Actions {
			switch a.Type {
			case ruleActionRequire:
				e.require[a.OptionID] = true
			case ruleActionOptional:
				e.optional[a.OptionID] = true
			case ruleActionSetQuantity:
				if e.forcedQty != nil && *e.forcedQty != a.Quantity {
					e.conflict = true
				}
				q := a.Quantity
				e.forcedQty = &q
			case ruleActionAdjustPrice:
				hasPrice = true
			}
		}
		if hasPrice {
			e.priced = append(e.priced, r)
		}
	}
	return e
}

// Required wins over optional when both fire; an option no rule touches keeps its own setting
func (e ruleEffects) required(opt ProductOptionConfig) bool {
	return e.require[opt.ID] || (opt.IsRequired && !e.optional[opt.ID])
}

// A fired rule's price actions summed - PERCENT on the same base the options use
func ruleContribution(basePrice int64, r ProductOptionRuleConfig) (int64, bool) {
	var total int64
	for _, a := range r.Actions {
		if a.Type != ruleActionAdjustPrice {
			continue
		}
		if a.PriceUnavailable {
			return 0, false
		}
		if a.PriceType == "percent" {
			total += percentOf(basePrice, a.PriceValue)
		} else {
			total += int64(a.PriceValue)
		}
	}
	return total, true
}

// Rebuilds what a persisted line picked, so its rules can be re-read - the rule rows themselves are effects, not picks
func picksFromSnapshot(snapshot []CartLineOption) optionPicks {
	picks := optionPicks{filled: map[uuid.UUID]bool{}, values: map[uuid.UUID]map[uuid.UUID]bool{}}
	for _, row := range snapshot {
		if row.InputType == OptionRuleInputType {
			continue
		}
		if row.OptionValueID != nil {
			if picks.values[row.ProductOptionID] == nil {
				picks.values[row.ProductOptionID] = map[uuid.UUID]bool{}
			}
			picks.values[row.ProductOptionID][*row.OptionValueID] = true
			picks.filled[row.ProductOptionID] = true
			continue
		}
		if row.TextValue != "" || row.FileURL != "" {
			picks.filled[row.ProductOptionID] = true
		}
	}
	return picks
}
