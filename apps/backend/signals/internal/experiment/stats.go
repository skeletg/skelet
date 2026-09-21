package experiment

import "math"

// One variation's analysed result - raw counts plus a Wilson 95% CI on the conversion rate, relative uplift against control, and a one-sided chance-to-beat-control from a normal approximation of the rate difference
type VariantStat struct {
	Variation    string
	Name         string
	Exposed      int
	Converted    int
	RevenueCents int64   // cents from matched goal events (when the goal is a commerce event)
	Rate         float64 // conversion rate 0..1
	CILow        float64
	CIHigh       float64
	Uplift       float64 // relative vs control (0 for control / no data)
	ChanceToWin  float64 // P(variant > control), 0..1 (0 for control)
	IsControl    bool
	Significant  bool // 95% two-sided vs control (fixed-horizon)
	// The mSPRT always-valid p-value, which stays correct no matter how often the dashboard is refreshed mid-experiment, where fixed-horizon p-values inflate false positives under peeking
	// 1.0 until there's evidence, and the sequential winner requires below 0.05
	AlwaysValidP          float64
	SequentialSignificant bool
}

// An experiment's full result set - per-variation stats, a winner (a challenger that's significantly better, else ""), and a sample-ratio-mismatch flag, where the observed split not matching configured weights means broken randomization or tracking and the numbers can't be trusted
// SequentialWinner is peeking-safe at mSPRT p < 0.05 and is what operators should act on while an experiment RUNS; Winner keeps the fixed-horizon crowning for parity with pre-registered-horizon analyses, valid only when the sample size was decided up front
type Analysis struct {
	Variations          []VariantStat
	Winner              string
	SequentialWinner    string
	SampleRatioMismatch bool
	TotalExposed        int
	TotalRevenueCents   int64
}

// The standard-normal CDF
func normalCDF(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

// The Wilson score 95% interval for x successes in n trials
func wilson(x, n int) (lo, hi float64) {
	if n == 0 {
		return 0, 0
	}
	const z = 1.96
	p := float64(x) / float64(n)
	nn := float64(n)
	z2 := z * z
	denom := 1 + z2/nn
	center := (p + z2/(2*nn)) / denom
	margin := (z / denom) * math.Sqrt(p*(1-p)/nn+z2/(4*nn*nn))
	return math.Max(0, center-margin), math.Min(1, center+margin)
}

// The mSPRT always-valid p-value for a two-proportion comparison, following Howard/Johari-style Gaussian mixture sequential testing
// The z-statistic of the rate difference folds into a likelihood ratio with a mixture variance tau-squared, and 1/lambda is a valid p-value at EVERY sample size - so checking the dashboard daily or hourly never inflates the false-positive rate the way repeated fixed-horizon tests do
func alwaysValidP(vx, vn, cx, cn int) float64 {
	if vn == 0 || cn == 0 {
		return 1
	}
	vp, cp := rate(vx, vn), rate(cx, cn)
	// Pooled variance of the difference in rates
	varDiff := vp*(1-vp)/float64(vn) + cp*(1-cp)/float64(cn)
	if varDiff <= 0 {
		return 1
	}
	diff := vp - cp
	// tau-squared tunes sensitivity to effect size - 0.0001, or tau = 1pp on rates, is a reasonable prior scale for conversion deltas
	const tau2 = 0.0001
	v := varDiff
	lambda := math.Sqrt(v/(v+tau2)) * math.Exp(diff*diff*tau2/(2*v*(v+tau2)))
	if lambda <= 1 {
		return 1
	}
	p := 1 / lambda
	if p > 1 {
		return 1
	}
	return p
}

// Chi-squared critical values at p=0.001 for df 1..9, for the SRM check
var chi2Crit001 = []float64{10.828, 13.816, 16.266, 18.467, 20.515, 22.458, 24.322, 26.124, 27.877}

// Per-variation stats against the control at variations[0], plus the SRM check and the winner - raw maps a variation key to its exposed and converted counts
func Analyze(variations []Variation, raw map[string]VariationResult) Analysis {
	var out Analysis
	if len(variations) == 0 {
		return out
	}
	control := variations[0].Key
	cn := raw[control].Exposed
	cx := raw[control].Converted
	cp := rate(cx, cn)

	totalExposed, totalWeight := 0, 0
	for _, v := range variations {
		totalExposed += raw[v.Key].Exposed
		if v.Weight > 0 {
			totalWeight += v.Weight
		}
	}
	out.TotalExposed = totalExposed

	bestWin, winner := 0.975, "" // require ≥97.5% (z≥1.96, one-sided) to crown
	bestSeqP, seqWinner := 0.05, ""
	for _, v := range variations {
		r := raw[v.Key]
		lo, hi := wilson(r.Converted, r.Exposed)
		st := VariantStat{
			Variation: v.Key, Name: v.Name, Exposed: r.Exposed, Converted: r.Converted, RevenueCents: r.Revenue,
			Rate: rate(r.Converted, r.Exposed), CILow: lo, CIHigh: hi, IsControl: v.Key == control,
			AlwaysValidP: 1,
		}
		out.TotalRevenueCents += r.Revenue
		if v.Key != control {
			vp := st.Rate
			if cp > 0 {
				st.Uplift = (vp - cp) / cp
			}
			st.AlwaysValidP = alwaysValidP(r.Converted, r.Exposed, cx, cn)
			// Sequential crown - peeking-safe AND directionally better
			st.SequentialSignificant = st.AlwaysValidP < 0.05 && vp > cp
			if st.SequentialSignificant && st.AlwaysValidP < bestSeqP {
				bestSeqP = st.AlwaysValidP
				seqWinner = v.Key
			}
			se := math.Sqrt(vp*(1-vp)/safe(r.Exposed) + cp*(1-cp)/safe(cn))
			if se > 0 && r.Exposed > 0 && cn > 0 {
				z := (vp - cp) / se
				st.ChanceToWin = normalCDF(z)
				st.Significant = math.Abs(z) >= 1.96
				if st.Significant && st.ChanceToWin >= bestWin {
					bestWin = st.ChanceToWin
					winner = v.Key
				}
			} else if r.Exposed > 0 && cn > 0 {
				st.ChanceToWin = 0.5 // data exists but no separation (e.g. 0 conversions both arms)
			}
			// No exposures on one side means no evidence, so leave ChanceToWin 0
		}
		out.Variations = append(out.Variations, st)
	}
	out.Winner = winner
	out.SequentialWinner = seqWinner

	// SRM - a chi-squared goodness-of-fit of observed exposures against configured weights
	if totalExposed >= 100 && totalWeight > 0 && len(variations) >= 2 && len(variations) <= len(chi2Crit001)+1 {
		chi2 := 0.0
		ok := true
		for _, v := range variations {
			exp := float64(totalExposed) * float64(max0(v.Weight)) / float64(totalWeight)
			if exp <= 0 {
				ok = false
				break
			}
			d := float64(raw[v.Key].Exposed) - exp
			chi2 += d * d / exp
		}
		if ok && chi2 > chi2Crit001[len(variations)-2] {
			out.SampleRatioMismatch = true
		}
	}
	return out
}

func rate(x, n int) float64 {
	if n == 0 {
		return 0
	}
	return float64(x) / float64(n)
}
func safe(n int) float64 {
	if n == 0 {
		return 1
	}
	return float64(n)
}
func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
