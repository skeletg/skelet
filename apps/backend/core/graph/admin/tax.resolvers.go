package admin

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateTaxRule(ctx context.Context, input model.CreateTaxRuleInput) (*model.TaxRule, error) {
	channelID, err := uuid.Parse(input.ChannelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	// Every rule belongs to exactly one channel, so the gate is that channel - an unscoped settings.manage check let an
	// admin scoped to one channel write tax rules for every other
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.manage", channelID, nil); err != nil {
		return nil, err
	}
	conditions, err := taxConditionInputsToDomain(input.Conditions)
	if err != nil {
		return nil, err
	}
	effects, err := taxEffectInputsToDomain(input.Effects)
	if err != nil {
		return nil, err
	}
	created, err := r.TaxSvc.CreateRule(ctx, tax.CreateRuleInput{
		Rule: tax.Rule{
			ChannelID:        channelID,
			Name:             input.Name,
			Priority:         intDeref(input.Priority, 0),
			ExclusivityGroup: ptr.Deref(input.ExclusivityGroup),
			SortOrder:        intDeref(input.SortOrder, 0),
			IsActive:         boolDeref(input.IsActive, true),
		},
		Conditions: conditions,
		Effects:    effects,
	})
	if err != nil {
		return nil, err
	}
	return toTaxRuleModel(created), nil
}

func (r *mutationResolver) UpdateTaxRule(ctx context.Context, input model.UpdateTaxRuleInput) (*model.TaxRule, error) {
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	existing, err := r.TaxSvc.GetRuleHydrated(ctx, id)
	if err != nil {
		return nil, err
	}
	// Load first, then gate on the rule's own channel
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.manage", existing.ChannelID, nil); err != nil {
		return nil, err
	}
	if input.Name != nil {
		existing.Name = *input.Name
	}
	if input.Priority != nil {
		existing.Priority = *input.Priority
	}
	if input.ExclusivityGroup != nil {
		existing.ExclusivityGroup = *input.ExclusivityGroup
	}
	if input.SortOrder != nil {
		existing.SortOrder = *input.SortOrder
	}
	if input.IsActive != nil {
		existing.IsActive = *input.IsActive
	}

	upd := tax.UpdateRuleInput{Rule: existing}
	if input.Conditions != nil {
		conds, err := taxConditionInputsToDomain(input.Conditions)
		if err != nil {
			return nil, err
		}
		upd.Conditions = &conds
	}
	if input.Effects != nil {
		effs, err := taxEffectInputsToDomain(input.Effects)
		if err != nil {
			return nil, err
		}
		upd.Effects = &effs
	}

	updated, err := r.TaxSvc.UpdateRule(ctx, upd)
	if err != nil {
		return nil, err
	}
	return toTaxRuleModel(updated), nil
}

func (r *mutationResolver) DeleteTaxRule(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "settings.manage"); err != nil {
		return false, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	existing, err := r.TaxSvc.GetRuleHydrated(ctx, uid)
	if err != nil {
		return false, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.manage", existing.ChannelID, nil); err != nil {
		return false, err
	}
	if err := r.TaxSvc.DeleteRule(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

// TaxRules is the resolver for the taxRules field. With no channelId it returns every channel's rules (cross-channel
// GLOBAL view); with one it scopes to that channel
func (r *queryResolver) TaxRules(ctx context.Context, channelID *string) ([]*model.TaxRule, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "settings.view"); err != nil {
		return nil, err
	}
	chID, err := r.scopedChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	var rules []tax.Rule
	if chID == nil {
		if rules, err = r.TaxSvc.ListAllRulesHydrated(ctx); err != nil {
			return nil, err
		}
		// The cross-channel view returns every channel's rules - drop the ones this
		// admin has no settings.view on, the same way the other list queries do
		perm, permErr := auth.LoadPermittedChannels(ctx, r.AdminSvc, "settings.view")
		if permErr != nil {
			return nil, permErr
		}
		rules = auth.FilterByChannel(rules, perm, func(rule tax.Rule) uuid.UUID { return rule.ChannelID })
	} else {
		if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.view", *chID, nil); err != nil {
			return nil, err
		}
		if rules, err = r.TaxSvc.ListRulesByChannelHydrated(ctx, *chID); err != nil {
			return nil, err
		}
	}
	out := make([]*model.TaxRule, len(rules))
	for i := range rules {
		out[i] = toTaxRuleModel(rules[i])
	}
	return out, nil
}

func (r *queryResolver) TaxRule(ctx context.Context, id string) (*model.TaxRule, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "settings.view"); err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	rule, err := r.TaxSvc.GetRuleHydrated(ctx, uid)
	if err != nil {
		if errors.Is(err, tax.ErrRuleNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.view", rule.ChannelID, nil); err != nil {
		return nil, err
	}
	return toTaxRuleModel(rule), nil
}

func (r *queryResolver) TaxConditionTypes(ctx context.Context) ([]*model.TaxTypeDescriptor, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "settings.view"); err != nil {
		return nil, err
	}
	descs := r.TaxSvc.Registry().AllConditions()
	out := make([]*model.TaxTypeDescriptor, len(descs))
	for i, d := range descs {
		out[i] = toTaxTypeDescriptorModel(d)
	}
	return out, nil
}

func (r *queryResolver) TaxEffectTypes(ctx context.Context) ([]*model.TaxTypeDescriptor, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "settings.view"); err != nil {
		return nil, err
	}
	descs := r.TaxSvc.Registry().AllEffects()
	out := make([]*model.TaxTypeDescriptor, len(descs))
	for i, d := range descs {
		out[i] = toTaxTypeDescriptorModel(d)
	}
	return out, nil
}
