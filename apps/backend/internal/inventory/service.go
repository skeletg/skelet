package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
)

// The slim catalog view inventory needs to resolve SKUs to items - local, so the inventory to catalog dependency stays one-way
type ProductLookup interface {
	GetProductIDBySKU(ctx context.Context, sku string) (uuid.UUID, error)
	// A kit's component breakdown (product id plus qty per unit), nil for non-kits, used to expand a kit line into per-component reservations
	KitComponents(ctx context.Context, productID uuid.UUID) ([]KitComponentRef, error)
}

type KitComponentRef struct {
	ProductID uuid.UUID
	Quantity  float64
}

// OrderLineLookup fetches a placed order's lines - flat mode restores stock on cancellation with it, having no ledger to read back - Local so inventory doesn't import order
type OrderLine struct {
	ProductID uuid.UUID
	Quantity  float64
	// How much of this line already shipped - flat mode's restore subtracts it, since those units
	// left the building and must not come back - the placement announcement ignores it and uses the
	// full ordered Quantity - zero for lookups that don't track shipments
	FulfilledQuantity float64
	// Line identity for the deduct-at-placement paths, which key ledger chains by order line -
	// LineQuantity/LineFulfilled are in LINE units (a component row's Quantity is not), repeated
	// verbatim on every row of the same line, and zero-valued for lookups that don't provide them
	LineID        uuid.UUID
	LineQuantity  float64
	LineFulfilled float64
}
type OrderLineLookup interface {
	GetOrderLines(ctx context.Context, orderID uuid.UUID) ([]OrderLine, error)
}

// The global ledger_enabled flag - false short-circuits every stock write in BOTH modes, for cutovers from a platform that owns inventory truth
type LedgerToggle interface {
	LedgerEnabled(ctx context.Context) bool
}

// The global flat_mode flag - true makes the multi-location machinery dormant: Salable reads item.Quantity, placement decrements it atomically (no ledger, no locations), cancellation restores it, and ERP pushes go through SetItemQuantity
type FlatModeToggle interface {
	FlatModeEnabled(ctx context.Context) bool
}

// The sales/inventory/deduction_step select - true moves the on-hand decrement from fulfillment
// to placement in multi-location mode. Flat mode always deducts at placement and ignores this.
// The setting only steers NEW writes: every read-back (cancel, edit, return) is driven by what
// the ledger says actually happened, so flipping it with orders in flight stays consistent
type DeductionStepToggle interface {
	DeductAtPlacement(ctx context.Context) bool
}

type Service struct {
	repo     Repository
	tx       *tx.Manager
	products ProductLookup
	orders   OrderLineLookup
	ledger   LedgerToggle
	flatMode FlatModeToggle
	deduct   DeductionStepToggle
	bus      *events.Bus
	// Sources salable stock from an external WMS or ERP when wired - nil means native (remote.go)
	remoteAvail RemoteAvailabilitySource
}

func NewService(repo Repository, txMgr *tx.Manager) *Service {
	return &Service{repo: repo, tx: txMgr}
}

// Optional - wires the bus so inventory emits stock_low and stock_out
func (s *Service) SetEventBus(b *events.Bus) { s.bus = b }

// Optional - when nil, callers pass inventory item IDs directly
func (s *Service) SetProductLookup(p ProductLookup) { s.products = p }

// Wires the flat-mode cancellation restore - optional in multi-location, where the ledger drives it
func (s *Service) SetOrderLineLookup(o OrderLineLookup) { s.orders = o }

func (s *Service) SetLedgerToggle(t LedgerToggle) { s.ledger = t }

func (s *Service) SetFlatModeToggle(t FlatModeToggle) { s.flatMode = t }

func (s *Service) SetDeductionStepToggle(t DeductionStepToggle) { s.deduct = t }

func (s *Service) deductAtPlacement(ctx context.Context) bool {
	if s.deduct == nil {
		return false // fulfillment-time deduction, the historical behaviour, is the safe default
	}
	return s.deduct.DeductAtPlacement(ctx)
}

func (s *Service) ledgerEnabled(ctx context.Context) bool {
	if s.ledger == nil {
		return true
	}
	return s.ledger.LedgerEnabled(ctx)
}

func (s *Service) flatModeEnabled(ctx context.Context) bool {
	if s.flatMode == nil {
		return false
	}
	return s.flatMode.FlatModeEnabled(ctx)
}

// ==================== Locations ====================

func (s *Service) CreateLocation(ctx context.Context, input CreateLocationInput) (Location, error) {
	if err := validateSlug(input.Slug); err != nil {
		return Location{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return Location{}, fmt.Errorf("name is required")
	}
	input.CountryCode = strings.ToUpper(strings.TrimSpace(input.CountryCode))
	if err := validateCoords(input.Latitude, input.Longitude); err != nil {
		return Location{}, err
	}
	return s.repo.CreateLocation(ctx, input)
}

func (s *Service) GetLocation(ctx context.Context, id uuid.UUID) (Location, error) {
	return s.repo.GetLocationByID(ctx, id)
}

// Slug lookup - the CLI names locations by slug, being the stable handle an operator types
func (s *Service) GetLocationBySlug(ctx context.Context, slug string) (Location, error) {
	return s.repo.GetLocationBySlug(ctx, slug)
}

func (s *Service) ListLocations(ctx context.Context) ([]Location, error) {
	return s.repo.ListLocations(ctx)
}

func (s *Service) GetLocationsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Location, error) {
	return s.repo.ListLocationsByIDs(ctx, ids)
}

func (s *Service) CountLocations(ctx context.Context, search string) (int64, error) {
	return s.repo.CountLocations(ctx, strings.TrimSpace(search))
}

// ListLocationsPage answers the dashboard grid server-side - a nil-search full page keeps parity with ListLocations ordering
func (s *Service) ListLocationsPage(ctx context.Context, search string, limit, offset int32) ([]Location, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	search = strings.TrimSpace(search)
	locs, err := s.repo.ListLocationsPage(ctx, search, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.CountLocations(ctx, search)
	if err != nil {
		return nil, 0, err
	}
	return locs, total, nil
}

func (s *Service) PickupLocations(ctx context.Context) ([]Location, error) {
	return s.repo.ListPickupLocations(ctx)
}

func (s *Service) SetLocationPickup(ctx context.Context, id uuid.UUID, enabled bool, instructions string) (Location, error) {
	return s.repo.SetLocationPickup(ctx, id, enabled, instructions)
}

func (s *Service) UpdateLocation(ctx context.Context, input UpdateLocationInput) (Location, error) {
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return Location{}, fmt.Errorf("name is required")
	}
	if input.CountryCode != nil {
		v := strings.ToUpper(strings.TrimSpace(*input.CountryCode))
		input.CountryCode = &v
	}
	if err := validateCoords(input.Latitude, input.Longitude); err != nil {
		return Location{}, err
	}
	return s.repo.UpdateLocation(ctx, input)
}

func (s *Service) DeleteLocation(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteLocation(ctx, id)
}

// Replaces a channel's whole assignment list - lower priority is preferred
func (s *Service) SetChannelLocations(ctx context.Context, channelID uuid.UUID, assignments []LocationAssignment) error {
	return s.repo.SetChannelLocations(ctx, channelID, assignments)
}

