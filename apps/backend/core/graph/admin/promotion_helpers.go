package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Codes are NOT hydrated here - the PromotionRule.codes field resolver loads them lazily so list pages cost one SELECT count instead of N+1 code fetches
func promotionRuleToModel(r promotion.Rule) *model.PromotionRule {
	out := &model.PromotionRule{
		ID:               r.ID.String(),
		Slug:             r.Slug,
		Name:             r.Name,
		Description:      r.Description,
		PublicLabel:      r.PublicLabel,
		ChannelID:        uuidPtrToStringPtr(r.ChannelID),
		RequiresCode:     r.RequiresCode,
		Priority:         r.Priority,
		StopAfterApply:   stopModeToGQL(r.StopAfterApply),
		ExclusivityGroup: r.ExclusivityGroup,
		ValidFrom:        r.ValidFrom,
		ValidTo:          r.ValidTo,
		ScheduleJSON:     rawMessageToAny(r.ScheduleJSON),
		MaxUses:          r.MaxUses,
		PerCustomerLimit: r.PerCustomerLimit,
		UsedCount:        r.UsedCount,
		IsActive:         r.IsActive,
		Conditions:       conditionsToModel(r.Conditions),
		Actions:          actionsToModel(r.Actions),
		CreatedByAdminID: uuidPtrToStringPtr(r.CreatedByAdminID),
		UpdatedByAdminID: uuidPtrToStringPtr(r.UpdatedByAdminID),
		LabelMediaID:     uuidPtrToStringPtr(r.LabelMediaID),
		BannerMediaID:    uuidPtrToStringPtr(r.BannerMediaID),
		PillText:         r.PillText,
		PillTextColor:    r.PillTextColor,
		PillBgColor:      r.PillBgColor,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
	return out
}

func conditionsToModel(in []promotion.Condition) []*model.PromotionCondition {
	out := make([]*model.PromotionCondition, len(in))
	for i, c := range in {
		out[i] = &model.PromotionCondition{
			ID:            c.ID.String(),
			Type:          c.Type,
			Params:        rawMessageToAny(c.Params),
			ParamsVersion: c.ParamsVersion,
			SortOrder:     c.SortOrder,
		}
	}
	return out
}

func actionsToModel(in []promotion.Action) []*model.PromotionAction {
	out := make([]*model.PromotionAction, len(in))
	for i, a := range in {
		out[i] = &model.PromotionAction{
			ID:            a.ID.String(),
			Type:          a.Type,
			Params:        rawMessageToAny(a.Params),
			ParamsVersion: a.ParamsVersion,
			SortOrder:     a.SortOrder,
		}
	}
	return out
}

func promotionCodeToModel(c promotion.Code) *model.PromotionCode {
	return &model.PromotionCode{
		ID:                c.ID.String(),
		RuleID:            c.RuleID.String(),
		Code:              c.Code,
		MaxUses:           c.MaxUses,
		PerCustomerLimit:  c.PerCustomerLimit,
		ValidFrom:         c.ValidFrom,
		ValidTo:           c.ValidTo,
		RestrictedToEmail: c.RestrictedToEmail,
		UsedCount:         c.UsedCount,
		IsActive:          c.IsActive,
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
	}
}

// ID stays zero for the repo to set, UsedCount stays zero for the Redeem path to own
func createInputToRule(in model.CreatePromotionRuleInput) (promotion.Rule, error) {
	channelID, err := optionalUUID(in.ChannelID)
	if err != nil {
		return promotion.Rule{}, fmt.Errorf("channelId: %w", err)
	}
	r := promotion.Rule{
		Slug:             in.Slug,
		Name:             in.Name,
		Description:      ptr.Deref(in.Description),
		PublicLabel:      ptr.Deref(in.PublicLabel),
		ChannelID:        channelID,
		RequiresCode:     derefBoolDefault(in.RequiresCode, true),
		Priority:         ptr.Deref(in.Priority),
		StopAfterApply:   stopModeFromGQLPtr(in.StopAfterApply),
		ExclusivityGroup: ptr.Deref(in.ExclusivityGroup),
		ValidFrom:        in.ValidFrom,
		ValidTo:          in.ValidTo,
		MaxUses:          in.MaxUses,
		PerCustomerLimit: in.PerCustomerLimit,
		IsActive:         derefBoolDefault(in.IsActive, true),
	}
	if in.ScheduleJSON != nil {
		raw, err := anyToRawMessage(in.ScheduleJSON)
		if err != nil {
			return promotion.Rule{}, fmt.Errorf("scheduleJson: %w", err)
		}
		r.ScheduleJSON = raw
	}
	if in.LabelMediaID != nil {
		id, err := optionalUUID(in.LabelMediaID)
		if err != nil {
			return promotion.Rule{}, fmt.Errorf("labelMediaId: %w", err)
		}
		r.LabelMediaID = id
	}
	if in.BannerMediaID != nil {
		id, err := optionalUUID(in.BannerMediaID)
		if err != nil {
			return promotion.Rule{}, fmt.Errorf("bannerMediaId: %w", err)
		}
		r.BannerMediaID = id
	}
	if in.PillText != nil {
		r.PillText = *in.PillText
	}
	if in.PillTextColor != nil {
		r.PillTextColor = *in.PillTextColor
	}
	if in.PillBgColor != nil {
		r.PillBgColor = *in.PillBgColor
	}
	return r, nil
}

func updateInputToRule(existing promotion.Rule, in model.UpdatePromotionRuleInput) (promotion.Rule, error) {
	r := existing
	if in.Slug != nil {
		r.Slug = *in.Slug
	}
	if in.Name != nil {
		r.Name = *in.Name
	}
	if in.Description != nil {
		r.Description = *in.Description
	}
	if in.PublicLabel != nil {
		r.PublicLabel = *in.PublicLabel
	}
	if in.ChannelID != nil {
		if *in.ChannelID == "" { // empty string clears the scope, meaning all stores
			r.ChannelID = nil
		} else {
			id, err := uuid.Parse(*in.ChannelID)
			if err != nil {
				return promotion.Rule{}, fmt.Errorf("channelId: %w", err)
			}
			r.ChannelID = &id
		}
	}
	if in.RequiresCode != nil {
		r.RequiresCode = *in.RequiresCode
	}
	if in.Priority != nil {
		r.Priority = *in.Priority
	}
	if in.StopAfterApply != nil {
		r.StopAfterApply = stopModeFromGQL(*in.StopAfterApply)
	}
	if in.ExclusivityGroup != nil {
		r.ExclusivityGroup = *in.ExclusivityGroup
	}
	if in.ValidFrom != nil {
		r.ValidFrom = in.ValidFrom
	}
	if in.ValidTo != nil {
		r.ValidTo = in.ValidTo
	}
	if in.MaxUses != nil {
		r.MaxUses = in.MaxUses
	}
	if in.PerCustomerLimit != nil {
		r.PerCustomerLimit = in.PerCustomerLimit
	}
	if in.IsActive != nil {
		r.IsActive = *in.IsActive
	}
	if in.ScheduleJSON != nil {
		raw, err := anyToRawMessage(in.ScheduleJSON)
		if err != nil {
			return promotion.Rule{}, fmt.Errorf("scheduleJson: %w", err)
		}
		r.ScheduleJSON = raw
	}
	if in.LabelMediaID != nil {
		id, err := optionalUUID(in.LabelMediaID)
		if err != nil {
			return promotion.Rule{}, fmt.Errorf("labelMediaId: %w", err)
		}
		r.LabelMediaID = id
	}
	if in.BannerMediaID != nil {
		id, err := optionalUUID(in.BannerMediaID)
		if err != nil {
			return promotion.Rule{}, fmt.Errorf("bannerMediaId: %w", err)
		}
		r.BannerMediaID = id
	}
	if in.PillText != nil {
		r.PillText = *in.PillText
	}
	if in.PillTextColor != nil {
		r.PillTextColor = *in.PillTextColor
	}
	if in.PillBgColor != nil {
		r.PillBgColor = *in.PillBgColor
	}
	return r, nil
}

func conditionInputsToDomain(in []*model.PromotionConditionInput) ([]promotion.Condition, error) {
	out := make([]promotion.Condition, len(in))
	for i, c := range in {
		raw, err := anyToRawMessage(c.Params)
		if err != nil {
			return nil, fmt.Errorf("condition[%d].params: %w", i, err)
		}
		out[i] = promotion.Condition{
			Type:          c.Type,
			Params:        raw,
			ParamsVersion: derefInt32Default(c.ParamsVersion, 1),
			SortOrder:     derefInt32WithIndex(c.SortOrder, i),
		}
	}
	return out, nil
}

func actionInputsToDomain(in []*model.PromotionActionInput) ([]promotion.Action, error) {
	out := make([]promotion.Action, len(in))
	for i, a := range in {
		raw, err := anyToRawMessage(a.Params)
		if err != nil {
			return nil, fmt.Errorf("action[%d].params: %w", i, err)
		}
		out[i] = promotion.Action{
			Type:          a.Type,
			Params:        raw,
			ParamsVersion: derefInt32Default(a.ParamsVersion, 1),
			SortOrder:     derefInt32WithIndex(a.SortOrder, i),
		}
	}
	return out, nil
}

// ---- enum mapping ----

func stopModeToGQL(m promotion.StopMode) model.PromotionStopMode {
	switch m {
	case promotion.StopAll:
		return model.PromotionStopModeAll
	case promotion.StopSameActionType:
		return model.PromotionStopModeSameActionType
	default:
		return model.PromotionStopModeNone
	}
}

func stopModeFromGQL(m model.PromotionStopMode) promotion.StopMode {
	switch m {
	case model.PromotionStopModeAll:
		return promotion.StopAll
	case model.PromotionStopModeSameActionType:
		return promotion.StopSameActionType
	default:
		return promotion.StopNone
	}
}

func stopModeFromGQLPtr(m *model.PromotionStopMode) promotion.StopMode {
	if m == nil {
		return promotion.StopNone
	}
	return stopModeFromGQL(*m)
}

// ---- scalar conversion helpers ----

func uuidPtrToStringPtr(u *uuid.UUID) *string {
	if u == nil {
		return nil
	}
	s := u.String()
	return &s
}

func optionalUUID(s *string) (*uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func derefBoolDefault(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

func derefInt32Default(i *int32, def int32) int32 {
	if i == nil {
		return def
	}
	return *i
}

func derefInt32WithIndex(i *int32, idx int) int32 { // falling back to the list index keeps ordering stable when no sort order was supplied
	if i == nil {
		return int32(idx)
	}
	return *i
}

func anyToRawMessage(v any) (json.RawMessage, error) { // gqlgen already decoded the inbound JSON, so this re-marshals it
	if v == nil {
		return json.RawMessage("null"), nil
	}
	if raw, ok := v.(json.RawMessage); ok {
		return raw, nil
	}
	return json.Marshal(v)
}

func rawMessageToAny(raw json.RawMessage) any { // empty and null both surface as nil, which is GraphQL null
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}

// Quietly nil on a missing id or unknown asset rather than erroring - the UI already handles the "no media" state
func (r *promotionRuleResolver) loadPromotionMedia(ctx context.Context, mediaID *string) (*model.MediaAsset, error) {
	if mediaID == nil || *mediaID == "" || r.MediaLibrarySvc == nil {
		return nil, nil
	}
	id, err := uuid.Parse(*mediaID)
	if err != nil {
		return nil, nil
	}
	asset, err := r.MediaLibrarySvc.GetAsset(ctx, id)
	if err != nil {
		return nil, nil
	}
	return toMediaAssetModel(asset), nil
}

// The analytics SQL is inclusive on both bounds, so epoch-to-now turns "leave both blank" into "all time"
func defaultAnalyticsRange(start, end *time.Time) (time.Time, time.Time) {
	s := time.Unix(0, 0)
	e := time.Now()
	if start != nil {
		s = *start
	}
	if end != nil {
		e = *end
	}
	return s, e
}

// Synthetic payload only - the simulate path never touches a real cart or customer, so operators can what-if without persisting anything
func simulationInputToEnv(in model.PromotionSimulationInput) (promotion.Environment, error) {
	lines := make([]promotion.CartLineView, len(in.Lines))
	for i, l := range in.Lines {
		var categoryIDs []uuid.UUID
		for _, raw := range l.CategoryIds {
			id, err := uuid.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("lines[%d].categoryIds: %w", i, err)
			}
			categoryIDs = append(categoryIDs, id)
		}
		lines[i] = promotion.CartLineView{
			LineID:         uuid.New(),
			ProductSKU:     l.ProductSku,
			Quantity:       l.Quantity,
			UnitPriceCents: int64(l.UnitPriceCents),
			ProductKind:    ptr.Deref(l.ProductKind),
			CategoryIDs:    categoryIDs,
		}
	}
	return promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents:        int64(in.SubtotalCents),
		Currency:             ptr.Deref(in.Currency),
		CountryCode:          ptr.Deref(in.CountryCode),
		CustomerGroupSlug:    ptr.Deref(in.CustomerGroupSlug),
		CustomerSegmentSlugs: in.CustomerSegmentSlugs,
		Lines:                lines,
		AppliedCodes:         in.AppliedCodes,
	}), nil
}

