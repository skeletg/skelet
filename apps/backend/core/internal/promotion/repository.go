package promotion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

var (
	ErrRuleNotFound = errors.New("promotion: rule not found")
	ErrCodeNotFound = errors.New("promotion: code not found")
	ErrSlugTaken    = errors.New("promotion: rule slug already exists")
	ErrCodeTaken    = errors.New("promotion: code already exists")
)

// Repository is the DB layer for the v2 engine - Mirrors the existing payment / catalog repos: interface for
// swappability in tests, postgresRepository for prod, WithTx for transactional use inside order placement
type Repository interface {
	WithTx(t *sql.Tx) Repository

	// Rules
	CreateRule(ctx context.Context, r Rule) (Rule, error)
	UpdateRule(ctx context.Context, r Rule) (Rule, error)
	DeleteRule(ctx context.Context, id uuid.UUID) error
	GetRule(ctx context.Context, id uuid.UUID) (Rule, error)
	GetRulesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Rule, error)
	ListRules(ctx context.Context, f RuleFilter, limit, offset int32) ([]Rule, int32, error)
	ListActiveRulesForChannel(ctx context.Context, channelID *uuid.UUID) ([]Rule, error)
	IncrementRuleUsage(ctx context.Context, id uuid.UUID) error
	ReorderRules(ctx context.Context, orderedIDs []uuid.UUID) error

	// Codes
	CreateCode(ctx context.Context, c Code) (Code, error)
	UpdateCode(ctx context.Context, c Code) (Code, error)
	DeleteCode(ctx context.Context, id uuid.UUID) error
	GetCode(ctx context.Context, id uuid.UUID) (Code, error)
	GetCodeByCode(ctx context.Context, code string) (Code, error)
	// search matches the code and a nil active keeps both; the total counts every match, not just this page
	ListCodesByRule(ctx context.Context, ruleID uuid.UUID, search string, active *bool, limit, offset int32) ([]Code, int32, error)
	// The codes actually inserted - a collision with an existing code is skipped, so this can be fewer and in any order
	BulkInsertCodes(ctx context.Context, ruleID uuid.UUID, codes []string) ([]string, error)
	IncrementCodeUsage(ctx context.Context, id uuid.UUID) error
	// Redemptions by this shopper per rule and per code, for the per-customer limits - nil maps when the shopper is unknown
	ShopperRedemptions(ctx context.Context, sh Shopper, ruleIDs, codeIDs []uuid.UUID) (byRule, byCode map[uuid.UUID]int32, err error)

	// Conditions
	ReplaceRuleConditions(ctx context.Context, ruleID uuid.UUID, conditions []Condition) error
	ListConditionsByRule(ctx context.Context, ruleID uuid.UUID) ([]Condition, error)
	ListConditionsByRules(ctx context.Context, ruleIDs []uuid.UUID) ([]Condition, error)

	// Actions
	ReplaceRuleActions(ctx context.Context, ruleID uuid.UUID, actions []Action) error
	ListActionsByRule(ctx context.Context, ruleID uuid.UUID) ([]Action, error)
	ListActionsByRules(ctx context.Context, ruleIDs []uuid.UUID) ([]Action, error)

	// Cart linkage
	ApplyCodeToCart(ctx context.Context, cartID, ruleID uuid.UUID, codeID *uuid.UUID) error
	RemoveCodeFromCartByCode(ctx context.Context, cartID uuid.UUID, code string) error
	ClearCartAppliedCodes(ctx context.Context, cartID uuid.UUID) error
	ListCartAppliedCodes(ctx context.Context, cartID uuid.UUID) ([]AppliedCodeRecord, error)
	MarkCartCodeInvalidated(ctx context.Context, cartID, ruleID uuid.UUID, reason string) error
	// The shopper's email for the restricted_to_email code gate - empty for a guest cart with no checkout email yet
	GetCartEmail(ctx context.Context, cartID uuid.UUID) (string, error)

	// Redemptions
	RecordRedemption(ctx context.Context, ruleID uuid.UUID, codeID *uuid.UUID, customerID *uuid.UUID, orderID uuid.UUID, discountCents int64) error
	ListRedemptionsByOrder(ctx context.Context, orderID uuid.UUID) ([]OrderRedemption, error)

	// Cart-line enrichment - the catalog facts the product-scoped conditions gate on, one round-trip for the whole cart
	PromotionLineAttrs(ctx context.Context, skus []string) (map[string]LineProductAttrs, error)

	// i18n contents
	UpsertRuleContent(ctx context.Context, c RuleContent) (RuleContent, error)
	ListRuleContents(ctx context.Context, ruleID uuid.UUID) ([]RuleContent, error)
	DeleteRuleContent(ctx context.Context, ruleID uuid.UUID, locale string) error
	GetRuleContent(ctx context.Context, ruleID uuid.UUID, locale string) (RuleContent, error)
	GetRuleContentsByIDs(ctx context.Context, ruleIDs []uuid.UUID, locale string) (map[uuid.UUID]RuleContent, error)

	// Analytics
	AnalyticsByRule(ctx context.Context, start, end time.Time, scope AnalyticsScope) ([]RuleAnalytics, error)
	AnalyticsTotals(ctx context.Context, start, end time.Time, scope AnalyticsScope) (AnalyticsTotals, error)
	AnalyticsDaily(ctx context.Context, start, end time.Time, ruleID *uuid.UUID, scope AnalyticsScope) ([]AnalyticsDailyPoint, error)
}

