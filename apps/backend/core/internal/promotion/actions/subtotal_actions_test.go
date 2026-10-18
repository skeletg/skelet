package actions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// These actions decide what a shopper pays, and every one of them takes params an operator typed into the dashboard. So there are two jobs: ValidateParams has to refuse a rule that would misprice a cart, and Apply has to stay inside the cart's own subtotal however the numbers read

// fullLedger records every channel, unlike bogo_test's recordingLedger which only needs line adjustments
type fullLedger struct {
	subtotal     int64
	lines        map[uuid.UUID]int64
	gifts        []promotion.FreeGift
	freeShipping bool
	methods      []string
	override     *int64
	cappedAt     *int64
}

func newFullLedger() *fullLedger { return &fullLedger{lines: map[uuid.UUID]int64{}} }

func (l *fullLedger) AddSubtotalDiscount(cents int64)             { l.subtotal += cents }
func (l *fullLedger) AddLineAdjustment(id uuid.UUID, cents int64) { l.lines[id] += cents }
func (l *fullLedger) SetFreeShipping(methods []string)            { l.freeShipping = true; l.methods = methods }
func (l *fullLedger) SetShippingOverride(cents int64)             { l.override = &cents }
func (l *fullLedger) AddFreeGift(sku string, qty float64) {
	l.gifts = append(l.gifts, promotion.FreeGift{ProductSKU: sku, Quantity: qty})
}
func (l *fullLedger) CapSubtotalDiscount(max int64)        { l.cappedAt = &max }
func (l *fullLedger) SubtotalDiscountCents() int64         { return l.subtotal }
func (l *fullLedger) LineAdjustments() map[uuid.UUID]int64 { return l.lines }
func (l *fullLedger) LineAdjustment(id uuid.UUID) int64    { return l.lines[id] }
func (l *fullLedger) FreeShipping() bool                   { return l.freeShipping }
func (l *fullLedger) FreeShippingMethods() []string        { return l.methods }
func (l *fullLedger) ShippingOverride() *int64             { return l.override }
func (l *fullLedger) FreeGifts() []promotion.FreeGift      { return l.gifts }

// BuildEnvironment takes the subtotal explicitly rather than deriving it, the same way the engine's
// real caller hands over the cart's own total
func cartOf(unitCents int64, qty float64) promotion.Environment {
	return promotion.BuildEnvironment(promotion.EnvironmentInput{
		Currency:      "USD",
		SubtotalCents: money.LineTotal(unitCents, qty),
		Lines:         []promotion.CartLineView{{LineID: uuid.New(), ProductSKU: "SKU-1", UnitPriceCents: unitCents, Quantity: qty}},
	})
}

func params(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return raw
}