// The promo-code, rule-content and import mutations all reach a rule by id - they need the same channel gate as the rule CRUD, and a nil channel means a global rule needing an unscoped grant
func (r *mutationResolver) requirePromotionRuleManage(ctx context.Context, ruleID uuid.UUID) error {
	rule, err := r.PromotionSvc.Repo().GetRule(ctx, ruleID)
	if err != nil {
		return err
	}
	return auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "promotions.manage", rule.ChannelID)
}

func (r *Resolver) promotionAnalyticsByRule(ctx context.Context, s, e time.Time, channels []uuid.UUID) ([]promotion.RuleAnalytics, error) {
	if channels == nil {
		return r.PromotionSvc.AnalyticsByRule(ctx, s, e)
	}
	rows, err := r.Queries.PromotionAnalyticsByRuleForChannels(ctx, db.PromotionAnalyticsByRuleForChannelsParams{ChannelIds: channels, StartTime: s, EndTime: e})
	if err != nil {
		return nil, err
	}
	out := make([]promotion.RuleAnalytics, len(rows))
	for i, row := range rows {
		out[i] = promotion.RuleAnalytics{
			RuleID: row.RuleID, RuleSlug: row.RuleSlug, RuleName: row.RuleName, RedemptionCount: row.RedemptionCount,
			DiscountTotalCents: row.DiscountTotalCents, UniqueCustomers: row.UniqueCustomers, OrderRevenueCents: row.OrderRevenueCents,
		}
	}
	return out, nil
}

