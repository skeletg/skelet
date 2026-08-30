package order

import (
	"testing"

	"github.com/skeletg/skelet/internal/cart"
	"github.com/skeletg/skelet/internal/tax"
)

func TestAllocateLineTaxInputs(t *testing.T) {
	line := func(unit int64, qty float64) cart.CartLine {
		return cart.CartLine{UnitPrice: unit, Quantity: qty}
	}

	cases := []struct {
		name        string
		lines       []cart.CartLine
		subtotal    int64
		discount    int64
		classByLine []string
		want        []tax.LineTaxInput
	}{
		{
			name:        "single line, no discount, carries tax class",
			lines:       []cart.CartLine{line(1000, 1)},
			subtotal:    1000,
			discount:    0,
			classByLine: []string{"standard"},
			want:        []tax.LineTaxInput{{ProductTaxClass: "standard", TaxableCents: 1000}},
		},
		{
			name:        "two lines, clean proportional split",
			lines:       []cart.CartLine{line(1000, 1), line(3000, 1)},
			subtotal:    4000,
			discount:    400,
			classByLine: []string{"", ""},
			want:        []tax.LineTaxInput{{TaxableCents: 900}, {TaxableCents: 2700}},
		},
		{
			name:        "rounding remainder lands on the last taxable line",
			lines:       []cart.CartLine{line(333, 1), line(333, 1), line(334, 1)},
			subtotal:    1000,
			discount:    100,
			classByLine: []string{"", "", ""},
			// 33 + 33 + 33 allocated = 99; the 1-cent remainder shrinks line 2's
			// taxable from 301 to 300
			want: []tax.LineTaxInput{{TaxableCents: 300}, {TaxableCents: 300}, {TaxableCents: 300}},
		},
		{
			name:        "full discount floors every taxable at zero",
			lines:       []cart.CartLine{line(100, 1), line(1000, 1)},
			subtotal:    1100,
			discount:    1100,
			classByLine: []string{"", ""},
			want:        []tax.LineTaxInput{{TaxableCents: 0}, {TaxableCents: 0}},
		},
		{
			name:        "zero subtotal does not divide",
			lines:       []cart.CartLine{line(0, 1)},
			subtotal:    0,
			discount:    0,
			classByLine: []string{""},
			want:        []tax.LineTaxInput{{TaxableCents: 0}},
		},
		{
			name:        "free line skipped, discount falls on the priced line",
			lines:       []cart.CartLine{line(0, 1), line(1000, 1)},
			subtotal:    1000,
			discount:    100,
			classByLine: []string{"", ""},
			want:        []tax.LineTaxInput{{TaxableCents: 0}, {TaxableCents: 900}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := allocateLineTaxInputs(tc.lines, tc.subtotal, tc.discount, tc.classByLine)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("line %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
