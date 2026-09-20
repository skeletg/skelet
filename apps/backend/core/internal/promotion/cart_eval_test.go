package promotion_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// Guards against a future "engine assumes >0 rules" mistake
func TestEvaluateCart_NoRules(t *testing.T) {
	repo := newFakeRepo()
	svc := promotion.NewService(repo)
	res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
		CartID:        uuid.New(),
		SubtotalCents: 5000,
		Lines: []promotion.CartLineView{{
			LineID: uuid.New(), UnitPriceCents: 5000, Quantity: 1,
		}},
	})
	if err != nil {
		t.Fatalf("evaluate cart: %v", err)
	}
	if got := res.Result.Ledger.SubtotalDiscountCents(); got != 0 {
		t.Errorf("discount = %d, want 0", got)
	}
	if len(res.Result.Applied) != 0 {
		t.Errorf("applied = %d, want 0", len(res.Result.Applied))
	}
}

// EvaluateCart must read cart_applied_codes and feed the code texts into the env - without that wiring a code-gated rule never fires from cart pricing
func TestEvaluateCart_LoadsAppliedCodes_FromRepo(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	codeID := uuid.New()
	cartID := uuid.New()
	repo.rules = []promotion.Rule{{
		ID: ruleID, Slug: "save10", Name: "Save 10",
		RequiresCode: true, IsActive: true,
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.fixed_off_subtotal",
		Params: json.RawMessage(`{"cents": 1000}`),
	}}
	repo.codes["save10"] = promotion.Code{ID: codeID, RuleID: ruleID, Code: "save10", IsActive: true}
	repo.applied[cartID] = []promotion.AppliedCodeRecord{{
		CartID: cartID, RuleID: ruleID, CodeID: &codeID, CodeText: "save10",
	}}

	svc := promotion.NewService(repo)
	res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
		CartID:        cartID,
		SubtotalCents: 5000,
		Lines: []promotion.CartLineView{{
			LineID: uuid.New(), UnitPriceCents: 5000, Quantity: 1,
		}},
	})
	if err != nil {
		t.Fatalf("evaluate cart: %v", err)
	}
	if got := res.Result.Ledger.SubtotalDiscountCents(); got != 1000 {
		t.Errorf("discount = %d, want 1000 (code-gated rule should have fired via repo lookup)", got)
	}
	if len(res.ManuallyApplied) != 1 || res.ManuallyApplied[0].CodeText != "save10" {
		t.Errorf("manually-applied not propagated to result: %+v", res.ManuallyApplied)
	}
}

// A code with InvalidatedReason set must NOT reach the engine - the admin "remove code" flow leaves the row in place, so re-firing would double-count
func TestEvaluateCart_SkipsInvalidatedCodes(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	codeID := uuid.New()
	cartID := uuid.New()
	repo.rules = []promotion.Rule{{
		ID: ruleID, Slug: "save10", RequiresCode: true, IsActive: true,
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.fixed_off_subtotal",
		Params: json.RawMessage(`{"cents": 1000}`),
	}}
	repo.codes["save10"] = promotion.Code{ID: codeID, RuleID: ruleID, Code: "save10", IsActive: true}
	repo.applied[cartID] = []promotion.AppliedCodeRecord{{
		CartID: cartID, RuleID: ruleID, CodeID: &codeID, CodeText: "save10",
		InvalidatedReason: "rule_disabled_after_apply",
	}}

	svc := promotion.NewService(repo)
	res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
		CartID:        cartID,
		SubtotalCents: 5000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 5000, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("evaluate cart: %v", err)
	}
	if got := res.Result.Ledger.SubtotalDiscountCents(); got != 0 {
		t.Errorf("discount = %d, want 0 (invalidated code should not refire)", got)
	}
}

