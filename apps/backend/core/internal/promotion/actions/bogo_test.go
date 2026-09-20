package actions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	// Registers core.cart_subtotal_range (used as the target_condition)
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion/conditions"
)

// recordingLedger is a promotion.Ledger that records per-line adjustments so tests can assert the discount bogo
// applies. bogo only calls AddLineAdjustment; every other method is a no-op
type recordingLedger struct {
	lines map[uuid.UUID]int64
}

func newRecordingLedger() *recordingLedger {
	return &recordingLedger{lines: map[uuid.UUID]int64{}}
}

func (l *recordingLedger) AddLineAdjustment(lineID uuid.UUID, cents int64) { l.lines[lineID] += cents }
func (l *recordingLedger) LineAdjustment(lineID uuid.UUID) int64           { return l.lines[lineID] }
func (l *recordingLedger) LineAdjustments() map[uuid.UUID]int64            { return l.lines }
func (l *recordingLedger) AddSubtotalDiscount(int64)                       {}
func (l *recordingLedger) SetFreeShipping([]string)                        {}
func (l *recordingLedger) SetShippingOverride(int64)                       {}
func (l *recordingLedger) AddFreeGift(string, float64)                     {}
func (l *recordingLedger) CapSubtotalDiscount(int64)                       {}
func (l *recordingLedger) SubtotalDiscountCents() int64                    { return 0 }
func (l *recordingLedger) FreeShipping() bool                              { return false }
func (l *recordingLedger) FreeShippingMethods() []string                   { return nil }
func (l *recordingLedger) ShippingOverride() *int64                        { return nil }
func (l *recordingLedger) FreeGifts() []promotion.FreeGift                 { return nil }

// matchAll is a target_condition that matches any line with a non-negative
// subtotal - i.e. every line in these tests
var matchAll = json.RawMessage(`{"type":"core.cart_subtotal_range","params":{"min_cents":0}}`)

func TestBogoApply(t *testing.T) {
	tests := []struct {
		name           string
		buyQty, getQty float64
		getKind        string
		value          int32
		unitPriceCents int64
		quantity       float64
		wantDiscount   int64
	}{
		{
			name:   "free: every 2nd unit free",
			buyQty: 1, getQty: 1, getKind: "free",
			unitPriceCents: 1000, quantity: 2,
			wantDiscount: 1000, // one cluster, one free unit @ 1000
		},
		{
			name:   "percent: 50% off the get-unit",
			buyQty: 1, getQty: 1, getKind: "percent", value: 5000, // bps
			unitPriceCents: 1000, quantity: 2,
			wantDiscount: 500,
		},
		{
			name:   "fixed: 300c off the get-unit",
			buyQty: 1, getQty: 1, getKind: "fixed", value: 300,
			unitPriceCents: 1000, quantity: 2,
			wantDiscount: 300,
		},
		{
			name:   "fixed: clamped to unit price",
			buyQty: 1, getQty: 1, getKind: "fixed", value: 2000,
			unitPriceCents: 1000, quantity: 2,
			wantDiscount: 1000,
		},
		{
			name:   "no discount below one cluster",
			buyQty: 2, getQty: 1, getKind: "free",
			unitPriceCents: 1000, quantity: 1,
			wantDiscount: 0,
		},
		{
			// Rounding guard: discountedUnits is fractional (0.5), so the per-unit total must round half away from zero via
			// money.LineTotal, not truncate - Pre-fix this computed int64(101*0.5) = 50 instead of 51
			name:   "fractional get-quantity rounds, not truncates",
			buyQty: 1, getQty: 0.5, getKind: "free",
			unitPriceCents: 101, quantity: 1.5,
			wantDiscount: 51,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lineID := uuid.New()
			env := promotion.BuildEnvironment(promotion.EnvironmentInput{
				Currency: "USD",
				Lines: []promotion.CartLineView{{
					LineID:         lineID,
					UnitPriceCents: tt.unitPriceCents,
					Quantity:       tt.quantity,
				}},
			})
			params, err := json.Marshal(map[string]any{
				"buy_qty":          tt.buyQty,
				"get_qty":          tt.getQty,
				"get_kind":         tt.getKind,
				"value":            tt.value,
				"target_condition": matchAll,
			})
			if err != nil {
				t.Fatalf("marshal params: %v", err)
			}
			ledger := newRecordingLedger()
			if err := (bogo{}).Apply(context.Background(), env, params, ledger, promotion.AppliedRuleContext{}); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if got := ledger.LineAdjustment(lineID); got != tt.wantDiscount {
				t.Errorf("discount = %d, want %d", got, tt.wantDiscount)
			}
		})
	}
}

