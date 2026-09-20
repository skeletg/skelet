package promotion_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion/actions"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion/conditions"
)

// fakeRepo is an in-memory Repository for engine tests - Only the methods the engine actually calls are populated - the
// rest return zero values so a misbehaving test compiles but is easy to spot
type fakeRepo struct {
	rules      []promotion.Rule
	conditions map[uuid.UUID][]promotion.Condition
	actions    map[uuid.UUID][]promotion.Action
	codes      map[string]promotion.Code
	// applied keys by cart id - populated by cart-eval tests so EvaluateCart's internal ListCartAppliedCodes call returns
	// real data instead of the stub's nil - Engine-only tests can ignore
	applied map[uuid.UUID][]promotion.AppliedCodeRecord
	// invalidated records MarkCartCodeInvalidated calls by rule id - reconcile tests read it
	invalidated map[uuid.UUID]string
	// cartEmails keys by cart id - backs the restricted_to_email gate
	cartEmails map[uuid.UUID]string
	// lineAttrs keys by sku - the catalog facts EvaluateCart enriches cart lines with
	lineAttrs map[string]promotion.LineProductAttrs
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		conditions: map[uuid.UUID][]promotion.Condition{},
		actions:    map[uuid.UUID][]promotion.Action{},
		codes:      map[string]promotion.Code{},
		applied:    map[uuid.UUID][]promotion.AppliedCodeRecord{},
	}
}

func (f *fakeRepo) WithTx(_ *sql.Tx) promotion.Repository { return f }

