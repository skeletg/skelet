// Package shipping computes the storefront's default shipping rates - zones group countries under a channel, and methods are formulas on a zone: 'flat' is fixed cents, 'free' is zero, 'free_above' is zero past a subtotal threshold, 'table' is tiers over weight, subtotal or item count
// Pluggable seam - order.placing carries a *ShippingCents pointer, so a real-carrier plugin writes through it and these formulas never run; the order service falls back to Compute only when it's still nil after the sync chain
package shipping

import (
	"math"
	"time"

	"github.com/google/uuid"
)

type CalcType string

const (
	CalcFlat      CalcType = "flat"
	CalcFree      CalcType = "free"
	CalcFreeAbove CalcType = "free_above"
	CalcTable     CalcType = "table"
)

// TableBasis is what a table method's tiers measure
type TableBasis string

const (
	BasisWeight   TableBasis = "weight"   // grams the method's own lines weigh
	BasisSubtotal TableBasis = "subtotal" // the post-discount subtotal, in cents
	BasisQuantity TableBasis = "quantity" // units among the method's own lines
)

// RateTier is one row of a table method - a shipment measuring From or more pays PriceCents, until the next tier.
// A NotOffered tier is the table's ceiling: from there on the method isn't offered at all
type RateTier struct {
	From       int64 `json:"from"`
	PriceCents int64 `json:"priceCents"`
	NotOffered bool  `json:"notOffered,omitempty"`
}

