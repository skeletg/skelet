package shipping

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// The shipping-rate provider seam, an internal mirror of sdk.Carrier - plugins import the SDK type and core uses this one, with the built-in engine itself a registered Carrier
// The two types stay in lock-step by hand so the SDK is free of internal/* imports, and the host adapter translates and injects the per-channel config - which is why the internal RateRequest has no Config field
type Carrier interface {
	Quote(ctx context.Context, req RateRequest) ([]RatedMethod, error)
}

type Tracker interface {
	Track(ctx context.Context, req TrackRequest) (TrackResult, error)
}

type Labeler interface {
	Label(ctx context.Context, req LabelRequest) (LabelResult, error)
}

type RateRequest struct {
	ChannelID     uuid.UUID
	Destination   Address
	SubtotalCents int64
	CurrencyCode  string
	Items         []RateItem
	// The cartonized view of Items, packed from the channel's boxes, and nil when none are configured - carriers then fall back to Items
	Parcels []Parcel
}

type Dimensions struct {
	WeightGrams, LengthMm, WidthMm, HeightMm int32
	ProductTypeID                            uuid.UUID
}

type DimensionLookup interface {
	DimensionsBySKUs(ctx context.Context, skus []string) (map[string]Dimensions, error)
}

// One line summarized for rating - physical fields are 0 when the product doesn't specify them, and carriers should fall back to a default parcel. ProductTypeID is filled from the catalog for the built-in methods' per-type rates
type RateItem struct {
	SKU            string
	Quantity       float64
	UnitPriceCents int64
	WeightGrams    int32
	LengthMm       int32
	WidthMm        int32
	HeightMm       int32
	ProductTypeID  uuid.UUID
}

// One shipping option, where Code is carrier-stable like "ground" or "ups:03" - no DB id, since external carriers have none and the built-in re-resolves code to method by destination at placement
// Kind is the delivery kind the method serves (ship_to_address, store_pickup, parcel_locker, carrier_point) - empty takes the carrier's first declared kind, so a carrier offering one kind never has to set it
type RatedMethod struct {
	Code          string
	Name          string
	PriceCents    int64
	EstimatedDays int
	Kind          string
	// Whether Saturday and Sunday count toward EstimatedDays - core fills it from the method or the carrier's channel row, so a carrier never has to
	DeliversOnWeekends bool
}

// A ship-to address, declared locally so shipping doesn't import order
type Address struct {
	FirstName   string
	LastName    string
	Company     string
	Street1     string
	Street2     string
	City        string
	Region      string
	PostalCode  string
	CountryCode string
	Phone       string
}

type TrackRequest struct {
	CarrierSlug    string
	TrackingNumber string
}

type TrackResult struct {
	Status string
	URL    string
	Events []TrackEvent
}

type TrackEvent struct {
	Time     time.Time
	Status   string
	Location string
}

type LabelRequest struct {
	OrderNumber string
	ChannelID   uuid.UUID
	CarrierSlug string
	MethodCode  string
	ShipTo      Address
	Items       []RateItem
	// The cartonized view of Items, filled by the service when nil and the channel has boxes
	Parcels []Parcel
	// The declared insurance value recorded on the fulfillment - 0 is uninsured
	InsuredValueCents int64
}

type LabelResult struct {
	Doc            []byte
	Format         string
	TrackingNumber string
}

// The per-(channel, carrier) row - the carrier lives in code, and this records that an admin enabled it on a channel with a sort position and optional ship-to country limits - AllowedCountries is service-hydrated
type CarrierConfig struct {
	ID               uuid.UUID
	ChannelID        uuid.UUID
	CarrierSlug      string
	IsEnabled        bool
	DisplayName      string
	SortOrder        int32
	CountryMode      string
	AllowedCountries []string
	// Saturday and Sunday count toward the carrier's estimates - set per channel, since a carrier quotes days and not a calendar
	DeliversOnWeekends bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CarrierWithConfig struct {
	Slug          string
	DisplayName   string
	Config        CarrierConfig
	SupportsTrack bool
	SupportsLabel bool
}

type RatedQuote struct {
	CarrierSlug string
	CarrierName string
	Method      RatedMethod
}
