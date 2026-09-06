// Package inventory owns physical-stock bookkeeping - locations, per-product Items, per-(item, location) Levels, and an append-only reservation ledger decoupling physical from salable stock across placement to fulfillment
// Tracking is opt-in: products inherit tracks_inventory from their type with a per-product override, and a tracked=false item is managed externally (ERP), so the ledger short-circuits and raw inventory_levels.quantity is the truth
package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/permission"
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "inventory",
		Name:      "Inventory",
		SortOrder: 70,
	})
	permission.Register(permission.Definition{
		Group: "inventory",
		Slug:  "inventory.view",
		Name:  "View inventory",
	})
	permission.Register(permission.Definition{
		Group: "inventory",
		Slug:  "inventory.manage",
		Name:  "Manage inventory",
	})
}

// Tags a reservation row and fixes its quantity_delta sign - order_placed is negative, the rest positive or zero
type Reason string

const (
	ReasonOrderPlaced      Reason = "order_placed"
	ReasonOrderCancelled   Reason = "order_cancelled"
	ReasonOrderFulfilled   Reason = "order_fulfilled"
	ReasonManualAdjustment Reason = "manual_adjustment"
	ReasonOrderReturned    Reason = "order_returned"
	ReasonOrderEdited      Reason = "order_edited"
	// The deduct-at-placement comp: positive when placement consumes a hold on the spot, negative
	// when a cancel or edit restores - so a chain's order_deducted sum IS its outstanding deduction
	ReasonOrderDeducted Reason = "order_deducted"
)

// A physical place stock is held - warehouse, shop, drop-shipper - A channel sees N locations through the inventory_channel_locations junction
type Location struct {
	ID       uuid.UUID
	Slug     string
	Name     string
	IsActive bool

	Type string // free-form admin classifier ("warehouse", "retail", …), empty when unset

	// Structured address - CountryCode references countries.code, uppercased
	Line1       string
	Line2       string
	City        string
	Region      string
	PostalCode  string
	CountryCode string

	Latitude  *float64
	Longitude *float64

	Timezone          string          // IANA, e.g. "Europe/Berlin"
	Hours             json.RawMessage // opaque per-day schedule, admin UI is the only consumer
	Email             string
	Phone             string
	ContactName       string
	ExternalReference string // ERP/WMS sync code

	Metadata Metadata // free-form bucket for everything else

	// Pickup-enabled active locations surface as $0 shipping via the builtin "pickup" carrier
	PickupEnabled      bool
	PickupInstructions string

	CreatedAt time.Time
	UpdatedAt time.Time
	// Set only when listing for a channel - lower is preferred for allocation
	Priority int32
}

type Metadata map[string]string

