package front

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
)

// storeProduct resolves a product the channel publishes into what store stock is checked against. Inventory can't
// read the catalog, so the "only simple, physical products have store stock" rule lives here; nil means the product
// has none, and the store directory still answers
func (r *Resolver) storeProduct(ctx context.Context, rawID string, channelID uuid.UUID) (*inventory.StoreProduct, error) {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return nil, invalidID("product")
	}
	p, err := r.CatalogSvc.GetProductHeaderForChannel(ctx, id, channelID, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	pt, err := r.CatalogSvc.GetProductType(ctx, p.ProductTypeID)
	if err != nil {
		return nil, err
	}
	if pt.IsDigital || p.Kind != catalog.KindSimple {
		return nil, nil
	}
	return &inventory.StoreProduct{ID: p.ID, QuantityStep: p.QuantityStep, MinOrderQty: p.MinOrderQty}, nil
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
