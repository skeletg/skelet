package shipping

import (
	"reflect"
	"testing"
)

func box(code string, l, w, h, maxW, tare int32) Box {
	return Box{Code: code, InnerLengthMm: l, InnerWidthMm: w, InnerHeightMm: h, MaxWeightGrams: maxW, BoxWeightGrams: tare, IsActive: true}
}

func TestPackNoBoxes(t *testing.T) {
	items := []RateItem{{SKU: "A", Quantity: 2, WeightGrams: 100}}
	if got := Pack(items, nil); got != nil {
		t.Fatalf("no boxes should return nil, got %v", got)
	}
	inactive := []Box{{Code: "off", InnerLengthMm: 100, InnerWidthMm: 100, InnerHeightMm: 100}}
	if got := Pack(items, inactive); got != nil {
		t.Fatalf("inactive-only boxes should return nil, got %v", got)
	}
}

func TestPackSmallestBoxWins(t *testing.T) {
	boxes := []Box{box("large", 400, 400, 400, 0, 0), box("small", 100, 100, 100, 0, 50)}
	items := []RateItem{{SKU: "A", Quantity: 1, WeightGrams: 200, LengthMm: 80, WidthMm: 50, HeightMm: 30, UnitPriceCents: 999}}
	got := Pack(items, boxes)
	if len(got) != 1 {
		t.Fatalf("want 1 parcel, got %d", len(got))
	}
	p := got[0]
	if p.BoxCode != "small" {
		t.Errorf("want smallest fitting box, got %q", p.BoxCode)
	}
	if p.WeightGrams != 250 { // 200 contents + 50 tare
		t.Errorf("weight = %d, want 250 (tare included)", p.WeightGrams)
	}
	if p.ValueCents != 999 {
		t.Errorf("value = %d, want 999", p.ValueCents)
	}
}

func TestPackRotationFit(t *testing.T) {
	// Item 300x10x20 only fits box 20x300x15... no; use sorted-dims check:
	// item sorted (10,20,300) vs box 320x25x12 sorted (12,25,320) -> fits
	boxes := []Box{box("tube", 320, 25, 12, 0, 0)}
	items := []RateItem{{SKU: "ROD", Quantity: 1, LengthMm: 300, WidthMm: 10, HeightMm: 20}}
	got := Pack(items, boxes)
	if len(got) != 1 || got[0].BoxCode != "tube" {
		t.Fatalf("rotated item should fit tube box, got %+v", got)
	}
}

func TestPackWeightLimitSplits(t *testing.T) {
	// 5 units x 3kg into boxes capped at 10kg gross (500g tare):
	// 3 units = 9.5kg gross fits, 4th would be 12.5 -> second parcel
	boxes := []Box{box("crate", 500, 500, 500, 10000, 500)}
	items := []RateItem{{SKU: "TILE", Quantity: 5, WeightGrams: 3000, LengthMm: 100, WidthMm: 100, HeightMm: 20, UnitPriceCents: 1000}}
	got := Pack(items, boxes)
	if len(got) != 2 {
		t.Fatalf("want 2 parcels, got %d: %+v", len(got), got)
	}
	if got[0].Items[0].Quantity != 3 || got[1].Items[0].Quantity != 2 {
		t.Errorf("split = %v + %v, want 3 + 2", got[0].Items[0].Quantity, got[1].Items[0].Quantity)
	}
	if got[0].ValueCents+got[1].ValueCents != 5000 {
		t.Errorf("values sum to %d, want 5000", got[0].ValueCents+got[1].ValueCents)
	}
}

func TestPackVolumeSplits(t *testing.T) {
	// Box volume 1000cm3; each unit 400cm3 -> 2 per box, third opens a new one
	boxes := []Box{box("cube", 100, 100, 100, 0, 0)}
	items := []RateItem{{SKU: "B", Quantity: 3, LengthMm: 100, WidthMm: 80, HeightMm: 50}}
	got := Pack(items, boxes)
	if len(got) != 2 {
		t.Fatalf("want 2 parcels, got %d", len(got))
	}
}

