package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func (r *inventoryLocationResolver) Country(ctx context.Context, obj *model.InventoryLocation) (*model.Country, error) {
	if obj.CountryCode == "" {
		return nil, nil
	}
	c, err := r.CountrySvc.GetByCode(ctx, obj.CountryCode)
	if err != nil {
		// Stale code (no matching countries row): treat as null rather than
		// erroring the whole field.
		return nil, nil
	}
	return r.toCountryModel(ctx, c), nil
}

func (r *mutationResolver) CreateInventoryLocation(ctx context.Context, input model.CreateInventoryLocationInput) (*model.InventoryLocation, error) {
	if err := r.requireGlobal(ctx, "inventory.manage"); err != nil {
		return nil, err
	}
	loc, err := r.InventorySvc.CreateLocation(ctx, inventory.CreateLocationInput{
		StorefrontVisible:  boolOrDefault(input.StorefrontVisible, false),
		PickupEnabled:      boolOrDefault(input.PickupEnabled, false),
		PickupInstructions: ptr.Deref(input.PickupInstructions),
		Slug:               input.Slug,
		Name:               input.Name,
		IsActive:           boolOrDefault(input.IsActive, true),
		Type:               ptr.Deref(inventoryLocationTypeInput(input.Type)),
		Line1:              ptr.Deref(input.Line1),
		Line2:              ptr.Deref(input.Line2),
		City:               ptr.Deref(input.City),
		Region:             ptr.Deref(input.Region),
		PostalCode:         ptr.Deref(input.PostalCode),
		CountryCode:        ptr.Deref(input.CountryCode),
		Latitude:           input.Latitude,
		Longitude:          input.Longitude,
		Timezone:           ptr.Deref(input.Timezone),
		Hours:              marshalJSON(input.Hours),
		Email:              ptr.Deref(input.Email),
		Phone:              ptr.Deref(input.Phone),
		ContactName:        ptr.Deref(input.ContactName),
		ExternalReference:  ptr.Deref(input.ExternalReference),
		Metadata:           invMetadataFromInput(input.Metadata),
	})
	if err != nil {
		return nil, err
	}
	return toInventoryLocationModel(loc), nil
}

