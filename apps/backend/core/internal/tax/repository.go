package tax

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

// ErrRuleNotFound is the sentinel for "no rule with that id"
var ErrRuleNotFound = errors.New("tax: rule not found")

// Repository abstracts the DB layer - Rule rows and their typed conditions/effects live in separate tables - the loader
// assembles hydrated rules for the engine
type Repository interface {
	WithTx(t *sql.Tx) Repository

	CreateRule(ctx context.Context, in Rule) (Rule, error)
	UpdateRule(ctx context.Context, in Rule) (Rule, error)
	DeleteRule(ctx context.Context, id uuid.UUID) error
	// GetRule returns the bare rule row (no conditions/effects)
	GetRule(ctx context.Context, id uuid.UUID) (Rule, error)
	// ListRulesByChannel returns bare rule rows for the admin list
	ListRulesByChannel(ctx context.Context, channelID uuid.UUID) ([]Rule, error)
	// ListActiveRulesByChannel returns active rules fully hydrated with
	// their conditions + effects - the engine hot path
	ListActiveRulesByChannel(ctx context.Context, channelID uuid.UUID) ([]Rule, error)
	// Every rule of a channel, active or not, hydrated in bulk - the admin list
	ListRulesByChannelHydrated(ctx context.Context, channelID uuid.UUID) ([]Rule, error)
	// ListAllRulesHydrated returns every rule (any channel, active or
	// not) hydrated - the admin GLOBAL cross-channel view
	ListAllRulesHydrated(ctx context.Context) ([]Rule, error)

	ReplaceConditions(ctx context.Context, ruleID uuid.UUID, conditions []Condition) error
	ReplaceEffects(ctx context.Context, ruleID uuid.UUID, effects []Effect) error
	ListConditionsByRule(ctx context.Context, ruleID uuid.UUID) ([]Condition, error)
	ListEffectsByRule(ctx context.Context, ruleID uuid.UUID) ([]Effect, error)
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

func (r *postgresRepository) CreateRule(ctx context.Context, in Rule) (Rule, error) {
	row, err := r.queries.CreateTaxRule(ctx, db.CreateTaxRuleParams{
		ChannelID:        in.ChannelID,
		Name:             in.Name,
		Priority:         in.Priority,
		ExclusivityGroup: in.ExclusivityGroup,
		SortOrder:        in.SortOrder,
		IsActive:         in.IsActive,
	})
	if err != nil {
		return Rule{}, fmt.Errorf("create tax rule: %w", err)
	}
	return toRule(row), nil
}

func (r *postgresRepository) UpdateRule(ctx context.Context, in Rule) (Rule, error) {
	row, err := r.queries.UpdateTaxRule(ctx, db.UpdateTaxRuleParams{
		ID:               in.ID,
		Name:             in.Name,
		Priority:         in.Priority,
		ExclusivityGroup: in.ExclusivityGroup,
		SortOrder:        in.SortOrder,
		IsActive:         in.IsActive,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Rule{}, ErrRuleNotFound
		}
		return Rule{}, fmt.Errorf("update tax rule: %w", err)
	}
	return toRule(row), nil
}

func (r *postgresRepository) DeleteRule(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteTaxRule(ctx, id); err != nil {
		return fmt.Errorf("delete tax rule: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetRule(ctx context.Context, id uuid.UUID) (Rule, error) {
	row, err := r.queries.GetTaxRule(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Rule{}, ErrRuleNotFound
		}
		return Rule{}, fmt.Errorf("get tax rule: %w", err)
	}
	return toRule(row), nil
}

func (r *postgresRepository) ListRulesByChannel(ctx context.Context, channelID uuid.UUID) ([]Rule, error) {
	rows, err := r.queries.ListTaxRulesByChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list tax rules: %w", err)
	}
	out := make([]Rule, len(rows))
	for i, row := range rows {
		out[i] = toRule(row)
	}
	return out, nil
}

func (r *postgresRepository) ListActiveRulesByChannel(ctx context.Context, channelID uuid.UUID) ([]Rule, error) {
	rows, err := r.queries.ListActiveTaxRulesByChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list active tax rules: %w", err)
	}
	return r.hydrateRules(ctx, rows)
}

func (r *postgresRepository) ListRulesByChannelHydrated(ctx context.Context, channelID uuid.UUID) ([]Rule, error) {
	rows, err := r.queries.ListTaxRulesByChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list tax rules: %w", err)
	}
	return r.hydrateRules(ctx, rows)
}

func (r *postgresRepository) ListAllRulesHydrated(ctx context.Context) ([]Rule, error) {
	rows, err := r.queries.ListAllTaxRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all tax rules: %w", err)
	}
	return r.hydrateRules(ctx, rows)
}

