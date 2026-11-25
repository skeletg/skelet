package money

import (
	"math"
	"math/bits"
)

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

// Spread shares cents over the weights in proportion to them: none takes more than its own weight, and the shares add
// up to cents exactly - or to all the weights hold, when that is less. Floor division leaves a few cents over, which go
// to the last weights with room. A negative weight counts as none. The product is taken in 128 bits, so an order in a
// currency of small units doesn't overflow where cents * weight would
func Spread(weights []int64, cents int64) []int64 {
	out := make([]int64, len(weights))
	var sum uint64
	for _, w := range weights {
		if w > 0 {
			sum += uint64(w)
		}
	}
	if sum == 0 || cents <= 0 {
		return out
	}
	total := min(uint64(cents), sum)
	var given uint64
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		hi, lo := bits.Mul64(total, uint64(w))
		q, _ := bits.Div64(hi, lo, sum) // total <= sum, so the quotient is at most w
		out[i] = int64(q)
		given += q
	}
	for i := len(weights) - 1; i >= 0 && given < total; i-- {
		if weights[i] <= 0 {
			continue
		}
		take := min(total-given, uint64(weights[i]-out[i]))
		out[i] += int64(take)
		given += take
	}
	return out
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

func toCents(f float64) int64 {
	if !finite(f) {
		return 0
	}
	return int64(math.Round(f))
}
