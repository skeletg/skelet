package shipping

import (
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
)

// Cartonization: bin items into admin-defined boxes so carriers rate a real multi-parcel shipment, not one summed blob - Deterministic first-fit-decreasing (not optimal 3D packing) - stable across calls, which matters because PriceSelected must re-produce the quote-time price

// Box is one admin-defined carton - MaxWeightGrams is GROSS (contents + BoxWeightGrams tare), 0 = unlimited
type Box struct {
	ID             uuid.UUID
	ChannelID      uuid.UUID
	Code           string
	Name           string
	InnerLengthMm  int32
	InnerWidthMm   int32
	InnerHeightMm  int32
	MaxWeightGrams int32
	BoxWeightGrams int32
	IsActive       bool
	SortOrder      int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Parcel is one physical package - BoxCode is "" for an oversize item in its own
// packaging (dims are then the item's) - WeightGrams is gross (contents + tare)
type Parcel struct {
	BoxCode     string
	LengthMm    int32
	WidthMm     int32
	HeightMm    int32
	WeightGrams int64
	ValueCents  int64
	Items       []ParcelItem
}

// ParcelItem is one SKU (and how many units of it) inside a parcel
type ParcelItem struct {
	SKU      string
	Quantity float64
}

type packUnit struct {
	sku        string
	dims       [3]int32 // sorted ascending - all zero when unknown
	volume     int64    // mm^3 - 0 when dims unknown
	weight     int64    // grams - 0 when unknown
	valueCents int64
}

type openParcel struct {
	box        Box
	boxDims    [3]int32
	usedVolume int64
	weight     int64 // contents only
	value      int64
	qtyBySKU   map[string]float64
	order      []string // SKU insertion order for deterministic output
}

// Pack bins items into boxes, first-fit-decreasing on unit volume - Units that fit no box ship as their own parcel (BoxCode "") - Returns
// nil when no active boxes exist or the shipment is past maxPackUnits - callers treat that as cartonization-off and fall back to raw Items
func Pack(items []RateItem, boxes []Box) []Parcel {
	var active []Box
	for _, b := range boxes {
		if b.IsActive && b.InnerLengthMm > 0 && b.InnerWidthMm > 0 && b.InnerHeightMm > 0 {
			active = append(active, b)
		}
	}
	if len(active) == 0 || packedUnits(items) > maxPackUnits {
		return nil
	}
	// Smallest-volume-first so opening a new parcel picks the tightest box, ties by code
	sort.Slice(active, func(i, j int) bool {
		vi, vj := boxVolume(active[i]), boxVolume(active[j])
		if vi != vj {
			return vi < vj
		}
		return active[i].Code < active[j].Code
	})

	units := explodeUnits(items)
	// First-fit-DECREASING - big units claim boxes before small filler, ties by SKU
	sort.SliceStable(units, func(i, j int) bool {
		if units[i].volume != units[j].volume {
			return units[i].volume > units[j].volume
		}
		if units[i].weight != units[j].weight {
			return units[i].weight > units[j].weight
		}
		return units[i].sku < units[j].sku
	})

	var open []*openParcel
	var oversize []Parcel
	for _, u := range units {
		placed := false
		for _, p := range open {
			if fitsInParcel(u, p) {
				addUnit(p, u)
				placed = true
				break
			}
		}
		if placed {
			continue
		}
		// Open the smallest box that can hold this unit on its own
		found := false
		for _, b := range active {
			if fitsInBox(u, b) {
				p := &openParcel{box: b, boxDims: sortedDims(b.InnerLengthMm, b.InnerWidthMm, b.InnerHeightMm), qtyBySKU: map[string]float64{}}
				addUnit(p, u)
				open = append(open, p)
				found = true
				break
			}
		}
		if !found {
			// Oversize/overweight for every box: own-packaging parcel
			oversize = append(oversize, Parcel{
				LengthMm:    u.dims[2],
				WidthMm:     u.dims[1],
				HeightMm:    u.dims[0],
				WeightGrams: u.weight,
				ValueCents:  u.valueCents,
				Items:       []ParcelItem{{SKU: u.sku, Quantity: 1}},
			})
		}
	}

	out := make([]Parcel, 0, len(open)+len(oversize))
	for _, p := range open {
		items := make([]ParcelItem, 0, len(p.order))
		for _, sku := range p.order {
			items = append(items, ParcelItem{SKU: sku, Quantity: p.qtyBySKU[sku]})
		}
		out = append(out, Parcel{
			BoxCode:     p.box.Code,
			LengthMm:    p.box.InnerLengthMm,
			WidthMm:     p.box.InnerWidthMm,
			HeightMm:    p.box.InnerHeightMm,
			WeightGrams: p.weight + int64(p.box.BoxWeightGrams),
			ValueCents:  p.value,
			Items:       items,
		})
	}
	return append(out, oversize...)
}

// The most single units Pack bins one by one - a cart line may hold a billion, and past this the carriers rate the raw lines instead
const maxPackUnits = 10_000

func packedUnits(items []RateItem) float64 {
	var n float64
	for _, it := range items {
		if it.Quantity > 0 {
			n += math.Ceil(it.Quantity)
		}
	}
	return n
}

// explodeUnits expands rate items into single physical units - Fractional
// quantities round up, value spread evenly so parcels still sum to the line total
func explodeUnits(items []RateItem) []packUnit {
	var units []packUnit
	for _, it := range items {
		if it.Quantity <= 0 {
			continue
		}
		n := max(int(math.Ceil(it.Quantity)), 1)
		lineValue := int64(math.Round(it.Quantity * float64(it.UnitPriceCents)))
		dims := sortedDims(it.LengthMm, it.WidthMm, it.HeightMm)
		for i := range n {
			value := lineValue / int64(n)
			if i == 0 {
				value += lineValue % int64(n)
			}
			units = append(units, packUnit{
				sku:        it.SKU,
				dims:       dims,
				volume:     int64(dims[0]) * int64(dims[1]) * int64(dims[2]),
				weight:     int64(it.WeightGrams),
				valueCents: value,
			})
		}
	}
	return units
}

func boxVolume(b Box) int64 {
	return int64(b.InnerLengthMm) * int64(b.InnerWidthMm) * int64(b.InnerHeightMm)
}

func sortedDims(a, b, c int32) [3]int32 {
	d := [3]int32{a, b, c}
	if d[0] > d[1] {
		d[0], d[1] = d[1], d[0]
	}
	if d[1] > d[2] {
		d[1], d[2] = d[2], d[1]
	}
	if d[0] > d[1] {
		d[0], d[1] = d[1], d[0]
	}
	return d
}

// dimsFit reports whether sorted item dims fit sorted box dims (axis-aligned
// rotation check) - Unknown dims (all zero) always fit - packing then constrains on weight
func dimsFit(item, box [3]int32) bool {
	return item[0] <= box[0] && item[1] <= box[1] && item[2] <= box[2]
}

func fitsInBox(u packUnit, b Box) bool {
	if !dimsFit(u.dims, sortedDims(b.InnerLengthMm, b.InnerWidthMm, b.InnerHeightMm)) {
		return false
	}
	if b.MaxWeightGrams > 0 && u.weight+int64(b.BoxWeightGrams) > int64(b.MaxWeightGrams) {
		return false
	}
	return true
}

func fitsInParcel(u packUnit, p *openParcel) bool {
	if !dimsFit(u.dims, p.boxDims) {
		return false
	}
	if p.usedVolume+u.volume > boxVolume(p.box) {
		return false
	}
	if p.box.MaxWeightGrams > 0 && p.weight+u.weight+int64(p.box.BoxWeightGrams) > int64(p.box.MaxWeightGrams) {
		return false
	}
	return true
}

func addUnit(p *openParcel, u packUnit) {
	if _, seen := p.qtyBySKU[u.sku]; !seen {
		p.order = append(p.order, u.sku)
	}
	p.qtyBySKU[u.sku]++
	p.usedVolume += u.volume
	p.weight += u.weight
	p.value += u.valueCents
}
