package money

import "math"

// Integer-cents arithmetic - quantities are float64 for fractional weight and length, so price times qty must round in float64, since a naive int64(qty) truncates sub-unit quantities to free
// A non-finite input answers 0 everywhere here - int64(NaN) is the minimum int64 on amd64, a negative fortune
func LineTotal(unitPriceCents int64, qty float64) int64 {
	return toCents(float64(unitPriceCents) * qty)
}

func PerUnit(totalCents int64, qty float64) int64 {
	if qty <= 0 || !finite(qty) {
		return 0
	}
	return toCents(float64(totalCents) / qty)
}

// ScaleFraction takes the given share of an amount, rounded to the nearest cent and clamped to [0, amount]. Used to reverse only the unshipped part of a tender when a partially-shipped order is cancelled - a fraction outside 0..1 (a lookup that failed open, say) can never hand back more than was taken.
func ScaleFraction(amountCents int64, fraction float64) int64 {
	if math.IsNaN(fraction) {
		return 0
	}
	if fraction >= 1 {
		return amountCents
	}
	if fraction <= 0 {
		return 0
	}
	return int64(math.Round(float64(amountCents) * fraction))
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

func toCents(f float64) int64 {
	if !finite(f) {
		return 0
	}
	return int64(math.Round(f))
}
