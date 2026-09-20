package order

import "testing"

func i64(v int64) *int64 { return &v }

func TestOrderTotal(t *testing.T) {
	cases := []struct {
		name         string
		subtotal     int64
		discount     int64
		tax          int64
		shipping     int64
		taxInclusive bool
		override     *int64
		want         int64
	}{
		{name: "tax-exclusive adds tax + shipping", subtotal: 10000, discount: 0, tax: 825, shipping: 500, want: 11325},
		{name: "discount subtracts before tax", subtotal: 10000, discount: 2000, tax: 825, shipping: 500, want: 9325},
		{name: "tax-inclusive does not re-add tax", subtotal: 10000, discount: 0, tax: 1597, shipping: 500, taxInclusive: true, want: 10500},
		{name: "tax-inclusive still applies discount + shipping", subtotal: 10000, discount: 1000, tax: 1597, shipping: 500, taxInclusive: true, want: 9500},
		{name: "override wins over computed", subtotal: 10000, discount: 0, tax: 825, shipping: 500, override: i64(4242), want: 4242},
		{name: "override is also floored at zero", subtotal: 10000, discount: 0, tax: 0, shipping: 0, override: i64(-50), want: 0},
		{name: "computed floored at zero when discount exceeds", subtotal: 1000, discount: 5000, tax: 0, shipping: 0, want: 0},
		{name: "zero everything", want: 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := computeOrderTotal(c.subtotal, c.discount, c.tax, c.shipping, c.taxInclusive, c.override)
			if got != c.want {
				t.Errorf("orderTotal = %d, want %d", got, c.want)
			}
		})
	}
}
