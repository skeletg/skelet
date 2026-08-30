// Package money is integer-cents arithmetic - quantities are float64 for fractional weight and length, so price times qty must round in float64, since a naive int64(qty) truncates sub-unit quantities to free
package money

import "math"

func LineTotal(unitPriceCents int64, qty float64) int64 {
	return int64(math.Round(float64(unitPriceCents) * qty))
}

func PerUnit(totalCents int64, qty float64) int64 {
	if qty <= 0 {
		return 0
	}
	return int64(math.Round(float64(totalCents) / qty))
}

// ScaleFraction takes the given share of an amount, rounded to the nearest cent
// and clamped to [0, amount]. Used to reverse only the unshipped part of a
// tender when a partially-shipped order is cancelled - a fraction outside 0..1
// (a lookup that failed open, say) can never hand back more than was taken.
func ScaleFraction(amountCents int64, fraction float64) int64 {
	if fraction >= 1 {
		return amountCents
	}
	if fraction <= 0 {
		return 0
	}
	return int64(math.Round(float64(amountCents) * fraction))
}