// Which rules a listing may return - every rule, the given channels' rules, and optionally the channel-less global ones
type RuleFilter struct {
	AllChannels   bool
	ChannelIDs    []uuid.UUID
	IncludeGlobal bool
}

// Who a per-customer limit counts against - the account when signed in, and the order email either way, so a guest checkout counts too
type Shopper struct {
	CustomerID *uuid.UUID
	Email      string
}

func (sh Shopper) known() bool { return sh.CustomerID != nil || sh.Email != "" }

func NewShopper(customerID *uuid.UUID, email string) Shopper {
	return Shopper{CustomerID: customerID, Email: strings.ToLower(strings.TrimSpace(email))}
}

// Codes match case-insensitively for the shopper, so every write stores the canonical form ApplyCode looks up
func normalizeCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

// Every channel only when All says so - empty ChannelIDs reads nothing, so a scope built up conditionally can't widen by accident
type AnalyticsScope struct {
	All        bool
	ChannelIDs []uuid.UUID
}

type RuleAnalytics struct {
	RuleID             uuid.UUID
	RuleSlug           string
	RuleName           string
	RedemptionCount    int32
	DiscountTotalCents int64
	UniqueCustomers    int32
	OrderRevenueCents  int64
}

// AnalyticsTotals is the dashboard's "all promotions combined" tile
type AnalyticsTotals struct {
	RedemptionCount    int32
	DiscountTotalCents int64
	UniqueCustomers    int32
}

// AnalyticsDailyPoint is one day's redemption + discount bucket - Used for the
// promotions hub trend chart (all rules) and per-promotion trends (one rule)
type AnalyticsDailyPoint struct {
	Day                time.Time
	RedemptionCount    int32
	DiscountTotalCents int64
}

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) WithTx(t *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(t)}
}

// ---- Rules ----

func (r *postgresRepository) CreateRule(ctx context.Context, in Rule) (Rule, error) {
	row, err := r.queries.CreatePromotionRule(ctx, db.CreatePromotionRuleParams{
		Slug:             in.Slug,
		Name:             in.Name,
		Description:      in.Description,
		PublicLabel:      in.PublicLabel,
		ChannelID:        sqlnull.UUID(in.ChannelID),
		RequiresCode:     in.RequiresCode,
		Priority:         in.Priority,
		StopAfterApply:   string(in.StopAfterApply),
		ExclusivityGroup: sqlnull.StringValueNonEmpty(in.ExclusivityGroup),
		ValidFrom:        sqlnull.Time(in.ValidFrom),
		ValidTo:          sqlnull.Time(in.ValidTo),
		ScheduleJson:     sqlnull.Raw(in.ScheduleJSON),
		MaxUses:          sqlnull.Int32(in.MaxUses),
		PerCustomerLimit: sqlnull.Int32(in.PerCustomerLimit),
		IsActive:         in.IsActive,
		// Same attribution column for both on create - admin who
		// pressed "Save" is both creator and last editor
		CreatedByAdminID: sqlnull.UUID(in.CreatedByAdminID),
		UpdatedByAdminID: sqlnull.UUID(in.CreatedByAdminID),
		LabelMediaID:     sqlnull.UUID(in.LabelMediaID),
		BannerMediaID:    sqlnull.UUID(in.BannerMediaID),
		PillText:         in.PillText,
		PillTextColor:    in.PillTextColor,
		PillBgColor:      in.PillBgColor,
	})
	if err != nil {
		if pgdb.IsUniqueViolation(err) {
			return Rule{}, ErrSlugTaken
		}
		return Rule{}, fmt.Errorf("create rule: %w", err)
	}
	return toRule(row), nil
}

