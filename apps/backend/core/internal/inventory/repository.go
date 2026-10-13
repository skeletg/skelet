package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *txrunner.Manager
}

// NewRepository builds a postgres-backed inventory repository
func NewRepository(queries *db.Queries, txMgr *txrunner.Manager) Repository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

func (r *postgresRepository) WithTx(t *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(t), tx: r.tx}
}

// ==================== Locations ====================

func (r *postgresRepository) CreateLocation(ctx context.Context, input CreateLocationInput) (Location, error) {
	hours := input.Hours
	if len(hours) == 0 {
		hours = json.RawMessage(`{}`)
	}
	row, err := r.queries.CreateInventoryLocation(ctx, db.CreateInventoryLocationParams{
		Slug:              input.Slug,
		Name:              input.Name,
		IsActive:          input.IsActive,
		Line1:             input.Line1,
		Line2:             input.Line2,
		City:              input.City,
		Region:            input.Region,
		PostalCode:        input.PostalCode,
		CountryCode:       sqlnull.StringValueNonEmpty(input.CountryCode),
		Latitude:          floatToNullString(input.Latitude),
		Longitude:         floatToNullString(input.Longitude),
		Type:              input.Type,
		Timezone:          input.Timezone,
		Hours:             hours,
		Email:             input.Email,
		Phone:             input.Phone,
		ContactName:       input.ContactName,
		ExternalReference: input.ExternalReference,
		Metadata:          metadataToJSON(input.Metadata),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Location{}, ErrLocationSlugTaken
		}
		return Location{}, fmt.Errorf("create location: %w", err)
	}
	return toLocation(row), nil
}

func (r *postgresRepository) GetLocationByID(ctx context.Context, id uuid.UUID) (Location, error) {
	row, err := r.queries.GetInventoryLocationByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Location{}, ErrLocationNotFound
		}
		return Location{}, fmt.Errorf("get location: %w", err)
	}
	return toLocation(row), nil
}

func (r *postgresRepository) GetLocationBySlug(ctx context.Context, slug string) (Location, error) {
	row, err := r.queries.GetInventoryLocationBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Location{}, ErrLocationNotFound
		}
		return Location{}, fmt.Errorf("get location by slug: %w", err)
	}
	return toLocation(row), nil
}

func (r *postgresRepository) ListLocations(ctx context.Context) ([]Location, error) {
	rows, err := r.queries.ListInventoryLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	out := make([]Location, len(rows))
	for i, row := range rows {
		out[i] = toLocation(row)
	}
	return out, nil
}

func (r *postgresRepository) ListLocationsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Location, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]Location{}, nil
	}
	rows, err := r.queries.ListInventoryLocationsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list locations by ids: %w", err)
	}
	out := make(map[uuid.UUID]Location, len(rows))
	for _, row := range rows {
		out[row.ID] = toLocation(row)
	}
	return out, nil
}

func (r *postgresRepository) ListLocationsPage(ctx context.Context, search string, limit, offset int32) ([]Location, error) {
	rows, err := r.queries.ListInventoryLocationsPage(ctx, db.ListInventoryLocationsPageParams{Search: search, Lim: limit, Off: offset})
	if err != nil {
		return nil, fmt.Errorf("list locations page: %w", err)
	}
	out := make([]Location, len(rows))
	for i, row := range rows {
		out[i] = toLocation(row)
	}
	return out, nil
}

