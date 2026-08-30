package shipping

import "testing"

func TestComputePrice(t *testing.T) {
	tests := []struct {
		name     string
		method   Method
		subtotal int64
		want     int64
	}{
		{
			name:   "flat is constant",
			method: Method{CalcType: CalcFlat, FlatCents: 500},
			want:   500,
		},
		{
			name:   "free is always zero",
			method: Method{CalcType: CalcFree, FlatCents: 999},
		},
		{
			name:     "free_above above threshold → 0",
			method:   Method{CalcType: CalcFreeAbove, FlatCents: 500, FreeAboveCents: 5000},
			subtotal: 5000,
			want:     0,
		},
		{
			name:     "free_above below threshold → flat",
			method:   Method{CalcType: CalcFreeAbove, FlatCents: 500, FreeAboveCents: 5000},
			subtotal: 4999,
			want:     500,
		},
		{
			name:   "unknown calc_type falls back to flat",
			method: Method{CalcType: "weight_band", FlatCents: 700},
			want:   700,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputePrice(tc.method, tc.subtotal)
			if got != tc.want {
				t.Errorf("ComputePrice: got %d, want %d", got, tc.want)
			}
		})
	}
}