func (r *postgresRepository) UpdateRule(ctx context.Context, in Rule) (Rule, error) {
	row, err := r.queries.UpdatePromotionRule(ctx, db.UpdatePromotionRuleParams{
		ID:               in.ID,
		Slug:             in.Slug,
		Name:             in.Name,
		Description:      in.Description,
		PublicLabel:      in.PublicLabel,
		ChannelID:        sqlnull.UUID(in.ChannelID),
		RequiresCode:     in.RequiresCode,
		Priority:         in.Priority,
		StopAfterApply:   string(in.StopAfterApply),
		ExclusivityGroup: sqlnull.StringValueNonEmpty(in.ExclusivityGroup),
		ValidFrom:        sqlnull.Time(in.ValidFrom),
		ValidTo:          sqlnull.Time(in.ValidTo),
		ScheduleJson:     sqlnull.Raw(in.ScheduleJSON),
		MaxUses:          sqlnull.Int32(in.MaxUses),
		PerCustomerLimit: sqlnull.Int32(in.PerCustomerLimit),
		IsActive:         in.IsActive,
		UpdatedByAdminID: sqlnull.UUID(in.UpdatedByAdminID),
		LabelMediaID:     sqlnull.UUID(in.LabelMediaID),
		BannerMediaID:    sqlnull.UUID(in.BannerMediaID),
		PillText:         in.PillText,
		PillTextColor:    in.PillTextColor,
		PillBgColor:      in.PillBgColor,
	})
	if err != nil {
		return Rule{}, fmt.Errorf("update rule: %w", err)
	}
	return toRule(row), nil
}

func (r *postgresRepository) DeleteRule(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeletePromotionRule(ctx, id); err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetRule(ctx context.Context, id uuid.UUID) (Rule, error) {
	row, err := r.queries.GetPromotionRule(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Rule{}, ErrRuleNotFound
		}
		return Rule{}, fmt.Errorf("get rule: %w", err)
	}
	return toRule(row), nil
}

func (r *postgresRepository) GetRulesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Rule, error) {
	out := make(map[uuid.UUID]Rule, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListPromotionRulesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get rules by ids: %w", err)
	}
	for _, row := range rows {
		out[row.ID] = toRule(row)
	}
	return out, nil
}

func (r *postgresRepository) ListRules(ctx context.Context, f RuleFilter, limit, offset int32) ([]Rule, int32, error) {
	rows, err := r.queries.ListPromotionRules(ctx, db.ListPromotionRulesParams{
		AllChannels:   f.AllChannels,
		ChannelIds:    f.ChannelIDs,
		IncludeGlobal: f.IncludeGlobal,
		Lim:           limit,
		Off:           offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list rules: %w", err)
	}
	count, err := r.queries.CountPromotionRules(ctx, db.CountPromotionRulesParams{
		AllChannels:   f.AllChannels,
		ChannelIds:    f.ChannelIDs,
		IncludeGlobal: f.IncludeGlobal,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count rules: %w", err)
	}
	out := make([]Rule, len(rows))
	for i, row := range rows {
		out[i] = toRule(row)
	}
	return out, count, nil
}

func (r *postgresRepository) ListActiveRulesForChannel(ctx context.Context, channelID *uuid.UUID) ([]Rule, error) {
	rows, err := r.queries.ListActiveRulesForChannel(ctx, sqlnull.UUID(channelID))
	if err != nil {
		return nil, fmt.Errorf("list active rules: %w", err)
	}
	out := make([]Rule, len(rows))
	for i, row := range rows {
		out[i] = toRule(row)
	}
	return out, nil
}

func (r *postgresRepository) IncrementRuleUsage(ctx context.Context, id uuid.UUID) error {
	n, err := r.queries.IncrementRuleUsage(ctx, id)
	if err != nil {
		return fmt.Errorf("increment rule usage: %w", err)
	}
	if n == 0 {
		// Cap-guarded UPDATE matched no row: used_count reached max_uses (the row itself is known to exist - the engine just
		// evaluated it) - Surface the same sentinel ApplyCode uses so the storefront shows the right message
		return ErrRuleMaxUses
	}
	return nil
}

func (r *postgresRepository) ReorderRules(ctx context.Context, orderedIDs []uuid.UUID) error {
	if len(orderedIDs) == 0 {
		return nil
	}
	if err := r.queries.ReorderPromotionRules(ctx, orderedIDs); err != nil {
		return fmt.Errorf("reorder rules: %w", err)
	}
	return nil
}

// ---- Codes ----

func (r *postgresRepository) CreateCode(ctx context.Context, in Code) (Code, error) {
	row, err := r.queries.CreatePromotionCode(ctx, db.CreatePromotionCodeParams{
		RuleID:            in.RuleID,
		Code:              normalizeCode(in.Code),
		MaxUses:           sqlnull.Int32(in.MaxUses),
		PerCustomerLimit:  sqlnull.Int32(in.PerCustomerLimit),
		ValidFrom:         sqlnull.Time(in.ValidFrom),
		ValidTo:           sqlnull.Time(in.ValidTo),
		RestrictedToEmail: sqlnull.StringValueNonEmpty(in.RestrictedToEmail),
		IsActive:          in.IsActive,
	})
	if err != nil {
		if pgdb.IsUniqueViolation(err) {
			return Code{}, ErrCodeTaken
		}
		return Code{}, fmt.Errorf("create code: %w", err)
	}
	return toCode(row), nil
}

func (r *postgresRepository) UpdateCode(ctx context.Context, in Code) (Code, error) {
	row, err := r.queries.UpdatePromotionCode(ctx, db.UpdatePromotionCodeParams{
		ID:                in.ID,
		Code:              normalizeCode(in.Code),
		MaxUses:           sqlnull.Int32(in.MaxUses),
		PerCustomerLimit:  sqlnull.Int32(in.PerCustomerLimit),
		ValidFrom:         sqlnull.Time(in.ValidFrom),
		ValidTo:           sqlnull.Time(in.ValidTo),
		RestrictedToEmail: sqlnull.StringValueNonEmpty(in.RestrictedToEmail),
		IsActive:          in.IsActive,
	})
	if err != nil {
		if pgdb.IsUniqueViolation(err) {
			return Code{}, ErrCodeTaken
		}
		return Code{}, fmt.Errorf("update code: %w", err)
	}
	return toCode(row), nil
}

func (r *postgresRepository) DeleteCode(ctx context.Context, id uuid.UUID) error {
	return r.queries.DeletePromotionCode(ctx, id)
}

func (r *postgresRepository) GetCode(ctx context.Context, id uuid.UUID) (Code, error) {
	row, err := r.queries.GetPromotionCode(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Code{}, ErrCodeNotFound
		}
		return Code{}, fmt.Errorf("get code: %w", err)
	}
	return toCode(row), nil
}

func (r *postgresRepository) GetCodeByCode(ctx context.Context, code string) (Code, error) {
	row, err := r.queries.GetPromotionCodeByCode(ctx, code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Code{}, ErrCodeNotFound
		}
		return Code{}, fmt.Errorf("get code by code: %w", err)
	}
	return toCode(row), nil
}

