package front

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
)

// A live feature gate is required even when the product page was cached.
func (r *Resolver) publicStores(ctx context.Context, input *model.PublicStoresInput) (*model.PublicStoresResult, error) {
	out := &model.PublicStoresResult{Cities: []string{}, Items: []*model.PublicStore{}}
	if r.ConfigSvc == nil || r.InventorySvc == nil || channelIDFromContext(ctx) == nil {
		return out, nil
	}
	enabled, err := r.ConfigSvc.GetBool(ctx, "sales/store_availability/enabled", requestConfigScope(ctx))
	if err != nil {
		return nil, err
	}
	if !enabled {
		return out, nil
	}
	if err = r.checkCatalogRate(ctx); err != nil {
		return nil, err
	}
	out.Enabled = true
	if input == nil {
		input = &model.PublicStoresInput{}
	}
	q := inventory.StoreQuery{ChannelID: *channelIDFromContext(ctx), Latitude: input.Latitude, Longitude: input.Longitude}
	if input.City != nil {
		q.City = *input.City
	}
	if input.Search != nil {
		q.Search = *input.Search
	}
	if input.PickupOnly != nil {
		q.PickupOnly = *input.PickupOnly
	}
	if input.InStockOnly != nil {
		q.InStockOnly = *input.InStockOnly
	}
	if input.Limit != nil {
		q.Limit = int(*input.Limit)
	}
	if input.Offset != nil {
		q.Offset = int(*input.Offset)
	}
	if input.PreferredStoreID != nil {
		id, e := uuid.Parse(*input.PreferredStoreID)
		if e != nil {
			return nil, invalidID("store")
		}
		q.PreferredStoreID = &id
	}
	if input.ProductID != nil {
		id, e := uuid.Parse(*input.ProductID)
		if e != nil {
			return nil, invalidID("product")
		}
		p, e := r.CatalogSvc.GetProductHeaderForChannel(ctx, id, q.ChannelID, r.activeLocale(ctx))
		if e != nil {
			return nil, e
		}
		pt, e := r.CatalogSvc.GetProductType(ctx, p.ProductTypeID)
		if e != nil {
			return nil, e
		}
		if pt.IsDigital || p.Kind != catalog.KindSimple {
			return out, nil
		}
		q.Product = &inventory.StoreProduct{ID: p.ID, QuantityStep: p.QuantityStep, MinOrderQty: p.MinOrderQty}
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

func toPublicStore(row inventory.PublicLocation) *model.PublicStore {
	l := row.Location
	var hours any
	if json.Unmarshal(l.Hours, &hours) != nil || hours == nil {
		hours = map[string]any{}
	}
	out := &model.PublicStore{ID: l.ID.String(), Slug: l.Slug, Name: l.Name, Line1: l.Line1, Line2: l.Line2, City: l.City, Region: l.Region, PostalCode: l.PostalCode, CountryCode: l.CountryCode, Latitude: l.Latitude, Longitude: l.Longitude, Timezone: l.Timezone, Hours: hours, Phone: l.Phone, PickupEnabled: l.PickupEnabled, DistanceKm: row.DistanceKm}
	if row.Status != nil {
		status := model.StoreStockStatus(*row.Status)
		out.StockStatus = &status
	}
	return out
}
