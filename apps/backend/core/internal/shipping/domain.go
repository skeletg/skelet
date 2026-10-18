// Package shipping computes the storefront's default shipping rates - zones group countries under a channel, and methods are formulas on a zone: 'flat' is fixed cents, 'free' is zero, 'free_above' is zero past a subtotal threshold
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
)

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
	CreatedAt time.Time
	UpdatedAt time.Time
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
func ComputePrice(m Method, subtotalCents int64, items []PricedItem) int64 {
	waiveDefault, waiveEligible := false, false
	switch m.CalcType {
	case CalcFree:
		waiveDefault = true
	case CalcFreeAbove:
		if subtotalCents >= m.FreeAboveCents {
			waiveDefault, waiveEligible = true, true
		}
	}
	// 'flat' and anything unrecognized fall through to flat - safer than panicking on a stored value we can't read, e.g. after a downgrade
	if len(items) == 0 {
		if waiveDefault {
			return 0
		}
		return m.FlatCents
	}
	rates := make(map[uuid.UUID]TypeRate, len(m.TypeRates))
	for _, r := range m.TypeRates {
		rates[r.ProductTypeID] = r
	}
	var defaultUnits float64
	typeUnits := map[uuid.UUID]float64{}
	for _, it := range items {
		if _, ok := rates[it.ProductTypeID]; ok && it.ProductTypeID != uuid.Nil {
			typeUnits[it.ProductTypeID] += it.Quantity
			continue
		}
		defaultUnits += it.Quantity
	}
	var total int64
	if defaultUnits > 0 && !waiveDefault {
		total += bundlePrice(m.FlatCents, m.UnitsPerCharge, defaultUnits)
	}
	for id, units := range typeUnits {
		r := rates[id]
		if waiveEligible && r.FreeShippingEligible {
			continue
		}
		total += bundlePrice(r.PriceCents, r.UnitsPerCharge, units)
	}
	return total
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
		out = append(out, PricedItem{ProductTypeID: it.ProductTypeID, Quantity: it.Quantity})
	}
	return out
}
