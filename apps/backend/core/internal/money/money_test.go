package money

import "testing"

func TestLineTotal(t *testing.T) {
	cases := []struct {
		name  string
		price int64
		qty   float64
		want  int64
	}{
		{"integer quantity", 1000, 3, 3000},
		{"fractional quantity", 1000, 2.5, 2500},
		{"sub-unit quantity not free", 1000, 0.5, 500},
		{"rounds half up", 999, 0.5, 500}, // 499.5 → 500
		{"rounds down below half", 333, 0.4, 133},
		{"zero quantity", 1000, 0, 0},
		{"zero price", 0, 2.5, 0},
		{"negative price (refund line)", -1000, 2.5, -2500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := LineTotal(c.price, c.qty); got != c.want {
				t.Errorf("LineTotal(%d, %v) = %d, want %d", c.price, c.qty, got, c.want)
			}
		})
	}
}

func TestPerUnit(t *testing.T) {
	cases := []struct {
		name  string
		total int64
		qty   float64
		want  int64
	}{
		{"integer split", 3000, 3, 1000},
		{"discounted basis rounds", 2900, 3, 967}, // 966.67 → 967
		{"single unit", 1499, 1, 1499},
		{"fractional quantity", 2500, 2.5, 1000},
		{"zero quantity is zero", 1000, 0, 0},
		{"negative quantity is zero", 1000, -1, 0},
		{"zero total", 0, 4, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PerUnit(c.total, c.qty); got != c.want {
				t.Errorf("PerUnit(%d, %v) = %d, want %d", c.total, c.qty, got, c.want)
			}
		})
	}
}