// Guard: passing CustomerID without CustomerGroupSlug makes core.customer_group_in silently fail for every logged-in customer
func TestEvaluateCart_CustomerGroupReachesCondition(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	condID := uuid.New()
	repo.rules = []promotion.Rule{{
		ID: ruleID, Slug: "vip-only", IsActive: true,
	}}
	repo.conditions[ruleID] = []promotion.Condition{{
		ID:     condID,
		RuleID: ruleID,
		Type:   "core.customer_group_in",
		Params: json.RawMessage(`{"group_slugs": ["vip"]}`),
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 1500}`),
	}}

	svc := promotion.NewService(repo)
	custID := uuid.New()
	cartID := uuid.New()
	line := promotion.CartLineView{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}

	// Without the slug → rule does NOT fire
	guestRes, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
		CartID: cartID, SubtotalCents: 10000, Lines: []promotion.CartLineView{line},
		CustomerID: &custID,
	})
	if err != nil {
		t.Fatalf("evaluate without slug: %v", err)
	}
	if got := guestRes.Result.Ledger.SubtotalDiscountCents(); got != 0 {
		t.Errorf("without slug: discount = %d, want 0", got)
	}

	// With the matching slug → fires
	vipRes, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
		CartID: cartID, SubtotalCents: 10000, Lines: []promotion.CartLineView{line},
		CustomerID: &custID, CustomerGroupSlug: "vip",
	})
	if err != nil {
		t.Fatalf("evaluate with slug: %v", err)
	}
	if got := vipRes.Result.Ledger.SubtotalDiscountCents(); got != 1500 {
		t.Errorf("with vip slug: discount = %d, want 1500", got)
	}

	// With a different slug → still no match
	retailRes, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
		CartID: cartID, SubtotalCents: 10000, Lines: []promotion.CartLineView{line},
		CustomerID: &custID, CustomerGroupSlug: "retail",
	})
	if err != nil {
		t.Fatalf("evaluate with retail slug: %v", err)
	}
	if got := retailRes.Result.Ledger.SubtotalDiscountCents(); got != 0 {
		t.Errorf("with retail slug: discount = %d, want 0", got)
	}
}

// Manual (coded) rules come first, auto-apply after - the customer chose the manual ones, so the storefront renders them that way
func TestBuildAppliedSummary_ManualBeforeAuto(t *testing.T) {
	repo := newFakeRepo()
	manualID := uuid.New()
	autoID := uuid.New()
	codeID := uuid.New()
	cartID := uuid.New()

	repo.rules = []promotion.Rule{
		{ID: manualID, Slug: "manual", Name: "Manual 10", PublicLabel: "10 off (code)", RequiresCode: true, IsActive: true},
		{ID: autoID, Slug: "auto", Name: "Auto 5", PublicLabel: "Auto 5", IsActive: true},
	}
	repo.actions[manualID] = []promotion.Action{{
		ID: uuid.New(), RuleID: manualID, Type: "core.fixed_off_subtotal",
		Params: json.RawMessage(`{"cents": 1000}`),
	}}
	repo.actions[autoID] = []promotion.Action{{
		ID: uuid.New(), RuleID: autoID, Type: "core.fixed_off_subtotal",
		Params: json.RawMessage(`{"cents": 500}`),
	}}
	repo.codes["MAN10"] = promotion.Code{ID: codeID, RuleID: manualID, Code: "MAN10", IsActive: true}
	repo.applied[cartID] = []promotion.AppliedCodeRecord{{
		CartID: cartID, RuleID: manualID, CodeID: &codeID, CodeText: "MAN10",
	}}

	svc := promotion.NewService(repo)
	res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
		CartID:        cartID,
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("evaluate cart: %v", err)
	}

	summary := svc.BuildAppliedSummary(context.Background(), res)
	if len(summary) != 2 {
		t.Fatalf("summary rows = %d, want 2 (one manual + one auto)", len(summary))
	}
	if summary[0].Code != "MAN10" {
		t.Errorf("summary[0].Code = %q, want \"MAN10\" (manual first)", summary[0].Code)
	}
	if summary[1].Code != "" {
		t.Errorf("summary[1].Code = %q, want empty string (auto-apply second)", summary[1].Code)
	}
	if summary[0].SubtotalDiscount != 1000 {
		t.Errorf("manual row discount = %d, want 1000", summary[0].SubtotalDiscount)
	}
	if summary[1].SubtotalDiscount != 500 {
		t.Errorf("auto row discount = %d, want 500", summary[1].SubtotalDiscount)
	}
}

// An invalidated code still shows in the summary, with the reason and zero discount, so the storefront can say "no longer valid" instead of silently dropping it
func TestBuildAppliedSummary_InvalidatedCodePreserved(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	codeID := uuid.New()
	cartID := uuid.New()

	repo.rules = []promotion.Rule{{
		ID: ruleID, Slug: "expired", Name: "Expired", PublicLabel: "X",
		RequiresCode: true, IsActive: true,
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.fixed_off_subtotal",
		Params: json.RawMessage(`{"cents": 1000}`),
	}}
	repo.codes["X"] = promotion.Code{ID: codeID, RuleID: ruleID, Code: "X", IsActive: true}
	repo.applied[cartID] = []promotion.AppliedCodeRecord{{
		CartID: cartID, RuleID: ruleID, CodeID: &codeID, CodeText: "X",
		InvalidatedReason: "expired",
	}}

	svc := promotion.NewService(repo)
	res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
		CartID:        cartID,
		SubtotalCents: 5000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 5000, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("evaluate cart: %v", err)
	}
	summary := svc.BuildAppliedSummary(context.Background(), res)
	if len(summary) != 1 {
		t.Fatalf("summary rows = %d, want 1 (invalidated row should still appear)", len(summary))
	}
	if summary[0].InvalidatedReason != "expired" {
		t.Errorf("summary[0].InvalidatedReason = %q, want \"expired\"", summary[0].InvalidatedReason)
	}
	if summary[0].SubtotalDiscount != 0 {
		t.Errorf("invalidated row discount = %d, want 0", summary[0].SubtotalDiscount)
	}
}

// The reconcile pass stamps WHY an applied code stopped firing - terminal states
// persist via MarkCartCodeInvalidated, a transient conditions miss stays in-memory
func TestEvaluateCart_ReconcileStampsInvalidation(t *testing.T) {
	subtotalLine := []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 5000, Quantity: 1}}

	newApplied := func(repo *fakeRepo, cartID, ruleID, codeID uuid.UUID) {
		repo.applied[cartID] = []promotion.AppliedCodeRecord{{
			CartID: cartID, RuleID: ruleID, CodeID: &codeID, CodeText: "SAVE10",
		}}
	}

	t.Run("expired rule stamps expired and persists", func(t *testing.T) {
		repo := newFakeRepo()
		ruleID, codeID, cartID := uuid.New(), uuid.New(), uuid.New()
		past := time.Now().Add(-time.Hour)
		repo.rules = []promotion.Rule{{
			ID: ruleID, Slug: "save10", RequiresCode: true, IsActive: true, ValidTo: &past,
		}}
		repo.codes["SAVE10"] = promotion.Code{ID: codeID, RuleID: ruleID, Code: "SAVE10", IsActive: true}
		newApplied(repo, cartID, ruleID, codeID)

		svc := promotion.NewService(repo)
		res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
			CartID: cartID, SubtotalCents: 5000, Lines: subtotalLine,
		})
		if err != nil {
			t.Fatalf("evaluate cart: %v", err)
		}
		if got := res.Result.Ledger.SubtotalDiscountCents(); got != 0 {
			t.Fatalf("discount = %d, want 0", got)
		}
		if got := res.ManuallyApplied[0].InvalidatedReason; got != "expired" {
			t.Fatalf("reason = %q, want expired", got)
		}
		if got := repo.invalidated[ruleID]; got != "expired" {
			t.Fatalf("persisted reason = %q, want expired (terminal states must persist)", got)
		}
	})

	t.Run("disabled rule stamps no longer available", func(t *testing.T) {
		repo := newFakeRepo()
		ruleID, codeID, cartID := uuid.New(), uuid.New(), uuid.New()
		repo.rules = []promotion.Rule{{
			ID: ruleID, Slug: "save10", RequiresCode: true, IsActive: false,
		}}
		repo.codes["SAVE10"] = promotion.Code{ID: codeID, RuleID: ruleID, Code: "SAVE10", IsActive: true}
		newApplied(repo, cartID, ruleID, codeID)

		svc := promotion.NewService(repo)
		res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
			CartID: cartID, SubtotalCents: 5000, Lines: subtotalLine,
		})
		if err != nil {
			t.Fatalf("evaluate cart: %v", err)
		}
		if got := res.ManuallyApplied[0].InvalidatedReason; got != "no longer available" {
			t.Fatalf("reason = %q, want no longer available", got)
		}
		if got := repo.invalidated[ruleID]; got != "no longer available" {
			t.Fatalf("persisted reason = %q, want no longer available", got)
		}
	})

	t.Run("conditions miss is transient - shown but never persisted", func(t *testing.T) {
		repo := newFakeRepo()
		ruleID, codeID, cartID := uuid.New(), uuid.New(), uuid.New()
		repo.rules = []promotion.Rule{{
			ID: ruleID, Slug: "save10", RequiresCode: true, IsActive: true,
		}}
		// Threshold above the cart subtotal, so the rule stops matching after apply
		repo.conditions[ruleID] = []promotion.Condition{{
			ID: uuid.New(), RuleID: ruleID, Type: "core.cart_subtotal_range",
			Params: json.RawMessage(`{"min_cents": 10000}`),
		}}
		repo.codes["SAVE10"] = promotion.Code{ID: codeID, RuleID: ruleID, Code: "SAVE10", IsActive: true}
		newApplied(repo, cartID, ruleID, codeID)

		svc := promotion.NewService(repo)
		res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
			CartID: cartID, SubtotalCents: 5000, Lines: subtotalLine,
		})
		if err != nil {
			t.Fatalf("evaluate cart: %v", err)
		}
		if got := res.ManuallyApplied[0].InvalidatedReason; got != "cart no longer qualifies" {
			t.Fatalf("reason = %q, want cart no longer qualifies", got)
		}
		if got, ok := repo.invalidated[ruleID]; ok {
			t.Fatalf("transient miss persisted %q - the code could never refire", got)
		}
	})

	t.Run("firing rule stays clean", func(t *testing.T) {
		repo := newFakeRepo()
		ruleID, codeID, cartID := uuid.New(), uuid.New(), uuid.New()
		repo.rules = []promotion.Rule{{
			ID: ruleID, Slug: "save10", RequiresCode: true, IsActive: true,
		}}
		repo.actions[ruleID] = []promotion.Action{{
			ID: uuid.New(), RuleID: ruleID, Type: "core.fixed_off_subtotal",
			Params: json.RawMessage(`{"cents": 1000}`),
		}}
		repo.codes["SAVE10"] = promotion.Code{ID: codeID, RuleID: ruleID, Code: "SAVE10", IsActive: true}
		newApplied(repo, cartID, ruleID, codeID)

		svc := promotion.NewService(repo)
		res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
			CartID: cartID, SubtotalCents: 5000, Lines: subtotalLine,
		})
		if err != nil {
			t.Fatalf("evaluate cart: %v", err)
		}
		if got := res.Result.Ledger.SubtotalDiscountCents(); got != 1000 {
			t.Fatalf("discount = %d, want 1000", got)
		}
		if got := res.ManuallyApplied[0].InvalidatedReason; got != "" {
			t.Fatalf("firing rule got stamped %q", got)
		}
	})
}

// core.product_in_category gates on catalog facts a cart line doesn't carry, so EvaluateCart resolves them from the SKUs. Without that enrichment this rule - and core.product_kind and core.product_has_attribute with it - could never match a real cart, while the admin simulator (handed kind and category ids directly) made all three look like they worked
func TestEvaluateCart_EnrichesLinesWithCatalogFacts(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	shoes := uuid.New()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "shoes", Name: "20% off shoes", IsActive: true}}
	repo.conditions[ruleID] = []promotion.Condition{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.product_in_category",
		Params: json.RawMessage(`{"category_ids": ["` + shoes.String() + `"]}`),
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 2000}`),
	}}
	repo.lineAttrs = map[string]promotion.LineProductAttrs{
		"SNEAKER": {ProductID: uuid.New(), Kind: "simple", CategoryIDs: []uuid.UUID{shoes}},
	}
	svc := promotion.NewService(repo)
	cartID := uuid.New()

	res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
		CartID:        cartID,
		SubtotalCents: 10000,
		Lines: []promotion.CartLineView{{
			LineID: uuid.New(), ProductSKU: "SNEAKER", UnitPriceCents: 10000, Quantity: 1,
		}},
	})
	if err != nil {
		t.Fatalf("evaluate cart: %v", err)
	}
	if got := res.Result.Ledger.SubtotalDiscountCents(); got != 2000 {
		t.Errorf("subtotal discount = %d, want 2000 (the line's category has to reach the condition)", got)
	}

	// A SKU the catalog doesn't know stays unenriched, so the rule simply doesn't match
	res, err = svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
		CartID:        cartID,
		SubtotalCents: 10000,
		Lines: []promotion.CartLineView{{
			LineID: uuid.New(), ProductSKU: "SOCK", UnitPriceCents: 10000, Quantity: 1,
		}},
	})
	if err != nil {
		t.Fatalf("evaluate cart (unknown sku): %v", err)
	}
	if got := res.Result.Ledger.SubtotalDiscountCents(); got != 0 {
		t.Errorf("subtotal discount = %d, want 0", got)
	}
}