// Wires a new channel to one location by slug at priority 0, the shared-warehouse channel-create hook - best-effort, returning nil on an unresolvable slug so it can't fail an otherwise-successful store creation
func (s *Service) AssignDefaultLocation(ctx context.Context, channelID uuid.UUID, locationSlug string) error {
	loc, err := s.repo.GetLocationBySlug(ctx, locationSlug)
	if err != nil {
		return nil
	}
	return s.repo.SetChannelLocations(ctx, channelID, []LocationAssignment{
		{LocationID: loc.ID, Priority: 0},
	})
}

func (s *Service) ListChannelLocations(ctx context.Context, channelID uuid.UUID) ([]Location, error) {
	return s.repo.ListChannelLocations(ctx, channelID)
}

// ==================== Items ====================

// Inserts an inventory_items row for the product or returns the existing one - idempotent
func (s *Service) EnsureItem(ctx context.Context, productID uuid.UUID, tracked bool) (Item, error) {
	if existing, err := s.repo.GetItemByProductID(ctx, productID); err == nil {
		// Trust the existing tracked flag over the requested one, since an admin may have set it - flips go through SetItemTracking
		return existing, nil
	} else if !errors.Is(err, ErrItemNotFound) {
		return Item{}, err
	}
	return s.repo.CreateItem(ctx, productID, tracked, false, 0)
}

// ErrItemNotFound when untracked
func (s *Service) GetItemByProduct(ctx context.Context, productID uuid.UUID) (Item, error) {
	return s.repo.GetItemByProductID(ctx, productID)
}

// Provisions the row on first use - flipping off keeps the row, levels and ledger and just stops gating sales, so the toggle is reversible without data loss
func (s *Service) SetItemTracking(ctx context.Context, productID uuid.UUID, tracked bool) (Item, error) {
	item, err := s.repo.GetItemByProductID(ctx, productID)
	if err != nil {
		if errors.Is(err, ErrItemNotFound) {
			return s.repo.CreateItem(ctx, productID, tracked, false, 0)
		}
		return Item{}, err
	}
	return s.repo.UpdateItem(ctx, item.ID, tracked, item.Backorderable, item.LowStockThreshold)
}

// The update-path counterpart to EnsureItem, reconciling the item's tracked flag with the catalog's effective decision - idempotent
// tracked=false flips an existing row off but never creates one: an absent row is the "always in stock" sentinel, whereas a tracked=false row would make Salable read the zero sum and report out of stock
func (s *Service) SyncProductTracking(ctx context.Context, productID uuid.UUID, tracked bool) (Item, error) {
	item, err := s.repo.GetItemByProductID(ctx, productID)
	if err != nil {
		if errors.Is(err, ErrItemNotFound) {
			if !tracked {
				return Item{}, nil
			}
			return s.repo.CreateItem(ctx, productID, true, false, 0)
		}
		return Item{}, err
	}
	if item.Tracked == tracked {
		return item, nil
	}
	return s.repo.UpdateItem(ctx, item.ID, tracked, item.Backorderable, item.LowStockThreshold)
}

func (s *Service) SetItemBackorder(ctx context.Context, productID uuid.UUID, backorderable bool, lowStockThreshold float64) (Item, error) {
	item, err := s.repo.GetItemByProductID(ctx, productID)
	if err != nil {
		return Item{}, err
	}
	return s.repo.UpdateItem(ctx, item.ID, item.Tracked, backorderable, lowStockThreshold)
}

// ==================== Levels ====================

// Sets a (item, location) quantity to a target atomically, writing a manual_adjustment row for the delta - rejected in flat mode, where SetItemQuantity is the write
func (s *Service) AdjustLevel(ctx context.Context, input AdjustLevelInput) (Level, error) {
	if s.flatModeEnabled(ctx) {
		return Level{}, ErrFlatModePerLocation
	}
	if input.NewQuantity < 0 {
		return Level{}, ErrNegativeQuantity
	}
	var (
		out       Level
		before    float64
		threshold float64
		productID uuid.UUID
	)
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)

		// Capture the threshold and product mapping for the post-tx event
		if item, err := r.GetItemByID(ctx, input.InventoryItemID); err == nil {
			threshold = item.LowStockThreshold
			productID = item.ProductID
		}

		// Compute the delta against the current level, or 0 if none
		var current float64
		if existing, err := r.GetLevel(ctx, input.InventoryItemID, input.LocationID); err == nil {
			current = existing.Quantity
		} else if !errors.Is(err, ErrLevelNotFound) {
			return err
		}
		before = current
		level, err := r.UpsertLevel(ctx, input.InventoryItemID, input.LocationID, input.NewQuantity)
		if err != nil {
			return err
		}
		delta := input.NewQuantity - current
		if delta != 0 {
			_, err = r.InsertReservation(ctx, Reservation{
				InventoryItemID: input.InventoryItemID,
				LocationID:      input.LocationID,
				QuantityDelta:   delta,
				Reason:          ReasonManualAdjustment,
				AdminID:         input.AdminID,
				Note:            input.Note,
			})
			if err != nil {
				return err
			}
		}
		out = level
		return nil
	})
	if err != nil {
		return Level{}, err
	}
	// Threshold-crossing events fire post-commit so subscribers only see committed state - stock_out on >0 to <=0, stock_low on >threshold to <=threshold while positive, and threshold=0 disables it
	if s.bus != nil {
		now := out.Quantity
		s.bus.Publish(ctx, &StockChanged{
			InventoryItemID: input.InventoryItemID,
			LocationID:      input.LocationID,
			ProductID:       productID,
			Before:          before,
			After:           now,
			Delta:           now - before,
			Reason:          ReasonManualAdjustment,
		})
		if before > 0 && now <= 0 {
			s.bus.Publish(ctx, &StockOut{
				InventoryItemID: input.InventoryItemID,
				LocationID:      input.LocationID,
				ProductID:       productID,
			})
		} else if threshold > 0 && before > threshold && now <= threshold && now > 0 {
			s.bus.Publish(ctx, &StockLow{
				InventoryItemID: input.InventoryItemID,
				LocationID:      input.LocationID,
				ProductID:       productID,
				Quantity:        now,
				Threshold:       threshold,
			})
		}
	}
	return out, nil
}

// The flat-mode write - overwrite the per-product quantity plus audit trail - It works in both modes, since multi-location still updates the audit fields so a later flip has a meaningful "last synced"
// Fires the same events as AdjustLevel, with LocationID = uuid.Nil
func (s *Service) SetItemQuantity(ctx context.Context, input SetItemQuantityInput) (Item, error) {
	if input.Quantity < 0 {
		return Item{}, ErrNegativeQuantity
	}
	// Prior quantity and threshold for the post-tx event
	var (
		before    float64
		threshold float64
	)
	if existing, err := s.repo.GetItemByProductID(ctx, input.ProductID); err == nil {
		before = existing.Quantity
		threshold = existing.LowStockThreshold
	} else if !errors.Is(err, ErrItemNotFound) {
		return Item{}, err
	}
	item, err := s.repo.SetItemQuantity(ctx, input)
	if err != nil {
		return Item{}, err
	}
	if s.bus != nil {
		now := item.Quantity
		s.bus.Publish(ctx, &StockChanged{
			InventoryItemID: item.ID,
			ProductID:       item.ProductID,
			Before:          before,
			After:           now,
			Delta:           now - before,
			Reason:          ReasonManualAdjustment,
		})
		if before > 0 && now <= 0 {
			s.bus.Publish(ctx, &StockOut{
				InventoryItemID: item.ID,
				ProductID:       item.ProductID,
			})
		} else if threshold > 0 && before > threshold && now <= threshold && now > 0 {
			s.bus.Publish(ctx, &StockLow{
				InventoryItemID: item.ID,
				ProductID:       item.ProductID,
				Quantity:        now,
				Threshold:       threshold,
			})
		}
	}
	return item, nil
}