type Zone struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Code      string
	Name      string
	SortOrder int32
	IsActive  bool
	Countries []string // populated by service-level reads, not the bare repo
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Method struct {
	ID             uuid.UUID
	ZoneID         uuid.UUID
	Code           string
	Name           string
	CalcType       CalcType
	FlatCents      int64
	FreeAboveCents int64
	SortOrder      int32
	IsActive       bool
	// "all" is offered everywhere the zone covers, "specific" only in AllowedCountries - which is service-hydrated, like Zone.Countries
	CountryMode      string
	AllowedCountries []string
	// The delivery kind the method prices - ship_to_address, parcel_locker for a pickup point the shopper picks, or carrier_point when the carrier routes to the point nearest the address
	DeliveryKind string
	// Charge FlatCents again for every started bundle of this many units - 0 charges once per shipment
	UnitsPerCharge int32
	// Delivery days the method takes, counted from tomorrow - 0 gives no estimate. Saturday and Sunday count only when DeliversOnWeekends
	EstimatedDays      int32
	DeliversOnWeekends bool
	// Prices for particular product types, service-hydrated - their lines are priced by their row instead of pooling into FlatCents
	TypeRates []TypeRate
	// A table method's measure and its tiers, ascending by From
	TableBasis TableBasis
	RateTiers  []RateTier
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TypeRate is a method's own price for one product type - a fridge at 30€ per unit, never free unless FreeShippingEligible
type TypeRate struct {
	ProductTypeID        uuid.UUID
	ProductTypeName      string
	PriceCents           int64
	UnitsPerCharge       int32
	FreeShippingEligible bool
}

type QuoteInput struct {
	ChannelID     uuid.UUID
	CountryCode   string
	SubtotalCents int64 // post-discount subtotal - drives free_above threshold
	// What is shipped, by product type - empty prices the method once, as for a quote with no cart
	Items []PricedItem
}

// PricedItem is a line as a built-in method prices it
type PricedItem struct {
	ProductTypeID uuid.UUID
	Quantity      float64
	WeightGrams   int32 // per unit
}

type Quote struct {
	MethodID     uuid.UUID
	Code         string
	Name         string
	PriceCents   int64
	DeliveryKind string
	// From the method - see Method.EstimatedDays
	EstimatedDays      int32
	DeliversOnWeekends bool
}

// ComputePrice prices a shipment with a built-in method. Lines whose product type has a TypeRate are priced by it; every other line pools into the method's own rate. Each pool costs its price once per started bundle of UnitsPerCharge units (once in all when 0), and the pools are summed. A free method waives the method's own pool, and free_above at the threshold waives it and every FreeShippingEligible type pool - a fridge's rate still applies unless ticked
// A table method prices its own pool by the tier the pool reaches instead, and isn't offered (false) below its first
// tier or from a NotOffered one. A subtotal table's free tier counts as reaching free shipping, so it waives the
// FreeShippingEligible type pools as free_above does
func ComputePrice(m Method, subtotalCents int64, items []PricedItem) (int64, bool) {
	waiveDefault, waiveEligible := false, false
	switch m.CalcType {
	case CalcFree:
		waiveDefault = true
	case CalcFreeAbove:
		if subtotalCents >= m.FreeAboveCents {
			waiveDefault, waiveEligible = true, true
		}
	case CalcTable:
		if m.TableBasis == BasisSubtotal {
			price, ok := tablePrice(m, subtotalCents, 0, 0)
			if !ok {
				return 0, false
			}
			waiveEligible = price == 0
		}
	}
	// 'flat' and anything unrecognized fall through to flat - safer than panicking on a stored value we can't read, e.g. after a downgrade
	if len(items) == 0 {
		if m.CalcType == CalcTable {
			return tablePrice(m, subtotalCents, 0, 0)
		}
		if waiveDefault {
			return 0, true
		}
		return m.FlatCents, true
	}
	rates := make(map[uuid.UUID]TypeRate, len(m.TypeRates))
	for _, r := range m.TypeRates {
		rates[r.ProductTypeID] = r
	}
	var defaultUnits, defaultGrams float64
	typeUnits := map[uuid.UUID]float64{}
	for _, it := range items {
		if _, ok := rates[it.ProductTypeID]; ok && it.ProductTypeID != uuid.Nil {
			typeUnits[it.ProductTypeID] += it.Quantity
			continue
		}
		defaultUnits += it.Quantity
		defaultGrams += float64(it.WeightGrams) * it.Quantity
	}
	var total int64
	switch {
	case defaultUnits <= 0 || waiveDefault:
	case m.CalcType == CalcTable:
		price, ok := tablePrice(m, subtotalCents, defaultUnits, defaultGrams)
		if !ok {
			return 0, false
		}
		total += price
	default:
		total += bundlePrice(m.FlatCents, m.UnitsPerCharge, defaultUnits)
	}
	for id, units := range typeUnits {
		r := rates[id]
		if waiveEligible && r.FreeShippingEligible {
			continue
		}
		total += bundlePrice(r.PriceCents, r.UnitsPerCharge, units)
	}
	return total, true
}

// tablePrice is the last tier the shipment's measure reaches - below the first, or at a NotOffered one, the method isn't
// offered. Weights round to the gram and counts to a thousandth first, so 0.2 + 0.7 + 0.1 items reach a tier from 1
func tablePrice(m Method, subtotalCents int64, units, grams float64) (int64, bool) {
	var measure float64
	switch m.TableBasis {
	case BasisSubtotal:
		measure = float64(subtotalCents)
	case BasisQuantity:
		measure = math.Round(units*1000) / 1000
	default:
		measure = math.Round(grams)
	}
	var reached *RateTier
	for i := range m.RateTiers {
		if measure < float64(m.RateTiers[i].From) {
			break
		}
		reached = &m.RateTiers[i]
	}
	if reached == nil || reached.NotOffered {
		return 0, false
	}
	return reached.PriceCents, true
}

// bundlePrice charges price per started bundle of perCharge units - 5€ per 5 units is 5€ for 1-5 and 10€ for 6 - or once when perCharge is 0
func bundlePrice(price int64, perCharge int32, units float64) int64 {
	if perCharge <= 0 {
		return price
	}
	return price * int64(math.Ceil(units/float64(perCharge)))
}

// pricedItems is the rated lines as the built-in engine prices them
func pricedItems(items []RateItem) []PricedItem {
	out := make([]PricedItem, 0, len(items))
	for _, it := range items {
		out = append(out, PricedItem{ProductTypeID: it.ProductTypeID, Quantity: it.Quantity, WeightGrams: it.WeightGrams})
	}
	return out
}