// The inventory record for one product, one row when tracked - an absent row reads as "always in stock", preserving behaviour for un-opted products
// Quantity and LastSynced* are the flat-mode fields, ignored by multi-location salability math but still written by SetItemQuantity so the audit trail survives a mode flip
type Item struct {
	ID                uuid.UUID
	ProductID         uuid.UUID
	Tracked           bool
	Backorderable     bool
	LowStockThreshold float64
	Quantity          float64
	LastSyncedAt      *time.Time
	LastSyncedSource  *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Level struct {
	ID              uuid.UUID
	InventoryItemID uuid.UUID
	LocationID      uuid.UUID
	Quantity        float64
	// A denormalized cache of -(ledger net) at this cell, maintained in the reservation tx, so salable is Quantity - ReservedQuantity read off the row instead of aggregating the ledger - Rebuildable via reconcile
	ReservedQuantity float64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// One append-only ledger row - QuantityDelta is negative when stock is held at placement, positive when freed on cancel or finalised on fulfill
type Reservation struct {
	ID              uuid.UUID
	InventoryItemID uuid.UUID
	LocationID      uuid.UUID
	QuantityDelta   float64
	Reason          Reason
	OrderID         *uuid.UUID
	OrderLineID     *uuid.UUID
	AdminID         *uuid.UUID
	Note            string
	CreatedAt       time.Time
}

type CreateLocationInput struct {
	Slug     string
	Name     string
	IsActive bool
	Type     string

	Line1       string
	Line2       string
	City        string
	Region      string
	PostalCode  string
	CountryCode string

	Latitude  *float64
	Longitude *float64

	Timezone          string
	Hours             json.RawMessage // nil treated as '{}' at the repo layer
	Email             string
	Phone             string
	ContactName       string
	ExternalReference string

	Metadata Metadata
}

type UpdateLocationInput struct {
	ID       uuid.UUID
	Name     *string
	IsActive *bool
	Type     *string

	Line1       *string
	Line2       *string
	City        *string
	Region      *string
	PostalCode  *string
	CountryCode *string // pointer-to-empty-string clears the FK to NULL

	// Latitude/Longitude are tri-state - nil = no change, set = set, ClearCoords = clear
	Latitude    *float64
	Longitude   *float64
	ClearCoords bool

	Timezone          *string
	Hours             *json.RawMessage // nil = no change; pointer to empty raw = clear to '{}'
	Email             *string
	Phone             *string
	ContactName       *string
	ExternalReference *string

	Metadata *Metadata
}

// Binds a location to a channel at a priority - SetChannelLocations replaces the whole list
type LocationAssignment struct {
	LocationID uuid.UUID
	Priority   int32
}

// One line to reserve for - the service resolves productID to an item and picks locations by the channel's priority order
type ReserveLine struct {
	ProductID   uuid.UUID
	OrderLineID uuid.UUID
	Quantity    float64
}

type ReserveAllocation struct {
	InventoryItemID uuid.UUID
	LocationID      uuid.UUID
	OrderLineID     uuid.UUID
	Quantity        float64
}

// Bundles a Reserve's allocations so callers can compensate on failure
type ReservationResult struct {
	Allocations []ReserveAllocation
}

// Sets a (item, location) quantity to a target - the service writes a manual_adjustment row for the delta and updates quantity atomically
type AdjustLevelInput struct {
	InventoryItemID uuid.UUID
	LocationID      uuid.UUID
	NewQuantity     float64
	AdminID         *uuid.UUID
	Note            string
}

// The flat-mode write - one quantity per product, no location - Source is free-text attribution for the "Last synced X by Y" audit line
type SetItemQuantityInput struct {
	ProductID uuid.UUID
	Quantity  float64
	Source    string
}

type SalableQuery struct {
	InventoryItemID uuid.UUID
	ChannelID       uuid.UUID
}

// One dashboard low-stock alert row, with threshold=0 items excluded, in two flavours
// Per-location has LocationID/slug/name set and Quantity at or below threshold; "unstocked" is tracked with threshold>0 but no inventory_levels anywhere, so the location fields are nil and Quantity 0 - it flags a setup gap that would otherwise hide
type LowStockLevel struct {
	InventoryItemID   uuid.UUID
	ProductID         uuid.UUID
	ProductSlug       string
	ProductSKU        string
	QuantityDecimals  int16
	QuantityStep      float64
	LocationID        *uuid.UUID // nil = unstocked alert
	LocationSlug      *string
	LocationName      *string
	Quantity          float64
	LowStockThreshold float64
	Backorderable     bool
	UpdatedAt         time.Time
}

// One product's before/after for a mode-flip dry run
type ModeBackfillRow struct {
	SKU     string
	Current float64
	New     float64
}

var (
	ErrLocationNotFound      = errors.New("inventory location not found")
	ErrLocationSlugTaken     = errors.New("inventory location slug already in use")
	ErrItemNotFound          = errors.New("inventory item not found")
	ErrItemAlreadyExists     = errors.New("inventory item already exists for this product")
	ErrLevelNotFound         = errors.New("inventory level not found")
	ErrInsufficientStock     = errors.New("insufficient stock")
	ErrNoLocationsForChannel = errors.New("channel has no inventory locations assigned")
	ErrNegativeQuantity      = errors.New("quantity cannot be negative")
	ErrFlatModePerLocation   = errors.New("per-location adjustments are disabled in flat inventory mode; use syncProductInventory")
)

// Safe for concurrent use - the ledger keeps writes commutative, so only per-row ordering matters
type Repository interface {
	WithTx(tx *sql.Tx) Repository

	CreateLocation(ctx context.Context, input CreateLocationInput) (Location, error)
	GetLocationByID(ctx context.Context, id uuid.UUID) (Location, error)
	GetLocationBySlug(ctx context.Context, slug string) (Location, error)
	ListLocations(ctx context.Context) ([]Location, error)
	ListLocationsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Location, error)
	ListLocationsPage(ctx context.Context, search string, limit, offset int32) ([]Location, error)
	CountLocations(ctx context.Context, search string) (int64, error)
	ListPickupLocations(ctx context.Context) ([]Location, error)
	SetLocationPickup(ctx context.Context, id uuid.UUID, enabled bool, instructions string) (Location, error)
	UpdateLocation(ctx context.Context, input UpdateLocationInput) (Location, error)
	DeleteLocation(ctx context.Context, id uuid.UUID) error

	ListChannelLocations(ctx context.Context, channelID uuid.UUID) ([]Location, error)
	SetChannelLocations(ctx context.Context, channelID uuid.UUID, assignments []LocationAssignment) error

	CreateItem(ctx context.Context, productID uuid.UUID, tracked, backorderable bool, lowStock float64) (Item, error)
	GetItemByID(ctx context.Context, id uuid.UUID) (Item, error)
	GetItemByProductID(ctx context.Context, productID uuid.UUID) (Item, error)
	// Row-locks the item so flat-mode reservation's stock check can't race
	GetItemByProductIDForUpdate(ctx context.Context, productID uuid.UUID) (Item, error)
	ListItemsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]Item, error)
	UpdateItem(ctx context.Context, id uuid.UUID, tracked, backorderable bool, lowStock float64) (Item, error)
	SetItemQuantity(ctx context.Context, input SetItemQuantityInput) (Item, error)
	AdjustItemQuantity(ctx context.Context, itemID uuid.UUID, delta float64) (Item, error)
	DeleteItem(ctx context.Context, id uuid.UUID) error

	UpsertLevel(ctx context.Context, itemID, locationID uuid.UUID, quantity float64) (Level, error)
	AdjustLevelQuantity(ctx context.Context, itemID, locationID uuid.UUID, delta float64) (Level, error)
	GetLevel(ctx context.Context, itemID, locationID uuid.UUID) (Level, error)
	// Row-locks the level so concurrent reservers of one cell can't oversell
	GetLevelForUpdate(ctx context.Context, itemID, locationID uuid.UUID) (Level, error)
	ListLevelsByItem(ctx context.Context, itemID uuid.UUID) ([]Level, error)
	ListLevelsByItems(ctx context.Context, itemIDs []uuid.UUID) ([]Level, error)
	// Bumps the denormalized reserved counter, in lockstep with the ledger
	AdjustLevelReserved(ctx context.Context, itemID, locationID uuid.UUID, delta float64) error
	// Rebuilds every reserved_quantity from the ledger, for reconcile and drift repair
	RecomputeAllReserved(ctx context.Context) error

	// The mode-flip backfills - see Service.MigrateToFlatMode / MigrateToMultiLocationMode
	BackfillItemQuantityFromLevels(ctx context.Context) (int64, error)
	PreviewItemQuantityFromLevels(ctx context.Context) ([]ModeBackfillRow, error)
	BackfillLevelsFromItemQuantity(ctx context.Context, locationID uuid.UUID) (int64, error)
	ZeroLevelsOutsideLocation(ctx context.Context, locationID uuid.UUID) (int64, error)

	InsertReservation(ctx context.Context, r Reservation) (Reservation, error)
	NetReservation(ctx context.Context, itemID, locationID uuid.UUID) (float64, error)
	NetReservationsByPairs(ctx context.Context, itemIDs, locationIDs []uuid.UUID) (map[uuid.UUID]map[uuid.UUID]float64, error)
	ListReservationsForOrder(ctx context.Context, orderID uuid.UUID) ([]Reservation, error)
	DeleteReservationsForOrder(ctx context.Context, orderID uuid.UUID) error
	ListSettledOrderIDsBefore(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error)

	ListLowStockLevels(ctx context.Context, limit int32) ([]LowStockLevel, error)
	ListLowStockItemsFlat(ctx context.Context, limit int32) ([]LowStockLevel, error)
}