func (s *Service) ListLevelsForItem(ctx context.Context, itemID uuid.UUID) ([]Level, error) {
	return s.repo.ListLevelsByItem(ctx, itemID)
}

// The net reservation total per location for one item - GraphQL resolvers use it to render reservedQuantity per level without re-walking the ledger
func (s *Service) NetReservationsForItem(ctx context.Context, itemID uuid.UUID) (map[uuid.UUID]float64, error) {
	levels, err := s.repo.ListLevelsByItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]float64, len(levels))
	for _, lvl := range levels {
		// reserved_quantity is the denormalized cache of -(ledger net), kept in sync by the reservation tx and the same source Salable reads - using it avoids re-walking the ledger once per location, which was an N+1
		out[lvl.LocationID] = -lvl.ReservedQuantity
	}
	return out, nil
}

// ==================== Salable / Reserve ====================

// The per-channel salable quantity for one product - returns -1 ("always in stock") when no inventory item exists, the raw physical sum for untracked items, and the sum minus net reservations for tracked ones
func (s *Service) Salable(ctx context.Context, productID, channelID uuid.UUID) (float64, error) {
	// A remote provider is authoritative when it answers, else fail open to native
	if s.remoteAvail != nil {
		if m, ok := s.remoteAvail.Salable(ctx, []uuid.UUID{productID}, channelID); ok {
			if qty, present := m[productID]; present {
				return qty, nil
			}
		}
	}
	item, err := s.repo.GetItemByProductID(ctx, productID)
	if err != nil {
		if errors.Is(err, ErrItemNotFound) {
			return -1, nil // always-in-stock sentinel
		}
		return 0, err
	}

	// Flat mode - salability is just item.Quantity, or the sentinel for untracked, and it's channel-agnostic
	if s.flatModeEnabled(ctx) {
		if !item.Tracked {
			return -1, nil
		}
		return item.Quantity, nil
	}

	locations, err := s.repo.ListChannelLocations(ctx, channelID)
	if err != nil {
		return 0, err
	}
	if len(locations) == 0 {
		return 0, nil
	}
	inChannel := make(map[uuid.UUID]bool, len(locations))
	for _, l := range locations {
		inChannel[l.ID] = true
	}

	levels, err := s.repo.ListLevelsByItems(ctx, []uuid.UUID{item.ID})
	if err != nil {
		return 0, err
	}
	var salable float64
	for _, lvl := range levels {
		if !inChannel[lvl.LocationID] {
			continue
		}
		// Tracked subtracts the denormalized reserved counter off the row rather than aggregating the ledger, with clampReserved guarding drift - untracked uses raw physical
		if item.Tracked {
			salable += lvl.Quantity - clampReserved(lvl.ReservedQuantity)
		} else {
			salable += lvl.Quantity
		}
	}
	return salable, nil
}

// Floors the reserved counter at 0 so a drifted negative can't inflate salable into an oversell - Reconcile repairs the drift itself
func clampReserved(r float64) float64 {
	if r < 0 {
		return 0
	}
	return r
}

// Emits one StockChanged per affected product, keyed by inventory item since a product has exactly one - Callers MUST have committed already
// A single order touches many ledger rows across lines and locations while every subscriber (search indexer, back-in-stock, cache busting) keys off ProductID, so the deltas are folded per item rather than fired per row
// Delta is the change in SALABLE quantity, and Before/After are the product's salable total across all its locations, read back after the commit - LocationID stays nil because one event can span cells
func (s *Service) publishStockChanged(ctx context.Context, deltaByItem map[uuid.UUID]float64, reason Reason) {
	if s.bus == nil || len(deltaByItem) == 0 {
		return
	}
	itemIDs := make([]uuid.UUID, 0, len(deltaByItem))
	for id, delta := range deltaByItem {
		if id == uuid.Nil || delta == 0 {
			continue
		}
		itemIDs = append(itemIDs, id)
	}
	if len(itemIDs) == 0 {
		return
	}
	flat := s.flatModeEnabled(ctx)
	salable := make(map[uuid.UUID]float64, len(itemIDs))
	haveSalable := flat // flat reads it straight off the item row below
	if !flat {
		levels, err := s.repo.ListLevelsByItems(ctx, itemIDs)
		if err != nil {
			// A read-back blip must not swallow the event - the indexer only needs ProductID
			log.Printf("[inventory] stock event level read-back: %v", err)
		} else {
			haveSalable = true
			for _, lvl := range levels {
				salable[lvl.InventoryItemID] += lvl.Quantity - clampReserved(lvl.ReservedQuantity)
			}
		}
	}
	for _, itemID := range itemIDs {
		item, err := s.repo.GetItemByID(ctx, itemID)
		if err != nil {
			log.Printf("[inventory] stock event product for item %s: %v", itemID, err)
			continue
		}
		if flat {
			salable[itemID] = item.Quantity
		}
		ev := &StockChanged{
			InventoryItemID: itemID,
			ProductID:       item.ProductID,
			Delta:           deltaByItem[itemID],
			Reason:          reason,
		}
		if haveSalable {
			ev.After = salable[itemID]
			ev.Before = ev.After - ev.Delta
		}
		s.bus.Publish(ctx, ev)
	}
}

// publishStockChanged for callers that only know product ids - the flat-mode paths, which have no ledger to read item ids back from
func (s *Service) publishStockChangedForProducts(ctx context.Context, deltaByProduct map[uuid.UUID]float64, reason Reason) {
	if s.bus == nil || len(deltaByProduct) == 0 {
		return
	}
	productIDs := make([]uuid.UUID, 0, len(deltaByProduct))
	for id := range deltaByProduct {
		productIDs = append(productIDs, id)
	}
	items, err := s.repo.ListItemsByProductIDs(ctx, productIDs)
	if err != nil {
		log.Printf("[inventory] stock event items by product: %v", err)
		return
	}
	byItem := make(map[uuid.UUID]float64, len(items))
	for pid, item := range items {
		byItem[item.ID] = deltaByProduct[pid]
	}
	s.publishStockChanged(ctx, byItem, reason)
}

// Per-product backorderable flags in one query - a missing key (untracked, or no item) is not backorderable, which lets the cart suppress "only N left" warnings on lines orderable past zero
func (s *Service) BackorderableByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}
	items, err := s.repo.ListItemsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	for pid, item := range items {
		out[pid] = item.Backorderable
	}
	return out, nil
}

// The batched Salable - native stock overlaid with the remote provider for products it knows, falling back to native when the provider is down - A missing key is the -1 sentinel, as in Salable
func (s *Service) SalableByProductIDs(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]float64, error) {
	native, err := s.salableNativeByProductIDs(ctx, productIDs, channelID)
	if err != nil {
		return nil, err
	}
	if s.remoteAvail != nil {
		if remote, ok := s.remoteAvail.Salable(ctx, productIDs, channelID); ok {
			maps.Copy(native, remote)
		}
	}
	return native, nil
}

