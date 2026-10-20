package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreatePromotionRule(ctx context.Context, input model.CreatePromotionRuleInput) (*model.PromotionRule, error) {
	if r.PromotionSvc == nil {
		return nil, errNotConfigured("promotions")
	}
	rule, err := createInputToRule(input)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "promotions.manage", rule.ChannelID); err != nil {
		return nil, err
	}
	if claims, ok := auth.AdminFromContext(ctx); ok {
		rule.CreatedByAdminID = &claims.AdminID
	}
	conds, err := conditionInputsToDomain(input.Conditions)
	if err != nil {
		return nil, err
	}
	actions, err := actionInputsToDomain(input.Actions)
	if err != nil {
		return nil, err
	}
	created, err := r.PromotionSvc.CreateRule(ctx, promotion.CreateRuleInput{
		Rule:       rule,
		Conditions: conds,
		Actions:    actions,
	})
	if err != nil {
		return nil, err
	}
	return promotionRuleToModel(created), nil
}

func (r *mutationResolver) UpdatePromotionRule(ctx context.Context, input model.UpdatePromotionRuleInput) (*model.PromotionRule, error) {
	if r.PromotionSvc == nil {
		return nil, errNotConfigured("promotions")
	}
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	existing, err := r.PromotionSvc.Repo().GetRule(ctx, id)
	if err != nil {
		return nil, err
	}
	// Gate twice - once on the existing rule's channel (the admin must be able to touch the rule in its current shape) and once on the merged
	// rule (so an update that *moves* a rule to another channel also requires permission on the destination)
	if err := auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "promotions.manage", existing.ChannelID); err != nil {
		return nil, err
	}
	merged, err := updateInputToRule(existing, input)
	if err != nil {
		return nil, err
	}
	if merged.ChannelID != existing.ChannelID {
		if err := auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "promotions.manage", merged.ChannelID); err != nil {
			return nil, err
		}
	}
	if claims, ok := auth.AdminFromContext(ctx); ok {
		merged.UpdatedByAdminID = &claims.AdminID
	}
	// Nil-or-non-nil semantics so callers can patch just the rule row (priority, stop_after_apply, etc.) without re-submitting the full conditions/actions arrays and accidentally wiping them. The DnD reorder path takes advantage of this; the full rule editor sends both arrays explicitly.
	var condsPtr *[]promotion.Condition
	if input.Conditions != nil {
		conds, err := conditionInputsToDomain(input.Conditions)
		if err != nil {
			return nil, err
		}
		condsPtr = &conds
	}
	var actsPtr *[]promotion.Action
	if input.Actions != nil {
		actions, err := actionInputsToDomain(input.Actions)
		if err != nil {
			return nil, err
		}
		actsPtr = &actions
	}
	updated, err := r.PromotionSvc.UpdateRule(ctx, promotion.UpdateRuleInput{
		Rule:       merged,
		Conditions: condsPtr,
		Actions:    actsPtr,
	})
	if err != nil {
		return nil, err
	}
	return promotionRuleToModel(updated), nil
}

func (r *mutationResolver) DeletePromotionRule(ctx context.Context, id string) (bool, error) {
	if r.PromotionSvc == nil {
		return false, errNotConfigured("promotions")
	}
	rid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	existing, err := r.PromotionSvc.Repo().GetRule(ctx, rid)
	if err != nil {
		return false, err
	}
	if err := auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "promotions.manage", existing.ChannelID); err != nil {
		return false, err
	}
	if err := r.PromotionSvc.DeleteRule(ctx, rid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) GeneratePromotionCodes(ctx context.Context, ruleID string, pattern string, count int32, prefix *string, suffix *string) ([]*model.PromotionCode, error) {
	if r.PromotionSvc == nil {
		return nil, errNotConfigured("promotions")
	}
	rid, err := uuid.Parse(ruleID)
	if err != nil {
		return nil, errInvalidID("ruleId", err)
	}
	if err := r.requirePromotionRuleManage(ctx, rid); err != nil {
		return nil, err
	}
	codes, err := r.PromotionSvc.GenerateCodes(ctx, rid, pattern, int(count), ptr.Deref(prefix), ptr.Deref(suffix))
	if err != nil {
		return nil, err
	}
	// Hydrate back the inserted code rows so the dashboard can show the full per-code state. GenerateCodes returned the strings that were
	// inserted (post-collision filter), so a single GetCodeByCode loop is fine for typical batch sizes (<10k)
	out := make([]*model.PromotionCode, 0, len(codes))
	for _, c := range codes {
		row, err := r.PromotionSvc.Repo().GetCodeByCode(ctx, c)
		if err != nil {
			continue
		}
		out = append(out, promotionCodeToModel(row))
	}
	return out, nil
}

