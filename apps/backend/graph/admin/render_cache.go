package admin

import (
	"context"
	"maps"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/inventory"
)

// renderCache is a request-scoped batch of pre-fetched rows - list resolvers prime it once up-front so per-product field resolvers (and renderProduct / renderProductListings) read the maps instead of firing one round-trip per row; nil-safe, helpers fall through to a normal single-row fetch when the entry is missing
type renderCache struct {
	mu sync.RWMutex

	productTypes     map[uuid.UUID]catalog.ProductType
	listings         map[uuid.UUID][]catalog.ProductChannelListing
	channels         map[uuid.UUID]channel.Channel
	salable          map[uuid.UUID]float64
	channelLocations map[uuid.UUID][]inventory.Location
}

type renderCacheCtxKey struct{}

func withRenderCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, renderCacheCtxKey{}, newRenderCache())
}

// Without this middleware on the request chain the primeX/cachedX/getX helpers all fall through to single-row fetches
func RenderCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(withRenderCache(r.Context())))
	})
}

func getRenderCache(ctx context.Context) *renderCache {
	c, _ := ctx.Value(renderCacheCtxKey{}).(*renderCache)
	return c
}

func newRenderCache() *renderCache {
	return &renderCache{
		productTypes:     map[uuid.UUID]catalog.ProductType{},
		listings:         map[uuid.UUID][]catalog.ProductChannelListing{},
		channels:         map[uuid.UUID]channel.Channel{},
		salable:          map[uuid.UUID]float64{},
		channelLocations: map[uuid.UUID][]inventory.Location{},
	}
}

func (r *Resolver) getProductType(ctx context.Context, id uuid.UUID) (catalog.ProductType, error) {
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		pt, ok := c.productTypes[id]
		c.mu.RUnlock()
		if ok {
			return pt, nil
		}
	}
	pt, err := r.CatalogSvc.GetProductType(ctx, id)
	if err != nil {
		return catalog.ProductType{}, err
	}
	if c != nil {
		c.mu.Lock()
		c.productTypes[id] = pt
		c.mu.Unlock()
	}
	return pt, nil
}

func (r *Resolver) getChannel(ctx context.Context, id uuid.UUID) (channel.Channel, error) {
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		sv, ok := c.channels[id]
		c.mu.RUnlock()
		if ok {
			return sv, nil
		}
	}
	sv, err := r.ChannelSvc.GetByID(ctx, id)
	if err != nil {
		return channel.Channel{}, err
	}
	if c != nil {
		c.mu.Lock()
		c.channels[id] = sv
		c.mu.Unlock()
	}
	return sv, nil
}

func (r *Resolver) getProductChannelListings(ctx context.Context, productID uuid.UUID) ([]catalog.ProductChannelListing, error) {
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		rows, ok := c.listings[productID]
		c.mu.RUnlock()
		if ok {
			return rows, nil
		}
	}
	rows, err := r.CatalogSvc.ListProductChannelListings(ctx, productID)
	if err != nil {
		return nil, err
	}
	if c != nil {
		c.mu.Lock()
		c.listings[productID] = rows
		c.mu.Unlock()
	}
	return rows, nil
}

// primeListingsAndChannels loads listings + their channels in two queries - mirrors the front-surface helper
func (r *Resolver) primeListingsAndChannels(ctx context.Context, productIDs []uuid.UUID) error {
	c := getRenderCache(ctx)
	if c == nil || len(productIDs) == 0 {
		return nil
	}
	byProduct, err := r.CatalogSvc.ListProductChannelListingsByProductIDs(ctx, productIDs)
	if err != nil {
		return err
	}
	svIDSet := map[uuid.UUID]struct{}{}
	for _, rows := range byProduct {
		for _, row := range rows {
			svIDSet[row.ChannelID] = struct{}{}
		}
	}
	c.mu.Lock()
	maps.Copy(c.listings, byProduct)
	for _, pid := range productIDs {
		if _, ok := c.listings[pid]; !ok {
			c.listings[pid] = nil
		}
	}
	c.mu.Unlock()

	if len(svIDSet) > 0 {
		ids := make([]uuid.UUID, 0, len(svIDSet))
		for id := range svIDSet {
			ids = append(ids, id)
		}
		byID, err := r.ChannelSvc.ListByIDs(ctx, ids)
		if err != nil {
			return err
		}
		c.mu.Lock()
		maps.Copy(c.channels, byID)
		c.mu.Unlock()
	}
	return nil
}

func (r *Resolver) primeSalable(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) error {
	c := getRenderCache(ctx)
	if c == nil || len(productIDs) == 0 || channelID == uuid.Nil || r.InventorySvc == nil {
		return nil
	}
	bySalable, err := r.InventorySvc.SalableByProductIDs(ctx, productIDs, channelID)
	if err != nil {
		return err
	}
	c.mu.Lock()
	maps.Copy(c.salable, bySalable)
	c.mu.Unlock()
	return nil
}

func (r *Resolver) cachedSalable(ctx context.Context, productID uuid.UUID) (float64, bool) {
	c := getRenderCache(ctx)
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.salable[productID]
	return v, ok
}
