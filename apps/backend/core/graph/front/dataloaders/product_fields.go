package dataloaders

import (
	"context"

	"github.com/google/uuid"
	"github.com/graph-gophers/dataloader/v7"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
)

// A key the read doesn't return gets the zero value - no media or no roles is a normal answer
func byIDBatchFn[V any](load func(context.Context, []uuid.UUID) (map[uuid.UUID]V, error)) dataloader.BatchFunc[uuid.UUID, V] {
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[V] {
		out := make([]*dataloader.Result[V], len(keys))
		byID, err := load(ctx, keys)
		for i, key := range keys {
			if err != nil {
				out[i] = &dataloader.Result[V]{Error: err}
				continue
			}
			out[i] = &dataloader.Result[V]{Data: byID[key]}
		}
		return out
	}
}

func newByIDLoader[V any](load func(context.Context, []uuid.UUID) (map[uuid.UUID]V, error)) *dataloader.Loader[uuid.UUID, V] {
	return dataloader.NewBatchedLoader(
		byIDBatchFn(load),
		dataloader.WithWait[uuid.UUID, V](loaderWait),
		dataloader.WithCache[uuid.UUID, V](dataloader.NewCache[uuid.UUID, V]()),
	)
}

type ProductLocaleKey struct {
	ProductID uuid.UUID
	Locale    string
}

func newLocaleLoader[V any](load func(context.Context, []uuid.UUID, string) (map[uuid.UUID]V, error)) *dataloader.Loader[ProductLocaleKey, V] {
	batch := func(ctx context.Context, keys []ProductLocaleKey) []*dataloader.Result[V] {
		out := make([]*dataloader.Result[V], len(keys))
		idsByLocale := map[string][]uuid.UUID{}
		for _, k := range keys {
			idsByLocale[k.Locale] = append(idsByLocale[k.Locale], k.ProductID)
		}
		byLocale := map[string]map[uuid.UUID]V{}
		errByLocale := map[string]error{}
		for loc, ids := range idsByLocale {
			byLocale[loc], errByLocale[loc] = load(ctx, ids, loc)
		}
		for i, k := range keys {
			if err := errByLocale[k.Locale]; err != nil {
				out[i] = &dataloader.Result[V]{Error: err}
				continue
			}
			out[i] = &dataloader.Result[V]{Data: byLocale[k.Locale][k.ProductID]}
		}
		return out
	}
	return dataloader.NewBatchedLoader(
		batch,
		dataloader.WithWait[ProductLocaleKey, V](loaderWait),
		dataloader.WithCache[ProductLocaleKey, V](dataloader.NewCache[ProductLocaleKey, V]()),
	)
}

// Categories and primary category with everything unpublished on the channel dropped
type CategoryInfo struct {
	Categories []catalog.Category
	Primary    *catalog.Category
}

func categoryInfoBatchFn(svc *catalog.Service) func(context.Context, []uuid.UUID, string) (map[uuid.UUID]CategoryInfo, error) {
	return func(ctx context.Context, ids []uuid.UUID, locale string) (map[uuid.UUID]CategoryInfo, error) {
		out := make(map[uuid.UUID]CategoryInfo, len(ids))
		ch, ok := channel.FromContext(ctx)
		if !ok || ch == nil {
			return out, nil
		}
		cats, err := svc.ListCategoriesByProductIDs(ctx, ids, locale)
		if err != nil {
			return nil, err
		}
		primary, err := svc.ListPrimaryCategoryByProductIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		seen := map[uuid.UUID]struct{}{}
		var catIDs []uuid.UUID
		add := func(id uuid.UUID) {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				catIDs = append(catIDs, id)
			}
		}
		for _, cs := range cats {
			for _, c := range cs {
				add(c.ID)
			}
		}
		for _, p := range primary {
			if p != nil {
				add(p.ID)
			}
		}
		visible := map[uuid.UUID]struct{}{}
		if len(catIDs) > 0 {
			if visible, err = svc.FilterVisibleCategoryIDsForChannel(ctx, catIDs, ch.ID); err != nil {
				return nil, err
			}
		}
		for _, id := range ids {
			info := CategoryInfo{Categories: make([]catalog.Category, 0, len(cats[id]))}
			for _, c := range cats[id] {
				if _, ok := visible[c.ID]; ok {
					info.Categories = append(info.Categories, c)
				}
			}
			if p := primary[id]; p != nil {
				if _, ok := visible[p.ID]; ok {
					info.Primary = p
				}
			}
			out[id] = info
		}
		return out, nil
	}
}

func LoadCategoryInfo(ctx context.Context, svc *catalog.Service, key ProductLocaleKey) (CategoryInfo, error) {
	byID, err := categoryInfoBatchFn(svc)(ctx, []uuid.UUID{key.ProductID}, key.Locale)
	if err != nil {
		return CategoryInfo{}, err
	}
	return byID[key.ProductID], nil
}

type productFieldLoaders struct {
	ProductMedia      *dataloader.Loader[uuid.UUID, []catalog.ProductMedia]
	ProductMediaRoles *dataloader.Loader[uuid.UUID, map[catalog.MediaRole]uuid.UUID]
	ProductTypeID     *dataloader.Loader[uuid.UUID, uuid.UUID]
	ChildRows         *dataloader.Loader[uuid.UUID, []catalog.Product] // visibility is applied by the caller

	ProductCategories  *dataloader.Loader[ProductLocaleKey, CategoryInfo]
	ProductCollections *dataloader.Loader[ProductLocaleKey, []catalog.Collection]

	ProductRow          *dataloader.Loader[uuid.UUID, catalog.Product]
	ProductAllContent   *dataloader.Loader[uuid.UUID, []catalog.ProductContent]
	ProductSelections   *dataloader.Loader[uuid.UUID, []catalog.ProductAttributeSelection]
	ProductReferences   *dataloader.Loader[uuid.UUID, []catalog.ProductAttributeReference]
	ConfigurableOptions *dataloader.Loader[uuid.UUID, []catalog.Attribute]

	svc *catalog.Service
}

func newProductFieldLoaders(svc *catalog.Service) productFieldLoaders {
	if svc == nil {
		return productFieldLoaders{}
	}
	return productFieldLoaders{
		ProductMedia:      newByIDLoader(svc.ListProductMediaByProductIDs),
		ProductMediaRoles: newByIDLoader(svc.ListProductMediaRolesByProductIDs),
		ProductTypeID:     newByIDLoader(svc.ListProductTypeIDsByProductIDs),
		ChildRows:         newByIDLoader(svc.ListProductChildrenByParentIDs),

		ProductCategories:  newLocaleLoader(categoryInfoBatchFn(svc)),
		ProductCollections: newLocaleLoader(svc.ListCollectionsByProductIDs),

		ProductRow:          newByIDLoader(svc.ListProductRowsByIDs),
		ProductAllContent:   newByIDLoader(svc.ListProductContentByProductIDs),
		ProductSelections:   newByIDLoader(svc.LoadProductsAttributeSelections),
		ProductReferences:   newByIDLoader(svc.LoadProductsAttributeReferences),
		ConfigurableOptions: newByIDLoader(svc.ListConfigurableOptionsByProductIDs),

		svc: svc,
	}
}
