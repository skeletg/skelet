package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
)

// Lives in its own file so gqlgen's regen of inventory.resolvers.go doesn't strip it on every schema change
func resolveSyncProductID(ctx context.Context, catalogSvc *catalog.Service, input model.SyncProductInventoryInput) (uuid.UUID, error) {
	if input.ProductSku != nil && strings.TrimSpace(*input.ProductSku) != "" { // SKU wins so an ERP syncing by SKU can't accidentally hit an admin-pasted UUID
		p, err := catalogSvc.GetProductBySKU(ctx, strings.TrimSpace(*input.ProductSku))
		if err != nil {
			return uuid.Nil, fmt.Errorf("product not found for sku %q: %w", *input.ProductSku, err)
		}
		return p.ID, nil
	}
	if input.ProductID != nil && strings.TrimSpace(*input.ProductID) != "" {
		pid, err := uuid.Parse(strings.TrimSpace(*input.ProductID))
		if err != nil {
			return uuid.Nil, fmt.Errorf("invalid product id %q", *input.ProductID)
		}
		return pid, nil
	}
	return uuid.Nil, fmt.Errorf("productSku or productId is required")
}

func toInventoryLocationModel(l inventory.Location) *model.InventoryLocation {
	var lat, lng *float64
	if l.Latitude != nil {
		v := *l.Latitude
		lat = &v
	}
	if l.Longitude != nil {
		v := *l.Longitude
		lng = &v
	}
	hours := l.Hours
	if len(hours) == 0 {
		hours = json.RawMessage(`{}`)
	}
	var hoursAny any
	_ = json.Unmarshal(hours, &hoursAny)
	if hoursAny == nil {
		hoursAny = map[string]any{}
	}
	return &model.InventoryLocation{
		StorefrontVisible:  l.StorefrontVisible,
		PickupEnabled:      l.PickupEnabled,
		PickupInstructions: l.PickupInstructions,
		ID:                 l.ID.String(),
		Slug:               l.Slug,
		Name:               l.Name,
		IsActive:           l.IsActive,
		Type:               toInventoryLocationType(l.Type),
		Line1:              l.Line1,
		Line2:              l.Line2,
		City:               l.City,
		Region:             l.Region,
		PostalCode:         l.PostalCode,
		CountryCode:        l.CountryCode,
		Latitude:           lat,
		Longitude:          lng,
		Timezone:           l.Timezone,
		Hours:              hoursAny,
		Email:              l.Email,
		Phone:              l.Phone,
		ContactName:        l.ContactName,
		ExternalReference:  l.ExternalReference,
		Metadata:           invMetadataItems(l.Metadata),
		CreatedAt:          l.CreatedAt,
		UpdatedAt:          l.UpdatedAt,
	}
}

func toInventoryLevelModel(l inventory.Level, location inventory.Location, netReservation float64) *model.InventoryLevel {
	reserved := netReservation // negative when held
	available := l.Quantity
	if reserved < 0 {
		available = l.Quantity + reserved
		if available < 0 {
			available = 0
		}
	}
	return &model.InventoryLevel{
		Location:         toInventoryLocationModel(location),
		Quantity:         l.Quantity,
		ReservedQuantity: reserved,
		Available:        available,
	}
}

func (r *Resolver) loadInventoryItemModel(ctx context.Context, item inventory.Item) (*model.InventoryItem, error) {
	product, err := r.CatalogSvc.GetProductByID(ctx, item.ProductID)
	if err != nil {
		return nil, err
	}
	levels, err := r.InventorySvc.ListLevelsForItem(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	netByLoc, err := r.InventorySvc.NetReservationsForItem(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	locIDs := make([]uuid.UUID, len(levels))
	for i, lvl := range levels {
		locIDs[i] = lvl.LocationID
	}
	// One batched read instead of a GetLocation per level - a product stocked in hundreds of locations was hundreds of sequential round trips
	locsByID, err := r.InventorySvc.GetLocationsByIDs(ctx, locIDs)
	if err != nil {
		return nil, err
	}
	out := make([]*model.InventoryLevel, 0, len(levels))
	for _, lvl := range levels {
		loc, ok := locsByID[lvl.LocationID]
		if !ok {
			continue
		}
		out = append(out, toInventoryLevelModel(lvl, loc, netByLoc[lvl.LocationID]))
	}
	productModel, err := r.renderProduct(ctx, product)
	if err != nil {
		return nil, err
	}
	return &model.InventoryItem{
		ID:                item.ID.String(),
		Product:           productModel,
		Tracked:           item.Tracked,
		Backorderable:     item.Backorderable,
		LowStockThreshold: item.LowStockThreshold,
		Levels:            out,
		Quantity:          item.Quantity,
		LastSyncedAt:      item.LastSyncedAt,
		LastSyncedSource:  item.LastSyncedSource,
	}, nil
}

func invMetadataItems(m inventory.Metadata) []*model.MetadataItem {
	if len(m) == 0 {
		return []*model.MetadataItem{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*model.MetadataItem, len(keys))
	for i, k := range keys {
		out[i] = &model.MetadataItem{Key: k, Value: m[k]}
	}
	return out
}

func invMetadataFromInput(in []*model.MetadataInput) inventory.Metadata {
	if len(in) == 0 {
		return inventory.Metadata{}
	}
	out := make(inventory.Metadata, len(in))
	for _, item := range in {
		if item == nil {
			continue
		}
		out[item.Key] = item.Value
	}
	return out
}

// The one mapping between the GraphQL enum and the stored location type, read in both directions
var locationTypes = map[model.InventoryLocationType]string{
	model.InventoryLocationTypeRetailStore:        inventory.LocationRetail,
	model.InventoryLocationTypeWarehouse:          inventory.LocationWarehouse,
	model.InventoryLocationTypeStorage:            inventory.LocationStorage,
	model.InventoryLocationTypeDropshipper:        inventory.LocationDropshipper,
	model.InventoryLocationTypeFulfillmentPartner: inventory.LocationFulfillment,
	model.InventoryLocationTypeUnclassified:       inventory.LocationUnclassified,
}

func toInventoryLocationType(value string) model.InventoryLocationType {
	for enum, stored := range locationTypes {
		if stored == value {
			return enum
		}
	}
	return model.InventoryLocationTypeUnclassified
}

func inventoryLocationTypeInput(value *model.InventoryLocationType) *string {
	if value == nil {
		return nil
	}
	v, ok := locationTypes[*value]
	if !ok {
		v = string(*value) // unmapped reaches the service, which rejects it rather than filing it as unclassified
	}
	return &v
}

func inventoryLocationFilter(input *model.InventoryLocationFilter) inventory.LocationFilter {
	if input == nil {
		return inventory.LocationFilter{}
	}
	out := inventory.LocationFilter{City: ptr.Deref(input.City), IsActive: input.IsActive, StorefrontVisible: input.StorefrontVisible, PickupEnabled: input.PickupEnabled}
	for _, typ := range input.Types {
		out.Types = append(out.Types, *inventoryLocationTypeInput(&typ))
	}
	return out
}