func (f *fakeRepo) ListActiveRulesForChannel(_ context.Context, _ *uuid.UUID) ([]promotion.Rule, error) {
	// Return copies - the engine mutates rule.Conditions and rule.Actions, so handing back the underlying slice would accumulate state across Evaluate calls Mirrors the real query's is_active + validity-window + max_uses filter so disabled, expired and used-up rules drop out of candidacy here too
	now := time.Now()
	out := make([]promotion.Rule, 0, len(f.rules))
	for _, r := range f.rules {
		if !r.IsActive {
			continue
		}
		if r.ValidFrom != nil && r.ValidFrom.After(now) {
			continue
		}
		if r.ValidTo != nil && r.ValidTo.Before(now) {
			continue
		}
		if r.MaxUses != nil && r.UsedCount >= *r.MaxUses {
			continue
		}
		r.Conditions = nil
		r.Actions = nil
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeRepo) ListConditionsByRules(_ context.Context, _ []uuid.UUID) ([]promotion.Condition, error) {
	var out []promotion.Condition
	for _, cs := range f.conditions {
		out = append(out, cs...)
	}
	return out, nil
}

func (f *fakeRepo) ListActionsByRules(_ context.Context, _ []uuid.UUID) ([]promotion.Action, error) {
	var out []promotion.Action
	for _, as := range f.actions {
		out = append(out, as...)
	}
	return out, nil
}

func (f *fakeRepo) GetCodeByCode(_ context.Context, code string) (promotion.Code, error) {
	if c, ok := f.codes[code]; ok {
		return c, nil
	}
	return promotion.Code{}, promotion.ErrCodeNotFound
}

// ---- Stubs for the rest of the Repository surface - Engine tests
// don't call these; they exist to satisfy the interface

func (f *fakeRepo) CreateRule(context.Context, promotion.Rule) (promotion.Rule, error) {
	return promotion.Rule{}, nil
}
func (f *fakeRepo) UpdateRule(context.Context, promotion.Rule) (promotion.Rule, error) {
	return promotion.Rule{}, nil
}
func (f *fakeRepo) DeleteRule(context.Context, uuid.UUID) error { return nil }
func (f *fakeRepo) GetRule(_ context.Context, id uuid.UUID) (promotion.Rule, error) {
	for _, r := range f.rules {
		if r.ID == id {
			return r, nil
		}
	}
	return promotion.Rule{}, promotion.ErrRuleNotFound
}
func (f *fakeRepo) GetRuleBySlug(context.Context, string) (promotion.Rule, error) {
	return promotion.Rule{}, nil
}
func (f *fakeRepo) ListRules(context.Context, int32, int32) ([]promotion.Rule, int32, error) {
	return nil, 0, nil
}
func (f *fakeRepo) IncrementRuleUsage(context.Context, uuid.UUID) error { return nil }
func (f *fakeRepo) ReorderRules(context.Context, []uuid.UUID) error     { return nil }

func (f *fakeRepo) UpsertRuleContent(context.Context, promotion.RuleContent) (promotion.RuleContent, error) {
	return promotion.RuleContent{}, nil
}
func (f *fakeRepo) ListRuleContents(context.Context, uuid.UUID) ([]promotion.RuleContent, error) {
	return nil, nil
}
func (f *fakeRepo) DeleteRuleContent(context.Context, uuid.UUID, string) error { return nil }
func (f *fakeRepo) GetRuleContent(context.Context, uuid.UUID, string) (promotion.RuleContent, error) {
	return promotion.RuleContent{}, nil
}
func (f *fakeRepo) GetRuleContentsByIDs(context.Context, []uuid.UUID, string) (map[uuid.UUID]promotion.RuleContent, error) {
	return nil, nil
}
func (f *fakeRepo) AnalyticsByRule(context.Context, time.Time, time.Time) ([]promotion.RuleAnalytics, error) {
	return nil, nil
}
func (f *fakeRepo) AnalyticsTotals(context.Context, time.Time, time.Time) (promotion.AnalyticsTotals, error) {
	return promotion.AnalyticsTotals{}, nil
}
func (f *fakeRepo) AnalyticsDaily(context.Context, time.Time, time.Time, *uuid.UUID) ([]promotion.AnalyticsDailyPoint, error) {
	return nil, nil
}

func (f *fakeRepo) CreateCode(context.Context, promotion.Code) (promotion.Code, error) {
	return promotion.Code{}, nil
}
func (f *fakeRepo) UpdateCode(context.Context, promotion.Code) (promotion.Code, error) {
	return promotion.Code{}, nil
}
func (f *fakeRepo) DeleteCode(context.Context, uuid.UUID) error { return nil }
func (f *fakeRepo) GetCode(_ context.Context, id uuid.UUID) (promotion.Code, error) {
	for _, c := range f.codes {
		if c.ID == id {
			return c, nil
		}
	}
	return promotion.Code{}, nil
}
func (f *fakeRepo) ListCodesByRule(context.Context, uuid.UUID, int32, int32) ([]promotion.Code, int32, error) {
	return nil, 0, nil
}
func (f *fakeRepo) BulkInsertCodes(context.Context, uuid.UUID, []string) (int, error) {
	return 0, nil
}
func (f *fakeRepo) IncrementCodeUsage(context.Context, uuid.UUID) error { return nil }
func (f *fakeRepo) CountCodeUsageByCustomer(context.Context, uuid.UUID, uuid.UUID) (int32, error) {
	return 0, nil
}
func (f *fakeRepo) CountRuleUsageByCustomer(context.Context, uuid.UUID, uuid.UUID) (int32, error) {
	return 0, nil
}

func (f *fakeRepo) ReplaceRuleConditions(context.Context, uuid.UUID, []promotion.Condition) error {
	return nil
}
func (f *fakeRepo) ListConditionsByRule(context.Context, uuid.UUID) ([]promotion.Condition, error) {
	return nil, nil
}

func (f *fakeRepo) ReplaceRuleActions(context.Context, uuid.UUID, []promotion.Action) error {
	return nil
}
func (f *fakeRepo) ListActionsByRule(context.Context, uuid.UUID) ([]promotion.Action, error) {
	return nil, nil
}

func (f *fakeRepo) ApplyCodeToCart(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error {
	return nil
}
func (f *fakeRepo) RemoveCodeFromCartByCode(context.Context, uuid.UUID, string) error    { return nil }
func (f *fakeRepo) RemoveCodeFromCartByRule(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeRepo) ClearCartAppliedCodes(context.Context, uuid.UUID) error               { return nil }
func (f *fakeRepo) ListCartAppliedCodes(_ context.Context, cartID uuid.UUID) ([]promotion.AppliedCodeRecord, error) {
	return f.applied[cartID], nil
}
func (f *fakeRepo) GetCartEmail(_ context.Context, cartID uuid.UUID) (string, error) {
	return f.cartEmails[cartID], nil
}
func (f *fakeRepo) MarkCartCodeInvalidated(_ context.Context, _ uuid.UUID, ruleID uuid.UUID, reason string) error {
	if f.invalidated == nil {
		f.invalidated = map[uuid.UUID]string{}
	}
	f.invalidated[ruleID] = reason
	return nil
}

func (f *fakeRepo) RecordRedemption(context.Context, uuid.UUID, *uuid.UUID, *uuid.UUID, uuid.UUID, int64) error {
	return nil
}

func (f *fakeRepo) ListRedemptionsByOrder(context.Context, uuid.UUID) ([]promotion.OrderRedemption, error) {
	return nil, nil
}

// lineAttrs keys by sku - the cart-eval tests that exercise product-scoped conditions populate it, and an empty map
// means "catalog knows nothing", which is what an unenriched cart looks like
func (f *fakeRepo) PromotionLineAttrs(_ context.Context, skus []string) (map[string]promotion.LineProductAttrs, error) {
	out := make(map[string]promotion.LineProductAttrs, len(skus))
	for _, sku := range skus {
		if a, ok := f.lineAttrs[sku]; ok {
			out[sku] = a
		}
	}
	return out, nil
}

func TestEngine_SimplePercentOffSubtotal(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	repo.rules = []promotion.Rule{{
		ID:           ruleID,
		Slug:         "ten-pct",
		Name:         "10% off",
		RequiresCode: false,
		Priority:     0,
		IsActive:     true,
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID:     uuid.New(),
		RuleID: ruleID,
		Type:   "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 1000}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines: []promotion.CartLineView{{
			LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1,
		}},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Ledger.SubtotalDiscountCents(); got != 1000 {
		t.Errorf("subtotal discount = %d, want 1000", got)
	}
	if len(res.Applied) != 1 {
		t.Fatalf("applied count = %d, want 1", len(res.Applied))
	}
	if res.Applied[0].RuleID != ruleID {
		t.Errorf("applied[0].RuleID = %s, want %s", res.Applied[0].RuleID, ruleID)
	}
}

func TestEngine_CodeGatedRule_FiltersByAppliedCode(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	codeID := uuid.New()
	repo.rules = []promotion.Rule{{
		ID: ruleID, Slug: "save10", Name: "Save 10",
		RequiresCode: true, IsActive: true,
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.fixed_off_subtotal",
		Params: json.RawMessage(`{"cents": 1000}`),
	}}
	repo.codes["save10"] = promotion.Code{ID: codeID, RuleID: ruleID, Code: "save10", IsActive: true}

	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)

	// Without the code applied, rule should NOT fire
	envNoCode := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 5000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 5000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), envNoCode)
	if err != nil {
		t.Fatalf("evaluate no-code: %v", err)
	}
	if res.Ledger.SubtotalDiscountCents() != 0 {
		t.Errorf("subtotal discount without code = %d, want 0", res.Ledger.SubtotalDiscountCents())
	}

	// With the code applied, rule fires
	envWithCode := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 5000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 5000, Quantity: 1}},
		AppliedCodes:  []string{"save10"},
	})
	res, err = engine.Evaluate(context.Background(), envWithCode)
	if err != nil {
		t.Fatalf("evaluate with-code: %v", err)
	}
	if res.Ledger.SubtotalDiscountCents() != 1000 {
		t.Errorf("subtotal discount with code = %d, want 1000", res.Ledger.SubtotalDiscountCents())
	}
}

