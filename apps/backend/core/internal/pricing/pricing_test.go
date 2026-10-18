package pricing

import (
	"testing"
	"time"
)

func i64(v int64) *int64 { return &v }

func TestResolve(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	before := now.Add(-time.Hour)
	after := now.Add(time.Hour)

	tests := []struct {
		name        string
		in          Inputs
		wantPrice   int64
		wantCompare *int64
		wantOnSale  bool
	}{
		{"base only", Inputs{BasePrice: 1000}, 1000, nil, false},
		{"active special", Inputs{BasePrice: 1000, Special: i64(800), SpecialFrom: &before, SpecialTo: &after}, 800, i64(1000), true},
		{"unscheduled special is always on", Inputs{BasePrice: 1000, Special: i64(700)}, 700, i64(1000), true},
		{"special not started", Inputs{BasePrice: 1000, Special: i64(800), SpecialFrom: &after}, 1000, nil, false},
		{"special expired", Inputs{BasePrice: 1000, Special: i64(800), SpecialTo: &before}, 1000, nil, false},
		{"special >= base ignored", Inputs{BasePrice: 1000, Special: i64(1200)}, 1000, nil, false},
		{"compare-at alone", Inputs{BasePrice: 1000, CompareAt: i64(1500)}, 1000, i64(1500), false},
		{"compare-at below price dropped", Inputs{BasePrice: 1000, CompareAt: i64(900)}, 1000, nil, false},
		{"compare-at wins over implied base strike", Inputs{BasePrice: 1000, Special: i64(800), CompareAt: i64(1500)}, 800, i64(1500), true},
		{"compare-at below special price falls back to base strike", Inputs{BasePrice: 1000, Special: i64(800), CompareAt: i64(750)}, 800, i64(1000), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.in, now)
			if got.Price != tt.wantPrice {
				t.Errorf("price = %d, want %d", got.Price, tt.wantPrice)
			}
			if (got.CompareAt == nil) != (tt.wantCompare == nil) {
				t.Fatalf("compareAt = %v, want %v", got.CompareAt, tt.wantCompare)
			}
			if got.CompareAt != nil && *got.CompareAt != *tt.wantCompare {
				t.Errorf("compareAt = %d, want %d", *got.CompareAt, *tt.wantCompare)
			}
			if got.OnSale != tt.wantOnSale {
				t.Errorf("onSale = %v, want %v", got.OnSale, tt.wantOnSale)
			}
		})
	}
}

func TestSpecialActiveBounds(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	sp := i64(1)
	if !SpecialActive(sp, nil, nil, now) {
		t.Error("unbounded special should be active")
	}
	from := now
	if !SpecialActive(sp, &from, nil, now) {
		t.Error("special starting exactly now should be active (inclusive from)")
	}
	to := now
	if SpecialActive(sp, nil, &to, now) {
		t.Error("special ending exactly now should be inactive (exclusive to)")
	}
}

func TestApplyTiers(t *testing.T) {
	tiers := []Tier{{MinQty: 5, Price: 900}, {MinQty: 10, Price: 800}, {MinQty: 2, Price: 1100}}
	for _, tc := range []struct {
		unit int64
		qty  float64
		want int64
	}{
		{1000, 1, 1000},  // below every break
		{1000, 3, 1000},  // a tier above the unit price never raises it
		{1000, 5, 900},   // MinQty is inclusive
		{1000, 12, 800},  // the lowest reached tier wins
		{750, 12, 750},   // a unit already under every tier stays
		{1000, 9.5, 900}, // fractional quantities
	} {
		if got := ApplyTiers(tc.unit, tiers, tc.qty); got != tc.want {
			t.Errorf("ApplyTiers(%d, qty %v) = %d, want %d", tc.unit, tc.qty, got, tc.want)
		}
	}
	if got := ApplyTiers(1000, nil, 50); got != 1000 {
		t.Errorf("no tiers: got %d", got)
	}
}
