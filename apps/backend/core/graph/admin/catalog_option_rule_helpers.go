package admin

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func toProductOptionRuleModels(rules []catalog.ProductOptionRule, perm auth.PermittedChannels) []*model.ProductOptionRule {
	out := make([]*model.ProductOptionRule, len(rules))
	for i, r := range rules {
		locales := make([]string, 0, len(r.Labels))
		for loc := range r.Labels {
			locales = append(locales, loc)
		}
		slices.Sort(locales)
		labels := make([]*model.ProductOptionRuleLabel, len(locales))
		for j, loc := range locales {
			labels[j] = &model.ProductOptionRuleLabel{Locale: loc, Label: r.Labels[loc]}
		}
		actions := make([]*model.ProductOptionRuleAction, len(r.Actions))
		for j, a := range r.Actions {
			m := &model.ProductOptionRuleAction{
				Type:          model.ProductOptionRuleActionType(strings.ToUpper(string(a.Type))),
				Quantity:      a.Quantity,
				ChannelPrices: []*model.OptionChannelPrice{},
			}
			if a.OptionID != nil {
				id := a.OptionID.String()
				m.OptionID = &id
			}
			if a.Type == catalog.RuleActionAdjustPrice {
				pt := model.ProductOptionPriceType(strings.ToUpper(string(a.PriceType)))
				pv := a.PriceValue
				m.PriceType, m.PriceValue = &pt, &pv
				for _, p := range a.ChannelPrices {
					if perm.Allows(p.ChannelID) {
						m.ChannelPrices = append(m.ChannelPrices, &model.OptionChannelPrice{ChannelID: p.ChannelID.String(), PriceValue: p.PriceValue})
					}
				}
			}
			actions[j] = m
		}
		out[i] = &model.ProductOptionRule{
			ID:        r.ID.String(),
			Label:     r.Label,
			Labels:    labels,
			Condition: visibleIfToAny(&r.Condition),
			Actions:   actions,
		}
		if r.TemplateID != uuid.Nil {
			id, name := r.TemplateID.String(), r.TemplateName
			out[i].TemplateID, out[i].TemplateName = &id, &name
		}
	}
	return out
}

func optionRuleInputs(rules []*model.ProductOptionRuleInput) ([]catalog.ProductOptionRuleInput, error) {
	out := make([]catalog.ProductOptionRuleInput, len(rules))
	for i, in := range rules {
		var rule catalog.ProductOptionRuleInput
		if in.ID != nil {
			id, err := parseID(*in.ID, "rule")
			if err != nil {
				return nil, err
			}
			rule.ID = &id
		}
		cond, err := visibleIfFromInput(in.Condition)
		if err != nil {
			return nil, err
		}
		if cond == nil {
			return nil, fmt.Errorf("%w: rule %d needs a condition", catalog.ErrOptionRuleInvalid, i+1)
		}
		rule.Condition = *cond
		rule.Labels = make(map[string]string, len(in.Labels))
		for _, l := range in.Labels {
			rule.Labels[l.Locale] = l.Label
		}
		for _, a := range in.Actions {
			action := catalog.ProductOptionRuleAction{
				Type:     catalog.ProductOptionRuleActionType(strings.ToLower(string(a.Type))),
				Quantity: a.Quantity,
			}
			if a.OptionID != nil {
				id, err := parseID(*a.OptionID, "option")
				if err != nil {
					return nil, err
				}
				action.OptionID = &id
			}
			if a.PriceType != nil {
				action.PriceType = catalog.ProductOptionPriceType(strings.ToLower(string(*a.PriceType)))
			}
			if a.PriceValue != nil {
				action.PriceValue = *a.PriceValue
			}
			for _, p := range a.ChannelPrices {
				ch, err := parseID(p.ChannelID, "channel")
				if err != nil {
					return nil, err
				}
				action.ChannelPrices = append(action.ChannelPrices, catalog.RuleChannelPrice{ChannelID: ch, PriceValue: p.PriceValue})
			}
			rule.Actions = append(rule.Actions, action)
		}
		out[i] = rule
	}
	return out, nil
}