func TestEngine_PrioritySortAndStopAll(t *testing.T) {
	repo := newFakeRepo()
	low := uuid.New() // priority 0 - runs first
	high := uuid.New()
	repo.rules = []promotion.Rule{
		{ID: high, Slug: "high", Name: "high", Priority: 10, IsActive: true},
		{ID: low, Slug: "low", Name: "low", Priority: 0, IsActive: true, StopAfterApply: promotion.StopAll},
	}
	repo.actions[low] = []promotion.Action{{
		ID: uuid.New(), RuleID: low, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 1000}`),
	}}
	repo.actions[high] = []promotion.Action{{
		ID: uuid.New(), RuleID: high, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 2000}`),
	}}

	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// Low priority rule fires (10% = 1000) and StopAll halts, so the
	// high-priority rule (20%) never runs
	if got := res.Ledger.SubtotalDiscountCents(); got != 1000 {
		t.Errorf("subtotal discount = %d, want 1000 (stop_all should prevent the 20%% rule)", got)
	}
	if len(res.Applied) != 1 {
		t.Errorf("applied count = %d, want 1", len(res.Applied))
	}
}

// Two rules at the same priority are peers and both fire even when both carry StopAfterApply=StopAll, because the halt takes effect only as the engine moves to the next tier
// So peers do not block each other, and every rule below them is shadowed
func TestEngine_StopAllPeersAtSamePriority(t *testing.T) {
	repo := newFakeRepo()
	a := uuid.New()
	b := uuid.New()
	c := uuid.New()
	repo.rules = []promotion.Rule{
		{ID: a, Slug: "a", Name: "a", Priority: 10, IsActive: true, StopAfterApply: promotion.StopAll},
		{ID: b, Slug: "b", Name: "b", Priority: 10, IsActive: true, StopAfterApply: promotion.StopAll},
		{ID: c, Slug: "c", Name: "c", Priority: 20, IsActive: true},
	}
	repo.actions[a] = []promotion.Action{{
		ID: uuid.New(), RuleID: a, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 1000}`),
	}}
	repo.actions[b] = []promotion.Action{{
		ID: uuid.New(), RuleID: b, Type: "core.fixed_off_subtotal",
		Params: json.RawMessage(`{"cents": 500}`),
	}}
	repo.actions[c] = []promotion.Action{{
		ID: uuid.New(), RuleID: c, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 5000}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// A: 10% of 10000 = 1000 - B: 500 - Total: 1500 - C blocked
	if got := res.Ledger.SubtotalDiscountCents(); got != 1500 {
		t.Errorf("subtotal discount = %d, want 1500 (both peers should fire, C blocked)", got)
	}
	if len(res.Applied) != 2 {
		t.Errorf("applied count = %d, want 2", len(res.Applied))
	}
	for _, ar := range res.Applied {
		if ar.RuleID == c {
			t.Errorf("rule C (lower priority) should not have fired")
		}
	}
}

// TestEngine_StopSameActionTypeDoesNotBlockPeers verifies the same tier-boundary semantics for SAME_ACTION_TYPE: two rules at the same
// priority both running core.percent_off_subtotal are peers, so both apply - A later-tier rule that shares the action type is blocked
func TestEngine_StopSameActionTypeDoesNotBlockPeers(t *testing.T) {
	repo := newFakeRepo()
	a := uuid.New()
	b := uuid.New()
	c := uuid.New()
	repo.rules = []promotion.Rule{
		{ID: a, Slug: "a", Name: "a", Priority: 10, IsActive: true, StopAfterApply: promotion.StopSameActionType},
		{ID: b, Slug: "b", Name: "b", Priority: 10, IsActive: true},
		{ID: c, Slug: "c", Name: "c", Priority: 20, IsActive: true},
	}
	repo.actions[a] = []promotion.Action{{
		ID: uuid.New(), RuleID: a, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 1000}`),
	}}
	repo.actions[b] = []promotion.Action{{
		ID: uuid.New(), RuleID: b, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 500}`),
	}}
	repo.actions[c] = []promotion.Action{{
		ID: uuid.New(), RuleID: c, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 2000}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// A: 1000 - B (peer at same priority): 500 - C (next tier with
	// blocked action type): 0 - Total: 1500
	if got := res.Ledger.SubtotalDiscountCents(); got != 1500 {
		t.Errorf("subtotal discount = %d, want 1500 (peers should both fire; C blocked by action-type)", got)
	}
	for _, ar := range res.Applied {
		if ar.RuleID == c {
			t.Errorf("rule C should be blocked by tier 10's SAME_ACTION_TYPE")
		}
	}
}

func TestEngine_ExclusivityGroup(t *testing.T) {
	repo := newFakeRepo()
	r1 := uuid.New()
	r2 := uuid.New()
	repo.rules = []promotion.Rule{
		{ID: r1, Slug: "a", Name: "a", Priority: 0, IsActive: true, ExclusivityGroup: "g1"},
		{ID: r2, Slug: "b", Name: "b", Priority: 1, IsActive: true, ExclusivityGroup: "g1"},
	}
	repo.actions[r1] = []promotion.Action{{
		ID: uuid.New(), RuleID: r1, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 1000}`),
	}}
	repo.actions[r2] = []promotion.Action{{
		ID: uuid.New(), RuleID: r2, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 2000}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Ledger.SubtotalDiscountCents(); got != 1000 {
		t.Errorf("subtotal discount = %d, want 1000 (only r1 should fire due to exclusivity group)", got)
	}
	if len(res.Applied) != 1 || res.Applied[0].RuleID != r1 {
		t.Errorf("applied = %+v, want only r1", res.Applied)
	}
}