// Eligible units pool across every matching line, so units spread thin still earn
// the discount - deciding per line gave nothing to a cart holding one of each widget
func TestBogoPoolsUnitsAcrossLines(t *testing.T) {
	params, err := json.Marshal(map[string]any{
		"buy_qty": 1.0, "get_qty": 1.0, "get_kind": "free",
		"target_condition": matchAll,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	a, b := uuid.New(), uuid.New()
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		Currency: "USD",
		Lines: []promotion.CartLineView{
			{LineID: a, ProductSKU: "WIDGET-A", UnitPriceCents: 1000, Quantity: 1},
			{LineID: b, ProductSKU: "WIDGET-B", UnitPriceCents: 1000, Quantity: 1},
		},
	})
	ledger := newRecordingLedger()
	if err := (bogo{}).Apply(context.Background(), env, params, ledger, promotion.AppliedRuleContext{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	var total int64
	for _, c := range ledger.LineAdjustments() {
		total += c
	}
	if total != 1000 {
		t.Errorf("pooled discount = %d, want 1000 (two eligible units = one free)", total)
	}
	// Cart order decides which unit is free, so the first line takes it
	if got := ledger.LineAdjustment(a); got != 1000 {
		t.Errorf("line A adjustment = %d, want 1000 (units are spent in cart order)", got)
	}
}

// A trailing cluster whose buy leg is satisfied earns the units it can, which is
// what carry-over machinery would aim at and miss
func TestBogoPartialTrailingCluster(t *testing.T) {
	params, err := json.Marshal(map[string]any{
		"buy_qty": 1.0, "get_qty": 2.0, "get_kind": "free",
		"target_condition": matchAll,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	line := uuid.New()
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		Currency: "USD",
		Lines:    []promotion.CartLineView{{LineID: line, UnitPriceCents: 1000, Quantity: 5}},
	})
	ledger := newRecordingLedger()
	if err := (bogo{}).Apply(context.Background(), env, params, ledger, promotion.AppliedRuleContext{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// buy 1 get 2 covers units 1-3, then unit 4 buys and unit 5 comes free
	if got := ledger.LineAdjustment(line); got != 3000 {
		t.Errorf("discount = %d, want 3000 (3 free units of 5)", got)
	}
}

// The per-line projection a matching condition sees must carry the whole
// environment - dropping segments made a segment-gated matching rule unsatisfiable
func TestMatchingConditionSeesCustomerSegments(t *testing.T) {
	line := uuid.New()
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		Currency:             "USD",
		CustomerSegmentSlugs: []string{"vip"},
		Lines:                []promotion.CartLineView{{LineID: line, ProductSKU: "W", UnitPriceCents: 1000, Quantity: 1}},
	})
	params, err := json.Marshal(map[string]any{
		"bps":       1000,
		"condition": json.RawMessage(`{"type":"core.customer_in_segment","params":{"slugs":["vip"]}}`),
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	ledger := newRecordingLedger()
	if err := (percentOffMatching{}).Apply(context.Background(), env, params, ledger, promotion.AppliedRuleContext{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := ledger.LineAdjustment(line); got != 100 {
		t.Errorf("line adjustment = %d, want 100 (the vip segment must reach the nested condition)", got)
	}
}

func init() { conditions.Register(promotion.DefaultRegistry) }