func (r *mutationResolver) UpdatePromotionCode(ctx context.Context, input model.UpdatePromotionCodeInput) (*model.PromotionCode, error) {
	if r.PromotionSvc == nil {
		return nil, errNotConfigured("promotions")
	}
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	existing, err := r.PromotionSvc.Repo().GetCode(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := r.requirePromotionRuleManage(ctx, existing.RuleID); err != nil {
		return nil, err
	}
	if input.Code != nil {
		existing.Code = *input.Code
	}
	if input.MaxUses != nil {
		existing.MaxUses = input.MaxUses
	}
	if input.PerCustomerLimit != nil {
		existing.PerCustomerLimit = input.PerCustomerLimit
	}
	if input.ValidFrom != nil {
		existing.ValidFrom = input.ValidFrom
	}
	if input.ValidTo != nil {
		existing.ValidTo = input.ValidTo
	}
	if input.RestrictedToEmail != nil {
		existing.RestrictedToEmail = *input.RestrictedToEmail
	}
	if input.IsActive != nil {
		existing.IsActive = *input.IsActive
	}
	updated, err := r.PromotionSvc.Repo().UpdateCode(ctx, existing)
	if err != nil {
		return nil, err
	}
	return promotionCodeToModel(updated), nil
}

func (r *mutationResolver) DeletePromotionCode(ctx context.Context, id string) (bool, error) {
	if r.PromotionSvc == nil {
		return false, errNotConfigured("promotions")
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	code, err := r.PromotionSvc.Repo().GetCode(ctx, cid)
	if err != nil {
		return false, err
	}
	if err := r.requirePromotionRuleManage(ctx, code.RuleID); err != nil {
		return false, err
	}
	if err := r.PromotionSvc.Repo().DeleteCode(ctx, cid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) ReorderPromotionRules(ctx context.Context, ruleIds []string) (bool, error) {
	if r.PromotionSvc == nil {
		return false, errNotConfigured("promotions")
	}
	// Reordering changes priority across ALL rules (including other
	// channels' and global rules) - requires the unscoped grant.
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "promotions.manage"); err != nil {
		return false, err
	}
	ids := make([]uuid.UUID, len(ruleIds))
	for i, raw := range ruleIds {
		id, err := uuid.Parse(raw)
		if err != nil {
			return false, fmt.Errorf("ruleIds[%d]: %w", i, err)
		}
		ids[i] = id
	}
	if err := r.PromotionSvc.Reorder(ctx, ids); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UpsertPromotionRuleContent(ctx context.Context, input model.UpsertPromotionRuleContentInput) (*model.PromotionRuleContent, error) {
	if r.PromotionSvc == nil {
		return nil, errNotConfigured("promotions")
	}
	rid, err := uuid.Parse(input.RuleID)
	if err != nil {
		return nil, errInvalidID("ruleId", err)
	}
	if err := r.requirePromotionRuleManage(ctx, rid); err != nil {
		return nil, err
	}
	terms := ""
	if input.Terms != nil {
		terms = *input.Terms
	}
	row, err := r.PromotionSvc.UpsertRuleContent(ctx, rid, input.Locale, input.Label, terms)
	if err != nil {
		return nil, err
	}
	return &model.PromotionRuleContent{
		Locale:    row.Locale,
		Label:     row.Label,
		Terms:     row.Terms,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *mutationResolver) DeletePromotionRuleContent(ctx context.Context, ruleID string, locale string) (bool, error) {
	if r.PromotionSvc == nil {
		return false, errNotConfigured("promotions")
	}
	rid, err := uuid.Parse(ruleID)
	if err != nil {
		return false, errInvalidID("ruleId", err)
	}
	if err := r.requirePromotionRuleManage(ctx, rid); err != nil {
		return false, err
	}
	if err := r.PromotionSvc.DeleteRuleContent(ctx, rid, locale); err != nil {
		return false, err
	}
	return true, nil
}

// Codes is the resolver for the codes field. Loads codes lazily so the list page (which doesn't ask for codes) doesn't
// pay the join, and the detail page can paginate
func (r *promotionRuleResolver) Codes(ctx context.Context, obj *model.PromotionRule, search *string, isActive *bool, limit *int32, offset *int32) (*model.PromotionCodeList, error) {
	if r.PromotionSvc == nil {
		return &model.PromotionCodeList{Items: []*model.PromotionCode{}, TotalCount: 0}, nil
	}
	rid, err := parseID(obj.ID, "rule")
	if err != nil {
		return nil, err
	}
	codes, total, err := r.PromotionSvc.Repo().ListCodesByRule(ctx, rid, strings.TrimSpace(ptrStr(search)), isActive, derefInt32Default(limit, 50), derefInt32Default(offset, 0))
	if err != nil {
		return nil, err
	}
	items := make([]*model.PromotionCode, len(codes))
	for i, c := range codes {
		items[i] = promotionCodeToModel(c)
	}
	return &model.PromotionCodeList{Items: items, TotalCount: total}, nil
}

// Contents is the resolver for the contents field. Loaded lazily so the list page (which doesn't ask for contents)
// doesn't pay the query. Detail / edit pages ask for it explicitly
func (r *promotionRuleResolver) Contents(ctx context.Context, obj *model.PromotionRule) ([]*model.PromotionRuleContent, error) {
	if r.PromotionSvc == nil {
		return []*model.PromotionRuleContent{}, nil
	}
	rid, err := parseID(obj.ID, "rule")
	if err != nil {
		return nil, err
	}
	rows, err := r.PromotionSvc.ListRuleContents(ctx, rid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.PromotionRuleContent, len(rows))
	for i, row := range rows {
		out[i] = &model.PromotionRuleContent{
			Locale:    row.Locale,
			Label:     row.Label,
			Terms:     row.Terms,
			UpdatedAt: row.UpdatedAt,
		}
	}
	return out, nil
}

// LabelMedia is the resolver for the labelMedia field. Lazily hydrates the corner-sticker asset from media library so
// list pages that don't ask for the URL don't pay the lookup
func (r *promotionRuleResolver) LabelMedia(ctx context.Context, obj *model.PromotionRule) (*model.MediaAsset, error) {
	return r.loadPromotionMedia(ctx, obj.LabelMediaID)
}

func (r *promotionRuleResolver) BannerMedia(ctx context.Context, obj *model.PromotionRule) (*model.MediaAsset, error) {
	return r.loadPromotionMedia(ctx, obj.BannerMediaID)
}

func (r *queryResolver) PromotionRules(ctx context.Context, limit *int32, offset *int32) (*model.PromotionRuleList, error) {
	if r.PromotionSvc == nil {
		return &model.PromotionRuleList{Items: []*model.PromotionRule{}, TotalCount: 0}, nil
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "promotions.view"); err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "promotions.view")
	if err != nil {
		return nil, err
	}
	// Rules with ChannelID = nil ("global" - applies to every store) are hidden from scoped admins, who couldn't deactivate them against their own channel anyway - filtered in SQL so the page and its total match what they may see
	filter := promotion.RuleFilter{AllChannels: perm.Bypass(), ChannelIDs: perm.IDs()}
	rules, total, err := r.PromotionSvc.ListRules(ctx, filter, derefInt32Default(limit, 25), derefInt32Default(offset, 0))
	if err != nil {
		return nil, err
	}
	items := make([]*model.PromotionRule, len(rules))
	for i, rule := range rules {
		items[i] = promotionRuleToModel(rule)
	}
	displayTotal := total
	return &model.PromotionRuleList{Items: items, TotalCount: displayTotal}, nil
}

// PromotionRule is the resolver for the promotionRule field. A scoped admin asking for a rule whose ChannelID isn't in
// their permitted set (or for a global ChannelID=nil rule without global access) gets FORBIDDEN
func (r *queryResolver) PromotionRule(ctx context.Context, id string) (*model.PromotionRule, error) {
	if r.PromotionSvc == nil {
		return nil, nil
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "promotions.view"); err != nil {
		return nil, err
	}
	rid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	rule, err := r.PromotionSvc.GetRuleHydrated(ctx, rid)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "promotions.view", rule.ChannelID); err != nil {
		return nil, err
	}
	return promotionRuleToModel(rule), nil
}

func (r *queryResolver) PromotionConditionTypes(ctx context.Context) ([]*model.PromotionTypeDescriptor, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "promotions.view"); err != nil {
		return nil, err
	}
	all := promotion.DefaultRegistry.AllConditions()
	out := make([]*model.PromotionTypeDescriptor, len(all))
	for i, c := range all {
		out[i] = &model.PromotionTypeDescriptor{
			Slug:         c.Slug(),
			Label:        c.Label(),
			Description:  c.Description(),
			ParamsSchema: rawMessageToAny(c.ParamsSchema()),
		}
	}
	return out, nil
}

func (r *queryResolver) PromotionActionTypes(ctx context.Context) ([]*model.PromotionTypeDescriptor, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "promotions.view"); err != nil {
		return nil, err
	}
	all := promotion.DefaultRegistry.AllActions()
	out := make([]*model.PromotionTypeDescriptor, len(all))
	for i, a := range all {
		out[i] = &model.PromotionTypeDescriptor{
			Slug:         a.Slug(),
			Label:        a.Label(),
			Description:  a.Description(),
			ParamsSchema: rawMessageToAny(a.ParamsSchema()),
		}
	}
	return out, nil
}