func (s *Service) salableNativeByProductIDs(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]float64, error) {
	if len(productIDs) == 0 || channelID == uuid.Nil {
		return map[uuid.UUID]float64{}, nil
	}
	out := make(map[uuid.UUID]float64, len(productIDs))

	items, err := s.repo.ListItemsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	// Untracked products (no row) get the sentinel up front, so callers needn't re-check the input
	for _, pid := range productIDs {
		if _, ok := items[pid]; !ok {
			out[pid] = -1
		}
	}
	if len(items) == 0 {
		return out, nil
	}

	// Flat mode - tracked products' salability is item.Quantity, untracked keep the sentinel above
	if s.flatModeEnabled(ctx) {
		for pid, item := range items {
			if !item.Tracked {
				out[pid] = -1
				continue
			}
			out[pid] = item.Quantity
		}
		return out, nil
	}

	locations, err := s.repo.ListChannelLocations(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if len(locations) == 0 {
		// No assigned locations means zero stock for every tracked product - untracked keep the sentinel
		for pid, item := range items {
			_ = item
			out[pid] = 0
		}
		return out, nil
	}
	locationIDs := make([]uuid.UUID, len(locations))
	for i, l := range locations {
		locationIDs[i] = l.ID
	}
	locationSet := make(map[uuid.UUID]struct{}, len(locationIDs))
	for _, id := range locationIDs {
		locationSet[id] = struct{}{}
	}

	itemIDs := make([]uuid.UUID, 0, len(items))
	itemToProduct := make(map[uuid.UUID]uuid.UUID, len(items))
	for pid, item := range items {
		itemIDs = append(itemIDs, item.ID)
		itemToProduct[item.ID] = pid
	}

	levels, err := s.repo.ListLevelsByItems(ctx, itemIDs)
	if err != nil {
		return nil, err
	}
	trackedByItem := make(map[uuid.UUID]bool, len(items))
	for _, item := range items {
		trackedByItem[item.ID] = item.Tracked
	}
	// Sum salable per item over the channel's locations - tracked subtract the reserved counter off the row, untracked use raw physical, and no level is 0 salable
	salableByItem := make(map[uuid.UUID]float64, len(itemIDs))
	for _, lvl := range levels {
		if _, ok := locationSet[lvl.LocationID]; !ok {
			continue
		}
		if trackedByItem[lvl.InventoryItemID] {
			salableByItem[lvl.InventoryItemID] += lvl.Quantity - clampReserved(lvl.ReservedQuantity)
		} else {
			salableByItem[lvl.InventoryItemID] += lvl.Quantity
		}
	}
	for _, item := range items {
		out[itemToProduct[item.ID]] = salableByItem[item.ID]
	}
	return out, nil
}

// Rebuilds every reserved_quantity from the ledger, the source of truth - the counter is maintained in-tx with the ledger, so this is a safety sweep for suspected drift, cheap and safe to re-run
func (s *Service) ReconcileReserved(ctx context.Context) error {
	return s.repo.RecomputeAllReserved(ctx)
}

// Allocates stock for order lines under one channel - a shortage with no backorder returns ErrInsufficientStock and rolls back partial allocations
// Flat mode takes a separate path with no routing or ledger, just one atomic decrement per line, restored on cancel by reading the order's lines back
func (s *Service) Reserve(ctx context.Context, channelID uuid.UUID, orderID uuid.UUID, lines []ReserveLine) (ReservationResult, error) {
	var out ReservationResult
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		var err error
		out, err = s.ReserveTx(ctx, t, channelID, orderID, lines)
		return err
	})
	if err != nil {
		return ReservationResult{}, err
	}
	return out, nil
}

// Reserve joining a caller-owned tx, so reservations commit or roll back atomically with the order - a placement that fails after reserving mustn't orphan a committed reservation
func (s *Service) ReserveTx(ctx context.Context, t *sql.Tx, channelID uuid.UUID, orderID uuid.UUID, lines []ReserveLine) (ReservationResult, error) {
	// The ledger toggle is the enforcement kill switch for BOTH modes - a cutover where another platform owns inventory truth must not write anything here
	if !s.ledgerEnabled(ctx) {
		return ReservationResult{}, nil
	}
	r := s.repo.WithTx(t)
	if s.flatModeEnabled(ctx) {
		return s.reserveFlat(ctx, r, lines)
	}

	// Pre-commit hook, sync and able to abort - routing subscribers may pin a line to a location, explicitly and with no silent fallback, while unlisted lines use the default algorithm
	var routing map[uuid.UUID]uuid.UUID
	if s.bus != nil {
		allocating := &Allocating{
			OrderID:   orderID,
			ChannelID: channelID,
			Lines:     lines,
			Routing:   &routing,
		}
		if err := s.bus.PublishSync(ctx, allocating); err != nil {
			return ReservationResult{}, fmt.Errorf("allocate rejected: %w", err)
		}
	}

	locations, err := r.ListChannelLocations(ctx, channelID)
	if err != nil {
		return ReservationResult{}, err
	}
	if len(locations) == 0 {
		// No locations means nothing to reserve from - treat it as ungated, like "no item row = always in stock"
		return ReservationResult{}, nil
	}
	locationByID := make(map[uuid.UUID]Location, len(locations))
	for _, loc := range locations {
		locationByID[loc.ID] = loc
	}

	plan := make([]ReserveAllocation, 0, len(lines)*len(locations))
	// planned accumulates the real stock already committed to earlier lines in THIS call but not yet written to the ledger
	// Without it two lines for the same product - a component shared by two bundles, or one also bought standalone - each read the same availability and oversell - The GetLevelForUpdate lock can't catch it: within one transaction the second read is re-entrant and still can't see the first line's uninserted reservation
	planned := make(map[allocKey]float64)
	// Preload every line's inventory item in one query so allocateLine reads a map instead of firing a per-line GetItemByProductID - a plain non-locking read within this tx, since the FOR UPDATE level locks stay per (item, location)
	itemsByProduct, err := r.ListItemsByProductIDs(ctx, reserveLineProductIDs(lines))
	if err != nil {
		return ReservationResult{}, err
	}
	// Preload which (item, location) cells have level rows at all - allocateLine then only FOR-UPDATEs real cells, instead of paying two queries per empty location per line (stock only exists where a level row does, so a rowless cell can never allocate)
	stocked, err := stockedCells(ctx, r, itemsByProduct)
	if err != nil {
		return ReservationResult{}, err
	}
	for _, line := range lines {
		lineLocations := locations
		if routedID, ok := routing[line.OrderLineID]; ok {
			routed, known := locationByID[routedID]
			if !known {
				// A routed pick outside the channel's locations is a subscriber bug - fail loudly rather than ship from a location the store can't fulfill from
				return ReservationResult{}, fmt.Errorf("allocation routing for line %s targets location %s outside channel %s", line.OrderLineID, routedID, channelID)
			}
			lineLocations = []Location{routed}
		}
		alloc, err := s.allocateLine(ctx, r, line, lineLocations, planned, itemsByProduct, stocked)
		if err != nil {
			return ReservationResult{}, err
		}
		plan = append(plan, alloc...)
	}
	// Persist reservation rows for the allocations
	oid := orderID
	for _, a := range plan {
		lineID := a.OrderLineID
		_, err := r.InsertReservation(ctx, Reservation{
			InventoryItemID: a.InventoryItemID,
			LocationID:      a.LocationID,
			QuantityDelta:   -a.Quantity,
			Reason:          ReasonOrderPlaced,
			OrderID:         &oid,
			OrderLineID:     &lineID,
		})
		if err != nil {
			return ReservationResult{}, err
		}
	}
	// Deduct-at-placement: consume every hold just written, in the same tx - the same comp +
	// on-hand decrement FulfillLines performs at ship time, so downstream ledger algebra is
	// identical and the eventual fulfillment event finds nothing held and no-ops. Salable is
	// unchanged by the consume itself (hold freed = level decremented), so availability reads
	// stay mode-agnostic
	if s.deductAtPlacement(ctx) {
		for _, a := range plan {
			lineID := a.OrderLineID
			if _, err := r.InsertReservation(ctx, Reservation{
				InventoryItemID: a.InventoryItemID,
				LocationID:      a.LocationID,
				QuantityDelta:   a.Quantity,
				Reason:          ReasonOrderDeducted,
				OrderID:         &oid,
				OrderLineID:     &lineID,
			}); err != nil {
				return ReservationResult{}, err
			}
			if _, err := r.AdjustLevelQuantity(ctx, a.InventoryItemID, a.LocationID, -a.Quantity); err != nil {
				// A backorder parked on a rowless cell has no level to decrement - the comp still
				// records the consumption, mirroring FulfillLines' cell-missing edge
				if !errors.Is(err, ErrLevelNotFound) {
					return ReservationResult{}, err
				}
			}
		}
	}
	return ReservationResult{Allocations: plan}, nil
}