func (r *postgresRepository) CountLocations(ctx context.Context, search string) (int64, error) {
	n, err := r.queries.CountInventoryLocations(ctx, search)
	if err != nil {
		return 0, fmt.Errorf("count locations: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) UpdateLocation(ctx context.Context, input UpdateLocationInput) (Location, error) {
	current, err := r.queries.GetInventoryLocationByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Location{}, ErrLocationNotFound
		}
		return Location{}, fmt.Errorf("load location: %w", err)
	}

	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}
	isActive := current.IsActive
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	typ := current.Type
	if input.Type != nil {
		typ = *input.Type
	}
	line1 := current.Line1
	if input.Line1 != nil {
		line1 = *input.Line1
	}
	line2 := current.Line2
	if input.Line2 != nil {
		line2 = *input.Line2
	}
	city := current.City
	if input.City != nil {
		city = *input.City
	}
	region := current.Region
	if input.Region != nil {
		region = *input.Region
	}
	postalCode := current.PostalCode
	if input.PostalCode != nil {
		postalCode = *input.PostalCode
	}
	countryCode := current.CountryCode
	if input.CountryCode != nil {
		countryCode = sqlnull.StringValueNonEmpty(*input.CountryCode)
	}
	latitude := current.Latitude
	longitude := current.Longitude
	if input.ClearCoords {
		latitude = sql.NullString{}
		longitude = sql.NullString{}
	} else {
		if input.Latitude != nil {
			latitude = floatToNullString(input.Latitude)
		}
		if input.Longitude != nil {
			longitude = floatToNullString(input.Longitude)
		}
	}
	timezone := current.Timezone
	if input.Timezone != nil {
		timezone = *input.Timezone
	}
	hours := current.Hours
	if input.Hours != nil {
		raw := *input.Hours
		if len(raw) == 0 {
			raw = json.RawMessage(`{}`)
		}
		hours = raw
	}
	email := current.Email
	if input.Email != nil {
		email = *input.Email
	}
	phone := current.Phone
	if input.Phone != nil {
		phone = *input.Phone
	}
	contactName := current.ContactName
	if input.ContactName != nil {
		contactName = *input.ContactName
	}
	externalReference := current.ExternalReference
	if input.ExternalReference != nil {
		externalReference = *input.ExternalReference
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = metadataToJSON(*input.Metadata)
	}

	row, err := r.queries.UpdateInventoryLocation(ctx, db.UpdateInventoryLocationParams{
		ID:                input.ID,
		Name:              name,
		IsActive:          isActive,
		Line1:             line1,
		Line2:             line2,
		City:              city,
		Region:            region,
		PostalCode:        postalCode,
		CountryCode:       countryCode,
		Latitude:          latitude,
		Longitude:         longitude,
		Type:              typ,
		Timezone:          timezone,
		Hours:             hours,
		Email:             email,
		Phone:             phone,
		ContactName:       contactName,
		ExternalReference: externalReference,
		Metadata:          metadata,
	})
	if err != nil {
		return Location{}, fmt.Errorf("update location: %w", err)
	}
	return toLocation(row), nil
}

func (r *postgresRepository) DeleteLocation(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteInventoryLocation(ctx, id); err != nil {
		return fmt.Errorf("delete location: %w", err)
	}
	return nil
}

// ==================== Junction ====================

func (r *postgresRepository) ListChannelLocations(ctx context.Context, channelID uuid.UUID) ([]Location, error) {
	rows, err := r.queries.ListChannelLocations(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list channel locations: %w", err)
	}
	out := make([]Location, len(rows))
	for i, row := range rows {
		out[i] = Location{
			ID:                 row.ID,
			Slug:               row.Slug,
			Name:               row.Name,
			IsActive:           row.IsActive,
			Type:               row.Type,
			Line1:              row.Line1,
			Line2:              row.Line2,
			City:               row.City,
			Region:             row.Region,
			PostalCode:         row.PostalCode,
			CountryCode:        row.CountryCode.String,
			Latitude:           nullStringToFloat(row.Latitude),
			Longitude:          nullStringToFloat(row.Longitude),
			Timezone:           row.Timezone,
			Hours:              row.Hours,
			Email:              row.Email,
			Phone:              row.Phone,
			ContactName:        row.ContactName,
			ExternalReference:  row.ExternalReference,
			Metadata:           metadataFromJSON(row.Metadata),
			PickupEnabled:      row.PickupEnabled,
			PickupInstructions: row.PickupInstructions,
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
			Priority:           row.Priority,
		}
	}
	return out, nil
}

