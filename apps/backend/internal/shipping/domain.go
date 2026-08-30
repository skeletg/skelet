// Package shipping computes the storefront's default shipping rates - zones group countries under a channel, and methods are formulas on a zone: 'flat' is fixed cents, 'free' is zero, 'free_above' is zero past a subtotal threshold
// Pluggable seam - order.placing carries a *ShippingCents pointer, so a real-carrier plugin writes through it and these formulas never run; the order service falls back to Compute only when it's still nil after the sync chain
package shipping

import (
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
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type QuoteInput struct {
	ChannelID     uuid.UUID
	CountryCode   string
	SubtotalCents int64 // post-discount subtotal - drives free_above threshold
}

type Quote struct {
	MethodID   uuid.UUID
	Code       string
	Name       string
	PriceCents int64
}

func ComputePrice(m Method, subtotalCents int64) int64 {
	switch m.CalcType {
	case CalcFree:
		return 0
	case CalcFreeAbove:
		if subtotalCents >= m.FreeAboveCents {
			return 0
		}
		return m.FlatCents
	default:
		// 'flat' and anything unrecognized fall through to flat - safer than panicking on a stored value we can't read, e.g. after a downgrade
		return m.FlatCents
	}
}
