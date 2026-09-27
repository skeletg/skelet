package front

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/dataloaders"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

func (r *Resolver) productTypeOf(ctx context.Context, obj *model.Product) (*model.ProductType, error) {
	typeID, err := r.productTypeIDOf(ctx, obj)
	if err != nil {
		return nil, err
	}
	pt, err := r.getProductType(ctx, typeID)
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

// Part stubs carry no type id, so resolve it by product id
func (r *Resolver) productTypeIDOf(ctx context.Context, obj *model.Product) (uuid.UUID, error) {
	if obj == nil {
		return uuid.Nil, fmt.Errorf("product context missing")
	}
	if obj.ProductType != nil {
		if id, err := uuid.Parse(obj.ProductType.ID); err == nil && id != uuid.Nil {
			return id, nil
		}
	}
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return uuid.Nil, err
	}
	typeID, err := r.loadProductTypeID(ctx, pid)
	if err != nil {
		return uuid.Nil, err
	}
	if typeID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("product %s has no product type", pid)
	}
	return typeID, nil
}

func (r *Resolver) loadProductTypeID(ctx context.Context, pid uuid.UUID) (uuid.UUID, error) {
	if l := dataloaders.For(ctx); l != nil && l.ProductTypeID != nil {
		return l.ProductTypeID.Load(ctx, pid)()
	}
	m, err := r.CatalogSvc.ListProductTypeIDsByProductIDs(ctx, []uuid.UUID{pid})
	if err != nil {
		return uuid.Nil, err
	}
	return m[pid], nil
}

func (r *Resolver) productMediaModels(ctx context.Context, obj *model.Product) ([]*model.ProductMedia, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	var items []catalog.ProductMedia
	if l := dataloaders.For(ctx); l != nil && l.ProductMedia != nil {
		items, err = l.LoadProductMedia(ctx, pid)
	} else {
		items, err = r.CatalogSvc.ListProductMedia(ctx, pid)
	}
	if err != nil {
		return nil, err
	}
	out := make([]*model.ProductMedia, len(items))
	for i, m := range items {
		out[i] = toProductMediaModel(m)
	}
	return out, nil
}

func (r *Resolver) productMediaRole(ctx context.Context, obj *model.Product, role catalog.MediaRole) (*model.ProductMedia, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	var roles map[catalog.MediaRole]uuid.UUID
	if l := dataloaders.For(ctx); l != nil && l.ProductMediaRoles != nil {
		roles, err = l.LoadProductMediaRoles(ctx, pid)
	} else {
		var byID map[uuid.UUID]map[catalog.MediaRole]uuid.UUID
		byID, err = r.CatalogSvc.ListProductMediaRolesByProductIDs(ctx, []uuid.UUID{pid})
		roles = byID[pid]
	}
	if err != nil {
		return nil, err
	}
	media, err := r.productMediaModels(ctx, obj)
	if err != nil {
		return nil, err
	}
	return pickMediaRole(roles, role, media), nil
}

// Only main falls back to the first image - a hero is the wrong size for a thumbnail or og_image
func pickMediaRole(roles map[catalog.MediaRole]uuid.UUID, role catalog.MediaRole, media []*model.ProductMedia) *model.ProductMedia {
	img := resolveMediaRole(roles, role, media)
	if img == nil && role == catalog.MediaRoleMain && len(media) > 0 {
		img = media[0]
	}
	return img
}

func (r *Resolver) productCategoryInfo(ctx context.Context, obj *model.Product) (dataloaders.CategoryInfo, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return dataloaders.CategoryInfo{}, err
	}
	key := dataloaders.ProductLocaleKey{ProductID: pid, Locale: r.activeLocale(ctx)}
	if l := dataloaders.For(ctx); l != nil && l.ProductCategories != nil {
		return l.ProductCategories.Load(ctx, key)()
	}
	return dataloaders.LoadCategoryInfo(ctx, r.CatalogSvc, key)
}

func (r *Resolver) productCategories(ctx context.Context, obj *model.Product) ([]*model.Category, error) {
	info, err := r.productCategoryInfo(ctx, obj)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Category, len(info.Categories))
	for i, c := range info.Categories {
		out[i] = toCategoryModel(c)
	}
	return out, nil
}

func (r *Resolver) productPrimaryCategory(ctx context.Context, obj *model.Product) (*model.Category, error) {
	info, err := r.productCategoryInfo(ctx, obj)
	if err != nil || info.Primary == nil {
		return nil, err
	}
	primary := *info.Primary
	if storeID := channelIDFromContext(ctx); storeID != nil {
		locale := r.activeLocale(ctx)
		if hydrated, err := r.CatalogSvc.GetPublishedCategoryForChannel(ctx, *storeID, primary.ID, locale); err == nil {
			ancestors, err := r.CatalogSvc.GetCategoryAncestors(ctx, primary.ID, locale)
			if err != nil {
				return nil, err
			}
			hydrated.Ancestors = ancestors
			primary = hydrated
		}
	}
	return toCategoryModel(primary), nil
}

func (r *Resolver) productCollections(ctx context.Context, obj *model.Product) ([]*model.Collection, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	locale := r.activeLocale(ctx)
	var cols []catalog.Collection
	if l := dataloaders.For(ctx); l != nil && l.ProductCollections != nil {
		cols, err = l.ProductCollections.Load(ctx, dataloaders.ProductLocaleKey{ProductID: pid, Locale: locale})()
	} else {
		var byID map[uuid.UUID][]catalog.Collection
		byID, err = r.CatalogSvc.ListCollectionsByProductIDs(ctx, []uuid.UUID{pid}, locale)
		cols = byID[pid]
	}
	if err != nil {
		return nil, err
	}
	return toCollectionModels(publishedCollections(cols)), nil
}
