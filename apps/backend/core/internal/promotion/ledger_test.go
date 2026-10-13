package promotion

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Per-line adjustments must sum to exactly the aggregate discount - the two feed per-line refunds and EU VAT, so they can never drift apart
// The cap does not force that on its own: it rescales what's already line-assigned and leaves the cart-level share for Finalize to spread, so the invariant is asserted after Finalize
func TestFinalizeKeepsLineSumEqualToAggregate(t *testing.T) {
	l := newLedger()
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	lines := []CartLineView{
		{LineID: ids[0], UnitPriceCents: 1000, Quantity: 1},
		{LineID: ids[1], UnitPriceCents: 1000, Quantity: 1},
		{LineID: ids[2], UnitPriceCents: 1000, Quantity: 1},
	}
	// 1000 assigned to specific lines, 1000 cart-level - 2000 total, capped to 990
	l.AddLineAdjustment(ids[0], 300)
	l.AddLineAdjustment(ids[1], 300)
	l.AddLineAdjustment(ids[2], 400)
	l.AddSubtotalDiscount(1000)
	l.CapSubtotalDiscount(990)
	l.Finalize(lines, 3000)

	if got := l.SubtotalDiscountCents(); got != 990 {
		t.Fatalf("subtotal discount = %d, want 990", got)
	}
	var sum int64
	for _, c := range l.LineAdjustments() {
		sum += c
	}
	if sum != 990 {
		t.Errorf("per-line adjustments sum = %d, want 990 (must equal the aggregate)", sum)
	}
	// Unspread, the 1000c cart-level share lands entirely on whichever line sorts last, so guard that no line runs away
	// with it - equal-sized lines should stay in the same ballpark as each other
	for id, c := range l.LineAdjustments() {
		if c > 500 {
			t.Errorf("line %s took %d of 990 - the cart-level share is being dumped, not distributed", id, c)
		}
	}
}

// A granted free gift is a zero-priced line, and sorting it last makes it absorb the rounding shortfall, which prices it below nothing, and plugins reading LineAdjustment saw a negative unit
func TestFinalizeKeepsTheShortfallOffZeroPricedLines(t *testing.T) {
	l := newLedger()
	paidA, paidB, gift := uuid.New(), uuid.New(), uuid.New()
	lines := []CartLineView{
		{LineID: paidA, UnitPriceCents: 3333, Quantity: 1},
		{LineID: paidB, UnitPriceCents: 3334, Quantity: 1},
		{LineID: gift, UnitPriceCents: 0, Quantity: 1},
	}
	// 1000 over 6667 leaves a 1c shortfall that floor division can't place
	l.AddSubtotalDiscount(1000)
	l.Finalize(lines, 6667)

	if got := l.LineAdjustment(gift); got != 0 {
		t.Errorf("free line took %d cents of discount, want 0 - its price would be -%d", got, got)
	}
	var sum int64
	for _, c := range l.LineAdjustments() {
		sum += c
	}
	if sum != 1000 {
		t.Errorf("per-line adjustments sum = %d, want 1000 (must still equal the aggregate)", sum)
	}
}

// Every line being free means there is nothing to discount at all - the shortfall has nowhere to go and must not be forced onto one
func TestFinalizeDropsTheShortfallWhenEveryLineIsFree(t *testing.T) {
	l := newLedger()
	a, b := uuid.New(), uuid.New()
	lines := []CartLineView{
		{LineID: a, UnitPriceCents: 0, Quantity: 1},
		{LineID: b, UnitPriceCents: 0, Quantity: 1},
	}
	l.AddSubtotalDiscount(500)
	l.Finalize(lines, 0)

	for id, c := range l.LineAdjustments() {
		if c != 0 {
			t.Errorf("free line %s took %d cents of discount, want 0", id, c)
		}
	}
}

// A cap recorded by core.cap_total_discount has to survive rules that discount AFTER it, otherwise the operator's ceiling depends on priority ties broken by rule uuid
func TestFinalizeReappliesTheCap(t *testing.T) {
	l := newLedger()
	line := uuid.New()
	lines := []CartLineView{{LineID: line, UnitPriceCents: 10000, Quantity: 1}}

	l.AddSubtotalDiscount(2000)
	l.CapSubtotalDiscount(2500) // not binding yet
	l.AddSubtotalDiscount(3000) // a later rule pushes past the ceiling
	l.Finalize(lines, 10000)

	if got := l.SubtotalDiscountCents(); got != 2500 {
		t.Errorf("subtotal discount = %d, want 2500 (the cap must re-apply after every rule ran)", got)
	}
}

// The tightest of several caps wins
func TestFinalizeUsesTightestCap(t *testing.T) {
	l := newLedger()
	l.AddSubtotalDiscount(9000)
	l.CapSubtotalDiscount(5000)
	l.CapSubtotalDiscount(3000)
	l.CapSubtotalDiscount(7000)
	l.Finalize([]CartLineView{{LineID: uuid.New(), UnitPriceCents: 10000, Quantity: 1}}, 10000)

	if got := l.SubtotalDiscountCents(); got != 3000 {
		t.Errorf("subtotal discount = %d, want 3000 (tightest cap)", got)
	}
}