// Reserve's flat-mode path - decrements inventory_items.quantity per tracked line, failing the whole order with ErrInsufficientStock on any non-backorderable shortage, and passing untracked lines through
// The repo must be tx-bound so a partial decrement rolls back cleanly, and the item is read FOR UPDATE because the stock check is check-then-act - without the lock two concurrent orders both pass and drive quantity negative
func (s *Service) reserveFlat(ctx context.Context, r Repository, lines []ReserveLine) (ReservationResult, error) {
	for _, line := range lines {
		item, err := r.GetItemByProductIDForUpdate(ctx, line.ProductID)
		if err != nil {
			if errors.Is(err, ErrItemNotFound) {
				continue // untracked product (no row), unbounded
			}
			return ReservationResult{}, err
		}
		if !item.Tracked {
			continue
		}
		if !item.Backorderable && item.Quantity < line.Quantity {
			return ReservationResult{}, ErrInsufficientStock
		}
		if _, err := r.AdjustItemQuantity(ctx, item.ID, -line.Quantity); err != nil {
			return ReservationResult{}, err
		}
	}
	return ReservationResult{}, nil
}

// ReleaseForOrder's flat-mode path - with no ledger it fetches the order's lines through OrderLineLookup and increments each tracked item back - No-op without a lookup wired
// Restores only the UNSHIPPED remainder per line. partially_fulfilled -> cancelled is a legal
// transition, and restoring the full line set there put the shipped units back on the shelf - the
// multi-location path avoids it by compensating only net-negative ledger chains
func (s *Service) releaseForOrderFlat(ctx context.Context, orderID uuid.UUID, reason Reason) error {
	if s.orders == nil {
		return nil
	}
	lines, err := s.orders.GetOrderLines(ctx, orderID)
	if err != nil {
		return fmt.Errorf("load order lines: %w", err)
	}
	restored := make(map[uuid.UUID]float64)
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		for _, line := range lines {
			restore := line.Quantity - line.FulfilledQuantity
			if restore <= 0 {
				continue // fully shipped, so there is nothing held to give back
			}
			item, err := r.GetItemByProductID(ctx, line.ProductID)
			if err != nil {
				if errors.Is(err, ErrItemNotFound) {
					continue
				}
				return err
			}
			if !item.Tracked {
				continue
			}
			if _, err := r.AdjustItemQuantity(ctx, item.ID, restore); err != nil {
				return err
			}
			restored[item.ID] += restore
		}
		return nil
	}); err != nil {
		return err
	}
	s.publishStockChanged(ctx, restored, reason)
	return nil
}

type allocKey struct {
	item uuid.UUID
	loc  uuid.UUID
}

func reserveLineProductIDs(lines []ReserveLine) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(lines))
	ids := make([]uuid.UUID, 0, len(lines))
	for _, line := range lines {
		if _, ok := seen[line.ProductID]; ok {
			continue
		}
		seen[line.ProductID] = struct{}{}
		ids = append(ids, line.ProductID)
	}
	return ids
}

// Slices a line's quantity across the priority-ordered locations, returning ErrInsufficientStock when tracked, non-backorderable and uncoverable
// planned carries stock already allocated to earlier lines in this call, so repeated products can't double-spend the same units
func stockedCells(ctx context.Context, r Repository, items map[uuid.UUID]Item) (map[allocKey]bool, error) {
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	levels, err := r.ListLevelsByItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[allocKey]bool, len(levels))
	for _, lvl := range levels {
		out[allocKey{item: lvl.InventoryItemID, loc: lvl.LocationID}] = true
	}
	return out, nil
}

func (s *Service) allocateLine(ctx context.Context, r Repository, line ReserveLine, locations []Location, planned map[allocKey]float64, items map[uuid.UUID]Item, stocked map[allocKey]bool) ([]ReserveAllocation, error) {
	// item comes from the ReserveTx preload and is non-locking - the stock lock is GetLevelForUpdate below - Missing means no row, so untracked, so no reservation
	item, ok := items[line.ProductID]
	if !ok {
		return nil, nil
	}
	if !item.Tracked {
		return nil, nil
	}

	remaining := line.Quantity
	plan := make([]ReserveAllocation, 0, len(locations))

	for _, loc := range locations {
		if remaining <= 0 {
			break
		}
		k := allocKey{item: item.ID, loc: loc.ID}
		if !stocked[k] {
			continue // no level row = nothing to take, and skipping saves the lock + net read on every empty cell
		}
		// FOR UPDATE, because the availability check plus InsertReservation is check-then-act - locking the level serializes concurrent reservers of this cell, and the loser blocks then re-reads NetReservation seeing the winner's rows
		level, err := r.GetLevelForUpdate(ctx, item.ID, loc.ID)
		var available float64
		if err == nil {
			net, err := r.NetReservation(ctx, item.ID, loc.ID)
			if err != nil {
				return nil, err
			}
			// Subtract stock promised to earlier lines in this call, not yet in the ledger
			a := level.Quantity + net - planned[k]
			if a < 0 {
				a = 0
			}
			available = a
		} else if !errors.Is(err, ErrLevelNotFound) {
			return nil, err
		}
		if available <= 0 {
			continue
		}
		take := available
		if take > remaining {
			take = remaining
		}
		plan = append(plan, ReserveAllocation{
			InventoryItemID: item.ID,
			LocationID:      loc.ID,
			OrderLineID:     line.OrderLineID,
			Quantity:        take,
		})
		planned[k] += take
		remaining -= take
	}

	if remaining > 0 {
		if !item.Backorderable {
			return nil, fmt.Errorf("%w: product=%s short=%g", ErrInsufficientStock, line.ProductID, remaining)
		}
		// Backorder - park the shortfall at the top location - It's phantom stock, so deliberately NOT added to planned, since only real takes consume the pool
		plan = append(plan, ReserveAllocation{
			InventoryItemID: item.ID,
			LocationID:      locations[0].ID,
			OrderLineID:     line.OrderLineID,
			Quantity:        remaining,
		})
	}
	return plan, nil
}

// chainKey identifies one reservation chain for netting - the line is held by value so every row
// of a chain shares a bucket, with uuid.Nil standing in for line-less rows
type chainKey struct {
	item, loc, line uuid.UUID
}