func (r *mutationResolver) UpdateInventoryLocation(ctx context.Context, input model.UpdateInventoryLocationInput) (*model.InventoryLocation, error) {
	if err := r.requireGlobal(ctx, "inventory.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(input.ID, "inventory location")
	if err != nil {
		return nil, err
	}
	domainInput := inventory.UpdateLocationInput{
		PickupEnabled:      input.PickupEnabled,
		PickupInstructions: input.PickupInstructions,
		ID:                 id,
		Name:               input.Name,
		StorefrontVisible:  input.StorefrontVisible,
		IsActive:           input.IsActive,
		Type:               inventoryLocationTypeInput(input.Type),
		Line1:              input.Line1,
		Line2:              input.Line2,
		City:               input.City,
		Region:             input.Region,
		PostalCode:         input.PostalCode,
		CountryCode:        input.CountryCode,
		Latitude:           input.Latitude,
		Longitude:          input.Longitude,
		ClearCoords:        input.ClearCoords != nil && *input.ClearCoords,
		Timezone:           input.Timezone,
		Email:              input.Email,
		Phone:              input.Phone,
		ContactName:        input.ContactName,
		ExternalReference:  input.ExternalReference,
	}
	if input.Hours != nil {
		raw := marshalJSON(input.Hours)
		domainInput.Hours = &raw
	}
	if input.Metadata != nil {
		m := invMetadataFromInput(input.Metadata)
		domainInput.Metadata = &m
	}
	loc, err := r.InventorySvc.UpdateLocation(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toInventoryLocationModel(loc), nil
}

func (r *mutationResolver) DeleteInventoryLocation(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "inventory.manage"); err != nil {
		return false, err
	}
	lid, err := parseID(id, "inventory location")
	if err != nil {
		return false, err
	}
	if err := r.InventorySvc.DeleteLocation(ctx, lid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetChannelLocations(ctx context.Context, channelID string, assignments []*model.LocationAssignmentInput) (*model.Channel, error) {
	svID, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "inventory.manage", svID, nil); err != nil {
		return nil, err
	}
	domainAssignments := make([]inventory.LocationAssignment, len(assignments))
	for i, a := range assignments {
		lid, err := parseID(a.LocationID, "location")
		if err != nil {
			return nil, err
		}
		domainAssignments[i] = inventory.LocationAssignment{LocationID: lid, Priority: a.Priority}
	}
	if err := r.InventorySvc.SetChannelLocations(ctx, svID, domainAssignments); err != nil {
		return nil, err
	}
	sv, err := r.ChannelSvc.GetByID(ctx, svID)
	if err != nil {
		return nil, err
	}
	return toChannelModel(sv), nil
}

func (r *mutationResolver) SetInventoryItemTracking(ctx context.Context, productID string, tracked bool) (*model.InventoryItem, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnProductChannels(ctx, "inventory.manage", pid); err != nil {
		return nil, err
	}
	// Lazily provision an item row if none exists; idempotent.
	if _, err := r.InventorySvc.EnsureItem(ctx, pid, tracked); err != nil {
		return nil, err
	}
	item, err := r.InventorySvc.SetItemTracking(ctx, pid, tracked)
	if err != nil {
		return nil, err
	}
	return r.loadInventoryItemModel(ctx, item)
}

func (r *mutationResolver) SetInventoryItemBackorder(ctx context.Context, productID string, backorderable bool, lowStockThreshold *float64) (*model.InventoryItem, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnProductChannels(ctx, "inventory.manage", pid); err != nil {
		return nil, err
	}
	threshold := float64(0)
	if lowStockThreshold != nil {
		threshold = *lowStockThreshold
	}
	item, err := r.InventorySvc.SetItemBackorder(ctx, pid, backorderable, threshold)
	if err != nil {
		return nil, err
	}
	return r.loadInventoryItemModel(ctx, item)
}

func (r *mutationResolver) AdjustInventoryLevel(ctx context.Context, input model.AdjustInventoryLevelInput) (*model.InventoryLevel, error) {
	pid, err := parseID(input.ProductID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnProductChannels(ctx, "inventory.manage", pid); err != nil {
		return nil, err
	}
	lid, err := parseID(input.LocationID, "location")
	if err != nil {
		return nil, err
	}
	item, err := r.InventorySvc.EnsureItem(ctx, pid, true)
	if err != nil {
		return nil, err
	}
	domainInput := inventory.AdjustLevelInput{
		InventoryItemID: item.ID,
		LocationID:      lid,
		NewQuantity:     input.NewQuantity,
		Note:            ptr.Deref(input.Note),
	}
	if claims, err := auth.RequireAuth(ctx); err == nil {
		domainInput.AdminID = &claims.AdminID
	}
	level, err := r.InventorySvc.AdjustLevel(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	loc, err := r.InventorySvc.GetLocation(ctx, lid)
	if err != nil {
		return nil, err
	}
	return toInventoryLevelModel(level, loc, 0), nil
}

// Flat-mode write surface for external ERP integrations. Resolves the product by SKU or ID, fills in the source attribution from the authenticated admin's email when the caller omits one, and overwrites inventory_items.quantity atomically. Permitted in both modes so the audit trail is always populated.
func (r *mutationResolver) SyncProductInventory(ctx context.Context, input model.SyncProductInventoryInput) (*model.InventoryItem, error) {
	pid, err := resolveSyncProductID(ctx, r.CatalogSvc, input)
	if err != nil {
		return nil, err
	}
	if err := r.requireOnProductChannels(ctx, "inventory.manage", pid); err != nil {
		return nil, err
	}
	source := strings.TrimSpace(stringOr(input.Source, ""))
	if source == "" {
		if claims, ok := auth.AdminFromContext(ctx); ok {
			source = "admin:" + claims.Email
		} else {
			source = "manual"
		}
	}
	item, err := r.InventorySvc.SetItemQuantity(ctx, inventory.SetItemQuantityInput{
		ProductID: pid,
		Quantity:  input.Quantity,
		Source:    source,
	})
	if err != nil {
		return nil, err
	}
	return r.loadInventoryItemModel(ctx, item)
}

// SyncProductInventoryBatch fans the single-item resolver out across an ERP batch. Partial success is the norm - one bad SKU shouldn't
// block the rest of the batch (the next sync run retries failures). Each failure carries the offending SKU/ID + reason for replay
func (r *mutationResolver) SyncProductInventoryBatch(ctx context.Context, items []*model.SyncProductInventoryInput) (*model.SyncProductInventoryBatchResult, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "inventory.manage"); err != nil {
		return nil, err
	}
	var defaultSource string
	if claims, ok := auth.AdminFromContext(ctx); ok {
		defaultSource = "admin:" + claims.Email
	} else {
		defaultSource = "manual"
	}
	result := &model.SyncProductInventoryBatchResult{
		Failed: []*model.SyncProductInventoryFailure{},
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		fail := func(reason string) {
			f := &model.SyncProductInventoryFailure{Reason: reason}
			if item.ProductSku != nil {
				f.ProductSku = item.ProductSku
			}
			if item.ProductID != nil {
				f.ProductID = item.ProductID
			}
			result.Failed = append(result.Failed, f)
		}
		pid, err := resolveSyncProductID(ctx, r.CatalogSvc, *item)
		if err != nil {
			fail(err.Error())
			continue
		}
		if err := r.requireOnProductChannels(ctx, "inventory.manage", pid); err != nil {
			fail(err.Error())
			continue
		}
		source := strings.TrimSpace(stringOr(item.Source, ""))
		if source == "" {
			source = defaultSource
		}
		if _, err := r.InventorySvc.SetItemQuantity(ctx, inventory.SetItemQuantityInput{
			ProductID: pid,
			Quantity:  item.Quantity,
			Source:    source,
		}); err != nil {
			fail(err.Error())
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

func (r *queryResolver) InventoryLocations(ctx context.Context, search *string, limit *int32, offset *int32, filter *model.InventoryLocationFilter) ([]*model.InventoryLocation, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "inventory.view"); err != nil {
		return nil, err
	}
	var locs []inventory.Location
	var err error
	if filter == nil && search == nil && limit == nil && offset == nil {
		locs, err = r.InventorySvc.ListLocations(ctx) // the bare query stays the full list, so pickers and existing clients keep working
	} else {
		locs, _, err = r.InventorySvc.ListLocationsPage(ctx, ptr.Deref(search), inventoryLocationFilter(filter), int32Or(limit, 0), int32Or(offset, 0))
	}
	if err != nil {
		return nil, err
	}
	out := make([]*model.InventoryLocation, len(locs))
	for i, l := range locs {
		out[i] = toInventoryLocationModel(l)
	}
	return out, nil
}

func (r *queryResolver) InventoryLocationsCount(ctx context.Context, search *string, filter *model.InventoryLocationFilter) (int32, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "inventory.view"); err != nil {
		return 0, err
	}
	n, err := r.InventorySvc.CountLocations(ctx, ptr.Deref(search), inventoryLocationFilter(filter))
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

func (r *queryResolver) InventoryLocation(ctx context.Context, id string) (*model.InventoryLocation, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "inventory.view"); err != nil {
		return nil, err
	}
	lid, err := parseID(id, "inventory location")
	if err != nil {
		return nil, err
	}
	loc, err := r.InventorySvc.GetLocation(ctx, lid)
	if err != nil {
		return nil, err
	}
	return toInventoryLocationModel(loc), nil
}

func (r *queryResolver) ProductInventory(ctx context.Context, productID string) (*model.InventoryItem, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "inventory.view"); err != nil {
		return nil, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	item, err := r.InventorySvc.GetItemByProduct(ctx, pid)
	if err != nil {
		if errors.Is(err, inventory.ErrItemNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return r.loadInventoryItemModel(ctx, item)
}

func (r *queryResolver) LowStockLevels(ctx context.Context, limit *int32) ([]*model.LowStockLevel, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "inventory.view"); err != nil {
		return nil, err
	}
	rows, err := r.InventorySvc.ListLowStockLevels(ctx, int32Or(limit, 0))
	if err != nil {
		return nil, err
	}
	out := make([]*model.LowStockLevel, len(rows))
	for i, row := range rows {
		lsl := &model.LowStockLevel{
			InventoryItemID:   row.InventoryItemID.String(),
			ProductID:         row.ProductID.String(),
			ProductSlug:       row.ProductSlug,
			ProductSku:        row.ProductSKU,
			QuantityDecimals:  int32(row.QuantityDecimals),
			QuantityStep:      row.QuantityStep,
			Quantity:          row.Quantity,
			LowStockThreshold: row.LowStockThreshold,
			Backorderable:     row.Backorderable,
			UpdatedAt:         row.UpdatedAt,
		}
		if row.LocationID != nil {
			id := row.LocationID.String()
			lsl.LocationID = &id
		}
		lsl.LocationSlug = row.LocationSlug
		lsl.LocationName = row.LocationName
		out[i] = lsl
	}
	return out, nil
}

// InventoryMode reads sales/inventory/flat_mode and maps it to the GraphQL enum. Unauthenticated; the dashboard fetches
// this at boot before the admin context is materialized to pick the right inventory UI shape
func (r *queryResolver) InventoryMode(ctx context.Context) (model.InventoryMode, error) {
	// Any signed-in admin renders inventory UI, so this is authentication rather than a slug - it just must not answer anonymous callers
	if _, err := auth.RequireAuth(ctx); err != nil {
		return "", err
	}
	enabled, err := r.ConfigSvc.GetBool(ctx, "sales/inventory/flat_mode", config.DefaultScope)
	if err != nil {
		// Lookup failure defaults to the multi-location code path to
		// match the FlatModeToggle's failure mode in main.go.
		return model.InventoryModeMultiLocation, nil
	}
	if enabled {
		return model.InventoryModeFlat, nil
	}
	return model.InventoryModeMultiLocation, nil
}

func (r *Resolver) InventoryLocation() InventoryLocationResolver {
	return &inventoryLocationResolver{r}
}

type inventoryLocationResolver struct{ *Resolver }