func TestEngine_StackingAdditive(t *testing.T) {
	repo := newFakeRepo()
	r1 := uuid.New()
	r2 := uuid.New()
	repo.rules = []promotion.Rule{
		{ID: r1, Slug: "a", Name: "a", Priority: 0, IsActive: true},
		{ID: r2, Slug: "b", Name: "b", Priority: 1, IsActive: true},
	}
	repo.actions[r1] = []promotion.Action{{
		ID: uuid.New(), RuleID: r1, Type: "core.fixed_off_subtotal",
		Params: json.RawMessage(`{"cents": 500}`),
	}}
	repo.actions[r2] = []promotion.Action{{
		ID: uuid.New(), RuleID: r2, Type: "core.fixed_off_subtotal",
		Params: json.RawMessage(`{"cents": 300}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Ledger.SubtotalDiscountCents(); got != 800 {
		t.Errorf("stacked subtotal = %d, want 800 (500 + 300)", got)
	}
}

func TestEngine_ConditionFiltersOut(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "min50", Name: "min50", IsActive: true}}
	repo.conditions[ruleID] = []promotion.Condition{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.cart_subtotal_range",
		Params: json.RawMessage(`{"min_cents": 5000}`),
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 1000}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)

	// Below threshold: rule does not fire
	low := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 4000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 4000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), low)
	if err != nil || res.Ledger.SubtotalDiscountCents() != 0 {
		t.Errorf("low subtotal: got %d / err %v, want 0", res.Ledger.SubtotalDiscountCents(), err)
	}

	// At threshold: rule fires
	hi := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 5000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 5000, Quantity: 1}},
	})
	res, err = engine.Evaluate(context.Background(), hi)
	if err != nil || res.Ledger.SubtotalDiscountCents() != 500 {
		t.Errorf("at threshold: got %d / err %v, want 500", res.Ledger.SubtotalDiscountCents(), err)
	}
}

func TestEngine_LineAllocation_RoundingLandsOnLastLine(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "ten", Name: "ten", IsActive: true}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.fixed_off_subtotal",
		Params: json.RawMessage(`{"cents": 100}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)

	// 3 lines @ 333 cents each = subtotal 999 - A flat 100c discount
	// should split 33, 33, 34 (last line absorbs the +1 remainder)
	l1, l2, l3 := uuid.New(), uuid.New(), uuid.New()
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 999,
		Lines: []promotion.CartLineView{
			{LineID: l1, UnitPriceCents: 333, Quantity: 1},
			{LineID: l2, UnitPriceCents: 333, Quantity: 1},
			{LineID: l3, UnitPriceCents: 333, Quantity: 1},
		},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	got1 := res.Ledger.LineAdjustment(l1)
	got2 := res.Ledger.LineAdjustment(l2)
	got3 := res.Ledger.LineAdjustment(l3)
	if got1+got2+got3 != 100 {
		t.Errorf("per-line discounts sum to %d, want 100 (got %d/%d/%d)", got1+got2+got3, got1, got2, got3)
	}
	// Last line takes the rounding; first two are equal
	if got1 != got2 {
		t.Errorf("first two lines should split evenly: got %d, %d", got1, got2)
	}
	if got3 < got1 {
		t.Errorf("last line should absorb remainder: got %d (vs %d on first line)", got3, got1)
	}
}

func TestEngine_FreeShipping(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "ship", Name: "ship", IsActive: true}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.free_shipping",
		Params: json.RawMessage(`{}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 1000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 1000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !res.Ledger.FreeShipping() {
		t.Errorf("free shipping not set")
	}
}

func TestEngine_AndOrNotComposition(t *testing.T) {
	// any_of: matches when subtotal >= 5000 OR country == "DE"
	repo := newFakeRepo()
	ruleID := uuid.New()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "any", Name: "any", IsActive: true}}
	repo.conditions[ruleID] = []promotion.Condition{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.any_of",
		Params: json.RawMessage(`{"conditions": [
            {"type":"core.cart_subtotal_range","params":{"min_cents":5000}},
            {"type":"core.country_in","params":{"codes":["DE"]}}
        ]}`),
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.fixed_off_subtotal",
		Params: json.RawMessage(`{"cents": 100}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)

	// Low subtotal + DE country → second branch fires
	envDE := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 1000,
		CountryCode:   "DE",
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 1000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), envDE)
	if err != nil || res.Ledger.SubtotalDiscountCents() != 100 {
		t.Errorf("any_of DE: got %d / err %v, want 100", res.Ledger.SubtotalDiscountCents(), err)
	}

	// Low subtotal + FR country → neither branch fires
	envFR := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 1000,
		CountryCode:   "FR",
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 1000, Quantity: 1}},
	})
	res, err = engine.Evaluate(context.Background(), envFR)
	if err != nil || res.Ledger.SubtotalDiscountCents() != 0 {
		t.Errorf("any_of FR: got %d / err %v, want 0", res.Ledger.SubtotalDiscountCents(), err)
	}
}

