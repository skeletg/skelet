package shipping

import (
	"testing"

	"github.com/google/uuid"
)

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
			got := ComputePrice(tc.method, tc.subtotal, nil)
			if got != tc.want {
				t.Errorf("ComputePrice: got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestComputePriceByProductType(t *testing.T) {
	shirt, fridge := uuid.New(), uuid.New()
	items := func(shirts, fridges float64) []PricedItem {
		var out []PricedItem
		if shirts > 0 {
			out = append(out, PricedItem{ProductTypeID: shirt, Quantity: shirts})
		}
		if fridges > 0 {
			out = append(out, PricedItem{ProductTypeID: fridge, Quantity: fridges})
		}
		return out
	}
	bundle := Method{CalcType: CalcFlat, FlatCents: 500, UnitsPerCharge: 5}
	fridgeRate := TypeRate{ProductTypeID: fridge, PriceCents: 3000, UnitsPerCharge: 1}
	freeAbove := Method{CalcType: CalcFreeAbove, FlatCents: 500, FreeAboveCents: 10000, TypeRates: []TypeRate{fridgeRate}}
	eligible := freeAbove
	eligible.TypeRates = []TypeRate{{ProductTypeID: fridge, PriceCents: 3000, UnitsPerCharge: 1, FreeShippingEligible: true}}
	withFridge := bundle
	withFridge.TypeRates = []TypeRate{fridgeRate}
	tests := []struct {
		name     string
		method   Method
		subtotal int64
		items    []PricedItem
		want     int64
	}{
		{"five units are one bundle", bundle, 0, items(5, 0), 500},
		{"a sixth unit starts a second bundle", bundle, 0, items(6, 0), 1000},
		{"once per shipment when units per charge is 0", Method{CalcType: CalcFlat, FlatCents: 500}, 0, items(12, 0), 500},
		{"a typed line is priced by its own rate", withFridge, 0, items(3, 1), 500 + 3000},
		{"each fridge is charged", withFridge, 0, items(0, 2), 6000},
		{"only typed lines leave the default pool empty", withFridge, 0, items(0, 1), 3000},
		{"below the threshold both pools pay", freeAbove, 9999, items(3, 1), 3500},
		{"the threshold waives the default pool but not the fridge", freeAbove, 10000, items(3, 1), 3000},
		{"an eligible fridge ships free too", eligible, 10000, items(3, 1), 0},
		{"a free method still charges a typed rate", Method{CalcType: CalcFree, TypeRates: []TypeRate{fridgeRate}}, 0, items(3, 1), 3000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComputePrice(tc.method, tc.subtotal, tc.items); got != tc.want {
				t.Errorf("ComputePrice: got %d, want %d", got, tc.want)
			}
		})
	}
}