func (r *postgresRepository) ListCodesByRule(ctx context.Context, ruleID uuid.UUID, search string, active *bool, limit, offset int32) ([]Code, int32, error) {
	var isActive sql.NullBool
	if active != nil {
		isActive = sql.NullBool{Bool: *active, Valid: true}
	}
	rows, err := r.queries.ListPromotionCodesByRule(ctx, db.ListPromotionCodesByRuleParams{
		RuleID:   ruleID,
		Search:   search,
		IsActive: isActive,
		Lim:      limit,
		Off:      offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list codes by rule: %w", err)
	}
	count, err := r.queries.CountPromotionCodesByRule(ctx, db.CountPromotionCodesByRuleParams{RuleID: ruleID, Search: search, IsActive: isActive})
	if err != nil {
		return nil, 0, fmt.Errorf("count codes by rule: %w", err)
	}
	out := make([]Code, len(rows))
	for i, row := range rows {
		out[i] = toCode(row)
	}
	return out, count, nil
}

func (r *postgresRepository) BulkInsertCodes(ctx context.Context, ruleID uuid.UUID, codes []string) ([]string, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	clean := make([]string, len(codes))
	for i, c := range codes {
		clean[i] = normalizeCode(c)
	}
	inserted, err := r.queries.BulkInsertPromotionCodes(ctx, db.BulkInsertPromotionCodesParams{
		RuleID: ruleID,
		Codes:  clean,
	})
	if err != nil {
		return nil, fmt.Errorf("bulk insert codes: %w", err)
	}
	return inserted, nil
}

func (r *postgresRepository) IncrementCodeUsage(ctx context.Context, id uuid.UUID) error {
	n, err := r.queries.IncrementCodeUsage(ctx, id)
	if err != nil {
		return fmt.Errorf("increment code usage: %w", err)
	}
	if n == 0 {
		// Cap-guarded UPDATE matched no row → max_uses reached
		return ErrCodeMaxUses
	}
	return nil
}

func (r *postgresRepository) ShopperRedemptions(ctx context.Context, sh Shopper, ruleIDs, codeIDs []uuid.UUID) (map[uuid.UUID]int32, map[uuid.UUID]int32, error) {
	if !sh.known() || len(ruleIDs)+len(codeIDs) == 0 {
		return nil, nil, nil
	}
	rows, err := r.queries.CountCustomerRedemptions(ctx, db.CountCustomerRedemptionsParams{
		RuleIds:    ruleIDs,
		CodeIds:    codeIDs,
		CustomerID: sqlnull.UUID(sh.CustomerID),
		Email:      sh.Email,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("count shopper redemptions: %w", err)
	}
	byRule, byCode := map[uuid.UUID]int32{}, map[uuid.UUID]int32{}
	for _, row := range rows {
		if row.RuleID.Valid {
			byRule[row.RuleID.UUID] += row.N
		}
		if row.CodeID.Valid {
			byCode[row.CodeID.UUID] += row.N
		}
	}
	return byRule, byCode, nil
}

// ---- Conditions ----

func (r *postgresRepository) ReplaceRuleConditions(ctx context.Context, ruleID uuid.UUID, conditions []Condition) error {
	if err := r.queries.DeleteRuleConditions(ctx, ruleID); err != nil {
		return fmt.Errorf("clear conditions: %w", err)
	}
	for _, c := range conditions {
		if _, err := r.queries.CreatePromotionCondition(ctx, db.CreatePromotionConditionParams{
			RuleID:        ruleID,
			Type:          c.Type,
			Params:        json.RawMessage(c.Params),
			ParamsVersion: c.ParamsVersion,
			SortOrder:     c.SortOrder,
		}); err != nil {
			return fmt.Errorf("create condition: %w", err)
		}
	}
	return nil
}

func (r *postgresRepository) ListConditionsByRule(ctx context.Context, ruleID uuid.UUID) ([]Condition, error) {
	rows, err := r.queries.ListConditionsByRule(ctx, ruleID)
	if err != nil {
		return nil, fmt.Errorf("list conditions: %w", err)
	}
	out := make([]Condition, len(rows))
	for i, row := range rows {
		out[i] = toCondition(row)
	}
	return out, nil
}

func (r *postgresRepository) ListConditionsByRules(ctx context.Context, ruleIDs []uuid.UUID) ([]Condition, error) {
	if len(ruleIDs) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListConditionsByRules(ctx, ruleIDs)
	if err != nil {
		return nil, fmt.Errorf("list conditions by rules: %w", err)
	}
	out := make([]Condition, len(rows))
	for i, row := range rows {
		out[i] = toCondition(row)
	}
	return out, nil
}

// ---- Actions ----

func (r *postgresRepository) ReplaceRuleActions(ctx context.Context, ruleID uuid.UUID, actions []Action) error {
	if err := r.queries.DeleteRuleActions(ctx, ruleID); err != nil {
		return fmt.Errorf("clear actions: %w", err)
	}
	for _, a := range actions {
		if _, err := r.queries.CreatePromotionAction(ctx, db.CreatePromotionActionParams{
			RuleID:        ruleID,
			Type:          a.Type,
			Params:        json.RawMessage(a.Params),
			ParamsVersion: a.ParamsVersion,
			SortOrder:     a.SortOrder,
		}); err != nil {
			return fmt.Errorf("create action: %w", err)
		}
	}
	return nil
}

func (r *postgresRepository) ListActionsByRule(ctx context.Context, ruleID uuid.UUID) ([]Action, error) {
	rows, err := r.queries.ListActionsByRule(ctx, ruleID)
	if err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}
	out := make([]Action, len(rows))
	for i, row := range rows {
		out[i] = toAction(row)
	}
	return out, nil
}

func (r *postgresRepository) ListActionsByRules(ctx context.Context, ruleIDs []uuid.UUID) ([]Action, error) {
	if len(ruleIDs) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListActionsByRules(ctx, ruleIDs)
	if err != nil {
		return nil, fmt.Errorf("list actions by rules: %w", err)
	}
	out := make([]Action, len(rows))
	for i, row := range rows {
		out[i] = toAction(row)
	}
	return out, nil
}

// ---- Cart linkage ----

func (r *postgresRepository) ApplyCodeToCart(ctx context.Context, cartID, ruleID uuid.UUID, codeID *uuid.UUID) error {
	_, err := r.queries.ApplyCodeToCart(ctx, db.ApplyCodeToCartParams{
		CartID: cartID,
		RuleID: ruleID,
		CodeID: sqlnull.UUID(codeID),
	})
	if err != nil {
		return fmt.Errorf("apply code to cart: %w", err)
	}
	return nil
}

func (r *postgresRepository) RemoveCodeFromCartByCode(ctx context.Context, cartID uuid.UUID, code string) error {
	return r.queries.RemoveCodeFromCartByCode(ctx, db.RemoveCodeFromCartByCodeParams{
		CartID: cartID,
		Code:   code,
	})
}

func (r *postgresRepository) ClearCartAppliedCodes(ctx context.Context, cartID uuid.UUID) error {
	return r.queries.ClearCartAppliedCodes(ctx, cartID)
}

func (r *postgresRepository) ListCartAppliedCodes(ctx context.Context, cartID uuid.UUID) ([]AppliedCodeRecord, error) {
	rows, err := r.queries.ListCartAppliedCodes(ctx, cartID)
	if err != nil {
		return nil, fmt.Errorf("list cart applied codes: %w", err)
	}
	out := make([]AppliedCodeRecord, len(rows))
	for i, row := range rows {
		out[i] = AppliedCodeRecord{
			CartID:            row.CartID,
			RuleID:            row.RuleID,
			CodeID:            sqlnull.UUIDPtr(row.CodeID),
			CodeText:          sqlnull.StringValue(row.CodeText),
			AppliedAt:         row.AppliedAt,
			InvalidatedReason: sqlnull.StringValue(row.InvalidatedReason),
		}
	}
	return out, nil
}

// A vanished cart reads as "no email" rather than an error - the gate then fails closed on any restricted code, which is the safe direction
func (r *postgresRepository) GetCartEmail(ctx context.Context, cartID uuid.UUID) (string, error) {
	email, err := r.queries.GetCartEmailForPromotions(ctx, cartID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("get cart email: %w", err)
	}
	return email, nil
}

func (r *postgresRepository) MarkCartCodeInvalidated(ctx context.Context, cartID, ruleID uuid.UUID, reason string) error {
	return r.queries.MarkCartCodeInvalidated(ctx, db.MarkCartCodeInvalidatedParams{
		CartID:            cartID,
		RuleID:            ruleID,
		InvalidatedReason: sqlnull.StringValueNonEmpty(reason),
	})
}

// ---- Redemptions ----

func (r *postgresRepository) ListRedemptionsByOrder(ctx context.Context, orderID uuid.UUID) ([]OrderRedemption, error) {
	rows, err := r.queries.ListPromotionRedemptionsByOrder(ctx, uuid.NullUUID{UUID: orderID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list redemptions by order: %w", err)
	}
	out := make([]OrderRedemption, len(rows))
	for i, row := range rows {
		out[i] = OrderRedemption{
			ID:            row.ID,
			RuleName:      row.RuleName,
			RuleSlug:      row.RuleSlug,
			Code:          row.Code,
			DiscountCents: row.DiscountCents,
			RedeemedAt:    row.RedeemedAt,
		}
		if row.RuleID.Valid {
			id := row.RuleID.UUID
			out[i].RuleID = &id
		}
	}
	return out, nil
}

func (r *postgresRepository) RecordRedemption(ctx context.Context, ruleID uuid.UUID, codeID *uuid.UUID, customerID *uuid.UUID, orderID uuid.UUID, discountCents int64) error {
	return r.queries.RecordPromotionRedemption(ctx, db.RecordPromotionRedemptionParams{
		RuleID:        uuid.NullUUID{UUID: ruleID, Valid: true},
		CodeID:        sqlnull.UUID(codeID),
		CustomerID:    sqlnull.UUID(customerID),
		OrderID:       uuid.NullUUID{UUID: orderID, Valid: true},
		DiscountCents: discountCents,
	})
}

// ---- Cart-line enrichment ----

// Selectable attribute values arrive slug-keyed and comma-joined; free-text scalars ride along in the product's own attributes bag, keyed the same way
// Scalars are formatted the way the catalog resolvers do it, so a numeric attribute compares as "42" and a rule authored against the storefront's value still matches
func (r *postgresRepository) PromotionLineAttrs(ctx context.Context, skus []string) (map[string]LineProductAttrs, error) {
	if len(skus) == 0 {
		return map[string]LineProductAttrs{}, nil
	}
	rows, err := r.queries.GetPromotionLineAttrsBySKUs(ctx, skus)
	if err != nil {
		return nil, fmt.Errorf("promotion line attrs: %w", err)
	}
	out := make(map[string]LineProductAttrs, len(rows))
	for _, row := range rows {
		kvs := map[string]string{}
		var scalars map[string]any
		if len(row.ScalarAttrs) > 0 {
			_ = json.Unmarshal(row.ScalarAttrs, &scalars)
		}
		for slug, v := range scalars {
			if v == nil {
				continue
			}
			kvs[slug] = fmt.Sprintf("%v", v)
		}
		// Selectable values overwrite a same-slug scalar - an attribute is one or the other, and the join tables are the authority
		var selectable map[string]string
		if len(row.SelectableAttrs) > 0 {
			_ = json.Unmarshal(row.SelectableAttrs, &selectable)
		}
		maps.Copy(kvs, selectable)
		out[row.Sku] = LineProductAttrs{
			ProductID:    row.ProductID,
			Kind:         row.Kind,
			CategoryIDs:  row.CategoryIds,
			AttributeKVs: kvs,
		}
	}
	return out, nil
}

// ---- Row → domain conversions ----

func toRule(row db.PromotionRule) Rule {
	return Rule{
		ID:               row.ID,
		Slug:             row.Slug,
		Name:             row.Name,
		Description:      row.Description,
		PublicLabel:      row.PublicLabel,
		ChannelID:        sqlnull.UUIDPtr(row.ChannelID),
		RequiresCode:     row.RequiresCode,
		Priority:         row.Priority,
		StopAfterApply:   StopMode(row.StopAfterApply),
		ExclusivityGroup: sqlnull.StringValue(row.ExclusivityGroup),
		ValidFrom:        sqlnull.TimePtr(row.ValidFrom),
		ValidTo:          sqlnull.TimePtr(row.ValidTo),
		ScheduleJSON:     sqlnull.RawMessage(row.ScheduleJson),
		MaxUses:          sqlnull.Int32Ptr(row.MaxUses),
		PerCustomerLimit: sqlnull.Int32Ptr(row.PerCustomerLimit),
		UsedCount:        row.UsedCount,
		IsActive:         row.IsActive,
		CreatedByAdminID: sqlnull.UUIDPtr(row.CreatedByAdminID),
		UpdatedByAdminID: sqlnull.UUIDPtr(row.UpdatedByAdminID),
		LabelMediaID:     sqlnull.UUIDPtr(row.LabelMediaID),
		BannerMediaID:    sqlnull.UUIDPtr(row.BannerMediaID),
		PillText:         row.PillText,
		PillTextColor:    row.PillTextColor,
		PillBgColor:      row.PillBgColor,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func toCode(row db.PromotionCode) Code {
	return Code{
		ID:                row.ID,
		RuleID:            row.RuleID,
		Code:              row.Code,
		MaxUses:           sqlnull.Int32Ptr(row.MaxUses),
		PerCustomerLimit:  sqlnull.Int32Ptr(row.PerCustomerLimit),
		ValidFrom:         sqlnull.TimePtr(row.ValidFrom),
		ValidTo:           sqlnull.TimePtr(row.ValidTo),
		RestrictedToEmail: sqlnull.StringValue(row.RestrictedToEmail),
		UsedCount:         row.UsedCount,
		IsActive:          row.IsActive,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func toCondition(row db.PromotionCondition) Condition {
	return Condition{
		ID:            row.ID,
		RuleID:        row.RuleID,
		Type:          row.Type,
		Params:        row.Params,
		ParamsVersion: row.ParamsVersion,
		SortOrder:     row.SortOrder,
	}
}

func toAction(row db.PromotionAction) Action {
	return Action{
		ID:            row.ID,
		RuleID:        row.RuleID,
		Type:          row.Type,
		Params:        row.Params,
		ParamsVersion: row.ParamsVersion,
		SortOrder:     row.SortOrder,
	}
}

// ---- i18n rule contents ----

func (r *postgresRepository) UpsertRuleContent(ctx context.Context, in RuleContent) (RuleContent, error) {
	row, err := r.queries.UpsertPromotionRuleContent(ctx, db.UpsertPromotionRuleContentParams{
		RuleID: in.RuleID,
		Locale: in.Locale,
		Label:  in.Label,
		Terms:  in.Terms,
	})
	if err != nil {
		return RuleContent{}, fmt.Errorf("upsert rule content: %w", err)
	}
	return toRuleContent(row), nil
}

func (r *postgresRepository) ListRuleContents(ctx context.Context, ruleID uuid.UUID) ([]RuleContent, error) {
	rows, err := r.queries.ListPromotionRuleContents(ctx, ruleID)
	if err != nil {
		return nil, fmt.Errorf("list rule contents: %w", err)
	}
	out := make([]RuleContent, len(rows))
	for i, row := range rows {
		out[i] = toRuleContent(row)
	}
	return out, nil
}

func (r *postgresRepository) DeleteRuleContent(ctx context.Context, ruleID uuid.UUID, locale string) error {
	return r.queries.DeletePromotionRuleContent(ctx, db.DeletePromotionRuleContentParams{
		RuleID: ruleID,
		Locale: locale,
	})
}

func (r *postgresRepository) GetRuleContent(ctx context.Context, ruleID uuid.UUID, locale string) (RuleContent, error) {
	row, err := r.queries.GetPromotionRuleContent(ctx, db.GetPromotionRuleContentParams{
		RuleID: ruleID,
		Locale: locale,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RuleContent{}, sql.ErrNoRows
		}
		return RuleContent{}, fmt.Errorf("get rule content: %w", err)
	}
	return toRuleContent(row), nil
}

func (r *postgresRepository) GetRuleContentsByIDs(ctx context.Context, ruleIDs []uuid.UUID, locale string) (map[uuid.UUID]RuleContent, error) {
	if len(ruleIDs) == 0 || locale == "" {
		return map[uuid.UUID]RuleContent{}, nil
	}
	rows, err := r.queries.GetPromotionRuleContentsByIDs(ctx, db.GetPromotionRuleContentsByIDsParams{
		RuleIds: ruleIDs,
		Locale:  locale,
	})
	if err != nil {
		return nil, fmt.Errorf("get rule contents by ids: %w", err)
	}
	out := make(map[uuid.UUID]RuleContent, len(rows))
	for _, row := range rows {
		out[row.RuleID] = toRuleContent(row)
	}
	return out, nil
}

func toRuleContent(row db.PromotionRuleContent) RuleContent {
	return RuleContent{
		ID:        row.ID,
		RuleID:    row.RuleID,
		Locale:    row.Locale,
		Label:     row.Label,
		Terms:     row.Terms,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// ---- Analytics ----

func (r *postgresRepository) AnalyticsByRule(ctx context.Context, start, end time.Time, scope AnalyticsScope) ([]RuleAnalytics, error) {
	var rows []db.PromotionAnalyticsByRuleRow
	var err error
	if scope.All {
		rows, err = r.queries.PromotionAnalyticsByRule(ctx, db.PromotionAnalyticsByRuleParams{RedeemedAt: start, RedeemedAt_2: end})
	} else {
		var scoped []db.PromotionAnalyticsByRuleForChannelsRow
		scoped, err = r.queries.PromotionAnalyticsByRuleForChannels(ctx, db.PromotionAnalyticsByRuleForChannelsParams{ChannelIds: scope.ChannelIDs, StartTime: start, EndTime: end})
		rows = make([]db.PromotionAnalyticsByRuleRow, len(scoped))
		for i, row := range scoped {
			rows[i] = db.PromotionAnalyticsByRuleRow(row)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("analytics by rule: %w", err)
	}
	out := make([]RuleAnalytics, len(rows))
	for i, row := range rows {
		out[i] = RuleAnalytics{
			RuleID:             row.RuleID,
			RuleSlug:           row.RuleSlug,
			RuleName:           row.RuleName,
			RedemptionCount:    row.RedemptionCount,
			DiscountTotalCents: row.DiscountTotalCents,
			UniqueCustomers:    row.UniqueCustomers,
			OrderRevenueCents:  row.OrderRevenueCents,
		}
	}
	return out, nil
}

func (r *postgresRepository) AnalyticsTotals(ctx context.Context, start, end time.Time, scope AnalyticsScope) (AnalyticsTotals, error) {
	var row db.PromotionAnalyticsTotalsRow
	var err error
	if scope.All {
		row, err = r.queries.PromotionAnalyticsTotals(ctx, db.PromotionAnalyticsTotalsParams{RedeemedAt: start, RedeemedAt_2: end})
	} else {
		var scoped db.PromotionAnalyticsTotalsForChannelsRow
		scoped, err = r.queries.PromotionAnalyticsTotalsForChannels(ctx, db.PromotionAnalyticsTotalsForChannelsParams{ChannelIds: scope.ChannelIDs, StartTime: start, EndTime: end})
		row = db.PromotionAnalyticsTotalsRow(scoped)
	}
	if err != nil {
		return AnalyticsTotals{}, fmt.Errorf("analytics totals: %w", err)
	}
	return AnalyticsTotals{
		RedemptionCount:    row.RedemptionCount,
		DiscountTotalCents: row.DiscountTotalCents,
		UniqueCustomers:    row.UniqueCustomers,
	}, nil
}

func (r *postgresRepository) AnalyticsDaily(ctx context.Context, start, end time.Time, ruleID *uuid.UUID, scope AnalyticsScope) ([]AnalyticsDailyPoint, error) {
	var rid uuid.NullUUID
	if ruleID != nil {
		rid = uuid.NullUUID{UUID: *ruleID, Valid: true}
	}
	var rows []db.PromotionAnalyticsDailyRow
	var err error
	if scope.All {
		rows, err = r.queries.PromotionAnalyticsDaily(ctx, db.PromotionAnalyticsDailyParams{StartTime: start, EndTime: end, RuleID: rid})
	} else {
		var scoped []db.PromotionAnalyticsDailyForChannelsRow
		scoped, err = r.queries.PromotionAnalyticsDailyForChannels(ctx, db.PromotionAnalyticsDailyForChannelsParams{ChannelIds: scope.ChannelIDs, StartTime: start, EndTime: end, RuleID: rid})
		rows = make([]db.PromotionAnalyticsDailyRow, len(scoped))
		for i, row := range scoped {
			rows[i] = db.PromotionAnalyticsDailyRow(row)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("analytics daily: %w", err)
	}
	out := make([]AnalyticsDailyPoint, len(rows))
	for i, row := range rows {
		out[i] = AnalyticsDailyPoint{
			Day:                row.Day,
			RedemptionCount:    row.RedemptionCount,
			DiscountTotalCents: row.DiscountTotalCents,
		}
	}
	return out, nil
}
