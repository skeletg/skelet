package order

import (
	"slices"
	"testing"
)

func TestApplyShippingPromos(t *testing.T) {
	ships := []ShipmentInput{{MethodCode: "standard"}, {MethodCode: "express"}, {MethodCode: "standard"}}
	override := int64(250)
	cases := []struct {
		name        string
		free        bool
		freeMethods []string
		override    *int64
		want        []int64
	}{
		{"no promotion", false, nil, nil, []int64{500, 900, 300}},
		{"free shipping waives every package", true, nil, nil, []int64{0, 0, 0}},
		{"free standard leaves express charged", true, []string{"standard"}, nil, []int64{0, 900, 0}},
		{"an override is the whole order's, on the first package", false, nil, &override, []int64{250, 0, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := []int64{500, 900, 300}
			applyShippingPromos(got, ships, c.free, c.freeMethods, c.override)
			if !slices.Equal(got, c.want) {
				t.Fatalf("amounts = %v, want %v", got, c.want)
			}
		})
	}
}

// A group's packages are judged on the whole group's value less its share of the order's discount, so splitting it across warehouses keeps free shipping
func TestGroupSubtotal(t *testing.T) {
	if got := groupSubtotal(6000, 10000, 1000); got != 5400 {
		t.Errorf("groupSubtotal = %d, want 6000 less its 60%% share of the 1000 discount", got)
	}
	if got := groupSubtotal(100, 100, 500); got != 0 {
		t.Errorf("groupSubtotal = %d, want it floored at 0", got)
	}
}
