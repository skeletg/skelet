// Package cacheinvalidate turns domain events into storefront cache purges. The policy is config-driven per surface, which is why it isn't just a purge call per event, and living here rather than in the composition root is what makes that branching testable.
package cacheinvalidate

import (
	"context"
	"errors"
	"slices"

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

type ConfigReader interface { // reads the per-surface mode and per-trigger toggles, and reports setting changes
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
	OnChange(fn func(ctx context.Context, def config.Definition))
}

// Tag and surface names the storefront caches under - a contract with its fetch tags, so they live in one place
const (
	tagCMSAll         = "cms:all"
	tagBrowse         = "catalog:browse"
	tagCollections    = "catalog:collections"
	tagPublicSettings = "config:public"
	settingsSurface   = "config"
)

func productTag(id uuid.UUID) string    { return "product:" + id.String() }
func categoryTag(id uuid.UUID) string   { return "category:" + id.String() }
func collectionTag(id uuid.UUID) string { return "collection:" + id.String() }
func cmsPageTag(id uuid.UUID) string    { return "cms:page:" + id.String() }

type reactor struct {
	batch *Batcher
	purge Purger
	queue Rebuilder
	paths PathResolver
	cfg   ConfigReader
}

// Subscribes every reaction and returns the batcher their purges gather in, which the caller flushes on shutdown
func Register(bus *events.Bus, purger Purger, queue Rebuilder, paths PathResolver, cfg ConfigReader) *Batcher {
	r := reactor{batch: NewBatcher(purger, batchWindow), purge: purger, queue: queue, paths: paths, cfg: cfg}

	// Tags alone only reach the storefront's Data Cache - the rendered HTML is keyed by PATH, so a purge without one leaves the page served from the route cache until its timer runs out - every entity reaction below sends its canonical URLs for that reason
	on(bus, cms.EventPagePublished, func(ctx context.Context, e *cms.PagePublished) error {
		r.cmsPage(ctx, e.PageID, "page published")
		return nil
	})
	on(bus, cms.EventPageUnpublished, func(ctx context.Context, e *cms.PageUnpublished) error {
		r.cmsPage(ctx, e.PageID, "page unpublished")
		return nil
	})

	// Product edits also ripple into the grids that list them - grids stay tag-only on purpose: every product edit touches them, and purging each listing path would stampede the renderer for a view where timer staleness is fine
	on(bus, catalog.EventProductUpdated, func(ctx context.Context, e *catalog.ProductUpdated) error {
		r.grids("product updated")
		return r.product(ctx, e.ProductID, catalog.CacheProductOnUpdate, "product updated")
	})
	on(bus, inventory.EventStockChanged, func(ctx context.Context, e *inventory.StockChanged) error {
		return r.product(ctx, e.ProductID, catalog.CacheProductOnStock, "stock changed")
	})
	on(bus, catalog.EventProductDeleted, func(ctx context.Context, e *catalog.ProductDeleted) error {
		r.grids("product deleted")
		r.productDeleted(ctx, e.ProductID)
		return nil
	})
	on(bus, catalog.EventProductCreated, func(context.Context, *catalog.ProductCreated) error {
		r.grids("product created")
		return nil
	})
	on(bus, catalog.EventProductListingChanged, func(ctx context.Context, e *catalog.ProductListingChanged) error {
		r.grids("product listing changed")
		if e.Cleared || !e.IsPublished {
			r.productEvict(ctx, e.ProductID, "product unlisted")
			return nil
		}
		return r.product(ctx, e.ProductID, catalog.CacheProductOnUpdate, "product listing changed")
	})

	// Attribute edits reshape facets on every grid and the attribute block on each product carrying it - a visibility flip may hide a value, so it always evicts
	on(bus, catalog.EventAttributeUpdated, func(ctx context.Context, e *catalog.AttributeUpdated) error {
		r.grids("attribute updated")
		evict := slices.Contains(e.ChangedFields, catalog.AttributeFieldIsPublic)
		var errs []error
		for _, id := range e.ProductIDs {
			if evict {
				r.productEvict(ctx, id, "attribute visibility changed")
				continue
			}
			errs = append(errs, r.product(ctx, id, catalog.CacheProductOnUpdate, "attribute updated"))
		}
		return errors.Join(errs...)
	})
	on(bus, catalog.EventAttributeDeleted, func(ctx context.Context, e *catalog.AttributeDeleted) error {
		r.grids("attribute deleted")
		for _, id := range e.ProductIDs {
			r.productEvict(ctx, id, "attribute deleted")
		}
		return nil
	})

	// Any canonical or redirect row that moves, appears or disappears evicts its path - the only reaction that catches a RENAME (the old URL must stop serving the cached page) and a delete, where the entity's rows are gone before its own event lands
	on(bus, urlrewrite.EventChanged, func(_ context.Context, e *urlrewrite.Changed) error {
		if e.Path != "" {
			r.batch.Purge("", nil, []string{urlrewrite.EnsureLeadingSlash(e.Path)}, "url rewrite changed")
		}
		return nil
	})

	// Category and collection pages are cached at their pretty URL; a deleted one has no rows left to resolve, so the urlrewrite reaction above evicts its path
	category := func(id uuid.UUID) []string { return []string{categoryTag(id), tagBrowse} }
	collection := func(id uuid.UUID) []string { return []string{collectionTag(id), tagCollections} }
	catalogEntity(bus, r, catalog.EventCategoryCreated, func(e *catalog.CategoryCreated) uuid.UUID { return e.CategoryID }, urlrewrite.EntityTypeCategory, category, "category created")
	catalogEntity(bus, r, catalog.EventCategoryUpdated, func(e *catalog.CategoryUpdated) uuid.UUID { return e.CategoryID }, urlrewrite.EntityTypeCategory, category, "category updated")
	catalogEntity(bus, r, catalog.EventCategoryDeleted, func(e *catalog.CategoryDeleted) uuid.UUID { return e.CategoryID }, "", category, "category deleted")
	catalogEntity(bus, r, catalog.EventCollectionCreated, func(e *catalog.CollectionCreated) uuid.UUID { return e.CollectionID }, urlrewrite.EntityTypeCollection, collection, "collection created")
	catalogEntity(bus, r, catalog.EventCollectionUpdated, func(e *catalog.CollectionUpdated) uuid.UUID { return e.CollectionID }, urlrewrite.EntityTypeCollection, collection, "collection updated")
	catalogEntity(bus, r, catalog.EventCollectionDeleted, func(e *catalog.CollectionDeleted) uuid.UUID { return e.CollectionID }, "", collection, "collection deleted")
	catalogEntity(bus, r, catalog.EventCollectionProductsChanged, func(e *catalog.CollectionProductsChanged) uuid.UUID { return e.CollectionID }, urlrewrite.EntityTypeCollection,
		func(id uuid.UUID) []string { return append(collection(id), tagBrowse) }, "collection products changed")

	// The nav surface (channel fields, menus, locales) feeds the header on every page and changes rarely, so any change busts it whole - ungated, because the storefront caches pages on a statically-analysable revalidate that no config can switch off, so skipping the purge only ever yields a stale page nobody can evict
	// Categories and collections feed the header's menus too, so any change to one busts it like a menu edit
	for _, n := range []struct{ event, reason string }{
		{menu.EventMenuUpdated, "menu updated"},
		{channel.EventUpdated, "channel updated"},
		{channel.EventLocalesChanged, "channel locales changed"},
		{channel.EventDeleted, "channel deleted"},
		{catalog.EventCategoryCreated, "category created"},
		{catalog.EventCategoryUpdated, "category updated"},
		{catalog.EventCategoryDeleted, "category deleted"},
		{catalog.EventCollectionCreated, "collection created"},
		{catalog.EventCollectionUpdated, "collection updated"},
		{catalog.EventCollectionDeleted, "collection deleted"},
	} {
		bus.Subscribe(n.event, func(ctx context.Context, _ events.Event) error {
			purger.PurgeSurface(ctx, menu.CacheSurface, n.reason, nil)
			return nil
		})
	}

	// The storefront caches every public setting under one tag, and a save fires no domain event
	cfg.OnChange(func(_ context.Context, def config.Definition) {
		if def.Public {
			r.batch.Purge(settingsSurface, []string{tagPublicSettings}, nil, "setting changed: "+def.Path)
		}
	})
	return r.batch
}

// Subscribes fn to one event type, skipping anything that isn't a T instead of panicking on it
func on[T events.Event](bus *events.Bus, eventType string, fn func(context.Context, T) error) {
	bus.Subscribe(eventType, func(ctx context.Context, e events.Event) error {
		ev, ok := e.(T)
		if !ok {
			return nil
		}
		return fn(ctx, ev)
	})
}

// A category or collection event: its own tags plus, when it still resolves, its page's canonical paths
func catalogEntity[T events.Event](bus *events.Bus, r reactor, eventType string, id func(T) uuid.UUID, et urlrewrite.EntityType, tags func(uuid.UUID) []string, reason string) {
	on(bus, eventType, func(ctx context.Context, e T) error {
		entityID := id(e)
		var paths []string
		if et != "" {
			paths = r.paths.CanonicalPaths(ctx, et, entityID)
		}
		r.batch.Purge(catalog.CacheSurfaceCatalog, tags(entityID), paths, reason)
		return nil
	})
}

func (r reactor) cmsPage(ctx context.Context, pageID uuid.UUID, reason string) {
	r.batch.Purge(cms.CacheSurface, []string{cmsPageTag(pageID), tagCMSAll}, r.paths.CanonicalPaths(ctx, urlrewrite.EntityTypeCMSPage, pageID), reason)
}

func (r reactor) grids(reason string) {
	r.batch.Purge(catalog.CacheSurfaceCatalog, []string{tagBrowse}, nil, reason)
}

// Config-gated at Settings → Cache → Products: cache/product/mode = off | invalidate | rebuild, plus a toggle per trigger
func (r reactor) product(ctx context.Context, productID uuid.UUID, trigger, reason string) error {
	mode := r.productMode(ctx)
	if mode == "" || productID == uuid.Nil || !r.toggle(ctx, trigger) {
		return nil
	}
	paths := r.paths.CanonicalPaths(ctx, urlrewrite.EntityTypeProduct, productID) // the PDP is cached at its pretty URL, not at /products/<uuid> - that typed route is always dynamic, so purging it invalidates nothing
	tags := []string{productTag(productID)}
	if mode == "rebuild" {
		_, err := r.queue.EnqueuePaths(ctx, catalog.CacheSurfaceProduct, paths, tags, reason, nil)
		return err
	}
	r.batch.Purge(catalog.CacheSurfaceProduct, tags, paths, reason)
	return nil
}

// A deleted product can't be re-rendered, so it always invalidates, never rebuilds - its rewrite rows go with it, so the urlrewrite reaction is what evicts the URL
func (r reactor) productDeleted(ctx context.Context, productID uuid.UUID) {
	if r.productMode(ctx) == "" || !r.toggle(ctx, catalog.CacheProductOnDelete) {
		return
	}
	r.batch.Purge(catalog.CacheSurfaceProduct, []string{productTag(productID)}, nil, "product deleted")
}

func (r reactor) productEvict(ctx context.Context, productID uuid.UUID, reason string) {
	if productID == uuid.Nil {
		return
	}
	r.batch.Purge(catalog.CacheSurfaceProduct, []string{productTag(productID)}, r.paths.CanonicalPaths(ctx, urlrewrite.EntityTypeProduct, productID), reason)
}

// The configured product mode, "" when off
func (r reactor) productMode(ctx context.Context) string {
	mode, _ := r.cfg.GetString(ctx, catalog.CacheProductMode, config.DefaultScope)
	if mode == "off" {
		return ""
	}
	return mode
}

func (r reactor) toggle(ctx context.Context, path string) bool {
	enabled, _ := r.cfg.GetBool(ctx, path, config.DefaultScope)
	return enabled
}