func chainKeyFor(row Reservation) chainKey {
	k := chainKey{item: row.InventoryItemID, loc: row.LocationID}
	if row.OrderLineID != nil {
		k.line = *row.OrderLineID
	}
	return k
}

func (k chainKey) linePtr() *uuid.UUID {
	if k.line == uuid.Nil {
		return nil
	}
	l := k.line
	return &l
}

// Writes compensating positive reservations for a cancelled order's held rows - idempotent, so a chain already netting to zero is a no-op
func (s *Service) ReleaseForOrder(ctx context.Context, orderID uuid.UUID, reason Reason) error {
	// Same kill-switch-first ordering as ReserveTx - no writes in either mode during a cutover
	if !s.ledgerEnabled(ctx) {
		return nil
	}
	if s.flatModeEnabled(ctx) {
		return s.releaseForOrderFlat(ctx, orderID, reason)
	}
	// Sum the chain per (item, location, line) - anything net-negative gets a compensation row to zero
	// The line keys BY VALUE (uuid.Nil for line-less rows): a pointer key gave every row its own
	// bucket, so a partially fulfilled chain compensated its full hold while the settlement sat
	// ignored - reserved went negative and salable inflated by the shipped quantity
	freed := make(map[uuid.UUID]float64)
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		// Read inside the tx so a concurrent duplicate event can't double-compensate off the same snapshot
		rows, err := r.ListReservationsForOrder(ctx, orderID)
		if err != nil {
			return err
		}
		net := make(map[chainKey]float64)
		for _, row := range rows {
			net[chainKeyFor(row)] += row.QuantityDelta
		}
		for k, v := range net {
			if v >= 0 {
				continue
			}
			oid := orderID
			_, err := r.InsertReservation(ctx, Reservation{
				InventoryItemID: k.item,
				LocationID:      k.loc,
				QuantityDelta:   -v, // positive compensation
				Reason:          reason,
				OrderID:         &oid,
				OrderLineID:     k.linePtr(),
			})
			if err != nil {
				return err
			}
			freed[k.item] += -v // releasing a hold raises salable by that much
		}
		// Placement-deducted chains net to zero, so the loop above never sees them - their
		// unshipped share goes back to on-hand instead of back to a hold
		restored, err := s.restoreDeducted(ctx, r, orderID, rows, reason)
		if err != nil {
			return err
		}
		for item, qty := range restored {
			freed[item] += qty // restocking raises salable exactly like releasing a hold
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Post-commit - a rolled-back release must not have announced stock nobody can buy
	s.publishStockChanged(ctx, freed, reason)
	return nil
}

// restoreDeducted puts placement-deducted stock back on hand for the unshipped share of each
// chain, writing a net-zero row pair per chain: -restore order_deducted (spending the chain's
// outstanding balance, which is what makes a redelivered cancel and repeated edits restore
// nothing twice) and +restore under the caller's reason as the audit row - the pair cancels in
// both the chain net and the reserved counter, so only the level moves
// Shipped units never come back, the same rule the flat paths follow, and with no line lookup
// wired nothing is restored - mirroring releaseForOrderFlat's posture
func (s *Service) restoreDeducted(ctx context.Context, r Repository, orderID uuid.UUID, rows []Reservation, reason Reason) (map[uuid.UUID]float64, error) {
	outstanding := make(map[chainKey]float64)
	for _, row := range rows {
		if row.Reason != ReasonOrderDeducted {
			continue
		}
		outstanding[chainKeyFor(row)] += row.QuantityDelta
	}
	restored := make(map[uuid.UUID]float64)
	if len(outstanding) == 0 || s.orders == nil {
		return restored, nil
	}
	lines, err := s.orders.GetOrderLines(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("load order lines: %w", err)
	}
	// Unshipped share per line, in [0,1] - every lookup row of a line repeats the same pair, so
	// overwrites are harmless
	share := make(map[uuid.UUID]float64, len(lines))
	for _, l := range lines {
		if l.LineID == uuid.Nil || l.LineQuantity <= 0 {
			continue
		}
		f := (l.LineQuantity - l.LineFulfilled) / l.LineQuantity
		share[l.LineID] = min(1, max(0, f))
	}
	oid := orderID
	for k, v := range outstanding {
		if v <= 0 {
			continue
		}
		f, ok := share[k.line]
		if !ok {
			// No surviving order line (removed by an edit) - nothing of it can have shipped
			f = 1
		}
		restore := v * f
		if restore <= 0 {
			continue
		}
		for _, p := range []struct {
			delta  float64
			reason Reason
		}{{-restore, ReasonOrderDeducted}, {restore, reason}} {
			if _, err := r.InsertReservation(ctx, Reservation{
				InventoryItemID: k.item,
				LocationID:      k.loc,
				QuantityDelta:   p.delta,
				Reason:          p.reason,
				OrderID:         &oid,
				OrderLineID:     k.linePtr(),
			}); err != nil {
				return nil, err
			}
		}
		if _, err := r.AdjustLevelQuantity(ctx, k.item, k.loc, restore); err != nil {
			if !errors.Is(err, ErrLevelNotFound) {
				return nil, err
			}
		}
		restored[k.item] += restore
	}
	return restored, nil
}

// Re-reserves an order's stock for an edited line set in the caller's tx - releases the current holds tagged order_edited, then reserves the new lines from the cart
// A shortage rolls back the whole edit, which is why it joins the edit tx - No-op with the ledger disabled
func (s *Service) ReReserveForEditTx(ctx context.Context, sqlTx *sql.Tx, channelID, orderID uuid.UUID, c cart.Cart) error {
	if !s.ledgerEnabled(ctx) {
		return nil
	}
	if s.flatModeEnabled(ctx) {
		return s.reReserveForEditFlat(ctx, s.repo.WithTx(sqlTx), orderID, c)
	}
	r := s.repo.WithTx(sqlTx)
	// Read through the edit tx so the release below compensates exactly what this tx sees
	rows, err := r.ListReservationsForOrder(ctx, orderID)
	if err != nil {
		return err
	}
	net := make(map[chainKey]float64)
	for _, row := range rows {
		net[chainKeyFor(row)] += row.QuantityDelta // by-value key - see ReleaseForOrder
	}
	// Release - compensate every still-held (net-negative) chain back to zero
	for k, v := range net {
		if v >= 0 {
			continue
		}
		oid := orderID
		if _, err := r.InsertReservation(ctx, Reservation{
			InventoryItemID: k.item,
			LocationID:      k.loc,
			QuantityDelta:   -v, // positive compensation
			Reason:          ReasonOrderEdited,
			OrderID:         &oid,
			OrderLineID:     k.linePtr(),
		}); err != nil {
			return err
		}
	}
	// Placement-deducted chains netted to zero above - give their unshipped share back to on-hand
	// before re-deducting the new line set, or every edit would stack another full deduction
	if _, err := s.restoreDeducted(ctx, r, orderID, rows, ReasonOrderEdited); err != nil {
		return err
	}
	// Reserve the new line set - bundle and kit decomposition is handled by linesFromCart
	if s.products == nil {
		return nil
	}
	lines, err := s.linesFromCart(ctx, c)
	if err != nil {
		return err
	}
	// Deduct-at-placement re-deducts below, but shipped units already left at fulfillment and must
	// not be charged twice - trim each line to its unshipped share (component rows scale uniformly,
	// since shipments are per order line). Fulfillment mode keeps full holds, as it always has
	if s.deductAtPlacement(ctx) && s.orders != nil && len(lines) > 0 {
		prior, err := s.orders.GetOrderLines(ctx, orderID)
		if err != nil {
			return fmt.Errorf("load order lines: %w", err)
		}
		shippedUnits := make(map[uuid.UUID]float64, len(prior))
		for _, l := range prior {
			if l.LineID != uuid.Nil {
				shippedUnits[l.LineID] = l.LineFulfilled
			}
		}
		newQty := make(map[uuid.UUID]float64, len(c.Lines))
		for _, cl := range c.Lines {
			newQty[cl.ID] += cl.Quantity
		}
		scaled := lines[:0]
		for _, ln := range lines {
			done, q := shippedUnits[ln.OrderLineID], newQty[ln.OrderLineID]
			if done > 0 && q > 0 {
				factor := (q - done) / q
				if factor <= 0 {
					continue // the whole new quantity already shipped - nothing left to deduct
				}
				ln.Quantity *= factor
			}
			scaled = append(scaled, ln)
		}
		lines = scaled
	}
	if len(lines) == 0 {
		return nil
	}
	_, err = s.ReserveTx(ctx, sqlTx, channelID, orderID, lines)
	return err
}

// ReReserveForEditTx's flat-mode path - placement decremented the full ordered quantity, so an edit
// just moves that deduction to the new one, per product
//
// Never gives back what shipped: a product with S units gone stays deducted by at least S, the same
// rule releaseForOrderFlat follows on cancellation
//
// Joins the caller's edit tx, so a shortage here rolls the whole edit back and there is nothing to
// make idempotent - unlike the return path, which is retried and has no marker to replay against
func (s *Service) reReserveForEditFlat(ctx context.Context, r Repository, orderID uuid.UUID, c cart.Cart) error {
	if s.orders == nil || s.products == nil {
		return nil // no way to read what the order held, so leave the quantity alone
	}
	prior, err := s.orders.GetOrderLines(ctx, orderID)
	if err != nil {
		return fmt.Errorf("load order lines: %w", err)
	}
	type held struct{ ordered, shipped float64 }
	was := make(map[uuid.UUID]held, len(prior))
	for _, line := range prior {
		h := was[line.ProductID]
		h.ordered += line.Quantity
		h.shipped += line.FulfilledQuantity
		was[line.ProductID] = h
	}

	next, err := s.linesFromCart(ctx, c)
	if err != nil {
		return err
	}
	wants := make(map[uuid.UUID]float64, len(next))
	for _, line := range next {
		wants[line.ProductID] += line.Quantity
	}

	// Every product on either side - one dropped from the edit still has stock to give back
	for productID := range unionProductIDs(was, wants) {
		h := was[productID]
		// The new deduction can't fall below what already shipped
		target := max(wants[productID], h.shipped)
		delta := target - h.ordered
		if delta == 0 {
			continue
		}
		item, err := r.GetItemByProductIDForUpdate(ctx, productID)
		if err != nil {
			if errors.Is(err, ErrItemNotFound) {
				continue
			}
			return err
		}
		if !item.Tracked {
			continue
		}
		if delta > 0 && !item.Backorderable && item.Quantity < delta {
			return ErrInsufficientStock
		}
		if _, err := r.AdjustItemQuantity(ctx, item.ID, -delta); err != nil {
			return err
		}
	}
	return nil
}

func unionProductIDs[A any, B any](a map[uuid.UUID]A, b map[uuid.UUID]B) map[uuid.UUID]struct{} {
	out := make(map[uuid.UUID]struct{}, len(a)+len(b))
	for id := range a {
		out[id] = struct{}{}
	}
	for id := range b {
		out[id] = struct{}{}
	}
	return out
}

type FulfillLineQty struct {
	OrderLineID uuid.UUID
	Quantity    float64
}

// Decrements reserved stock for specific shipped lines and quantities, for partial shipments - compensating the reservation and decrementing physical stock per cell
// Each line converts up to its still-held quantity, capping there so a replay or over-request can't double-decrement - No-op in flat mode or with the ledger disabled
func (s *Service) FulfillLines(ctx context.Context, orderID uuid.UUID, lines []FulfillLineQty) error {
	if s.flatModeEnabled(ctx) || !s.ledgerEnabled(ctx) {
		return nil
	}
	type cell struct{ item, loc uuid.UUID }
	want := make(map[uuid.UUID]float64, len(lines))
	for _, l := range lines {
		want[l.OrderLineID] += l.Quantity
	}
	// Shipping normally nets to zero salable - the hold is freed and the same amount leaves physical stock - so this only carries the cell-missing edge, where the comp lands but the decrement can't
	moved := make(map[uuid.UUID]float64)
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		// Read inside the tx - a redelivered event racing this one must see the winner's comps, not a stale snapshot
		rows, err := r.ListReservationsForOrder(ctx, orderID)
		if err != nil {
			return err
		}
		// held[lineID][cell] = net reservation, negative while stock is held
		held := make(map[uuid.UUID]map[cell]float64)
		for _, row := range rows {
			if row.OrderLineID == nil {
				continue
			}
			c := cell{item: row.InventoryItemID, loc: row.LocationID}
			m := held[*row.OrderLineID]
			if m == nil {
				m = make(map[cell]float64)
				held[*row.OrderLineID] = m
			}
			m[c] += row.QuantityDelta
		}
		oid := orderID
		for lineID, qty := range want {
			remaining := qty
			for c, net := range held[lineID] {
				if remaining <= 0 {
					break
				}
				heldQty := -net // positive amount still reserved at this cell
				if heldQty <= 0 {
					continue
				}
				take := heldQty
				if take > remaining {
					take = remaining
				}
				lid := lineID
				if _, err := r.InsertReservation(ctx, Reservation{
					InventoryItemID: c.item,
					LocationID:      c.loc,
					QuantityDelta:   take, // positive comp consumes the hold
					Reason:          ReasonOrderFulfilled,
					OrderID:         &oid,
					OrderLineID:     &lid,
				}); err != nil {
					return err
				}
				moved[c.item] += take // freeing the hold raises salable
				if _, err := r.AdjustLevelQuantity(ctx, c.item, c.loc, -take); err != nil {
					if errors.Is(err, ErrLevelNotFound) {
						continue
					}
					return err
				}
				moved[c.item] -= take // the goods leaving lowers it right back
				remaining -= take
			}
		}
		return nil
	}); err != nil {
		return err
	}
	s.publishStockChanged(ctx, moved, ReasonOrderFulfilled)
	return nil
}