func (r *queryResolver) PromotionAnalytics(ctx context.Context, start *time.Time, end *time.Time) ([]*model.PromotionRuleAnalytics, error) {
	if r.PromotionSvc == nil {
		return nil, nil
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "promotions.view"); err != nil {
		return nil, err
	}
	s, e := defaultAnalyticsRange(start, end)
	channels, err := r.promotionAnalyticsChannels(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.promotionAnalyticsByRule(ctx, s, e, channels)
	if err != nil {
		return nil, err
	}
	out := make([]*model.PromotionRuleAnalytics, len(rows))
	for i, row := range rows {
		out[i] = &model.PromotionRuleAnalytics{
			RuleID:             row.RuleID.String(),
			RuleSlug:           row.RuleSlug,
			RuleName:           row.RuleName,
			RedemptionCount:    row.RedemptionCount,
			DiscountTotalCents: int(row.DiscountTotalCents),
			UniqueCustomers:    row.UniqueCustomers,
			OrderRevenueCents:  int(row.OrderRevenueCents),
		}
	}
	return out, nil
}

func (r *queryResolver) PromotionAnalyticsTotals(ctx context.Context, start *time.Time, end *time.Time) (*model.PromotionAnalyticsTotals, error) {
	if r.PromotionSvc == nil {
		return &model.PromotionAnalyticsTotals{}, nil
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "promotions.view"); err != nil {
		return nil, err
	}
	s, e := defaultAnalyticsRange(start, end)
	channels, err := r.promotionAnalyticsChannels(ctx)
	if err != nil {
		return nil, err
	}
	row, err := r.promotionAnalyticsTotals(ctx, s, e, channels)
	if err != nil {
		return nil, err
	}
	return &model.PromotionAnalyticsTotals{
		RedemptionCount:    row.RedemptionCount,
		DiscountTotalCents: int(row.DiscountTotalCents),
		UniqueCustomers:    row.UniqueCustomers,
	}, nil
}

func (r *queryResolver) PromotionAnalyticsDaily(ctx context.Context, start *time.Time, end *time.Time, ruleID *string) ([]*model.PromotionAnalyticsDailyPoint, error) {
	if r.PromotionSvc == nil {
		return nil, nil
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "promotions.view"); err != nil {
		return nil, err
	}
	var rid *uuid.UUID
	if ruleID != nil && *ruleID != "" {
		id, err := uuid.Parse(*ruleID)
		if err != nil {
			return nil, errInvalidID("ruleId", err)
		}
		rid = &id
	}
	s, e := defaultAnalyticsRange(start, end)
	channels, err := r.promotionAnalyticsChannels(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.promotionAnalyticsDaily(ctx, s, e, rid, channels)
	if err != nil {
		return nil, err
	}
	out := make([]*model.PromotionAnalyticsDailyPoint, len(rows))
	for i, row := range rows {
		out[i] = &model.PromotionAnalyticsDailyPoint{
			Date:               row.Day.Format("2006-01-02"),
			RedemptionCount:    row.RedemptionCount,
			DiscountTotalCents: int(row.DiscountTotalCents),
		}
	}
	return out, nil
}

func (r *queryResolver) SimulatePromotion(ctx context.Context, input model.PromotionSimulationInput) (*model.PromotionSimulationResult, error) {
	if r.PromotionSvc == nil {
		return nil, errNotConfigured("promotions")
	}
	// Reveals which rules fire + discount amounts across rules; gate on
	// the global promotions.view permission.
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "promotions.view"); err != nil {
		return nil, err
	}
	env, err := simulationInputToEnv(input)
	if err != nil {
		return nil, err
	}
	res, err := r.PromotionSvc.Simulate(ctx, env)
	if err != nil {
		return nil, err
	}
	appliedIDs := make([]string, len(res.Applied))
	for i, a := range res.Applied {
		appliedIDs[i] = a.RuleID.String()
	}
	trace := make([]*model.PromotionTraceEntry, len(res.Trace))
	for i, t := range res.Trace {
		trace[i] = &model.PromotionTraceEntry{
			RuleID:         t.RuleID.String(),
			RuleSlug:       t.RuleSlug,
			Priority:       t.Priority,
			Matched:        t.Matched,
			SkipReason:     t.SkipReason,
			DiscountCents:  int32(t.DiscountCents),
			StopAfterApply: stopModeToGQL(t.StopAfterApply),
		}
	}
	return &model.PromotionSimulationResult{
		AppliedRuleIds:        appliedIDs,
		SubtotalDiscountCents: int32(res.Ledger.SubtotalDiscountCents()),
		FreeShipping:          res.Ledger.FreeShipping(),
		Trace:                 trace,
	}, nil
}

func (r *Resolver) PromotionRule() PromotionRuleResolver { return &promotionRuleResolver{r} }

type promotionRuleResolver struct{ *Resolver }