// SetChannelLocations atomically replaces the entire assignment list for
// one channel (delete + insert in a single tx)
func (r *postgresRepository) SetChannelLocations(ctx context.Context, channelID uuid.UUID, assignments []LocationAssignment) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.DeleteChannelLocations(ctx, channelID); err != nil {
			return fmt.Errorf("clear assignments: %w", err)
		}
		for _, a := range assignments {
			if err := q.InsertChannelLocation(ctx, db.InsertChannelLocationParams{
				ChannelID:  channelID,
				LocationID: a.LocationID,
				Priority:   a.Priority,
			}); err != nil {
				return fmt.Errorf("insert assignment: %w", err)
			}
		}
		return nil
	})
}

// ==================== Items ====================

func (r *postgresRepository) CreateItem(ctx context.Context, productID uuid.UUID, tracked, backorderable bool, lowStock float64) (Item, error) {
	row, err := r.queries.CreateInventoryItem(ctx, db.CreateInventoryItemParams{
		ProductID:         productID,
		Tracked:           tracked,
		Backorderable:     backorderable,
		LowStockThreshold: lowStock,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Item{}, ErrItemAlreadyExists
		}
		return Item{}, fmt.Errorf("create item: %w", err)
	}
	return toItem(row), nil
}

func (r *postgresRepository) GetItemByID(ctx context.Context, id uuid.UUID) (Item, error) {
	row, err := r.queries.GetInventoryItemByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Item{}, ErrItemNotFound
		}
		return Item{}, fmt.Errorf("get item: %w", err)
	}
	return toItem(row), nil
}

func (r *postgresRepository) GetItemByProductID(ctx context.Context, productID uuid.UUID) (Item, error) {
	row, err := r.queries.GetInventoryItemByProductID(ctx, productID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Item{}, ErrItemNotFound
		}
		return Item{}, fmt.Errorf("get item: %w", err)
	}
	return toItem(row), nil
}

func (r *postgresRepository) GetItemByProductIDForUpdate(ctx context.Context, productID uuid.UUID) (Item, error) {
	row, err := r.queries.GetInventoryItemByProductIDForUpdate(ctx, productID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Item{}, ErrItemNotFound
		}
		return Item{}, fmt.Errorf("get item for update: %w", err)
	}
	return toItem(row), nil
}

