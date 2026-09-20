package tax

import (
	"context"
	"fmt"
	"sort"
)

// Engine evaluates tax rules against an Environment - Stateless beyond the repo
// + registry handles - safe for concurrent use
type Engine struct {
	repo     Repository
	registry *Registry
}

func NewEngine(repo Repository, registry *Registry) *Engine {
	return &Engine{repo: repo, registry: registry}
}

// Evaluate applies every matching rule in (priority, sort_order, id) order - A non-empty ExclusivityGroup applies only if no earlier rule
// in the same group did - "most specific jurisdiction wins" when they share a group, free stacking when groups are empty/distinct
func (e *Engine) Evaluate(ctx context.Context, env Environment) (EvalResult, error) {
	rules, err := e.repo.ListActiveRulesByChannel(ctx, env.ChannelID())
	if err != nil {
		return EvalResult{}, err
	}

	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		if rules[i].SortOrder != rules[j].SortOrder {
			return rules[i].SortOrder < rules[j].SortOrder
		}
		return rules[i].ID.String() < rules[j].ID.String()
	})

	var (
		ledger  Ledger
		applied []AppliedTax
		seen    = map[string]bool{}
	)

	for _, rule := range rules {
		if rule.ExclusivityGroup != "" && seen[rule.ExclusivityGroup] {
			continue
		}
		ok, err := e.matches(ctx, env, rule)
		if err != nil {
			return EvalResult{}, fmt.Errorf("rule %s: %w", rule.ID, err)
		}
		if !ok {
			continue
		}

		mark := ledger.mark()
		if err := e.applyEffects(ctx, env, rule, &ledger); err != nil {
			return EvalResult{}, fmt.Errorf("rule %s: %w", rule.ID, err)
		}
		added := ledger.since(mark)
		if len(added) == 0 {
			// Conditions matched but effects produced no tax (e.g. 0%) - Still mark
			// the group consumed so a lower-priority fallback can't sneak in
			if rule.ExclusivityGroup != "" {
				seen[rule.ExclusivityGroup] = true
			}
			applied = append(applied, AppliedTax{RuleID: rule.ID, Name: rule.Name})
			continue
		}

		var ruleCents int64
		var rateBPS int32
		for _, en := range added {
			ruleCents += en.Cents
			if rateBPS == 0 && en.RateBPS != 0 {
				rateBPS = en.RateBPS
			}
		}
		applied = append(applied, AppliedTax{
			RuleID:  rule.ID,
			Name:    rule.Name,
			RateBPS: rateBPS,
			Cents:   ruleCents,
		})
		if rule.ExclusivityGroup != "" {
			seen[rule.ExclusivityGroup] = true
		}
	}

	return EvalResult{TotalCents: ledger.Total(), Applied: applied}, nil
}

// matches reports whether every condition on the rule passes - An empty
// condition set matches (a rule with no predicates always applies)
func (e *Engine) matches(ctx context.Context, env Environment, rule Rule) (bool, error) {
	for _, c := range rule.Conditions {
		impl, ok := e.registry.LookupCondition(c.Type)
		if !ok {
			return false, fmt.Errorf("unknown condition type %q", c.Type)
		}
		pass, err := impl.Evaluate(ctx, env, c.Params)
		if err != nil {
			return false, fmt.Errorf("condition %q: %w", c.Type, err)
		}
		if !pass {
			return false, nil
		}
	}
	return true, nil
}

func (e *Engine) applyEffects(ctx context.Context, env Environment, rule Rule, ledger *Ledger) error {
	for _, ef := range rule.Effects {
		impl, ok := e.registry.LookupEffect(ef.Type)
		if !ok {
			return fmt.Errorf("unknown effect type %q", ef.Type)
		}
		if err := impl.Apply(ctx, env, ef.Params, ledger); err != nil {
			return fmt.Errorf("effect %q: %w", ef.Type, err)
		}
	}
	return nil
}
