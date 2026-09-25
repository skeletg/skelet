package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

func (h *harness) newRule(t *testing.T, in promotion.CreateRuleInput) promotion.Rule {
	t.Helper()
	rule, err := h.D.Promotion.CreateRule(h.Ctx, in)
	if err != nil {
		t.Fatalf("create promotion: %v", err)
	}
	return rule
}

// Scenario: an auto-apply SKU-gated discount lands on the order as money, not decoration - the
// discount reduces the total, the collected amount, and leaves an audit redemption row
func TestPromotionAutoApplyDiscountsPlacement(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2500)
	baseline := h.PlaceGuestCOD(t, f, 1) // ambient storewide rules may exist on a shared dev DB - assert the delta
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E $5 off",
			ChannelID: &f.Channel.ID, IsActive: true,
		},
		Conditions: []promotion.Condition{{Type: "core.cart_contains_sku",
			Params: json.RawMessage(`{"skus":["` + f.Product.SKU + `"]}`)}},
		Actions: []promotion.Action{{Type: "core.fixed_off_subtotal",
			Params: json.RawMessage(`{"cents":500}`)}},
	})

	o := h.PlaceGuestCOD(t, f, 1)
	if d := o.DiscountAmount - baseline.DiscountAmount; d != 500 {
		t.Errorf("rule added %d of discount, want 500", d)
	}
	if d := baseline.TotalAmount - o.TotalAmount; d != 500 {
		t.Errorf("rule reduced the total by %d, want 500", d)
	}
	reds, err := h.D.Promotion.RedemptionsForOrder(h.Ctx, o.ID)
	if err != nil || len(reds) != 1 {
		t.Errorf("redemptions = %d (%v), want the audit row", len(reds), err)
	}
	h.Settle(t)

	// The discount is real money: collecting the whole balance banks the discounted total
	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID, Method: "cod", Reference: "e2e-cash",
	}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	if sums := h.LedgerByKind(t, o.ID); sums[order.PaymentTxnCapture] != o.TotalAmount {
		t.Errorf("captured %d, want the discounted total %d", sums[order.PaymentTxnCapture], o.TotalAmount)
	}
}

// Scenario: a coupon-gated rule stays dark until its code attaches to the cart, and placement
// consumes the code - applied rows cleared, redemption recorded
func TestPromotionCouponPercentOff(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 3000)
	rule := h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E 10% off",
			ChannelID: &f.Channel.ID, IsActive: true, RequiresCode: true,
		},
		Actions: []promotion.Action{{Type: "core.percent_off_subtotal",
			Params: json.RawMessage(`{"bps":1000}`)}},
	})
	code := strings.ToUpper("E2E" + uuid.NewString()[:6]) // stored uppercase - ApplyCode uppercases its input and matches exactly
	if _, err := h.D.Promotion.Repo().CreateCode(h.Ctx, promotion.Code{
		RuleID: rule.ID, Code: code, IsActive: true,
	}); err != nil {
		t.Fatalf("create code: %v", err)
	}

	// Without the code the rule must not fire - the codeless placement is also the ambient baseline
	dark := h.PlaceGuestCOD(t, f, 1)

	cartID := h.NewGuestCart(t, f, 1)
	if _, err := h.D.Promotion.ApplyCode(h.Ctx, cartID, f.Channel.ID, code, nil); err != nil {
		t.Fatalf("apply code: %v", err)
	}
	o, err := h.TryPlace(t, f.Channel, cartID)
	if err != nil {
		t.Fatalf("place with coupon: %v", err)
	}
	if d := o.DiscountAmount - dark.DiscountAmount; d != 300 {
		t.Errorf("coupon added %d of discount over the codeless placement, want 300", d)
	}
	if codes, err := h.D.Promotion.ListCartAppliedCodes(h.Ctx, cartID); err != nil || len(codes) != 0 {
		t.Errorf("applied codes after placement = %d (%v), want cleared", len(codes), err)
	}
	h.Settle(t)
}

// Scenario: a discount can never exceed the subtotal - the engine clamps, the order floors at zero
func TestPromotionDiscountClampsToSubtotal(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1500)
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E absurd off",
			ChannelID: &f.Channel.ID, IsActive: true,
		},
		Actions: []promotion.Action{{Type: "core.fixed_off_subtotal",
			Params: json.RawMessage(`{"cents":999999}`)}},
	})

	o := h.PlaceGuestCOD(t, f, 1)
	if o.DiscountAmount != 1500 || o.TotalAmount != 0 {
		t.Errorf("discount/total = %d/%d, want clamped 1500/0", o.DiscountAmount, o.TotalAmount)
	}
	if sums := h.LedgerByKind(t, o.ID); len(sums) != 0 {
		t.Errorf("ledger = %+v for a zero-total order, want empty", sums)
	}
	h.Settle(t)
}