func TestPackOversizeOwnParcel(t *testing.T) {
	boxes := []Box{box("small", 100, 100, 100, 0, 0)}
	items := []RateItem{
		{SKU: "KAYAK", Quantity: 1, WeightGrams: 20000, LengthMm: 3000, WidthMm: 700, HeightMm: 400, UnitPriceCents: 50000},
		{SKU: "CAP", Quantity: 1, WeightGrams: 100, LengthMm: 50, WidthMm: 50, HeightMm: 20},
	}
	got := Pack(items, boxes)
	if len(got) != 2 {
		t.Fatalf("want 2 parcels, got %d: %+v", len(got), got)
	}
	var kayak *Parcel
	for i := range got {
		if got[i].BoxCode == "" {
			kayak = &got[i]
		}
	}
	if kayak == nil {
		t.Fatal("oversize item should ship as own-packaging parcel (BoxCode empty)")
	}
	if kayak.LengthMm != 3000 || kayak.WeightGrams != 20000 {
		t.Errorf("own-packaging parcel keeps item dims/weight, got %+v", kayak)
	}
}

func TestPackNoDimsWeightOnly(t *testing.T) {
	// Items without dims pack by weight alone: 4 x 300g into 1kg-max boxes
	// -> 3 + 1 (no tare)
	boxes := []Box{box("bag", 300, 300, 300, 1000, 0)}
	items := []RateItem{{SKU: "GRAIN", Quantity: 4, WeightGrams: 300}}
	got := Pack(items, boxes)
	if len(got) != 2 {
		t.Fatalf("want 2 parcels, got %d: %+v", len(got), got)
	}
	if got[0].Items[0].Quantity+got[1].Items[0].Quantity != 4 {
		t.Errorf("quantities lost: %+v", got)
	}
}

func TestPackMixedItemsShareBox(t *testing.T) {
	boxes := []Box{box("med", 200, 200, 200, 0, 0)}
	items := []RateItem{
		{SKU: "A", Quantity: 1, LengthMm: 100, WidthMm: 100, HeightMm: 100, UnitPriceCents: 100},
		{SKU: "B", Quantity: 2, LengthMm: 50, WidthMm: 50, HeightMm: 50, UnitPriceCents: 10},
	}
	got := Pack(items, boxes)
	if len(got) != 1 {
		t.Fatalf("want 1 shared parcel, got %d: %+v", len(got), got)
	}
	if got[0].ValueCents != 120 {
		t.Errorf("value = %d, want 120", got[0].ValueCents)
	}
	if len(got[0].Items) != 2 {
		t.Errorf("want both SKUs in parcel, got %+v", got[0].Items)
	}
}

func TestPackDeterministic(t *testing.T) {
	boxes := []Box{box("s", 150, 150, 150, 0, 0), box("l", 400, 400, 400, 5000, 100)}
	items := []RateItem{
		{SKU: "X", Quantity: 3, WeightGrams: 400, LengthMm: 120, WidthMm: 90, HeightMm: 60, UnitPriceCents: 250},
		{SKU: "Y", Quantity: 2, WeightGrams: 900, LengthMm: 200, WidthMm: 150, HeightMm: 100, UnitPriceCents: 1200},
	}
	first := Pack(items, boxes)
	for range 5 {
		if again := Pack(items, boxes); !reflect.DeepEqual(first, again) {
			t.Fatalf("non-deterministic packing:\n%+v\nvs\n%+v", first, again)
		}
	}
}

func TestPackFractionalQuantity(t *testing.T) {
	// 2.5 m of cable rounds up to 3 packable units; value split sums back
	boxes := []Box{box("spool", 300, 300, 300, 0, 0)}
	items := []RateItem{{SKU: "CABLE", Quantity: 2.5, WeightGrams: 100, UnitPriceCents: 1000}}
	got := Pack(items, boxes)
	total := int64(0)
	units := 0.0
	for _, p := range got {
		total += p.ValueCents
		for _, it := range p.Items {
			units += it.Quantity
		}
	}
	if total != 2500 {
		t.Errorf("value = %d, want 2500 (2.5 x 1000)", total)
	}
	if units != 3 {
		t.Errorf("units = %v, want 3 (ceil 2.5)", units)
	}
}
