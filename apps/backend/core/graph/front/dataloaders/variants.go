package dataloaders

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/graph-gophers/dataloader/v7"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

type variantSource interface {
	GetProductOptionsByProductIDs(ctx context.Context, productIDs []uuid.UUID, locale string) (map[uuid.UUID][]catalog.ProductOption, error)
	ListConfigurableParentByChildIDs(ctx context.Context, childIDs []uuid.UUID) (map[uuid.UUID]catalog.ConfigurableParentLink, error)
	ListReadyProductMediaByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]catalog.ProductMedia, error)
	ListProductMediaRolesByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]map[catalog.MediaRole]uuid.UUID, error)
}

// gqlgen resolves siblings in waves that miss the batch window, so the first lookup for any member fetches the whole group
type variantGroup struct {
	members []uuid.UUID

	mu    sync.Mutex
	onces map[string]*sync.Once
}

func (g *variantGroup) once(key string) *sync.Once {
	g.mu.Lock()
	defer g.mu.Unlock()
	o, ok := g.onces[key]
	if !ok {
		o = &sync.Once{}
		g.onces[key] = o
	}
	return o
}

func (l *Loaders) RegisterVariants(parentID uuid.UUID, childIDs []uuid.UUID) {
	if len(childIDs) == 0 {
		return
	}
	l.RegisterGroup(append([]uuid.UUID{parentID}, childIDs...))
}

func (l *Loaders) RegisterGroup(ids []uuid.UUID) {
	if l == nil || len(ids) < 2 {
		return
	}
	g := &variantGroup{members: append([]uuid.UUID(nil), ids...), onces: map[string]*sync.Once{}}
	l.variantsMu.Lock()
	defer l.variantsMu.Unlock()
	if l.variants == nil {
		l.variants = map[uuid.UUID]*variantGroup{}
	}
	for _, id := range g.members {
		if _, taken := l.variants[id]; !taken {
			l.variants[id] = g
		}
	}
}

func (l *Loaders) GroupMembers(id uuid.UUID) []uuid.UUID {
	if l == nil {
		return nil
	}
	if g := l.variantGroupOf(id); g != nil {
		return g.members
	}
	return nil
}

func (l *Loaders) variantGroupOf(id uuid.UUID) *variantGroup {
	l.variantsMu.Lock()
	defer l.variantsMu.Unlock()
	return l.variants[id]
}

// A failed group fetch primes nothing, so Load falls back to the batched path
func primeGroup[V any](ctx context.Context, l *Loaders, id uuid.UUID, key string, loader *dataloader.Loader[uuid.UUID, V], fetch func(context.Context, []uuid.UUID) (map[uuid.UUID]V, error)) {
	g := l.variantGroupOf(id)
	if g == nil || loader == nil {
		return
	}
	g.once(key).Do(func() {
		byID, err := fetch(ctx, g.members)
		if err != nil {
			return
		}
		for _, m := range g.members {
			loader.Prime(ctx, m, byID[m])
		}
	})
}

func (l *Loaders) LoadProductMedia(ctx context.Context, id uuid.UUID) ([]catalog.ProductMedia, error) {
	if l.catalog != nil {
		primeGroup(ctx, l, id, "media", l.ProductMedia, l.catalog.ListReadyProductMediaByProductIDs)
	}
	return l.ProductMedia.Load(ctx, id)()
}

func (l *Loaders) LoadProductMediaRoles(ctx context.Context, id uuid.UUID) (map[catalog.MediaRole]uuid.UUID, error) {
	if l.catalog != nil {
		primeGroup(ctx, l, id, "roles", l.ProductMediaRoles, l.catalog.ListProductMediaRolesByProductIDs)
	}
	return l.ProductMediaRoles.Load(ctx, id)()
}

func (l *Loaders) LoadProductRow(ctx context.Context, id uuid.UUID) (catalog.Product, error) {
	if l.svc != nil {
		primeGroup(ctx, l, id, "row", l.ProductRow, l.svc.ListProductRowsByIDs)
	}
	return l.ProductRow.Load(ctx, id)()
}

func (l *Loaders) PrimeProductRow(ctx context.Context, p catalog.Product) {
	if l != nil && l.ProductRow != nil {
		l.ProductRow.Prime(ctx, p.ID, p)
	}
}

func (l *Loaders) LoadProductAllContent(ctx context.Context, id uuid.UUID) ([]catalog.ProductContent, error) {
	if l.svc != nil {
		primeGroup(ctx, l, id, "content", l.ProductAllContent, l.svc.ListProductContentByProductIDs)
	}
	return l.ProductAllContent.Load(ctx, id)()
}

func (l *Loaders) LoadProductSelections(ctx context.Context, id uuid.UUID) ([]catalog.ProductAttributeSelection, error) {
	if l.svc != nil {
		primeGroup(ctx, l, id, "selections", l.ProductSelections, l.svc.LoadProductsAttributeSelections)
	}
	return l.ProductSelections.Load(ctx, id)()
}

func (l *Loaders) LoadProductReferences(ctx context.Context, id uuid.UUID) ([]catalog.ProductAttributeReference, error) {
	if l.svc != nil {
		primeGroup(ctx, l, id, "references", l.ProductReferences, l.svc.LoadProductsAttributeReferences)
	}
	return l.ProductReferences.Load(ctx, id)()
}

func (l *Loaders) LoadConfigurableOptions(ctx context.Context, id uuid.UUID) ([]catalog.Attribute, error) {
	if l.svc != nil {
		primeGroup(ctx, l, id, "configurable-options", l.ConfigurableOptions, l.svc.ListConfigurableOptionsByProductIDs)
	}
	return l.ConfigurableOptions.Load(ctx, id)()
}

func (l *Loaders) LoadProductListings(ctx context.Context, id uuid.UUID) ([]catalog.ProductChannelListing, error) {
	if l.svc != nil {
		primeGroup(ctx, l, id, "listings", l.ProductListings, func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]catalog.ProductChannelListing, error) {
			return StorefrontListings(ctx, l.svc, ids)
		})
	}
	return l.ProductListings.Load(ctx, id)()
}

func (l *Loaders) LoadConfigurableParent(ctx context.Context, childID uuid.UUID) (catalog.ConfigurableParentLink, error) {
	if l.catalog != nil {
		primeGroup(ctx, l, childID, "links", l.ConfigurableParent, l.catalog.ListConfigurableParentByChildIDs)
	}
	return l.ConfigurableParent.Load(ctx, childID)()
}

func (l *Loaders) LoadProductOptions(ctx context.Context, key ProductOptionsKey) ([]catalog.ProductOption, error) {
	if g := l.variantGroupOf(key.ProductID); g != nil && l.catalog != nil {
		g.once("options:" + key.Locale).Do(func() {
			byID, err := l.catalog.GetProductOptionsByProductIDs(ctx, g.members, key.Locale)
			if err != nil {
				return
			}
			for _, id := range g.members {
				opts := byID[id]
				if opts == nil {
					opts = []catalog.ProductOption{}
				}
				l.ProductOptions.Prime(ctx, ProductOptionsKey{ProductID: id, Locale: key.Locale}, opts)
			}
		})
	}
	return l.ProductOptions.Load(ctx, key)()
}