// Scenario: product-scoped conditions must match REAL
// carts, which requires the catalog enrichment only Service.EvaluateCart performs
func TestPromotionProductScopedConditionMatchesRealCart(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	baseline := h.PlaceGuestCOD(t, f, 1)
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E kind-scoped",
			ChannelID: &f.Channel.ID, IsActive: true,
		},
		Conditions: []promotion.Condition{{Type: "core.product_kind",
			Params: json.RawMessage(`{"kinds":["simple"]}`)}},
		Actions: []promotion.Action{{Type: "core.fixed_off_subtotal",
			Params: json.RawMessage(`{"cents":300}`)}},
	})

	o := h.PlaceGuestCOD(t, f, 1)
	if d := o.DiscountAmount - baseline.DiscountAmount; d != 300 {
		t.Errorf("product-scoped condition missed a real cart: added discount = %d, want 300", d)
	}
	h.Settle(t)
}

// Scenario: a free gift reaches the order as a zero-priced gift line - and is dropped, not fatal,
// once the gift's stock can no longer cover it
func TestPromotionFreeGiftGrantedAndDroppedWhenOutOfStock(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2200)
	gift := h.AddProduct(t, f, 400, true, 1) // one unit: first order takes it, second finds none
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E free gift",
			ChannelID: &f.Channel.ID, IsActive: true,
		},
		Conditions: []promotion.Condition{{Type: "core.cart_contains_sku",
			Params: json.RawMessage(`{"skus":["` + f.Product.SKU + `"]}`)}},
		Actions: []promotion.Action{{Type: "core.add_free_gift",
			Params: json.RawMessage(`{"product_sku":"` + gift.SKU + `","qty":1}`)}},
	})

	granted := h.PlaceGuestCOD(t, f, 1)
	giftLines := 0
	for _, l := range h.OrderLines(t, granted.ID) {
		if l.IsGift {
			giftLines++
			if l.UnitPrice != 0 {
				t.Errorf("gift line priced at %d, want 0", l.UnitPrice)
			}
		}
	}
	if giftLines != 1 {
		t.Fatalf("gift lines = %d, want 1", giftLines)
	}
	h.Settle(t)

	// The gift's single unit is now held - the next order must drop the gift rather than fail
	dropped := h.PlaceGuestCOD(t, f, 1)
	for _, l := range h.OrderLines(t, dropped.ID) {
		if l.IsGift {
			t.Error("gift granted from empty stock - placement should drop it")
		}
	}
	h.Settle(t)
}

// Scenario: a segment-gated discount fires for a member and nobody else - the condition reads
// membership the engine resolves for itself, so a cart that never carried slugs still matches
func TestSegmentGatedPromotionDiscountsAMember(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)

	slug := "e2e-vip-" + uuid.NewString()[:8]
	segID := h.newSegment(t, slug)
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-seg-" + uuid.NewString()[:8], Name: "E2E segment $7 off",
			ChannelID: &f.Channel.ID, IsActive: true,
		},
		Conditions: []promotion.Condition{{Type: "core.customer_in_segment",
			Params: json.RawMessage(`{"slugs":["` + slug + `"]}`)}},
		Actions: []promotion.Action{{Type: "core.fixed_off_subtotal",
			Params: json.RawMessage(`{"cents":700}`)}},
	})

	member, memberEmail, _ := h.registeredBuyer(t, f)
	outsider, outsiderEmail, _ := h.registeredBuyer(t, f)
	if err := h.D.SignalsClient.AssignSegmentMembers(h.Ctx, segID, []uuid.UUID{member}); err != nil {
		t.Fatalf("assign member: %v", err)
	}

	base := h.placeFor(t, f, outsider, outsiderEmail, 1)
	got := h.placeFor(t, f, member, memberEmail, 1)
	if d := got.DiscountAmount - base.DiscountAmount; d != 700 {
		t.Errorf("member got %d of discount over a non-member, want 700", d)
	}
	if d := base.TotalAmount - got.TotalAmount; d != 700 {
		t.Errorf("member paid %d less, want 700", d)
	}
}