func apply(t *testing.T, a promotion.ActionApplier, env promotion.Environment, p json.RawMessage) *fullLedger {
	t.Helper()
	l := newFullLedger()
	if err := a.Apply(context.Background(), env, p, l, promotion.AppliedRuleContext{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return l
}

func TestFixedOffSubtotalNeverExceedsTheCart(t *testing.T) {
	// a 50 off rule on a 30 cart would otherwise hand back money
	l := apply(t, fixedOffSubtotal{}, cartOf(3000, 1), params(t, map[string]any{"cents": 5000}))
	if l.subtotal != 3000 {
		t.Errorf("discount = %d, want it clamped to the 3000 subtotal", l.subtotal)
	}
}

func TestFixedOffSubtotalTakesTheAmountWhenItFits(t *testing.T) {
	l := apply(t, fixedOffSubtotal{}, cartOf(3000, 1), params(t, map[string]any{"cents": 500}))
	if l.subtotal != 500 {
		t.Errorf("discount = %d, want 500", l.subtotal)
	}
}

func TestFixedOffSubtotalRejectsAnAmountThatWouldDoNothing(t *testing.T) {
	for _, cents := range []int64{0, -100} {
		if err := (fixedOffSubtotal{}).ValidateParams(params(t, map[string]any{"cents": cents})); err == nil {
			t.Errorf("cents=%d accepted, want a validation error", cents)
		}
	}
	if err := (fixedOffSubtotal{}).ValidateParams(params(t, map[string]any{"cents": 1})); err != nil {
		t.Errorf("cents=1 rejected: %v", err)
	}
}

func TestPercentOffSubtotalComputesFromBasisPoints(t *testing.T) {
	// 1000 bps = 10% of 5000
	l := apply(t, percentOffSubtotal{}, cartOf(5000, 1), params(t, map[string]any{"bps": 1000}))
	if l.subtotal != 500 {
		t.Errorf("discount = %d, want 500", l.subtotal)
	}
}

func TestPercentOffSubtotalHonoursItsOwnCeiling(t *testing.T) {
	l := apply(t, percentOffSubtotal{}, cartOf(100000, 1), params(t, map[string]any{"bps": 5000, "max_cents": 2000}))
	if l.subtotal != 2000 {
		t.Errorf("discount = %d, want the 2000 ceiling rather than 50%% of the cart", l.subtotal)
	}
}

func TestPercentOffSubtotalStaysSilentWhenItRoundsToNothing(t *testing.T) {
	// 1 bps of 50c is 0 - recording a zero discount would put an empty line on the ledger
	l := apply(t, percentOffSubtotal{}, cartOf(50, 1), params(t, map[string]any{"bps": 1}))
	if l.subtotal != 0 {
		t.Errorf("discount = %d, want nothing recorded", l.subtotal)
	}
}

func TestPercentOffSubtotalRejectsRatesOutsideOneHundredPercent(t *testing.T) {
	for _, bps := range []int32{0, -1, 10001} {
		if err := (percentOffSubtotal{}).ValidateParams(params(t, map[string]any{"bps": bps})); err == nil {
			t.Errorf("bps=%d accepted, want a validation error", bps)
		}
	}
	// 10000 bps is exactly 100% off, which is a legitimate rule
	if err := (percentOffSubtotal{}).ValidateParams(params(t, map[string]any{"bps": 10000})); err != nil {
		t.Errorf("bps=10000 rejected: %v", err)
	}
	if err := (percentOffSubtotal{}).ValidateParams(params(t, map[string]any{"bps": 500, "max_cents": -1})); err == nil {
		t.Error("negative max_cents accepted, want a validation error")
	}
}

func TestCapTotalDiscountPassesTheCeilingToTheLedger(t *testing.T) {
	// the action itself only records the ceiling - the ledger rescales and re-applies at Finalize
	l := apply(t, capTotalDiscount{}, cartOf(5000, 1), params(t, map[string]any{"max_cents": 1500}))
	if l.cappedAt == nil || *l.cappedAt != 1500 {
		t.Errorf("cappedAt = %v, want 1500", l.cappedAt)
	}
}

func TestCapTotalDiscountRejectsACeilingOfNothing(t *testing.T) {
	for _, max := range []int64{0, -1} {
		if err := (capTotalDiscount{}).ValidateParams(params(t, map[string]any{"max_cents": max})); err == nil {
			t.Errorf("max_cents=%d accepted, want a validation error", max)
		}
	}
}

func TestAddFreeGiftGrantsAtTheStatedQuantity(t *testing.T) {
	l := apply(t, addFreeGift{}, cartOf(5000, 1), params(t, map[string]any{"product_sku": "GIFT-1", "qty": 2}))
	if len(l.gifts) != 1 || l.gifts[0].ProductSKU != "GIFT-1" || l.gifts[0].Quantity != 2 {
		t.Errorf("gifts = %+v, want one GIFT-1 at qty 2", l.gifts)
	}
}

func TestAddFreeGiftRejectsARuleThatCouldGrantNothing(t *testing.T) {
	cases := []map[string]any{
		{"product_sku": "", "qty": 1},
		{"product_sku": "GIFT-1", "qty": 0},
		{"product_sku": "GIFT-1", "qty": -1},
	}
	for _, c := range cases {
		if err := (addFreeGift{}).ValidateParams(params(t, c)); err == nil {
			t.Errorf("params %v accepted, want a validation error", c)
		}
	}
}

func TestAddFreeGiftChoiceOffersEveryCandidateWithoutGrantingOne(t *testing.T) {
	l := apply(t, addFreeGiftChoice{}, cartOf(5000, 1), params(t, map[string]any{
		"product_skus": []string{"A", "B", "C"}, "max_picks": 1,
	}))
	if len(l.gifts) != 3 {
		t.Fatalf("queued %d gifts, want all three candidates visible", len(l.gifts))
	}
	for _, g := range l.gifts {
		// qty 0 is the OFFER - anything above it creates a line and commits stock the shopper never chose
		if g.Quantity != 0 {
			t.Errorf("candidate %s queued at qty %v, want 0 so nothing is committed", g.ProductSKU, g.Quantity)
		}
	}
}

func TestAddFreeGiftChoiceRejectsAnUnofferableRule(t *testing.T) {
	cases := []map[string]any{
		{"product_skus": []string{}, "max_picks": 1},
		{"product_skus": []string{"A"}, "max_picks": 0},
	}
	for _, c := range cases {
		if err := (addFreeGiftChoice{}).ValidateParams(params(t, c)); err == nil {
			t.Errorf("params %v accepted, want a validation error", c)
		}
	}
}

func TestFreeShippingAppliesToEveryMethodWhenNoneAreNamed(t *testing.T) {
	l := apply(t, freeShipping{}, cartOf(5000, 1), params(t, map[string]any{}))
	if !l.freeShipping || len(l.methods) != 0 {
		t.Errorf("freeShipping=%v methods=%v, want it on with no method restriction", l.freeShipping, l.methods)
	}
}

func TestFreeShippingCanBeLimitedToNamedMethods(t *testing.T) {
	l := apply(t, freeShipping{}, cartOf(5000, 1), params(t, map[string]any{"method_slugs": []string{"standard"}}))
	if !l.freeShipping || len(l.methods) != 1 || l.methods[0] != "standard" {
		t.Errorf("methods = %v, want just standard", l.methods)
	}
}

func TestActionsRefuseParamsTheyCannotDecode(t *testing.T) {
	// an operator-authored rule row reaches this straight from the database
	bad := json.RawMessage(`{"cents": "not a number"}`)
	for _, a := range []promotion.ActionApplier{fixedOffSubtotal{}, percentOffSubtotal{}, capTotalDiscount{}, addFreeGift{}, addFreeGiftChoice{}} {
		if err := a.ValidateParams(bad); err == nil {
			t.Errorf("%s accepted undecodable params", a.Slug())
		}
	}
}