func TestCodeGenerator_PatternProducesUniqueCodes(t *testing.T) {
	gen := promotion.NewCodeGenerator("[A][A][N][N]", "WELCOME-", "")
	if err := gen.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	batch := gen.Batch(100)
	if len(batch) == 0 {
		t.Fatal("batch produced no codes")
	}
	seen := map[string]struct{}{}
	for _, c := range batch {
		if _, dup := seen[c]; dup {
			t.Errorf("duplicate code in batch: %s", c)
		}
		seen[c] = struct{}{}
		if len(c) < 12 { // WELCOME- + 4 chars = 12
			t.Errorf("code %q shorter than expected", c)
		}
	}
}

// A rule gated on a condition type the registry doesn't know must NOT fire - skipping the gate for
// an uninstalled plugin promotes "gold tier only, 50% off" into a storewide 50% off
func TestEngine_UnknownConditionTypeFailsClosed(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "vip-only", Name: "vip only", IsActive: true}}
	repo.conditions[ruleID] = []promotion.Condition{{
		ID: uuid.New(), RuleID: ruleID, Type: "plugin.loyalty_tier",
		Params: json.RawMessage(`{"tier": "gold"}`),
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 5000}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	res, err := engine.Evaluate(promotion.WithTrace(context.Background()), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Ledger.SubtotalDiscountCents(); got != 0 {
		t.Errorf("subtotal discount = %d, want 0 (an unknown gate must fail closed)", got)
	}
	if len(res.Applied) != 0 {
		t.Errorf("applied = %+v, want none", res.Applied)
	}
	// The trace has to say WHY, so an operator can tell a missing plugin from a cart that didn't qualify
	if len(res.Trace) != 1 {
		t.Fatalf("trace entries = %d, want 1", len(res.Trace))
	}
	if want := "unknown_condition_type:plugin.loyalty_tier"; res.Trace[0].SkipReason != want {
		t.Errorf("skip reason = %q, want %q", res.Trace[0].SkipReason, want)
	}
}

// Two stacked percent rules each compute off the gross subtotal, so the aggregate
// has to be clamped or the order records a discount larger than the cart
func TestEngine_StackedDiscountClampedToSubtotal(t *testing.T) {
	repo := newFakeRepo()
	r1, r2 := uuid.New(), uuid.New()
	repo.rules = []promotion.Rule{
		{ID: r1, Slug: "sixty", Name: "60% off", Priority: 10, IsActive: true},
		{ID: r2, Slug: "seventy", Name: "70% off", Priority: 20, IsActive: true},
	}
	repo.actions[r1] = []promotion.Action{{
		ID: uuid.New(), RuleID: r1, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 6000}`),
	}}
	repo.actions[r2] = []promotion.Action{{
		ID: uuid.New(), RuleID: r2, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 7000}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Ledger.SubtotalDiscountCents(); got != 10000 {
		t.Errorf("subtotal discount = %d, want 10000 (130%% clamped to the subtotal)", got)
	}
	// The per-rule audit rows have to add up to what was actually discounted
	var summed int64
	for _, ar := range res.Applied {
		summed += ar.DiscountCents
	}
	if summed != 10000 {
		t.Errorf("per-rule contributions sum = %d, want 10000", summed)
	}
}

// A cap rule holds no matter which priority tier it sits in - letting order decide would mean the
// ceiling binds or not depending on a uuid tiebreak
func TestEngine_CapHoldsWhenItRunsFirst(t *testing.T) {
	repo := newFakeRepo()
	capRule, discountRule := uuid.New(), uuid.New()
	repo.rules = []promotion.Rule{
		{ID: capRule, Slug: "cap", Name: "cap at 1500", Priority: 10, IsActive: true},
		{ID: discountRule, Slug: "half", Name: "50% off", Priority: 20, IsActive: true},
	}
	repo.actions[capRule] = []promotion.Action{{
		ID: uuid.New(), RuleID: capRule, Type: "core.cap_total_discount",
		Params: json.RawMessage(`{"max_cents": 1500}`),
	}}
	repo.actions[discountRule] = []promotion.Action{{
		ID: uuid.New(), RuleID: discountRule, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 5000}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Ledger.SubtotalDiscountCents(); got != 1500 {
		t.Errorf("subtotal discount = %d, want 1500 (the cap ran first but must still bind)", got)
	}
}

// Peers at one priority in the same exclusivity group both fire, matching the tier semantics the
// stop directives already follow - an immediate claim would let a uuid comparison pick a winner
func TestEngine_ExclusivityGroupPeersAtSamePriority(t *testing.T) {
	repo := newFakeRepo()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	repo.rules = []promotion.Rule{
		{ID: a, Slug: "a", Name: "a", Priority: 10, IsActive: true, ExclusivityGroup: "g1"},
		{ID: b, Slug: "b", Name: "b", Priority: 10, IsActive: true, ExclusivityGroup: "g1"},
		{ID: c, Slug: "c", Name: "c", Priority: 20, IsActive: true, ExclusivityGroup: "g1"},
	}
	for _, id := range []uuid.UUID{a, b, c} {
		repo.actions[id] = []promotion.Action{{
			ID: uuid.New(), RuleID: id, Type: "core.fixed_off_subtotal",
			Params: json.RawMessage(`{"cents": 100}`),
		}}
	}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Ledger.SubtotalDiscountCents(); got != 200 {
		t.Errorf("subtotal discount = %d, want 200 (both peers fire, the next tier is blocked)", got)
	}
	for _, ar := range res.Applied {
		if ar.RuleID == c {
			t.Errorf("rule C is a tier below and must be blocked by the group")
		}
	}
}

// A rule whose used_count reached max_uses must not be offered by the engine at all
func TestEngine_UsedUpRuleIsNotACandidate(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	max := int32(5)
	repo.rules = []promotion.Rule{{
		ID: ruleID, Slug: "spent", Name: "spent", IsActive: true,
		MaxUses: &max, UsedCount: 5,
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 1000}`),
	}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Ledger.SubtotalDiscountCents(); got != 0 {
		t.Errorf("subtotal discount = %d, want 0 (a used-up rule can never be redeemed, so it must not price)", got)
	}
}