// Segment slugs are resolved inside EvaluateCart, so no caller can forget them
func TestEvaluateCart_ResolvesCustomerSegments(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "vip", Name: "vip 10%", IsActive: true}}
	repo.conditions[ruleID] = []promotion.Condition{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.customer_in_segment",
		Params: json.RawMessage(`{"slugs": ["vip"]}`),
	}}
	repo.actions[ruleID] = []promotion.Action{{
		ID: uuid.New(), RuleID: ruleID, Type: "core.percent_off_subtotal",
		Params: json.RawMessage(`{"bps": 1000}`),
	}}
	customerID := uuid.New()
	svc := promotion.NewService(repo)
	svc.SetCustomerSegments(fakeSegments{customerID: []string{"vip"}})

	res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
		CartID:        uuid.New(),
		SubtotalCents: 10000,
		CustomerID:    &customerID,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("evaluate cart: %v", err)
	}
	if got := res.Result.Ledger.SubtotalDiscountCents(); got != 1000 {
		t.Errorf("subtotal discount = %d, want 1000 (the vip segment must be resolved for the caller)", got)
	}
}

type fakeSegments map[uuid.UUID][]string

func (f fakeSegments) SlugsForCustomer(_ context.Context, id uuid.UUID) ([]string, error) {
	return f[id], nil
}