type RestockLineQty struct {
	OrderLineID uuid.UUID
	Quantity    float64
}

// Adds a return's quantities back at the cells the goods shipped from - read from the order's positive order_fulfilled deltas plus the net order_deducted balance (deduct-at-placement's equivalent) - re-incrementing up to that ledger ceiling per cell, so a return can't inflate stock beyond what the order actually took; the RMA flow upstream is what caps requests at shipped quantities
// Idempotent - it writes one delta-0 marker row per return (reason=order_returned, note=returnID) that a replay finds and no-ops on, and the zero delta leaves reserved_quantity untouched - No-op in flat mode or with the ledger disabled
func (s *Service) RestockReturn(ctx context.Context, orderID, returnID uuid.UUID, lines []RestockLineQty) error {
	if s.flatModeEnabled(ctx) || !s.ledgerEnabled(ctx) {
		return nil
	}
	marker := returnID.String()
	type cell struct{ item, loc uuid.UUID }
	want := make(map[uuid.UUID]float64, len(lines))
	for _, l := range lines {
		want[l.OrderLineID] += l.Quantity
	}
	oid := orderID
	restocked := make(map[uuid.UUID]float64)
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		// Read inside the tx so the idempotency-marker check and the restock see one snapshot
		rows, err := r.ListReservationsForOrder(ctx, orderID)
		if err != nil {
			return err
		}
		// shippedFrom[lineID][cell] = quantity decremented from that cell at fulfillment
		shippedFrom := make(map[uuid.UUID]map[cell]float64)
		var anchor cell
		haveAnchor := false
		for _, row := range rows {
			// Already restocked for this return - a replayed event, so no-op
			if row.Reason == ReasonOrderReturned && row.Note == marker {
				return nil
			}
			if row.OrderLineID == nil {
				continue
			}
			// order_fulfilled comps say where ship-time deduction happened; order_deducted rows say
			// where placement-time deduction happened (summed NET, so a cancel/edit restore isn't
			// restocked twice) - either way, that cell is where the returned goods go back
			switch {
			case row.Reason == ReasonOrderFulfilled && row.QuantityDelta > 0:
			case row.Reason == ReasonOrderDeducted:
			default:
				continue
			}
			c := cell{item: row.InventoryItemID, loc: row.LocationID}
			if !haveAnchor {
				anchor, haveAnchor = c, true
			}
			m := shippedFrom[*row.OrderLineID]
			if m == nil {
				m = make(map[cell]float64)
				shippedFrom[*row.OrderLineID] = m
			}
			m[c] += row.QuantityDelta
		}
		if !haveAnchor {
			// Nothing was ever fulfilled, so nothing to restock
			return nil
		}
		for lineID, qty := range want {
			remaining := qty
			for c, shipped := range shippedFrom[lineID] {
				if remaining <= 0 {
					break
				}
				if shipped <= 0 {
					continue // a fully restored placement chain nets to zero - nothing left to return to
				}
				take := shipped
				if take > remaining {
					take = remaining
				}
				if _, err := r.AdjustLevelQuantity(ctx, c.item, c.loc, take); err != nil {
					if errors.Is(err, ErrLevelNotFound) {
						continue
					}
					return err
				}
				restocked[c.item] += take // physical back, reservations untouched, so salable rises
				remaining -= take
			}
		}
		// Idempotency marker - the zero delta leaves reserved_quantity unchanged
		if _, err := r.InsertReservation(ctx, Reservation{
			InventoryItemID: anchor.item,
			LocationID:      anchor.loc,
			QuantityDelta:   0,
			Reason:          ReasonOrderReturned,
			OrderID:         &oid,
			Note:            marker,
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	s.publishStockChanged(ctx, restocked, ReasonOrderReturned)
	return nil
}

// One row per (tracked item, location) cell at or below the item's low_stock_threshold, for the dashboard reorder list
// Sorted most-urgent first, so the limit keeps the worst cases - clamped to [1, 1000] with 200 as the unset default
func (s *Service) ListLowStockLevels(ctx context.Context, limit int32) ([]LowStockLevel, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if s.flatModeEnabled(ctx) {
		return s.repo.ListLowStockItemsFlat(ctx, limit)
	}
	return s.repo.ListLowStockLevels(ctx, limit)
}

// ==================== Mode migration ====================

// The two modes keep SEPARATE numbers - multi-location reads inventory_levels, flat reads
// inventory_items.quantity - and nothing keeps them in step: AdjustLevel writes only levels,
// SetItemQuantity writes only the item. Flipping sales/inventory/flat_mode therefore switches which
// number is authoritative, and a store that ran multi-location through the admin UI has
// item.quantity still at its 0 default - the whole catalogue would read out of stock
//
// These carry the stock across, and both are idempotent - re-running changes nothing

// Sums each product's SALABLE stock across every location into inventory_items.quantity, the number
// flat mode sells from
//
// Salable rather than raw physical, because flat mode's placement path decrements quantity, so its
// quantity means "available" - copying physical across would hand out the stock held for open orders
// a second time
//
// Caveat worth knowing: an order placed BEFORE the flip and cancelled after it restores its
// quantity on top of a number that already excluded it. Flip with no unshipped orders where you can
func (s *Service) MigrateToFlatMode(ctx context.Context, dryRun bool) ([]ModeBackfillRow, int64, error) {
	if dryRun {
		rows, err := s.repo.PreviewItemQuantityFromLevels(ctx)
		if err != nil {
			return nil, 0, err
		}
		changed := int64(0)
		for _, r := range rows {
			if r.Current != r.New {
				changed++
			}
		}
		return rows, changed, nil
	}
	n, err := s.repo.BackfillItemQuantityFromLevels(ctx)
	return nil, n, err
}

// Parks every product's flat quantity at one location and zeroes the rest, so multi-location
// salability matches what flat mode was selling
//
// The target must be assigned to the channels that should sell it - a location outside a channel's
// list contributes nothing to that channel's salable. Reserved counters are left alone: run
// `reconcile-reserved` afterwards to rebuild them from the ledger, since flat mode wrote none
func (s *Service) MigrateToMultiLocationMode(ctx context.Context, locationID uuid.UUID, dryRun bool) (int64, int64, error) {
	loc, err := s.repo.GetLocationByID(ctx, locationID)
	if err != nil {
		return 0, 0, err
	}
	if !loc.IsActive {
		return 0, 0, fmt.Errorf("location %s is inactive - stock parked there would be unsellable", loc.Slug)
	}
	if dryRun {
		rows, err := s.repo.PreviewItemQuantityFromLevels(ctx)
		if err != nil {
			return 0, 0, err
		}
		return int64(len(rows)), 0, nil
	}
	// Zero first: doing it after would wipe the rows just written when the target is the odd one out
	zeroed, err := s.repo.ZeroLevelsOutsideLocation(ctx, locationID)
	if err != nil {
		return 0, 0, err
	}
	placed, err := s.repo.BackfillLevelsFromItemQuantity(ctx, locationID)
	if err != nil {
		return 0, 0, err
	}
	return placed, zeroed, nil
}

// Deletes zero-netted reservation chains older than the cutoff, on a periodic cron
func (s *Service) PruneSettledReservations(ctx context.Context, retention time.Duration) (int, error) {
	cutoff := time.Now().Add(-retention)
	orderIDs, err := s.repo.ListSettledOrderIDsBefore(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	for _, oid := range orderIDs {
		if err := s.repo.DeleteReservationsForOrder(ctx, oid); err != nil {
			return 0, err
		}
	}
	return len(orderIDs), nil
}

// ==================== helpers ====================

// Either may be nil, and any combination is allowed
func validateCoords(lat, lng *float64) error {
	if lat != nil && (*lat < -90 || *lat > 90) {
		return fmt.Errorf("latitude must be between -90 and 90 (got %v)", *lat)
	}
	if lng != nil && (*lng < -180 || *lng > 180) {
		return fmt.Errorf("longitude must be between -180 and 180 (got %v)", *lng)
	}
	return nil
}

func validateSlug(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return fmt.Errorf("slug is required")
	}
	for _, r := range slug {
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		isSep := r == '-' || r == '_'
		if !(isLower || isDigit || isSep) {
			return fmt.Errorf("slug %q contains invalid character %q", slug, r)
		}
	}
	return nil
}