func (r *Resolver) promotionAnalyticsTotals(ctx context.Context, s, e time.Time, channels []uuid.UUID) (promotion.AnalyticsTotals, error) {
	if channels == nil {
		return r.PromotionSvc.AnalyticsTotals(ctx, s, e)
	}
	row, err := r.Queries.PromotionAnalyticsTotalsForChannels(ctx, db.PromotionAnalyticsTotalsForChannelsParams{ChannelIds: channels, StartTime: s, EndTime: e})
	if err != nil {
		return promotion.AnalyticsTotals{}, err
	}
	return promotion.AnalyticsTotals{RedemptionCount: row.RedemptionCount, DiscountTotalCents: row.DiscountTotalCents, UniqueCustomers: row.UniqueCustomers}, nil
}

func (r *Resolver) promotionAnalyticsDaily(ctx context.Context, s, e time.Time, ruleID *uuid.UUID, channels []uuid.UUID) ([]promotion.AnalyticsDailyPoint, error) {
	if channels == nil {
		return r.PromotionSvc.AnalyticsDaily(ctx, s, e, ruleID)
	}
	var rid uuid.NullUUID
	if ruleID != nil {
		rid = uuid.NullUUID{UUID: *ruleID, Valid: true}
	}
	rows, err := r.Queries.PromotionAnalyticsDailyForChannels(ctx, db.PromotionAnalyticsDailyForChannelsParams{ChannelIds: channels, StartTime: s, EndTime: e, RuleID: rid})
	if err != nil {
		return nil, err
	}
	out := make([]promotion.AnalyticsDailyPoint, len(rows))
	for i, row := range rows {
		out[i] = promotion.AnalyticsDailyPoint{Day: row.Day, RedemptionCount: row.RedemptionCount, DiscountTotalCents: row.DiscountTotalCents}
	}
	return out, nil
}

// nil means every channel, for a grant that holds everywhere
func (r *Resolver) promotionAnalyticsChannels(ctx context.Context) ([]uuid.UUID, error) {
	permitted, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "promotions.view")
	if err != nil {
		return nil, err
	}
	if permitted.Bypass() {
		return nil, nil
	}
	return permitted.IDs(), nil
}
