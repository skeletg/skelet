package signalsgrpc

import "testing"

func TestTopNClampsToTheCeiling(t *testing.T) {
	for _, tc := range []struct{ req, want int32 }{{0, 8}, {-3, 8}, {5, 5}, {12, 12}, {30, 12}} {
		if got := topN(tc.req, 8, 12); got != tc.want {
			t.Errorf("topN(%d, 8, 12) = %d, want %d", tc.req, got, tc.want)
		}
	}
}
