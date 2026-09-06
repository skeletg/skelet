package effects_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/tax"
)

// Every case is a single rule with a single effect, so the ledger total IS that effect's output
func TestPercentageRoundsHalfUp(t *testing.T) {
	tests := []struct {
		name       string
		effectType string
		base       int64
		bps        int32
		inclusive  bool
		want       int64
	}{
		// The finding's case - 1999*2000/10000 = 399.8, truncation collected 399
		{name: "19.99 at 20% exclusive rounds up", effectType: "core.percentage", base: 1999, bps: 2000, want: 400},
		// Exactly .5, the boundary half-up has to break upward
		{name: "10.00 at 8.75% exclusive is exactly half", effectType: "core.percentage", base: 1000, bps: 875, want: 88},
		{name: "0.50 at 1% exclusive is exactly half", effectType: "core.percentage", base: 50, bps: 100, want: 1},
		// Just under .5 still rounds down
		{name: "19.99 at 8.75% exclusive rounds up", effectType: "core.percentage", base: 1999, bps: 875, want: 175},
		{name: "1.01 at 1% exclusive rounds down", effectType: "core.percentage", base: 101, bps: 100, want: 1},
		{name: "exact division is untouched", effectType: "core.percentage", base: 2500, bps: 2000, want: 500},
		// Inclusive extracts base*r/(10000+r) - 1999*2000/12000 = 333.166
		{name: "inclusive 20% on 19.99 rounds down", effectType: "core.percentage", base: 1999, bps: 2000, inclusive: true, want: 333},
		// 1500*2000/12000 = 250 exactly
		{name: "inclusive 20% on 15.00 is exact", effectType: "core.percentage", base: 1500, bps: 2000, inclusive: true, want: 250},
		// 900*2000/12000 = 150 exactly
		{name: "inclusive 20% on 9.00 is exact", effectType: "core.percentage", base: 900, bps: 2000, inclusive: true, want: 150},
		// 1000*500/10500 = 47.619
		{name: "inclusive 5% on 10.00 rounds down", effectType: "core.percentage", base: 1000, bps: 500, inclusive: true, want: 48},
		// Compound with no prior tax degenerates to a plain percentage on the base
		{name: "compound alone rounds half-up", effectType: "core.compound_percentage", base: 1000, bps: 875, want: 88},
		{name: "compound inclusive rounds half-up", effectType: "core.compound_percentage", base: 1999, bps: 2000, inclusive: true, want: 333},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := tax.NewService(roundingRepo{rules: []tax.Rule{{
				ID:       uuid.New(),
				Name:     tc.name,
				IsActive: true,
				Effects: []tax.Effect{{
					Type:   tc.effectType,
					Params: json.RawMessage(fmt.Sprintf(`{"bps":%d}`, tc.bps)),
				}},
			}}})
			res, err := svc.Compute(context.Background(), tax.ComputeInput{
				ChannelID:    uuid.New(),
				TaxableCents: tc.base,
				TaxInclusive: tc.inclusive,
			})
			if err != nil {
				t.Fatalf("Compute: %v", err)
			}
			if res.Cents != tc.want {
				t.Errorf("got %d cents, want %d", res.Cents, tc.want)
			}
		})
	}
}

// Rounding up per line is what makes per-line evaluation stop under-collecting - 20 lines of 19.99 at 20% owes 20*400, not 20*399
func TestPerLineRoundingSumsWithoutLeak(t *testing.T) {
	svc := tax.NewService(roundingRepo{rules: []tax.Rule{{
		ID: uuid.New(), Name: "vat", IsActive: true,
		Effects: []tax.Effect{{Type: "core.percentage", Params: json.RawMessage(`{"bps":2000}`)}},
	}}})
	lines := make([]tax.LineTaxInput, 20)
	for i := range lines {
		lines[i] = tax.LineTaxInput{TaxableCents: 1999}
	}
	res, err := svc.ComputeOrder(context.Background(), tax.ComputeOrderInput{ChannelID: uuid.New(), Lines: lines})
	if err != nil {
		t.Fatalf("ComputeOrder: %v", err)
	}
	if res.TotalCents != 8000 {
		t.Errorf("got %d, want 8000 (20 x 400)", res.TotalCents)
	}
}

// Embeds the interface so only the one read the engine makes has to exist
type roundingRepo struct {
	tax.Repository
	rules []tax.Rule
}

func (r roundingRepo) ListActiveRulesByChannel(context.Context, uuid.UUID) ([]tax.Rule, error) {
	return r.rules, nil
}