// Stacked percent rules each compute off the gross subtotal, so without a clamp the aggregate can exceed what the cart is worth - the order total floors at zero but discount_amount, the tax base and per-line refunds are all computed from the raw number
func TestFinalizeClampsToSubtotal(t *testing.T) {
	l := newLedger()
	line := uuid.New()
	lines := []CartLineView{{LineID: line, UnitPriceCents: 10000, Quantity: 1}}

	l.AddSubtotalDiscount(6000)
	l.AddSubtotalDiscount(7000)
	l.Finalize(lines, 10000)

	if got := l.SubtotalDiscountCents(); got != 10000 {
		t.Errorf("subtotal discount = %d, want 10000 (clamped to the cart subtotal)", got)
	}
	if got := l.LineAdjustment(line); got != 10000 {
		t.Errorf("line adjustment = %d, want 10000", got)
	}
}

// Simulations and unit tests build an environment from lines alone - the clamp falls back to what those lines are worth
func TestFinalizeClampFallsBackToLineTotals(t *testing.T) {
	l := newLedger()
	line := uuid.New()
	l.AddSubtotalDiscount(9999)
	l.Finalize([]CartLineView{{LineID: line, UnitPriceCents: 250, Quantity: 2}}, 0)

	if got := l.SubtotalDiscountCents(); got != 500 {
		t.Errorf("subtotal discount = %d, want 500 (2 x 250)", got)
	}
}

// A candidate gift is queued at qty 0 by core.add_free_gift_choice - dropping it made that action a silent no-op
func TestAddFreeGiftKeepsZeroQtyCandidates(t *testing.T) {
	l := newLedger()
	l.AddFreeGift("GIFT-A", 0)
	l.AddFreeGift("GIFT-B", 2)
	l.AddFreeGift("", 1)
	l.AddFreeGift("GIFT-C", -1)

	got := l.FreeGifts()
	if len(got) != 2 {
		t.Fatalf("free gifts = %+v, want the qty-0 candidate and the qty-2 grant only", got)
	}
	if got[0].ProductSKU != "GIFT-A" || got[0].Quantity != 0 {
		t.Errorf("free gifts[0] = %+v, want GIFT-A qty 0", got[0])
	}
}

// Two 60% rules on one $100 line used to net it at -$20 while the cart total stayed positive
func TestNoLineIsDiscountedBelowZero(t *testing.T) {
	l := newLedger()
	a, b, stale := uuid.New(), uuid.New(), uuid.New()
	lines := []CartLineView{{LineID: a, UnitPriceCents: 10000, Quantity: 1}, {LineID: b, UnitPriceCents: 10000, Quantity: 1}}
	l.AddLineAdjustment(a, 6000)
	l.AddLineAdjustment(a, 6000)
	l.AddLineAdjustment(stale, 500) // a remote provider naming a line the cart doesn't hold
	l.Finalize(lines, 20000)
	if got := l.LineAdjustment(a); got != 10000 {
		t.Errorf("line A carries %d, want its own total 10000", got)
	}
	if got := l.LineAdjustment(stale); got != 0 {
		t.Errorf("a line not in the cart carries %d", got)
	}
	if got := l.SubtotalDiscountCents(); got != 10000 {
		t.Errorf("cart discount %d, want 10000 - the excess and the stale line come off it", got)
	}
}

// The cart-level share goes where there is room left - spreading it by gross size put half on a line already discounted to zero
func TestCartLevelDiscountAvoidsFullyDiscountedLines(t *testing.T) {
	l := newLedger()
	a, b := uuid.New(), uuid.New()
	lines := []CartLineView{{LineID: a, UnitPriceCents: 10000, Quantity: 1}, {LineID: b, UnitPriceCents: 10000, Quantity: 1}}
	l.AddLineAdjustment(a, 10000)
	l.AddSubtotalDiscount(2000)
	l.Finalize(lines, 20000)
	if got := l.LineAdjustment(a); got != 10000 {
		t.Errorf("line A carries %d, want 10000", got)
	}
	if got := l.LineAdjustment(b); got != 2000 {
		t.Errorf("line B carries %d, want the whole cart-level 2000", got)
	}
}

type evictingRepo struct {
	Repository
	cache *rulesCache
}

func (r evictingRepo) ListActiveRulesForChannel(context.Context, *uuid.UUID) ([]Rule, error) {
	r.cache.EvictAll() // an admin edit landing while the load is in flight
	return []Rule{{ID: uuid.New(), Slug: "pre-edit"}}, nil
}

// A load that raced an eviction must not store what it read, or the pre-edit rules price carts for a full TTL
func TestALoadRacingAnEvictionIsNotCached(t *testing.T) {
	c := newRulesCache(time.Minute)
	if _, err := loadCachedActiveRules(context.Background(), c, evictingRepo{cache: c}, nil); err != nil {
		t.Fatal(err)
	}
	if _, hit := c.load(nil); hit {
		t.Error("rules read before an eviction were cached after it")
	}
}
