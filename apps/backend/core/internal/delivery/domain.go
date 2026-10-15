// Package delivery is how an order reaches the shopper - the delivery kinds checkout can offer, the options a channel builds from them, and what each one asks of the shopper
//
// A kind lives in code: core registers ship_to_address, store_pickup, parcel_locker and carrier_point. It declares the blocks it needs and a floor for each - locked (always required), configurable (the admin picks required, optional or hidden) or n/a. A delivery option is a channel's named instance of a kind with the admin's picks, so the dashboard's checklist, the requirements checkout enforces and the readiness checks all read the same declaration
//
// Points (stores, lockers, parcel shops, PO boxes) come from point sources - a registry here filled by core, plus the channel's remote delivery_points providers
package delivery

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

func init() {
	permission.RegisterGroup(permission.Group{Slug: "checkout", Name: "Checkout", SortOrder: 41})
	permission.Register(permission.Definition{Group: "checkout", Slug: "checkout.view", Name: "View Checkout Settings", SortOrder: 1})
	permission.Register(permission.Definition{Group: "checkout", Slug: "checkout.manage", Name: "Manage Checkout Settings", SortOrder: 2})

	apperr.RegisterSentinel(ErrOptionNotFound, "DELIVERY_OPTION_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrUnknownKind, "DELIVERY_KIND_UNKNOWN", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrLockedBlock, "DELIVERY_BLOCK_LOCKED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNotReady, "DELIVERY_OPTION_NOT_READY", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrPointNotFound, "DELIVERY_POINT_NOT_FOUND", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrSourceUnavailable, "DELIVERY_SOURCE_UNAVAILABLE", apperr.ClassUnavailable)
}

var (
	ErrOptionNotFound    = errors.New("delivery option not found")
	ErrUnknownKind       = errors.New("unknown delivery kind")
	ErrLockedBlock       = errors.New("a locked block can't be made optional or hidden")
	ErrNotReady          = errors.New("the delivery option isn't ready to be enabled")
	ErrPointNotFound     = errors.New("that pickup point isn't available")
	ErrSourceUnavailable = errors.New("the point source isn't available")
)

// ShipTo says who supplies the address a delivery goes to - the shopper, or the point they pick (a store, a locker, a parcel shop, a PO box)
type ShipTo string

const (
	ShipToCustomer ShipTo = "customer"
	ShipToPoint    ShipTo = "point"
)

// Floor is how far an admin may relax a block - a locked block is always required, a configurable one follows the option's mode, and an n/a block is never asked because something else supplies it
type Floor string

const (
	FloorLocked       Floor = "locked"
	FloorConfigurable Floor = "configurable"
	FloorNA           Floor = "na"
)

// Mode is what checkout does with a block - ask and require it, ask it, or not ask it
type Mode string

const (
	ModeRequired Mode = "required"
	ModeOptional Mode = "optional"
	ModeHidden   Mode = "hidden"
)

func (m Mode) Valid() bool { return m == ModeRequired || m == ModeOptional || m == ModeHidden }

// rank orders modes so the strictest of several options wins - an order needs a phone when any option it uses requires one
func (m Mode) rank() int {
	switch m {
	case ModeRequired:
		return 2
	case ModeOptional:
		return 1
	}
	return 0
}