// Nested lookups must resolve through the evaluating engine's registry, not the
// package-level default
func TestEngine_NestedConditionUsesEngineRegistry(t *testing.T) {
	reg := promotion.NewRegistry()
	for _, c := range promotion.DefaultRegistry.AllConditions() {
		reg.RegisterCondition(c)
	}
	for _, a := range promotion.DefaultRegistry.AllActions() {
		reg.RegisterAction(a)
	}
	repo := newFakeRepo()
	ruleID := uuid.New()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "widget", Name: "widget", IsActive: true}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.percent_off_matching_items",
		Params: json.RawMessage(`{"bps": 1000, "condition": {"type": "core.cart_contains_sku", "params": {"skus": ["WIDGET"]}}}`),
	}}
	engine := promotion.NewEngine(repo, reg)
	line := uuid.New()
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines: []promotion.CartLineView{
			{LineID: line, ProductSKU: "WIDGET", UnitPriceCents: 10000, Quantity: 1},
		},
	})
	res, err := engine.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Ledger.LineAdjustment(line); got != 1000 {
		t.Errorf("line adjustment = %d, want 1000", got)
	}
}

// The built-ins are installed the same way wiring does it, never through a blank import
func init() {
	actions.Register(promotion.DefaultRegistry)
	conditions.Register(promotion.DefaultRegistry)
}
