package signalsgrpc

import (
	"math"
	"testing"
)

func TestTopNClampsToTheCeiling(t *testing.T) {
	for _, tc := range []struct{ req, want int32 }{{0, 8}, {-3, 8}, {5, 5}, {12, 12}, {30, 12}} {
		if got := topN(tc.req, 8, 12); got != tc.want {
			t.Errorf("topN(%d, 8, 12) = %d, want %d", tc.req, got, tc.want)
		}
	}
}

func TestCount32SaturatesInsteadOfWrapping(t *testing.T) {
	if got := count32(int64(3_000_000_000)); got != math.MaxInt32 {
		t.Errorf("count32(3e9) = %d, want the int32 ceiling", got)
	}
	if got := count32(int64(-3_000_000_000)); got != math.MinInt32 {
		t.Errorf("count32(-3e9) = %d, want the int32 floor", got)
	}
	if got := count32(42); got != 42 {
		t.Errorf("count32(42) = %d", got)
	}
}
