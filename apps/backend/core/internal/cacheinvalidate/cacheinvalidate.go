// Package cacheinvalidate turns domain events into storefront cache purges - the policy is config-driven per surface, which is why it isn't just a purge call per event, and living here rather than in the composition root is what makes that branching testable
package cacheinvalidate

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type Purger interface { // the subset of revalidate.Client this package needs
	Purge(ctx context.Context, surface string, tags, paths []string, reason string, actor *uuid.UUID)
	PurgeSurface(ctx context.Context, key, reason string, actor *uuid.UUID)
}

type Rebuilder interface { // the durable queue the rebuild mode enqueues onto
	EnqueuePaths(ctx context.Context, surface string, paths, tags []string, reason string, actor *uuid.UUID) (int, error)
}

type PathResolver interface { // gives an entity its canonical storefront paths
	CanonicalPaths(ctx context.Context, entityType urlrewrite.EntityType, entityID uuid.UUID) []string
}

type ConfigReader interface { // reads the per-surface mode and per-trigger toggles
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
}

func Register(bus *events.Bus, cacheClient Purger, cacheQueue Rebuilder, urlRewriteSvc PathResolver, configSvc ConfigReader) { // subscription order is preserved from the composition root this came out of, since order within a priority tier is registration order
	// Tags alone only reach the storefront's Data Cache - the rendered HTML is keyed by PATH, so a purge without one leaves the page served from the route cache until its timer runs out - every reaction below sends the entity's canonical URLs for that reason
	bustCMSPage := func(ctx context.Context, pageID uuid.UUID, reason string) {
		paths := urlRewriteSvc.CanonicalPaths(ctx, urlrewrite.EntityTypeCMSPage, pageID)
		cacheClient.Purge(ctx, "cms", []string{"cms:page:" + pageID.String(), "cms:all"}, paths, reason, nil)
	}
	bus.Subscribe(cms.EventPagePublished, func(ctx context.Context, e events.Event) error {
		bustCMSPage(ctx, e.(*cms.PagePublished).PageID, "page published")
		return nil
	})
	bus.Subscribe(cms.EventPageUnpublished, func(ctx context.Context, e events.Event) error {
		bustCMSPage(ctx, e.(*cms.PageUnpublished).PageID, "page unpublished")
		return nil
	})

	reactToProduct := func(ctx context.Context, productID uuid.UUID, reason string) { // config-gated at Settings → Cache → Products: cache/product/mode = off | invalidate | rebuild, plus per-trigger toggles
		mode, _ := configSvc.GetString(ctx, catalog.CacheProductMode, config.DefaultScope)
		if mode == "" || mode == "off" || productID == uuid.Nil {
			return
		}
		paths := urlRewriteSvc.CanonicalPaths(ctx, urlrewrite.EntityTypeProduct, productID) // The PDP is cached at its pretty URL, not at /products/<uuid> - that typed route is always dynamic, so purging it invalidates nothing
		tags := []string{"product:" + productID.String()}
		if mode == "rebuild" {
			_, _ = cacheQueue.EnqueuePaths(ctx, "product", paths, tags, reason, nil)
			return
		}
		cacheClient.Purge(ctx, "product", tags, paths, reason, nil) // "invalidate"
	}
	productTrigger := func(ctx context.Context, path string) bool {
		on, _ := configSvc.GetBool(ctx, path, config.DefaultScope)
		return on
	}
	bus.Subscribe(catalog.EventProductUpdated, func(ctx context.Context, e events.Event) error {
		if productTrigger(ctx, catalog.CacheProductOnUpdate) {
			reactToProduct(ctx, e.(*catalog.ProductUpdated).ProductID, "product updated")
		}
		return nil
	})
	bus.Subscribe(inventory.EventStockChanged, func(ctx context.Context, e events.Event) error {
		if productTrigger(ctx, catalog.CacheProductOnStock) {
			reactToProduct(ctx, e.(*inventory.StockChanged).ProductID, "stock changed")
		}
		return nil
	})
	bus.Subscribe(catalog.EventProductDeleted, func(ctx context.Context, e events.Event) error {
		mode, _ := configSvc.GetString(ctx, catalog.CacheProductMode, config.DefaultScope) // A deleted product can't be re-rendered → always invalidate (never rebuild), still gated by mode (off = ignore) + the toggle
		if mode == "" || mode == "off" || !productTrigger(ctx, catalog.CacheProductOnDelete) {
			return nil
		}
		id := e.(*catalog.ProductDeleted).ProductID
		cacheClient.Purge(ctx, "product", []string{"product:" + id.String()}, nil, "product deleted", nil) // Its rewrite rows go with it, so the pretty path is already unresolvable here - the urlrewrite.Changed subscriber below is what actually evicts the deleted URL
		return nil
	})

	// Any canonical or redirect row that moves, appears or disappears evicts its path - this is the only reaction that catches a RENAME (the old URL must stop serving the cached page) and a delete, where the entity's rows are gone before its own event lands
	bus.Subscribe(urlrewrite.EventChanged, func(ctx context.Context, e events.Event) error {
		ch, ok := e.(*urlrewrite.Changed)
		if !ok || ch.Path == "" {
			return nil
		}
		cacheClient.Purge(ctx, "", nil, []string{urlrewrite.EnsureLeadingSlash(ch.Path)}, "url rewrite changed", nil)
		return nil
	})

	// The "nav" surface (channel branding, menus, presentment currencies, free-shipping threshold) feeds the root layout + header on every page and changes rarely, so any nav change busts the whole surface (nav:all) - ungated, because the storefront caches pages on a statically-analysable revalidate that no config can switch off, so skipping the purge only ever yields a stale page nobody can evict
	bustNav := func(ctx context.Context, reason string) {
		cacheClient.PurgeSurface(ctx, "nav", reason, nil)
	}
	bus.Subscribe(menu.EventMenuUpdated, func(ctx context.Context, e events.Event) error {
		bustNav(ctx, "menu updated")
		return nil
	})
	bus.Subscribe(channel.EventLocalesChanged, func(ctx context.Context, e events.Event) error {
		bustNav(ctx, "channel locales changed")
		return nil
	})
	bus.Subscribe(channel.EventDeleted, func(ctx context.Context, e events.Event) error {
		bustNav(ctx, "channel deleted")
		return nil
	})

	// The "catalog" surface covers category pages, product-listing grids (homepage / PLP) and collections - category/collection changes bust their own entry, while product + per-channel listing changes bust the grids (catalog:browse) since a grid lists many products; PDPs are the separate "product" surface above - always invalidate (mark stale, rebuild lazily), gated on cache/catalog/enabled
	bustCatalog := func(ctx context.Context, tags, paths []string, reason string) {
		cacheClient.Purge(ctx, "catalog", tags, paths, reason, nil)
	}
	// An entity's own page is cached at its pretty URL, so that gets an exact path purge - grids stay tag-only on purpose: every product edit touches them, and purging each listing path would stampede the renderer for a view where timer staleness is fine
	bustCatalogEntity := func(ctx context.Context, et urlrewrite.EntityType, id uuid.UUID, tags []string, reason string) {
		bustCatalog(ctx, tags, urlRewriteSvc.CanonicalPaths(ctx, et, id), reason)
	}
	bus.Subscribe(catalog.EventCategoryCreated, func(ctx context.Context, e events.Event) error {
		bustCatalogEntity(ctx, urlrewrite.EntityTypeCategory, e.(*catalog.CategoryCreated).CategoryID, []string{"category:" + e.(*catalog.CategoryCreated).CategoryID.String(), "catalog:browse"}, "category created")
		return nil
	})
	bus.Subscribe(catalog.EventCategoryUpdated, func(ctx context.Context, e events.Event) error {
		bustCatalogEntity(ctx, urlrewrite.EntityTypeCategory, e.(*catalog.CategoryUpdated).CategoryID, []string{"category:" + e.(*catalog.CategoryUpdated).CategoryID.String(), "catalog:browse"}, "category updated")
		return nil
	})
	bus.Subscribe(catalog.EventCategoryDeleted, func(ctx context.Context, e events.Event) error {
		bustCatalog(ctx, []string{"category:" + e.(*catalog.CategoryDeleted).CategoryID.String(), "catalog:browse"}, nil, "category deleted")
		return nil
	})
	bus.Subscribe(catalog.EventCollectionCreated, func(ctx context.Context, e events.Event) error {
		bustCatalogEntity(ctx, urlrewrite.EntityTypeCollection, e.(*catalog.CollectionCreated).CollectionID, []string{"collection:" + e.(*catalog.CollectionCreated).CollectionID.String(), "catalog:collections"}, "collection created")
		return nil
	})
	bus.Subscribe(catalog.EventCollectionUpdated, func(ctx context.Context, e events.Event) error {
		bustCatalogEntity(ctx, urlrewrite.EntityTypeCollection, e.(*catalog.CollectionUpdated).CollectionID, []string{"collection:" + e.(*catalog.CollectionUpdated).CollectionID.String(), "catalog:collections"}, "collection updated")
		return nil
	})
	bus.Subscribe(catalog.EventCollectionDeleted, func(ctx context.Context, e events.Event) error {
		bustCatalog(ctx, []string{"collection:" + e.(*catalog.CollectionDeleted).CollectionID.String(), "catalog:collections"}, nil, "collection deleted")
		return nil
	})
	bus.Subscribe(catalog.EventCollectionProductsChanged, func(ctx context.Context, e events.Event) error {
		bustCatalogEntity(ctx, urlrewrite.EntityTypeCollection, e.(*catalog.CollectionProductsChanged).CollectionID, []string{"collection:" + e.(*catalog.CollectionProductsChanged).CollectionID.String(), "catalog:collections", "catalog:browse"}, "collection products changed")
		return nil
	})
	bustGrids := func(ctx context.Context, reason string) { // Product + per-channel listing changes ripple into the grids that list them
		bustCatalog(ctx, []string{"catalog:browse"}, nil, reason)
	}
	bus.Subscribe(catalog.EventProductCreated, func(ctx context.Context, e events.Event) error { bustGrids(ctx, "product created"); return nil })
	bus.Subscribe(catalog.EventProductUpdated, func(ctx context.Context, e events.Event) error { bustGrids(ctx, "product updated"); return nil })
	bus.Subscribe(catalog.EventProductDeleted, func(ctx context.Context, e events.Event) error { bustGrids(ctx, "product deleted"); return nil })
	bus.Subscribe(catalog.EventProductListingChanged, func(ctx context.Context, e events.Event) error { bustGrids(ctx, "product listing changed"); return nil })
}
