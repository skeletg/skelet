package sdk

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// The in-process way to add a carrier - compiled in, full context, no network hop per quote
// The other way is a remote provider (remoteprovider kind=shipping_carrier), which trades that for
// needing no Go and no rebuild - pick per integration, not per role
// Tracker and Labeler are optional: the host type-asserts for them, so a quote-only carrier implements Carrier alone
// A Quote error is logged and that carrier skipped, so checkout still shows the others
type Carrier interface {
	Quote(ctx context.Context, req RateRequest) ([]RatedMethod, error)
}

type Tracker interface {
	Track(ctx context.Context, req TrackRequest) (TrackResult, error)
}

type Labeler interface {
	Label(ctx context.Context, req LabelRequest) (LabelResult, error)
}

// Config is scoped to "shipping/<slug>/*" and injected per request - never persist or share it
type RateRequest struct {
	ChannelID     uuid.UUID
	Destination   Address
	SubtotalCents int64
	CurrencyCode  string
	Items         []RateItem
	// Admin-defined boxes packed first-fit-decreasing - nil when the channel configures no boxes, so rate from Items with your own default parcel
	Parcels []Parcel
	Config  ConfigReader
}

// BoxCode is "" for an oversize item shipping in its own packaging, and WeightGrams is gross (contents + box tare)
type Parcel struct {
	BoxCode     string
	LengthMm    int32
	WidthMm     int32
	HeightMm    int32
	WeightGrams int64
	ValueCents  int64
	Items       []ParcelItem
}

type ParcelItem struct {
	SKU      string
	Quantity float64
}

type RateItem struct {
	SKU            string
	Quantity       float64
	UnitPriceCents int64
	// 0 means the product doesn't specify it - fall back to a default parcel rather than failing the quote
	WeightGrams int32
	LengthMm    int32
	WidthMm     int32
	HeightMm    int32
}

// Code is carrier-stable ("ground", "ups:03") and what the storefront echoes back at checkout
// Kind is the delivery kind the method serves - empty takes the carrier's first WithDeliveryKinds kind, ship_to_address when it declares none
type RatedMethod struct {
	Code          string
	Name          string
	PriceCents    int64
	EstimatedDays int
	Kind          string
}

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
	Config         ConfigReader
}

// URL is always safe to render - Status and Events are best-effort
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
	OrderNumber       string
	ChannelID         uuid.UUID
	CarrierSlug       string
	MethodCode        string
	ShipTo            Address
	Items             []RateItem
	Parcels           []Parcel // host-cartonized view of Items, see RateRequest
	InsuredValueCents int64    // declared value recorded on the fulfillment, 0 = uninsured
	Config            ConfigReader
}

// TrackingNumber is set when the call also books the shipment - Format is "pdf", "png" or "zpl"
type LabelResult struct {
	Doc            []byte
	Format         string
	TrackingNumber string
}

type CarrierOption func(*CarrierRegistration)

type CarrierRegistration struct {
	PluginSlug  string // owning plugin's manifest slug - what the host keys the kill switch, timeout, rate limit and metrics on
	Slug        string
	Carrier     Carrier
	DisplayName string
	Kinds       []string // the delivery kinds its methods serve - see WithDeliveryKinds
}

// Falls back to the raw carrier slug when unset - not title-cased
func WithCarrierDisplayName(name string) CarrierOption {
	return func(r *CarrierRegistration) { r.DisplayName = name }
}

// The delivery kinds the carrier quotes - a delivery option of a kind is only ready once an enabled carrier declares it. Unset means ship_to_address
func WithDeliveryKinds(kinds ...string) CarrierOption {
	return func(r *CarrierRegistration) { r.Kinds = kinds }
}

// carrierSlug keys config paths (shipping/<slug>/*), shipping_carriers rows and the (carrierSlug, methodCode) pair the storefront sends at checkout - independent of the manifest slug, and duplicates panic
func (pl *Plugin) RegisterShippingCarrier(carrierSlug string, c Carrier, opts ...CarrierOption) {
	reg := CarrierRegistration{PluginSlug: pl.slug, Slug: carrierSlug, Carrier: c}
	for _, opt := range opts {
		opt(&reg)
	}
	withHost(func(h Host) error { return h.RegisterShippingCarrier(reg) })
}
