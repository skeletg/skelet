package experiment

import "testing"

func vars2() []Variation {
	return []Variation{
		{Key: "control", Name: "Control", Weight: 1},
		{Key: "variant", Name: "Variant", Weight: 1},
	}
}

func TestAnalyze(t *testing.T) {
	t.Run("clear winner is significant and crowned", func(t *testing.T) {
		a := Analyze(vars2(), map[string]VariationResult{
			"control": {Variation: "control", Exposed: 1000, Converted: 100}, // 10%
			"variant": {Variation: "variant", Exposed: 1000, Converted: 150}, // 15%
		})
		if a.Winner != "variant" {
			t.Fatalf("winner = %q, want variant", a.Winner)
		}
		v := a.Variations[1]
		if !v.Significant {
			t.Errorf("variant should be significant")
		}
		if v.ChanceToWin < 0.99 {
			t.Errorf("chanceToWin = %.4f, want ≥0.99", v.ChanceToWin)
		}
		if v.Uplift < 0.49 || v.Uplift > 0.51 {
			t.Errorf("uplift = %.4f, want ≈0.50", v.Uplift)
		}
		if a.Variations[0].IsControl != true || a.Variations[1].IsControl != false {
			t.Errorf("control flag wrong: %+v", a.Variations)
		}
		if a.SampleRatioMismatch {
			t.Errorf("balanced split should not flag SRM")
		}
	})

	t.Run("small sample is not significant, no winner", func(t *testing.T) {
		a := Analyze(vars2(), map[string]VariationResult{
			"control": {Variation: "control", Exposed: 50, Converted: 5},
			"variant": {Variation: "variant", Exposed: 50, Converted: 6},
		})
		if a.Winner != "" {
			t.Errorf("winner = %q, want empty (underpowered)", a.Winner)
		}
		if a.Variations[1].Significant {
			t.Errorf("variant should not be significant at n=50")
		}
	})

	t.Run("lopsided split flags SRM", func(t *testing.T) {
		a := Analyze(vars2(), map[string]VariationResult{
			"control": {Variation: "control", Exposed: 900, Converted: 90},
			"variant": {Variation: "variant", Exposed: 100, Converted: 15},
		})
		if !a.SampleRatioMismatch {
			t.Errorf("900/100 on equal weights should flag SRM")
		}
	})

	t.Run("zero exposures is safe", func(t *testing.T) {
		a := Analyze(vars2(), map[string]VariationResult{})
		if a.Winner != "" || a.SampleRatioMismatch || a.TotalExposed != 0 {
			t.Errorf("empty analysis dirty: %+v", a)
		}
		for _, v := range a.Variations {
			if v.Rate != 0 || v.ChanceToWin != 0 {
				t.Errorf("zero-data variant has nonzero stats: %+v", v)
			}
		}
	})

	t.Run("Wilson interval brackets the rate", func(t *testing.T) {
		a := Analyze(vars2(), map[string]VariationResult{
			"control": {Variation: "control", Exposed: 1000, Converted: 100},
			"variant": {Variation: "variant", Exposed: 1000, Converted: 150},
		})
		c := a.Variations[0]
		if !(c.CILow < c.Rate && c.Rate < c.CIHigh) {
			t.Errorf("CI [%.4f,%.4f] does not bracket rate %.4f", c.CILow, c.CIHigh, c.Rate)
		}
		if c.CILow < 0 || c.CIHigh > 1 {
			t.Errorf("CI out of [0,1]: [%.4f,%.4f]", c.CILow, c.CIHigh)
		}
	})
}

// Sequential (mSPRT) always-valid inference: a real effect must eventually cross p<0.05, no-effect data must NOT, and
// the p-value must be monotone enough to act as a stopping rule under continuous peeking
func TestSequentialAlwaysValidP(t *testing.T) {
	vars := []Variation{{Key: "control", Weight: 50}, {Key: "b", Weight: 50}}

	// Strong true effect: 5% vs 8% at n=8000 → sequentially significant
	a := Analyze(vars, map[string]VariationResult{
		"control": {Exposed: 8000, Converted: 400},
		"b":       {Exposed: 8000, Converted: 640},
	})
	var b VariantStat
	for _, v := range a.Variations {
		if v.Variation == "b" {
			b = v
		}
	}
	if !b.SequentialSignificant || b.AlwaysValidP >= 0.05 {
		t.Fatalf("true effect not detected: p=%v", b.AlwaysValidP)
	}
	if a.SequentialWinner != "b" {
		t.Fatalf("sequential winner = %q, want b", a.SequentialWinner)
	}

	// Null effect: identical rates stay non-significant even at large n
	a = Analyze(vars, map[string]VariationResult{
		"control": {Exposed: 50000, Converted: 2500},
		"b":       {Exposed: 50000, Converted: 2505},
	})
	for _, v := range a.Variations {
		if v.Variation == "b" && (v.SequentialSignificant || v.AlwaysValidP < 0.05) {
			t.Fatalf("null effect flagged significant: p=%v", v.AlwaysValidP)
		}
	}
	if a.SequentialWinner != "" {
		t.Fatalf("null data crowned %q", a.SequentialWinner)
	}

	// A WORSE variant must never be sequentially crowned even if separated
	a = Analyze(vars, map[string]VariationResult{
		"control": {Exposed: 8000, Converted: 640},
		"b":       {Exposed: 8000, Converted: 400},
	})
	for _, v := range a.Variations {
		if v.Variation == "b" && v.SequentialSignificant {
			t.Fatal("worse variant marked sequentially significant")
		}
	}

	// No data → p stays 1
	a = Analyze(vars, map[string]VariationResult{})
	for _, v := range a.Variations {
		if v.AlwaysValidP != 1 {
			t.Fatalf("empty data p=%v, want 1", v.AlwaysValidP)
		}
	}
}
