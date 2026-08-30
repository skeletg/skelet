package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/order"
	"github.com/skeletg/skelet/internal/promotion"
)

func (h *harness) newRule(t *testing.T, in promotion.CreateRuleInput) promotion.Rule {
	t.Helper()
	rule, err := h.d.Promotion.CreateRule(h.ctx, in)
	if err != nil {
		t.Fatalf("create promotion: %v", err)
	}
	return rule
}

// Scenario: an auto-apply SKU-gated discount lands on the order as money, not decoration - the
// discount reduces the total, the collected amount, and leaves an audit redemption row
func TestPromotionAutoApplyDiscountsPlacement(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 2500)
	baseline := h.placeGuestCOD(t, f, 1) // ambient storewide rules may exist on a shared dev DB - assert the delta
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

	o := h.placeGuestCOD(t, f, 1)
	if d := o.DiscountAmount - baseline.DiscountAmount; d != 500 {
		t.Errorf("rule added %d of discount, want 500", d)
	}
	if d := baseline.TotalAmount - o.TotalAmount; d != 500 {
		t.Errorf("rule reduced the total by %d, want 500", d)
	}
	reds, err := h.d.Promotion.RedemptionsForOrder(h.ctx, o.ID)
	if err != nil || len(reds) != 1 {
		t.Errorf("redemptions = %d (%v), want the audit row", len(reds), err)
	}
	h.settle(t)

	// The discount is real money: collecting the whole balance banks the discounted total
	if _, err := h.d.Order.RecordOfflinePayment(h.ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID, Method: "cod", Reference: "e2e-cash",
	}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	if sums := h.ledgerByKind(t, o.ID); sums[order.PaymentTxnCapture] != o.TotalAmount {
		t.Errorf("captured %d, want the discounted total %d", sums[order.PaymentTxnCapture], o.TotalAmount)
	}
}

// Scenario: a coupon-gated rule stays dark until its code attaches to the cart, and placement
// consumes the code - applied rows cleared, redemption recorded
func TestPromotionCouponPercentOff(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 3000)
	rule := h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E 10% off",
			ChannelID: &f.Channel.ID, IsActive: true, RequiresCode: true,
		},
		Actions: []promotion.Action{{Type: "core.percent_off_subtotal",
			Params: json.RawMessage(`{"bps":1000}`)}},
	})
	code := strings.ToUpper("E2E" + uuid.NewString()[:6]) // stored uppercase - ApplyCode uppercases its input and matches exactly
	if _, err := h.d.Promotion.Repo().CreateCode(h.ctx, promotion.Code{
		RuleID: rule.ID, Code: code, IsActive: true,
	}); err != nil {
		t.Fatalf("create code: %v", err)
	}

	// Without the code the rule must not fire - the codeless placement is also the ambient baseline
	dark := h.placeGuestCOD(t, f, 1)

	cartID := h.newGuestCart(t, f, 1)
	if _, err := h.d.Promotion.ApplyCode(h.ctx, cartID, code, nil); err != nil {
		t.Fatalf("apply code: %v", err)
	}
	o, err := h.tryPlace(t, f.Channel, cartID)
	if err != nil {
		t.Fatalf("place with coupon: %v", err)
	}
	if d := o.DiscountAmount - dark.DiscountAmount; d != 300 {
		t.Errorf("coupon added %d of discount over the codeless placement, want 300", d)
	}
	if codes, err := h.d.Promotion.ListCartAppliedCodes(h.ctx, cartID); err != nil || len(codes) != 0 {
		t.Errorf("applied codes after placement = %d (%v), want cleared", len(codes), err)
	}
	h.settle(t)
}

// Scenario: a discount can never exceed the subtotal - the engine clamps, the order floors at zero
func TestPromotionDiscountClampsToSubtotal(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 1500)
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E absurd off",
			ChannelID: &f.Channel.ID, IsActive: true,
		},
		Actions: []promotion.Action{{Type: "core.fixed_off_subtotal",
			Params: json.RawMessage(`{"cents":999999}`)}},
	})

	o := h.placeGuestCOD(t, f, 1)
	if o.DiscountAmount != 1500 || o.TotalAmount != 0 {
		t.Errorf("discount/total = %d/%d, want clamped 1500/0", o.DiscountAmount, o.TotalAmount)
	}
	if sums := h.ledgerByKind(t, o.ID); len(sums) != 0 {
		t.Errorf("ledger = %+v for a zero-total order, want empty", sums)
	}
	h.settle(t)
}

// Scenario: product-scoped conditions must match REAL
// carts, which requires the catalog enrichment only Service.EvaluateCart performs
func TestPromotionProductScopedConditionMatchesRealCart(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 2000)
	baseline := h.placeGuestCOD(t, f, 1)
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

	o := h.placeGuestCOD(t, f, 1)
	if d := o.DiscountAmount - baseline.DiscountAmount; d != 300 {
		t.Errorf("product-scoped condition missed a real cart: added discount = %d, want 300", d)
	}
	h.settle(t)
}

// Scenario: a free gift reaches the order as a zero-priced gift line - and is dropped, not fatal,
// once the gift's stock can no longer cover it
func TestPromotionFreeGiftGrantedAndDroppedWhenOutOfStock(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 2200)
	gift := h.addProduct(t, f, 400, true, 1) // one unit: first order takes it, second finds none
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

	granted := h.placeGuestCOD(t, f, 1)
	giftLines := 0
	for _, l := range h.orderLines(t, granted.ID) {
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
	h.settle(t)

	// The gift's single unit is now held - the next order must drop the gift rather than fail
	dropped := h.placeGuestCOD(t, f, 1)
	for _, l := range h.orderLines(t, dropped.ID) {
		if l.IsGift {
			t.Error("gift granted from empty stock - placement should drop it")
		}
	}
	h.settle(t)
}
