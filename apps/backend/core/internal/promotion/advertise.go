package promotion

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PublicPromotion is an automatic promotion as shoppers may see it: the label and terms the merchant published for them
type PublicPromotion struct {
	Label  string
	Terms  string
	EndsAt *time.Time
}

// Audience is who asks what's on offer - the store, and the signed-in customer's group, whose segments are looked up
type Audience struct {
	ChannelID         uuid.UUID
	ChannelSlug       string
	CustomerID        *uuid.UUID
	CustomerGroupSlug string
	Locale            string
}

// PublicPromotions lists the automatic promotions running on a store for this shopper that carry a public label, in the engine's
// priority order. A code-only promotion stays out - its code is the shopper's to bring, never the store's to hand out
func (s *Service) PublicPromotions(ctx context.Context, a Audience) ([]PublicPromotion, error) {
	rules, err := s.ActiveRulesForChannel(ctx, &a.ChannelID)
	if err != nil {
		return nil, err
	}
	in := CartEvalInput{ChannelID: &a.ChannelID, ChannelSlug: a.ChannelSlug, CustomerID: a.CustomerID, CustomerGroupSlug: a.CustomerGroupSlug}
	s.enrichCartEvalInput(ctx, &in)
	env := BuildEnvironment(EnvironmentInput{ChannelID: in.ChannelID, ChannelSlug: in.ChannelSlug, CustomerID: in.CustomerID,
		CustomerGroupSlug: in.CustomerGroupSlug, CustomerSegmentSlugs: in.CustomerSegmentSlugs})
	live, err := s.withConditions(ctx, runningAutomatic(rules, nowFunc()))
	if err != nil {
		return nil, err
	}
	live = slices.DeleteFunc(live, func(r Rule) bool { return !mightApply(ctx, s.engine.registry, r, env) })
	content := map[uuid.UUID]RuleContent{}
	if a.Locale != "" && len(live) > 0 {
		ids := make([]uuid.UUID, len(live))
		for i, r := range live {
			ids[i] = r.ID
		}
		if content, err = s.repo.GetRuleContentsByIDs(ctx, ids, a.Locale); err != nil {
			slog.WarnContext(ctx, "promotion: localized promotion labels", "locale", a.Locale, "err", err)
		}
	}
	var out []PublicPromotion
	for _, r := range live {
		row := content[r.ID]
		if label := cmp.Or(row.Label, r.PublicLabel); strings.TrimSpace(label) != "" {
			out = append(out, PublicPromotion{Label: label, Terms: row.Terms, EndsAt: r.ValidTo})
		}
	}
	return out, nil
}

// runningAutomatic keeps the automatic rules inside their window - the cache can hold one a little past it, as the engine knows
func runningAutomatic(rules []Rule, now time.Time) []Rule {
	return slices.DeleteFunc(slices.Clone(rules), func(r Rule) bool {
		return r.RequiresCode || !r.IsActive || (r.ValidFrom != nil && now.Before(*r.ValidFrom)) || (r.ValidTo != nil && now.After(*r.ValidTo))
	})
}

// withConditions attaches each rule's conditions, which the rules cache leaves out
func (s *Service) withConditions(ctx context.Context, rules []Rule) ([]Rule, error) {
	if len(rules) == 0 {
		return rules, nil
	}
	ids := make([]uuid.UUID, len(rules))
	for i, r := range rules {
		ids[i] = r.ID
	}
	conds, err := s.repo.ListConditionsByRules(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load conditions: %w", err)
	}
	byRule := make(map[uuid.UUID][]Condition, len(rules))
	for _, c := range conds {
		byRule[c.RuleID] = append(byRule[c.RuleID], c)
	}
	for i := range rules {
		rules[i].Conditions = byRule[rules[i].ID]
	}
	return rules, nil
}

// mightApply says whether this shopper could get the rule. What a list of offers can settle without a cart - the store, the clock,
// the customer's group and segments - is checked; what only a cart settles, like a minimum spend, is left to the cart
func mightApply(ctx context.Context, reg *Registry, r Rule, env Environment) bool {
	return !slices.ContainsFunc(r.Conditions, func(c Condition) bool { return offerVerdict(ctx, reg, env, c.Type, c.Params) == ruledOut })
}

type verdict int

const (
	cartDecides verdict = iota
	applies
	ruledOut
)

// The conditions a shopper's list of offers can settle - each reads only the store, the clock or who the customer is
var shopperConditions = map[string]bool{"core.channel_in": true, "core.schedule": true, "core.customer_group_in": true, "core.customer_in_segment": true}

type childCondition struct {
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params"`
}

// offerVerdict reads one condition for the offers list. An unregistered type rules the promotion out, as the engine never applies it
func offerVerdict(ctx context.Context, reg *Registry, env Environment, slug string, params json.RawMessage) verdict {
	switch slug {
	case "core.all_of", "core.any_of":
		var p struct {
			Conditions []childCondition `json:"conditions"`
		}
		if json.Unmarshal(params, &p) != nil {
			return ruledOut
		}
		seen := map[verdict]bool{}
		for _, c := range p.Conditions {
			seen[offerVerdict(ctx, reg, env, c.Type, c.Params)] = true
		}
		switch {
		case slug == "core.all_of" && seen[ruledOut], slug == "core.any_of" && !seen[applies] && !seen[cartDecides]:
			return ruledOut
		case slug == "core.all_of" && !seen[cartDecides], slug == "core.any_of" && seen[applies]:
			return applies
		}
		return cartDecides
	case "core.not":
		var p struct {
			Condition childCondition `json:"condition"`
		}
		if json.Unmarshal(params, &p) != nil {
			return ruledOut
		}
		switch offerVerdict(ctx, reg, env, p.Condition.Type, p.Condition.Params) {
		case applies:
			return ruledOut
		case ruledOut:
			return applies
		}
		return cartDecides
	}
	impl, ok := reg.LookupCondition(slug)
	switch {
	case !ok:
		return ruledOut
	case !shopperConditions[slug]:
		return cartDecides
	}
	if match, err := impl.Evaluate(ctx, env, params); err != nil || !match {
		return ruledOut
	}
	return applies
}
