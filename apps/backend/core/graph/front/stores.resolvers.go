package front

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
)

// The feature gate is read live, so a cached product page never shows stores after they were switched off
func (r *queryResolver) PublicStores(ctx context.Context, input *model.PublicStoresInput) (*model.PublicStoresResult, error) {
	out := &model.PublicStoresResult{Cities: []string{}, Items: []*model.PublicStore{}}
	channelID := channelIDFromContext(ctx)
	if r.ConfigSvc == nil || r.InventorySvc == nil || channelID == nil {
		return out, nil
	}
	enabled, err := r.ConfigSvc.GetBool(ctx, "sales/store_availability/enabled", requestConfigScope(ctx))
	if err != nil || !enabled {
		return out, err
	}
	if err = r.checkCatalogRate(ctx); err != nil {
		return nil, err
	}
	out.Enabled = true
	if input == nil {
		input = &model.PublicStoresInput{}
	}
	q := inventory.StoreQuery{
		ChannelID:   *channelID,
		City:        ptr.Deref(input.City),
		Search:      ptr.Deref(input.Search),
		PickupOnly:  ptr.Deref(input.PickupOnly),
		InStockOnly: ptr.Deref(input.InStockOnly),
		Latitude:    input.Latitude,
		Longitude:   input.Longitude,
		Limit:       int(ptr.Deref(input.Limit)),
		Offset:      int(ptr.Deref(input.Offset)),
	}
	if input.PreferredStoreID != nil {
		id, err := uuid.Parse(*input.PreferredStoreID)
		if err != nil {
			return nil, invalidID("store")
		}
		q.PreferredStoreID = &id
	}
	if input.ProductID != nil {
		if q.Product, err = r.storeProduct(ctx, *input.ProductID, *channelID); err != nil {
			return nil, err
		}
	}
	page, err := r.InventorySvc.PublicStores(ctx, q)
	if err != nil {
		return nil, err
	}
	out.StockCheckingAvailable = page.StockCheckingAvailable
	out.ProductEligible = page.ProductEligible
	out.HasPickupStores = page.HasPickupStores
	out.Cities = page.Cities
	out.TotalCount = int32(page.TotalCount)
	for _, row := range page.Items {
		out.Items = append(out.Items, toPublicStore(row))
	}
	if page.PreferredStore != nil {
		out.PreferredStore = toPublicStore(*page.PreferredStore)
	}
	return out, nil
}
