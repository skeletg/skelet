package e2e

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// Scenario: rules in one exclusivity group don't stack - the better-priority rule wins alone -
// and a cap action bounds whatever several actions would otherwise sum to
func TestPromotionExclusivityGroupAndCap(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2500)
	baseline := h.PlaceGuestCOD(t, f, 1)

	group := "e2e-x-" + uuid.NewString()[:8]
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E group winner",
			ChannelID: &f.Channel.ID, IsActive: true, Priority: 1, ExclusivityGroup: group,
		},
		Actions: []promotion.Action{{Type: "core.fixed_off_subtotal", Params: json.RawMessage(`{"cents":400}`)}},
	})
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E group loser",
			ChannelID: &f.Channel.ID, IsActive: true, Priority: 2, ExclusivityGroup: group,
		},
		Actions: []promotion.Action{{Type: "core.fixed_off_subtotal", Params: json.RawMessage(`{"cents":300}`)}},
	})

	o := h.PlaceGuestCOD(t, f, 1)
	if d := o.DiscountAmount - baseline.DiscountAmount; d != 400 {
		t.Errorf("exclusive group discounted %d, want only the priority winner's 400", d)
	}
	h.Settle(t)
}

func TestPromotionCapBoundsStackedActions(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2500)
	baseline := h.PlaceGuestCOD(t, f, 1)

	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E capped stack",
			ChannelID: &f.Channel.ID, IsActive: true,
		},
		Actions: []promotion.Action{
			{Type: "core.fixed_off_subtotal", Params: json.RawMessage(`{"cents":500}`), SortOrder: 1},
			{Type: "core.fixed_off_subtotal", Params: json.RawMessage(`{"cents":400}`), SortOrder: 2},
			{Type: "core.cap_total_discount", Params: json.RawMessage(`{"max_cents":600}`), SortOrder: 3},
		},
	})

	o := h.PlaceGuestCOD(t, f, 1)
	if d := o.DiscountAmount - baseline.DiscountAmount; d != 600 {
		t.Errorf("capped stack discounted %d, want 900 capped to 600", d)
	}
	h.Settle(t)
}

// Scenario: max_uses is a hard budget - sequentially the exhausted rule stops applying, and under
// a race the redemption counter never overspends no matter which side wins
func TestPromotionMaxUsesNeverOverspent(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	baseline := h.PlaceGuestCOD(t, f, 1)
	one := int32(1)
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E one use",
			ChannelID: &f.Channel.ID, IsActive: true, MaxUses: &one,
		},
		Actions: []promotion.Action{{Type: "core.fixed_off_subtotal", Params: json.RawMessage(`{"cents":500}`)}},
	})

	first := h.PlaceGuestCOD(t, f, 1)
	if d := first.DiscountAmount - baseline.DiscountAmount; d != 500 {
		t.Fatalf("first use discounted %d, want 500", d)
	}
	// The exhausted rule must never discount again - though within the rules cache's staleness window the redeem re-check refuses the
	// placement outright rather than skipping the rule, so both outcomes are legal here; only a second discount is not
	second, err := h.TryPlace(t, f.Channel, h.NewGuestCart(t, f, 1))
	if err == nil {
		if d := second.DiscountAmount - baseline.DiscountAmount; d != 0 {
			t.Errorf("exhausted rule still discounted %d, want 0", d)
		}
	}
	h.Settle(t)

	// The race: one use left on a fresh rule, two placements at once. The loser either places
	// undiscounted (saw the spent budget) or fails at redemption - it must never mint a second use
	f2 := h.NewStore(t, 10, 2000)
	base2 := h.PlaceGuestCOD(t, f2, 1)
	uses := int32(1)
	rule2 := h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E raced use",
			ChannelID: &f2.Channel.ID, IsActive: true, MaxUses: &uses,
		},
		Actions: []promotion.Action{{Type: "core.fixed_off_subtotal", Params: json.RawMessage(`{"cents":500}`)}},
	})
	results := []struct {
		id  uuid.UUID
		o   order.Order
		err error
	}{
		{id: h.NewGuestCart(t, f2, 1)},
		{id: h.NewGuestCart(t, f2, 1)},
	}
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			results[i].o, results[i].err = h.TryPlace(t, f2.Channel, results[i].id)
		})
	}
	wg.Wait()

	discounted := 0
	for _, r := range results {
		if r.err == nil && r.o.DiscountAmount-base2.DiscountAmount == 500 {
			discounted++
		}
	}
	if discounted > 1 {
		t.Errorf("%d racing placements got the single-use discount", discounted)
	}
	redemptions := 0
	for _, r := range results {
		if r.err != nil {
			continue
		}
		reds, err := h.D.Promotion.RedemptionsForOrder(h.Ctx, r.o.ID)
		if err != nil {
			t.Fatalf("redemptions: %v", err)
		}
		for _, red := range reds {
			if red.RuleID != nil && *red.RuleID == rule2.ID {
				redemptions++
			}
		}
	}
	if redemptions > 1 {
		t.Errorf("rule redeemed %d times against max_uses 1", redemptions)
	}
	h.Settle(t)
}