func (r *postgresRepository) ListItemsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]Item, error) {
	if len(productIDs) == 0 {
		return map[uuid.UUID]Item{}, nil
	}
	rows, err := r.queries.ListInventoryItemsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list items by product ids: %w", err)
	}
	out := make(map[uuid.UUID]Item, len(rows))
	for _, row := range rows {
		out[row.ProductID] = toItem(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateItem(ctx context.Context, id uuid.UUID, tracked, backorderable bool, lowStock float64) (Item, error) {
	row, err := r.queries.UpdateInventoryItem(ctx, db.UpdateInventoryItemParams{
		ID:                id,
		Tracked:           tracked,
		Backorderable:     backorderable,
		LowStockThreshold: lowStock,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Item{}, ErrItemNotFound
		}
		return Item{}, fmt.Errorf("update item: %w", err)
	}
	return toItem(row), nil
}

func (r *postgresRepository) DeleteItem(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteInventoryItem(ctx, id); err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	return nil
}

// AdjustItemQuantity applies a signed delta to inventory_items.quantity in one UPDATE (flat mode: negative on Reserve,
// positive on release) - Audit fields are left untouched - those are for operator/ERP sync writes
func (r *postgresRepository) AdjustItemQuantity(ctx context.Context, itemID uuid.UUID, delta float64) (Item, error) {
	row, err := r.queries.AdjustInventoryItemQuantity(ctx, db.AdjustInventoryItemQuantityParams{
		ID:       itemID,
		Quantity: delta,
	})
	if err != nil {
		return Item{}, fmt.Errorf("adjust item quantity: %w", err)
	}
	return toItem(row), nil
}

// SetItemQuantity is the flat-mode upsert: overwrites quantity + audit fields - Creates the row with default flags if
// it doesn't exist so an ERP push for a brand-new product Just Works
func (r *postgresRepository) SetItemQuantity(ctx context.Context, input SetItemQuantityInput) (Item, error) {
	row, err := r.queries.SetInventoryItemQuantity(ctx, db.SetInventoryItemQuantityParams{
		ProductID:        input.ProductID,
		Quantity:         input.Quantity,
		LastSyncedSource: sql.NullString{String: input.Source, Valid: input.Source != ""},
	})
	if err != nil {
		return Item{}, fmt.Errorf("set item quantity: %w", err)
	}
	return toItem(row), nil
}

// ==================== Levels ====================

func (r *postgresRepository) UpsertLevel(ctx context.Context, itemID, locationID uuid.UUID, quantity float64) (Level, error) {
	row, err := r.queries.UpsertInventoryLevel(ctx, db.UpsertInventoryLevelParams{
		InventoryItemID: itemID,
		LocationID:      locationID,
		Quantity:        quantity,
	})
	if err != nil {
		return Level{}, fmt.Errorf("upsert level: %w", err)
	}
	return toLevel(row), nil
}

func (r *postgresRepository) AdjustLevelQuantity(ctx context.Context, itemID, locationID uuid.UUID, delta float64) (Level, error) {
	row, err := r.queries.AdjustInventoryLevelQuantity(ctx, db.AdjustInventoryLevelQuantityParams{
		InventoryItemID: itemID,
		LocationID:      locationID,
		Quantity:        delta,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Level{}, ErrLevelNotFound
		}
		return Level{}, fmt.Errorf("adjust level: %w", err)
	}
	return toLevel(row), nil
}

func (r *postgresRepository) GetLevel(ctx context.Context, itemID, locationID uuid.UUID) (Level, error) {
	row, err := r.queries.GetInventoryLevel(ctx, db.GetInventoryLevelParams{
		InventoryItemID: itemID,
		LocationID:      locationID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Level{}, ErrLevelNotFound
		}
		return Level{}, fmt.Errorf("get level: %w", err)
	}
	return toLevel(row), nil
}

func (r *postgresRepository) GetLevelForUpdate(ctx context.Context, itemID, locationID uuid.UUID) (Level, error) {
	row, err := r.queries.GetInventoryLevelForUpdate(ctx, db.GetInventoryLevelForUpdateParams{
		InventoryItemID: itemID,
		LocationID:      locationID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Level{}, ErrLevelNotFound
		}
		return Level{}, fmt.Errorf("get level for update: %w", err)
	}
	return toLevel(row), nil
}

func (r *postgresRepository) ListLevelsByItem(ctx context.Context, itemID uuid.UUID) ([]Level, error) {
	rows, err := r.queries.ListInventoryLevelsByItem(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("list levels: %w", err)
	}
	out := make([]Level, len(rows))
	for i, row := range rows {
		out[i] = toLevel(row)
	}
	return out, nil
}

func (r *postgresRepository) ListLevelsByItems(ctx context.Context, itemIDs []uuid.UUID) ([]Level, error) {
	if len(itemIDs) == 0 {
		return []Level{}, nil
	}
	rows, err := r.queries.ListInventoryLevelsByItems(ctx, itemIDs)
	if err != nil {
		return nil, fmt.Errorf("list levels by items: %w", err)
	}
	out := make([]Level, len(rows))
	for i, row := range rows {
		out[i] = toLevel(row)
	}
	return out, nil
}

// ==================== Reservations ====================

func (r *postgresRepository) InsertReservation(ctx context.Context, res Reservation) (Reservation, error) {
	row, err := r.queries.InsertInventoryReservation(ctx, db.InsertInventoryReservationParams{
		InventoryItemID: res.InventoryItemID,
		LocationID:      res.LocationID,
		QuantityDelta:   res.QuantityDelta,
		Reason:          string(res.Reason),
		OrderID:         sqlnull.UUID(res.OrderID),
		OrderLineID:     sqlnull.UUID(res.OrderLineID),
		AdminID:         sqlnull.UUID(res.AdminID),
		Note:            res.Note,
	})
	if err != nil {
		return Reservation{}, fmt.Errorf("insert reservation: %w", err)
	}
	// Maintain the reserved counter in lockstep with the ledger (reserved += -delta), same tx as the insert manual_adjustment rows are quantity audit entries, not holds - bumping the counter for them corrupted it on every admin adjustment (phantom reserved on decreases, hidden negatives on increases)
	if res.Reason != ReasonManualAdjustment && res.QuantityDelta != 0 {
		if err := r.queries.AdjustInventoryLevelReserved(ctx, db.AdjustInventoryLevelReservedParams{
			InventoryItemID:  res.InventoryItemID,
			LocationID:       res.LocationID,
			ReservedQuantity: -res.QuantityDelta,
		}); err != nil {
			return Reservation{}, fmt.Errorf("maintain reserved counter: %w", err)
		}
	}
	return toReservation(row), nil
}

func (r *postgresRepository) AdjustLevelReserved(ctx context.Context, itemID, locationID uuid.UUID, delta float64) error {
	if err := r.queries.AdjustInventoryLevelReserved(ctx, db.AdjustInventoryLevelReservedParams{
		InventoryItemID:  itemID,
		LocationID:       locationID,
		ReservedQuantity: delta,
	}); err != nil {
		return fmt.Errorf("adjust level reserved: %w", err)
	}
	return nil
}

func (r *postgresRepository) RecomputeAllReserved(ctx context.Context) error {
	if err := r.queries.RecomputeAllLevelReserved(ctx); err != nil {
		return fmt.Errorf("recompute reserved: %w", err)
	}
	return nil
}

func (r *postgresRepository) BackfillItemQuantityFromLevels(ctx context.Context) (int64, error) {
	n, err := r.queries.BackfillItemQuantityFromLevels(ctx)
	if err != nil {
		return 0, fmt.Errorf("backfill item quantity from levels: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) PreviewItemQuantityFromLevels(ctx context.Context) ([]ModeBackfillRow, error) {
	rows, err := r.queries.PreviewItemQuantityFromLevels(ctx)
	if err != nil {
		return nil, fmt.Errorf("preview item quantity from levels: %w", err)
	}
	out := make([]ModeBackfillRow, len(rows))
	for i, row := range rows {
		out[i] = ModeBackfillRow{SKU: row.Sku, Current: row.CurrentQuantity, New: row.NewQuantity}
	}
	return out, nil
}

func (r *postgresRepository) BackfillLevelsFromItemQuantity(ctx context.Context, locationID uuid.UUID) (int64, error) {
	n, err := r.queries.BackfillLevelsFromItemQuantity(ctx, locationID)
	if err != nil {
		return 0, fmt.Errorf("backfill levels from item quantity: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) ZeroLevelsOutsideLocation(ctx context.Context, locationID uuid.UUID) (int64, error) {
	n, err := r.queries.ZeroLevelsOutsideLocation(ctx, locationID)
	if err != nil {
		return 0, fmt.Errorf("zero levels outside location: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) NetReservation(ctx context.Context, itemID, locationID uuid.UUID) (float64, error) {
	v, err := r.queries.SumReservationsForItemLocation(ctx, db.SumReservationsForItemLocationParams{
		InventoryItemID: itemID,
		LocationID:      locationID,
	})
	if err != nil {
		return 0, fmt.Errorf("sum reservations: %w", err)
	}
	return v, nil
}

func (r *postgresRepository) NetReservationsByPairs(ctx context.Context, itemIDs, locationIDs []uuid.UUID) (map[uuid.UUID]map[uuid.UUID]float64, error) {
	out := make(map[uuid.UUID]map[uuid.UUID]float64)
	if len(itemIDs) == 0 || len(locationIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.SumReservationsByItemLocations(ctx, db.SumReservationsByItemLocationsParams{
		Column1: itemIDs,
		Column2: locationIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("sum reservations batch: %w", err)
	}
	for _, row := range rows {
		byLoc, ok := out[row.InventoryItemID]
		if !ok {
			byLoc = make(map[uuid.UUID]float64)
			out[row.InventoryItemID] = byLoc
		}
		byLoc[row.LocationID] = row.NetDelta
	}
	return out, nil
}

func (r *postgresRepository) ListReservationsForOrder(ctx context.Context, orderID uuid.UUID) ([]Reservation, error) {
	rows, err := r.queries.ListReservationsForOrder(ctx, uuid.NullUUID{UUID: orderID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list reservations for order: %w", err)
	}
	out := make([]Reservation, len(rows))
	for i, row := range rows {
		out[i] = toReservation(row)
	}
	return out, nil
}

func (r *postgresRepository) DeleteReservationsForOrder(ctx context.Context, orderID uuid.UUID) error {
	if err := r.queries.DeleteSettledReservationsForOrder(ctx, uuid.NullUUID{UUID: orderID, Valid: true}); err != nil {
		return fmt.Errorf("delete reservations: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListSettledOrderIDsBefore(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error) {
	rows, err := r.queries.ListSettledOrderIDsBefore(ctx, cutoff)
	if err != nil {
		return nil, fmt.Errorf("list settled orders: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, n := range rows {
		if n.Valid {
			out = append(out, n.UUID)
		}
	}
	return out, nil
}

// ==================== Dashboard helpers ====================

func (r *postgresRepository) ListLowStockLevels(ctx context.Context, limit int32) ([]LowStockLevel, error) {
	rows, err := r.queries.ListLowStockLevels(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list low stock levels: %w", err)
	}
	out := make([]LowStockLevel, len(rows))
	for i, row := range rows {
		lsl := LowStockLevel{
			InventoryItemID:   row.InventoryItemID,
			ProductID:         row.ProductID,
			ProductSlug:       row.ProductSlug,
			ProductSKU:        row.ProductSku,
			QuantityDecimals:  row.QuantityDecimals,
			QuantityStep:      row.QuantityStep,
			Quantity:          row.Quantity,
			LowStockThreshold: row.LowStockThreshold,
			Backorderable:     row.Backorderable,
			UpdatedAt:         row.LevelUpdatedAt,
		}
		if row.LocationID.Valid {
			id := row.LocationID.UUID
			lsl.LocationID = &id
		}
		if row.LocationSlug.Valid {
			s := row.LocationSlug.String
			lsl.LocationSlug = &s
		}
		if row.LocationName.Valid {
			s := row.LocationName.String
			lsl.LocationName = &s
		}
		out[i] = lsl
	}
	return out, nil
}

// ListLowStockItemsFlat is the flat-mode counterpart - rows have LocationID nil
// (dashboard renders "(global)"), reusing LowStockLevel to avoid a second type
func (r *postgresRepository) ListLowStockItemsFlat(ctx context.Context, limit int32) ([]LowStockLevel, error) {
	rows, err := r.queries.ListLowStockItemsFlat(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list low stock items flat: %w", err)
	}
	out := make([]LowStockLevel, len(rows))
	for i, row := range rows {
		out[i] = LowStockLevel{
			InventoryItemID:   row.InventoryItemID,
			ProductID:         row.ProductID,
			ProductSlug:       row.ProductSlug,
			ProductSKU:        row.ProductSku,
			QuantityDecimals:  row.QuantityDecimals,
			QuantityStep:      row.QuantityStep,
			Quantity:          row.Quantity,
			LowStockThreshold: row.LowStockThreshold,
			Backorderable:     row.Backorderable,
			UpdatedAt:         row.LevelUpdatedAt,
		}
	}
	return out, nil
}

// ==================== mappers ====================

func toLocation(row db.InventoryLocation) Location {
	return Location{
		PickupEnabled:      row.PickupEnabled,
		PickupInstructions: row.PickupInstructions,
		ID:                 row.ID,
		Slug:               row.Slug,
		Name:               row.Name,
		IsActive:           row.IsActive,
		Type:               row.Type,
		Line1:              row.Line1,
		Line2:              row.Line2,
		City:               row.City,
		Region:             row.Region,
		PostalCode:         row.PostalCode,
		CountryCode:        row.CountryCode.String,
		Latitude:           nullStringToFloat(row.Latitude),
		Longitude:          nullStringToFloat(row.Longitude),
		Timezone:           row.Timezone,
		Hours:              row.Hours,
		Email:              row.Email,
		Phone:              row.Phone,
		ContactName:        row.ContactName,
		ExternalReference:  row.ExternalReference,
		Metadata:           metadataFromJSON(row.Metadata),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func toItem(row db.InventoryItem) Item {
	item := Item{
		ID:                row.ID,
		ProductID:         row.ProductID,
		Tracked:           row.Tracked,
		Backorderable:     row.Backorderable,
		LowStockThreshold: row.LowStockThreshold,
		Quantity:          row.Quantity,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
	if row.LastSyncedAt.Valid {
		t := row.LastSyncedAt.Time
		item.LastSyncedAt = &t
	}
	if row.LastSyncedSource.Valid {
		s := row.LastSyncedSource.String
		item.LastSyncedSource = &s
	}
	return item
}

func toLevel(row db.InventoryLevel) Level {
	return Level{
		ID:               row.ID,
		InventoryItemID:  row.InventoryItemID,
		LocationID:       row.LocationID,
		Quantity:         row.Quantity,
		ReservedQuantity: row.ReservedQuantity,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func toReservation(row db.InventoryReservation) Reservation {
	r := Reservation{
		ID:              row.ID,
		InventoryItemID: row.InventoryItemID,
		LocationID:      row.LocationID,
		QuantityDelta:   row.QuantityDelta,
		Reason:          Reason(row.Reason),
		Note:            row.Note,
		CreatedAt:       row.CreatedAt,
	}
	if row.OrderID.Valid {
		id := row.OrderID.UUID
		r.OrderID = &id
	}
	if row.OrderLineID.Valid {
		id := row.OrderLineID.UUID
		r.OrderLineID = &id
	}
	if row.AdminID.Valid {
		id := row.AdminID.UUID
		r.AdminID = &id
	}
	return r
}

// floatToNullString round-trips an optional float to the sql.NullString sqlc
// emits for NUMERIC - avoids pgtype.Numeric, lossless for DECIMAL(9,6)
func floatToNullString(p *float64) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{
		String: strconv.FormatFloat(*p, 'f', -1, 64),
		Valid:  true,
	}
}

func nullStringToFloat(n sql.NullString) *float64 {
	if !n.Valid || n.String == "" {
		return nil
	}
	v, err := strconv.ParseFloat(n.String, 64)
	if err != nil {
		return nil
	}
	return &v
}

func metadataToJSON(m Metadata) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage(`{}`)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func metadataFromJSON(raw json.RawMessage) Metadata {
	if len(raw) == 0 {
		return Metadata{}
	}
	var m Metadata
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return Metadata{}
	}
	return m
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching

func (r *postgresRepository) ListPickupLocations(ctx context.Context) ([]Location, error) {
	rows, err := r.queries.ListPickupLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pickup locations: %w", err)
	}
	out := make([]Location, len(rows))
	for i, row := range rows {
		out[i] = toLocation(row)
	}
	return out, nil
}

func (r *postgresRepository) SetLocationPickup(ctx context.Context, id uuid.UUID, enabled bool, instructions string) (Location, error) {
	row, err := r.queries.SetLocationPickup(ctx, db.SetLocationPickupParams{
		ID:                 id,
		PickupEnabled:      enabled,
		PickupInstructions: instructions,
	})
	if err != nil {
		return Location{}, fmt.Errorf("set location pickup: %w", err)
	}
	return toLocation(row), nil
}
