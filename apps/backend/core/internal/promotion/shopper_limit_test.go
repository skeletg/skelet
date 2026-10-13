package promotion_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// An auto-apply "once per customer" rule has no code to remove - if eval let it discount a returning customer's cart, Redeem refused and checkout failed for good
func TestEvalSkipsARuleTheShopperHasUsedUp(t *testing.T) {
	limit := int32(1)
	ruleID := uuid.New()
	repo := newFakeRepo()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "welcome", IsActive: true, PerCustomerLimit: &limit}}
	repo.actions[ruleID] = []promotion.Action{{ID: uuid.New(), RuleID: ruleID, Type: "core.percent_off_subtotal", Params: json.RawMessage(`{"bps": 1000}`)}}
	repo.usedByRule = map[uuid.UUID]int32{ruleID: 1}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	customer := uuid.New()
	for name, in := range map[string]promotion.EnvironmentInput{
		"signed in": {CustomerID: &customer},
		"guest":     {CustomerEmail: "Shopper@Example.test"},
	} {
		in.SubtotalCents = 10000
		in.Lines = []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}}
		res, err := engine.Evaluate(promotion.WithTrace(context.Background()), promotion.BuildEnvironment(in))
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Applied) != 0 {
			t.Errorf("%s: a used-up rule applied", name)
		}
		if len(res.Trace) != 1 || res.Trace[0].SkipReason != "per_customer_limit_reached" {
			t.Errorf("%s: trace %+v, want per_customer_limit_reached", name, res.Trace)
		}
	}
	// nobody to count against - the redemption-time check is all there is
	res, err := engine.Evaluate(context.Background(), promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	}))
	if err != nil || len(res.Applied) != 1 {
		t.Errorf("anonymous cart: applied %d, %v - want the rule", len(res.Applied), err)
	}
}

func TestEvalSkipsACodeTheShopperHasUsedUp(t *testing.T) {
	limit := int32(1)
	ruleID, codeID := uuid.New(), uuid.New()
	repo := newFakeRepo()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "coded", IsActive: true, RequiresCode: true}}
	repo.actions[ruleID] = []promotion.Action{{ID: uuid.New(), RuleID: ruleID, Type: "core.percent_off_subtotal", Params: json.RawMessage(`{"bps": 1000}`)}}
	repo.codes["ONCE"] = promotion.Code{ID: codeID, RuleID: ruleID, Code: "ONCE", IsActive: true, PerCustomerLimit: &limit}
	repo.usedByCode = map[uuid.UUID]int32{codeID: 1}
	res, err := promotion.NewEngine(repo, promotion.DefaultRegistry).Evaluate(context.Background(), promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		CustomerEmail: "g@example.test",
		AppliedCodes:  []string{"ONCE"},
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 0 {
		t.Error("a code used up by this shopper still discounted the cart")
	}
}

func TestGenerateCodesRefusesAGuessablePattern(t *testing.T) {
	repo := newFakeRepo()
	ruleID := uuid.New()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "r", IsActive: true}}
	svc := promotion.NewService(repo)
	if _, err := svc.GenerateCodes(context.Background(), ruleID, "SALE-[N]", 1000, "", ""); err == nil {
		t.Error("ten possible codes accepted for a batch of 1000")
	}
	codes, err := svc.GenerateCodes(context.Background(), ruleID, "spring-[*][*][*][*]", 10, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range codes {
		if c[:7] != "SPRING-" {
			t.Errorf("%q kept a lowercase literal - ApplyCode uppercases input, so it could never be redeemed", c)
		}
	}
}

// The order email is the guest's identity at checkout - redemption is the authoritative gate, so it has to count them too
func TestRedeemRefusesAGuestOverTheLimit(t *testing.T) {
	limit := int32(1)
	ruleID := uuid.New()
	repo := newFakeRepo()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "welcome", IsActive: true, PerCustomerLimit: &limit}}
	repo.usedByRule = map[uuid.UUID]int32{ruleID: 1}
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{CustomerEmail: "g@example.test"})
	err := promotion.NewService(repo).Redeem(context.Background(), nil, env, promotion.AppliedRule{RuleID: ruleID}, nil, uuid.New(), nil)
	if !errors.Is(err, promotion.ErrPerCustomerLimitRule) {
		t.Errorf("guest redemption over the limit: %v, want ErrPerCustomerLimitRule", err)
	}
}

func TestApplyCodeRefusesAGuestOverTheLimit(t *testing.T) {
	limit := int32(1)
	ruleID, codeID, cartID := uuid.New(), uuid.New(), uuid.New()
	repo := newFakeRepo()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "coded", IsActive: true, RequiresCode: true}}
	repo.codes["ONCE"] = promotion.Code{ID: codeID, RuleID: ruleID, Code: "ONCE", IsActive: true, PerCustomerLimit: &limit}
	repo.usedByCode = map[uuid.UUID]int32{codeID: 1}
	repo.cartEmails = map[uuid.UUID]string{cartID: "g@example.test"}
	_, err := promotion.NewService(repo).ApplyCode(context.Background(), cartID, uuid.New(), "once", nil)
	if !errors.Is(err, promotion.ErrPerCustomerLimitCode) {
		t.Errorf("guest applying a used-up code: %v, want ErrPerCustomerLimitCode", err)
	}
}

// The cached rule list was filtered by NOW() when it loaded - a rule that expires inside the TTL must stop firing then, not when the cache turns over
func TestACachedRuleStopsAtItsValidTo(t *testing.T) {
	ruleID := uuid.New()
	ends := time.Now().Add(time.Hour)
	repo := newFakeRepo()
	repo.rules = []promotion.Rule{{ID: ruleID, Slug: "flash", IsActive: true, ValidTo: &ends}}
	repo.actions[ruleID] = []promotion.Action{{ID: uuid.New(), RuleID: ruleID, Type: "core.percent_off_subtotal", Params: json.RawMessage(`{"bps": 1000}`)}}
	engine := promotion.NewEngine(repo, promotion.DefaultRegistry)
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		SubtotalCents: 10000,
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}},
	})
	if res, err := engine.Evaluate(context.Background(), env); err != nil || len(res.Applied) != 1 {
		t.Fatalf("live rule: applied %d, %v", len(res.Applied), err)
	}
	defer promotion.PinNowForTest(ends.Add(time.Minute))()
	if res, err := engine.Evaluate(context.Background(), env); err != nil || len(res.Applied) != 0 {
		t.Errorf("expired rule still applied from the cache: %d, %v", len(res.Applied), err)
	}
}
