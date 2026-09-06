package admin

import (
	"fmt"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/tax"
)

// toTaxRuleModel translates the internal tax.Rule (with hydrated conditions/effects) to the gqlgen model. Kept in this
// companion file so gqlgen codegen doesn't relocate it when the schema regenerates
func toTaxRuleModel(r tax.Rule) *model.TaxRule {
	conds := make([]*model.TaxCondition, len(r.Conditions))
	for i, c := range r.Conditions {
		conds[i] = &model.TaxCondition{
			ID:            c.ID.String(),
			Type:          c.Type,
			Params:        rawMessageToAny(c.Params),
			ParamsVersion: c.ParamsVersion,
			SortOrder:     c.SortOrder,
		}
	}
	effs := make([]*model.TaxEffect, len(r.Effects))
	for i, e := range r.Effects {
		effs[i] = &model.TaxEffect{
			ID:            e.ID.String(),
			Type:          e.Type,
			Params:        rawMessageToAny(e.Params),
			ParamsVersion: e.ParamsVersion,
			SortOrder:     e.SortOrder,
		}
	}
	return &model.TaxRule{
		ID:               r.ID.String(),
		ChannelID:        r.ChannelID.String(),
		Name:             r.Name,
		Priority:         r.Priority,
		ExclusivityGroup: r.ExclusivityGroup,
		SortOrder:        r.SortOrder,
		IsActive:         r.IsActive,
		Conditions:       conds,
		Effects:          effs,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

func toTaxTypeDescriptorModel(d tax.TypeDescriptor) *model.TaxTypeDescriptor {
	return &model.TaxTypeDescriptor{
		Slug:         d.Slug,
		Label:        d.Label,
		Description:  d.Description,
		ParamsSchema: rawMessageToAny(d.ParamsSchema),
	}
}

func taxConditionInputsToDomain(in []*model.TaxConditionInput) ([]tax.Condition, error) {
	out := make([]tax.Condition, len(in))
	for i, c := range in {
		raw, err := anyToRawMessage(c.Params)
		if err != nil {
			return nil, fmt.Errorf("condition[%d].params: %w", i, err)
		}
		out[i] = tax.Condition{
			Type:          c.Type,
			Params:        raw,
			ParamsVersion: intDeref(c.ParamsVersion, 1),
			SortOrder:     intDeref(c.SortOrder, int32(i)),
		}
	}
	return out, nil
}

func taxEffectInputsToDomain(in []*model.TaxEffectInput) ([]tax.Effect, error) {
	out := make([]tax.Effect, len(in))
	for i, e := range in {
		raw, err := anyToRawMessage(e.Params)
		if err != nil {
			return nil, fmt.Errorf("effect[%d].params: %w", i, err)
		}
		out[i] = tax.Effect{
			Type:          e.Type,
			Params:        raw,
			ParamsVersion: intDeref(e.ParamsVersion, 1),
			SortOrder:     intDeref(e.SortOrder, int32(i)),
		}
	}
	return out, nil
}