// hydrateRules attaches conditions + effects to a set of rule rows in two bulk
// queries (no N+1)
func (r *postgresRepository) hydrateRules(ctx context.Context, rows []db.TaxRule) ([]Rule, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	rules := make([]Rule, len(rows))
	ids := make([]uuid.UUID, len(rows))
	byID := make(map[uuid.UUID]int, len(rows))
	for i, row := range rows {
		rules[i] = toRule(row)
		ids[i] = row.ID
		byID[row.ID] = i
	}

	conds, err := r.queries.ListTaxRuleConditionsByRules(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load tax conditions: %w", err)
	}
	for _, c := range conds {
		if i, ok := byID[c.RuleID]; ok {
			rules[i].Conditions = append(rules[i].Conditions, toCondition(c))
		}
	}
	effs, err := r.queries.ListTaxRuleEffectsByRules(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load tax effects: %w", err)
	}
	for _, e := range effs {
		if i, ok := byID[e.RuleID]; ok {
			rules[i].Effects = append(rules[i].Effects, toEffect(e))
		}
	}
	return rules, nil
}

func (r *postgresRepository) ReplaceConditions(ctx context.Context, ruleID uuid.UUID, conditions []Condition) error {
	if err := r.queries.DeleteTaxRuleConditions(ctx, ruleID); err != nil {
		return fmt.Errorf("clear conditions: %w", err)
	}
	for i, c := range conditions {
		if _, err := r.queries.InsertTaxRuleCondition(ctx, db.InsertTaxRuleConditionParams{
			RuleID:        ruleID,
			Type:          c.Type,
			Params:        jsonOrEmpty(c.Params),
			ParamsVersion: defaultVersion(c.ParamsVersion),
			SortOrder:     int32(i),
		}); err != nil {
			return fmt.Errorf("insert condition %d: %w", i, err)
		}
	}
	return nil
}

func (r *postgresRepository) ReplaceEffects(ctx context.Context, ruleID uuid.UUID, effects []Effect) error {
	if err := r.queries.DeleteTaxRuleEffects(ctx, ruleID); err != nil {
		return fmt.Errorf("clear effects: %w", err)
	}
	for i, e := range effects {
		if _, err := r.queries.InsertTaxRuleEffect(ctx, db.InsertTaxRuleEffectParams{
			RuleID:        ruleID,
			Type:          e.Type,
			Params:        jsonOrEmpty(e.Params),
			ParamsVersion: defaultVersion(e.ParamsVersion),
			SortOrder:     int32(i),
		}); err != nil {
			return fmt.Errorf("insert effect %d: %w", i, err)
		}
	}
	return nil
}

func (r *postgresRepository) ListConditionsByRule(ctx context.Context, ruleID uuid.UUID) ([]Condition, error) {
	rows, err := r.queries.ListTaxRuleConditions(ctx, ruleID)
	if err != nil {
		return nil, fmt.Errorf("list conditions: %w", err)
	}
	out := make([]Condition, len(rows))
	for i, row := range rows {
		out[i] = toCondition(row)
	}
	return out, nil
}

func (r *postgresRepository) ListEffectsByRule(ctx context.Context, ruleID uuid.UUID) ([]Effect, error) {
	rows, err := r.queries.ListTaxRuleEffects(ctx, ruleID)
	if err != nil {
		return nil, fmt.Errorf("list effects: %w", err)
	}
	out := make([]Effect, len(rows))
	for i, row := range rows {
		out[i] = toEffect(row)
	}
	return out, nil
}

func toRule(row db.TaxRule) Rule {
	return Rule{
		ID:               row.ID,
		ChannelID:        row.ChannelID,
		Name:             row.Name,
		Priority:         row.Priority,
		ExclusivityGroup: row.ExclusivityGroup,
		SortOrder:        row.SortOrder,
		IsActive:         row.IsActive,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func toCondition(row db.TaxRuleCondition) Condition {
	return Condition{
		ID:            row.ID,
		RuleID:        row.RuleID,
		Type:          row.Type,
		Params:        json.RawMessage(row.Params),
		ParamsVersion: row.ParamsVersion,
		SortOrder:     row.SortOrder,
	}
}

func toEffect(row db.TaxRuleEffect) Effect {
	return Effect{
		ID:            row.ID,
		RuleID:        row.RuleID,
		Type:          row.Type,
		Params:        json.RawMessage(row.Params),
		ParamsVersion: row.ParamsVersion,
		SortOrder:     row.SortOrder,
	}
}

// jsonOrEmpty coerces nil/empty params to "{}" so the NOT NULL JSONB
// column accepts them
func jsonOrEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}

func defaultVersion(v int32) int32 {
	if v <= 0 {
		return 1
	}
	return v
}
