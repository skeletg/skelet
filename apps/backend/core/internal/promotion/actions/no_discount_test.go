package actions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// A line no promotion may lower - its price was set by hand - still counts toward what the cart holds, but no action
// works a discount out of it or puts one on it
func TestActionsLeaveANoDiscountLineAlone(t *testing.T) {
	set, plain := uuid.New(), uuid.New()
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		Currency:      "USD",
		SubtotalCents: 15_000,
		Lines: []promotion.CartLineView{
			{LineID: set, ProductSKU: "JACKET", UnitPriceCents: 5_000, Quantity: 2, NoDiscount: true},
			{LineID: plain, ProductSKU: "MUG", UnitPriceCents: 2_500, Quantity: 2},
		},
	})
	if got := promotion.DiscountableSubtotal(env); got != 5_000 {
		t.Fatalf("discountable subtotal = %d, want the 5000 of the line promotions may touch", got)
	}
	raw := func(v any) json.RawMessage {
		t.Helper()
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	t.Run("a percentage off the cart is a percentage of the rest", func(t *testing.T) {
		l := newFullLedger()
		if err := (percentOffSubtotal{}).Apply(context.Background(), env, raw(map[string]any{"bps": 1000}), l, promotion.AppliedRuleContext{}); err != nil {
			t.Fatal(err)
		}
		if l.subtotal != 500 {
			t.Errorf("10%% off came to %d, want 500 - a tenth of the 5000", l.subtotal)
		}
	})
	t.Run("an amount off the cart stops at the rest", func(t *testing.T) {
		l := newFullLedger()
		if err := (fixedOffSubtotal{}).Apply(context.Background(), env, raw(map[string]any{"cents": 9_000}), l, promotion.AppliedRuleContext{}); err != nil {
			t.Fatal(err)
		}
		if l.subtotal != 5_000 {
			t.Errorf("9000 off came to %d, want the 5000 there is to take it from", l.subtotal)
		}
	})
	t.Run("a discount on matching items skips it", func(t *testing.T) {
		l := newFullLedger()
		if err := (percentOffMatching{}).Apply(context.Background(), env, raw(map[string]any{"bps": 5000, "condition": matchAll}), l, promotion.AppliedRuleContext{}); err != nil {
			t.Fatal(err)
		}
		if l.lines[set] != 0 || l.lines[plain] != 2_500 {
			t.Errorf("half off everything: %d on the set-price line, %d on the other - want 0 and 2500", l.lines[set], l.lines[plain])
		}
	})
	t.Run("buy one get one neither counts it nor gives it away", func(t *testing.T) {
		l := newFullLedger()
		params := raw(map[string]any{"buy_qty": 1, "get_qty": 1, "get_kind": "free", "target_condition": matchAll})
		if err := (bogo{}).Apply(context.Background(), env, params, l, promotion.AppliedRuleContext{}); err != nil {
			t.Fatal(err)
		}
		if l.lines[set] != 0 || l.lines[plain] != 2_500 {
			t.Errorf("bogo: %d on the set-price line, %d on the other - want only the second mug free", l.lines[set], l.lines[plain])
		}
	})
}
