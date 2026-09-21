package actions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	// Registers core.cart_subtotal_range, used as the matching condition
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion/conditions"
)

// The "off matching items" discounts must compute on the fractional line quantity through money.LineTotal rather than truncating with int64(qty), which previously discounted only 2 of 2.5 units and short-changed the customer on every fractional line
func TestMatchingDiscountsUseFractionalQuantity(t *testing.T) {
	lineID := uuid.New()
	env := promotion.BuildEnvironment(promotion.EnvironmentInput{
		Currency: "USD",
		Lines: []promotion.CartLineView{{
			LineID:         lineID,
			UnitPriceCents: 1000,
			Quantity:       2.5,
		}},
	})

	t.Run("percent_off_matching", func(t *testing.T) {
		// 50% off 2.5 units at 1000 is LineTotal(1000, 2.5)=2500, so 1250 - pre-fix int64(2.5)=2 gave gross 2000 and a discount of 1000
		params, err := json.Marshal(map[string]any{"bps": 5000, "condition": matchAll})
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		ledger := newRecordingLedger()
		if err := (percentOffMatching{}).Apply(context.Background(), env, params, ledger, promotion.AppliedRuleContext{}); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if got := ledger.LineAdjustment(lineID); got != 1250 {
			t.Errorf("percent discount = %d, want 1250", got)
		}
	})

	t.Run("fixed_off_matching", func(t *testing.T) {
		// 200c/unit off 2.5 units is LineTotal(200, 2.5)=500 - pre-fix 200*int64(2.5)=400
		params, err := json.Marshal(map[string]any{"cents_per_unit": 200, "condition": matchAll})
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		ledger := newRecordingLedger()
		if err := (fixedOffMatching{}).Apply(context.Background(), env, params, ledger, promotion.AppliedRuleContext{}); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if got := ledger.LineAdjustment(lineID); got != 500 {
			t.Errorf("fixed discount = %d, want 500", got)
		}
	})
}

func init() { conditions.Register(promotion.DefaultRegistry) }