// Stricter is the stronger of two modes
func Stricter(a, b Mode) Mode {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// The blocks a kind can declare
const (
	BlockEmail           = "email"
	BlockPhone           = "phone"
	BlockShippingAddress = "shipping_address"
	BlockBillingAddress  = "billing_address"
	BlockPoint           = "point"
	BlockCollector       = "collector"
	BlockPayment         = "payment"
	BlockDeliveryOption  = "delivery_option"
)

// BlockSpec is one thing a kind asks of the shopper, with the floor that bounds the admin's choice and the reason the dashboard shows beside a locked or n/a block
type BlockSpec struct {
	Type    string
	Label   string
	Floor   Floor
	Default Mode // what a configurable block starts as on a new option
	Reason  string
}

// Kind is a way to deliver - registered in code, offered through a channel's delivery options
type Kind struct {
	Slug        string
	Label       string
	Description string
	ShipTo      ShipTo
	// What a point is called for this kind ("Store", "Pickup point") - empty when the kind needs none
	PointLabel string
	Blocks     []BlockSpec
	// Who registered it - "core"
	Owner string
}

func (k Kind) Block(t string) (BlockSpec, bool) {
	i := slices.IndexFunc(k.Blocks, func(b BlockSpec) bool { return b.Type == t })
	if i < 0 {
		return BlockSpec{}, false
	}
	return k.Blocks[i], true
}

// Needs reports whether the kind ever asks for the block
func (k Kind) Needs(t string) bool {
	b, ok := k.Block(t)
	return ok && b.Floor != FloorNA
}

// ResolvedBlock is a block with the mode an option gives it
type ResolvedBlock struct {
	Spec BlockSpec
	Mode Mode
}

// Resolve applies an option's modes to the kind's blocks - locked is required whatever was saved, n/a is hidden, and configurable takes the saved mode or the kind's default
func (k Kind) Resolve(modes map[string]Mode) []ResolvedBlock {
	out := make([]ResolvedBlock, 0, len(k.Blocks))
	for _, b := range k.Blocks {
		m := b.Default
		switch b.Floor {
		case FloorLocked:
			m = ModeRequired
		case FloorNA:
			m = ModeHidden
		default:
			if v, ok := modes[b.Type]; ok && v.Valid() {
				m = v
			}
			if !m.Valid() {
				m = ModeOptional
			}
		}
		out = append(out, ResolvedBlock{Spec: b, Mode: m})
	}
	return out
}

// ModeOf is the mode an option gives one block - hidden when the kind doesn't declare it
func (k Kind) ModeOf(modes map[string]Mode, t string) Mode {
	for _, rb := range k.Resolve(modes) {
		if rb.Spec.Type == t {
			return rb.Mode
		}
	}
	return ModeHidden
}

// CheckModes refuses modes that try to relax a locked block or name one the kind doesn't have
func (k Kind) CheckModes(modes map[string]Mode) error {
	for t, m := range modes {
		b, ok := k.Block(t)
		if !ok {
			return apperr.New("DELIVERY_BLOCK_UNKNOWN", apperr.ClassValidation, "the "+k.Label+" kind has no "+t+" block")
		}
		if !m.Valid() {
			return apperr.New("DELIVERY_BLOCK_MODE", apperr.ClassValidation, "a block is required, optional or hidden")
		}
		if b.Floor == FloorLocked && m != ModeRequired {
			return ErrLockedBlock
		}
	}
	return nil
}

// Option is a channel's delivery option - a named instance of a kind, with the admin's choices
type Option struct {
	ID          uuid.UUID
	ChannelID   uuid.UUID
	Kind        string
	Name        string
	Description string
	Enabled     bool
	SortOrder   int32
	BlockModes  map[string]Mode
	PointSource string
	CarrierSlug string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Implicit - one of the defaults a channel offers before its admin saves any, with no row an order could point at
func (o Option) Implicit() bool { return o.CreatedAt.IsZero() }

// SaveOptionInput is a create or a full update - an update keeps the kind it was created with
type SaveOptionInput struct {
	Kind        string
	Name        string
	Description string
	Enabled     bool
	SortOrder   int32
	BlockModes  map[string]Mode
	PointSource string
	CarrierSlug string
}

// Address is where a point is, or where a delivery goes
type Address struct {
	Name        string `json:"name,omitempty"`
	Street1     string `json:"street1"`
	Street2     string `json:"street2,omitempty"`
	City        string `json:"city"`
	Region      string `json:"region,omitempty"`
	PostalCode  string `json:"postalCode"`
	CountryCode string `json:"countryCode"`
}

// Point is somewhere a delivery can go other than the shopper's door - a store, a locker, a parcel shop, a PO box. Snapshotted onto the cart and the order, so later changes at the source don't rewrite either
type Point struct {
	ID           string            `json:"id"`
	Source       string            `json:"source"`
	Name         string            `json:"name"`
	Address      Address           `json:"address"`
	Lat          *float64          `json:"lat,omitempty"`
	Lng          *float64          `json:"lng,omitempty"`
	DistanceKm   *float64          `json:"distanceKm,omitempty"`
	Hours        string            `json:"hours,omitempty"`
	Instructions string            `json:"instructions,omitempty"`
	Phone        string            `json:"phone,omitempty"`
	Details      map[string]string `json:"details,omitempty"`
}

// Item is a line as a point source sees it
type Item struct {
	SKU      string  `json:"sku"`
	Quantity float64 `json:"quantity"`
}

// Near is what a search is near - the shopper's destination, as much of it as checkout has
type Near struct {
	CountryCode string `json:"countryCode"`
	Region      string `json:"region,omitempty"`
	PostalCode  string `json:"postalCode,omitempty"`
	City        string `json:"city,omitempty"`
	Street1     string `json:"street1,omitempty"`
}

type PointQuery struct {
	ChannelID uuid.UUID
	Kind      string
	Near      Near
	Search    string
	Items     []Item
	Limit     int
}

// PointSource finds points for a kind - Get re-reads one by id when the shopper picks it and again at placement, so a point that closed in between is refused
type PointSource interface {
	Search(ctx context.Context, q PointQuery) ([]Point, error)
	Get(ctx context.Context, channelID uuid.UUID, id string) (Point, error)
}

type PointSourceRegistration struct {
	Slug   string
	Label  string
	Kinds  []string // the kinds it serves - empty serves any kind that needs a point
	Source PointSource
	Owner  string
}
