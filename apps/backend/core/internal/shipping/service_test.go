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
			got, ok := ComputePrice(tc.method, tc.subtotal, nil)
			if !ok || got != tc.want {
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
			if got, ok := ComputePrice(tc.method, tc.subtotal, tc.items); !ok || got != tc.want {
				t.Errorf("ComputePrice: got %d, want %d", got, tc.want)
			}
		})
	}
}

// A table charges the last tier the shipment reaches, measured over the lines the method prices itself - a typed line
// keeps its own rate and stays off the scale - and below the first tier the method isn't offered at all
func TestComputePriceByTable(t *testing.T) {
	shirt, fridge := uuid.New(), uuid.New()
	tiers := []RateTier{{From: 0, PriceCents: 490}, {From: 2000, PriceCents: 890}, {From: 10000, PriceCents: 1990}}
	weight := Method{CalcType: CalcTable, TableBasis: BasisWeight, RateTiers: tiers}
	heavyFridge := weight
	heavyFridge.TypeRates = []TypeRate{{ProductTypeID: fridge, PriceCents: 3000, UnitsPerCharge: 1}}
	subtotal := Method{CalcType: CalcTable, TableBasis: BasisSubtotal, RateTiers: []RateTier{{From: 0, PriceCents: 590}, {From: 5000, PriceCents: 0}}}
	quantity := Method{CalcType: CalcTable, TableBasis: BasisQuantity, RateTiers: []RateTier{{From: 1, PriceCents: 300}, {From: 4, PriceCents: 600}}}
	capped := Method{CalcType: CalcTable, TableBasis: BasisWeight, RateTiers: []RateTier{{From: 0, PriceCents: 490}, {From: 30000, NotOffered: true}}}
	eligibleFridge := subtotal
	eligibleFridge.TypeRates = []TypeRate{{ProductTypeID: fridge, PriceCents: 3000, UnitsPerCharge: 1, FreeShippingEligible: true}}
	tenths := Method{CalcType: CalcTable, TableBasis: BasisQuantity, RateTiers: []RateTier{{From: 0, PriceCents: 300}, {From: 3, PriceCents: 600}}}
	shirts := func(n float64, grams int32) []PricedItem {
		return []PricedItem{{ProductTypeID: shirt, Quantity: n, WeightGrams: grams}}
	}
	tests := []struct {
		name     string
		method   Method
		subtotal int64
		items    []PricedItem
		want     int64
		offered  bool
	}{
		{"a light parcel pays the first tier", weight, 0, shirts(2, 300), 490, true},
		{"reaching a tier's start pays it", weight, 0, shirts(4, 500), 890, true},
		{"the heaviest tier holds past its start", weight, 0, shirts(30, 500), 1990, true},
		{"a fridge's own rate leaves the scale to the shirts", heavyFridge, 0, append(shirts(1, 300), PricedItem{ProductTypeID: fridge, Quantity: 1, WeightGrams: 60000}), 490 + 3000, true},
		{"subtotal below the free tier pays", subtotal, 4999, shirts(1, 0), 590, true},
		{"subtotal at the free tier ships free", subtotal, 5000, shirts(1, 0), 0, true},
		{"a quote with no cart reads the subtotal", subtotal, 6000, nil, 0, true},
		{"item count picks its tier", quantity, 0, shirts(5, 0), 600, true},
		{"below the first tier the method isn't offered", quantity, 0, shirts(0.5, 0), 0, false},
		{"a not-offered tier is a ceiling", capped, 0, shirts(1, 30000), 0, false},
		{"under the ceiling still pays", capped, 0, shirts(1, 29999), 490, true},
		{"a free subtotal tier waives an eligible type", eligibleFridge, 5000, []PricedItem{{ProductTypeID: fridge, Quantity: 1}}, 0, true},
		{"below the free tier the eligible type pays", eligibleFridge, 4999, []PricedItem{{ProductTypeID: fridge, Quantity: 1}}, 3000, true},
		{"three tenths of a metre reach the 3 tier", tenths, 0, []PricedItem{{Quantity: 0.1}, {Quantity: 0.1}, {Quantity: 0.1}, {Quantity: 2.7}}, 600, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ComputePrice(tc.method, tc.subtotal, tc.items)
			if ok != tc.offered || got != tc.want {
				t.Errorf("ComputePrice = %d, offered %v - want %d, offered %v", got, ok, tc.want, tc.offered)
			}
		})
	}
}

func TestARateTableIsCheckedBeforeItSaves(t *testing.T) {
	ok := Method{CalcType: CalcTable, TableBasis: BasisWeight, RateTiers: []RateTier{{From: 0, PriceCents: 500}}}
	if err := CheckMethodPricing(ok); err != nil {
		t.Fatalf("a valid table refused: %v", err)
	}
	for name, m := range map[string]Method{
		"no tiers":        {CalcType: CalcTable, TableBasis: BasisWeight},
		"unknown basis":   {CalcType: CalcTable, TableBasis: "volume", RateTiers: ok.RateTiers},
		"negative start":  {CalcType: CalcTable, TableBasis: BasisWeight, RateTiers: []RateTier{{From: -1, PriceCents: 500}}},
		"negative price":  {CalcType: CalcTable, TableBasis: BasisWeight, RateTiers: []RateTier{{From: 0, PriceCents: -5}}},
		"duplicate start": {CalcType: CalcTable, TableBasis: BasisWeight, RateTiers: []RateTier{{From: 0, PriceCents: 500}, {From: 0, PriceCents: 900}}},
		"nothing offered": {CalcType: CalcTable, TableBasis: BasisWeight, RateTiers: []RateTier{{From: 0, NotOffered: true}}},
		"start overflows": {CalcType: CalcTable, TableBasis: BasisWeight, RateTiers: []RateTier{{From: 1 << 40, PriceCents: 500}}},
		"too many tiers":  {CalcType: CalcTable, TableBasis: BasisWeight, RateTiers: manyTiers(maxRateTiers + 1)},
	} {
		if err := CheckMethodPricing(m); err == nil {
			t.Errorf("%s saved", name)
		}
	}
	if got := freeFrom(Method{CalcType: CalcTable, TableBasis: BasisSubtotal, RateTiers: []RateTier{{From: 0, PriceCents: 590}, {From: 5000, PriceCents: 0}}}); got != 5000 {
		t.Errorf("a subtotal table's free tier hints %d, want 5000", got)
	}
	// a ceiling's zero price isn't free shipping, so it never shows as the free-from hint
	if got := freeFrom(Method{CalcType: CalcTable, TableBasis: BasisSubtotal, RateTiers: []RateTier{{From: 0, PriceCents: 590}, {From: 90000, NotOffered: true}}}); got != 0 {
		t.Errorf("a not-offered ceiling hints free shipping from %d", got)
	}
}

func manyTiers(n int) []RateTier {
	out := make([]RateTier, n)
	for i := range out {
		out[i] = RateTier{From: int64(i), PriceCents: 100}
	}
	return out
}
