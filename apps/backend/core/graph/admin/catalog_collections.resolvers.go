package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Content is the resolver for the content field. Every locale's row, primed by the collections list
func (r *collectionResolver) Content(ctx context.Context, obj *model.Collection) ([]*model.CollectionContent, error) {
	// The detail path loaded every row already - lists leave it nil and read the primed cache instead
	if obj.Content != nil {
		return obj.Content, nil
	}
	id, err := parseID(obj.ID, "collection")
	if err != nil {
		return nil, err
	}
	rows, err := r.getCollectionContent(ctx, id)
	if err != nil {
		return nil, err
	}
	return toCollectionContentModels(rows), nil
}

// Lazy on purpose: collection list rows don't want a membership fetch each - a slice filled in toCollectionModel answers [] for every collection whatever catalog_collection_products holds
func (r *collectionResolver) Products(ctx context.Context, obj *model.Collection) ([]*model.Product, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	cid, err := parseID(obj.ID, "collection")
	if err != nil {
		return nil, err
	}
	products, err := r.CatalogSvc.ListProductsByCollection(ctx, cid, r.activeLocale(ctx), 0) // whole list - the editor saves its order from what it loaded
	if err != nil {
		return nil, err
	}
	if len(products) == 0 {
		return []*model.Product{}, nil
	}
	// One batched media fetch for the whole list - the table renders a
	// thumbnail per row and a per-product call would be N+1.
	ids := make([]uuid.UUID, len(products))
	for i, p := range products {
		ids[i] = p.ID
	}
	mediaByProduct, err := r.CatalogSvc.ListProductMediaByProductIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Product, len(products))
	for i, p := range products {
		media := mediaByProduct[p.ID]
		mediaModels := make([]*model.ProductMedia, len(media))
		for j, m := range media {
			mediaModels[j] = toProductMediaModel(m)
		}
		out[i] = toProductBaseModel(
			p,
			nil,
			nil,
			[]*model.Product{},
			mediaModels,
			[]*model.Attribute{},
			nil,
			[]*model.Collection{},
		)
	}
	return out, nil
}

func (r *collectionResolver) FacetOverrides(ctx context.Context, obj *model.Collection) ([]*model.FacetOverride, error) {
	id, err := parseID(obj.ID, "collection")
	if err != nil {
		return nil, err
	}
	rows, err := r.CatalogSvc.ListCollectionFacetOverrides(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]*model.FacetOverride, len(rows))
	for i, o := range rows {
		out[i] = &model.FacetOverride{
			AttributeID:   o.AttributeID.String(),
			AttributeSlug: o.AttributeSlug,
			SortOrder:     o.SortOrder,
			Hidden:        o.Hidden,
		}
	}
	return out, nil
}

func (r *mutationResolver) CreateCollection(ctx context.Context, input model.CreateCollectionInput) (*model.Collection, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.CreateCollection(ctx, catalog.CreateCollectionInput{
		Slug:            input.Slug,
		IsPublished:     boolOrFalse(input.IsPublished),
		SEONoindex:      boolOrFalse(input.SeoNoindex),
		Metadata:        metadataFromInput(input.Metadata),
		PrivateMetadata: metadataFromInput(input.PrivateMetadata),
		Locale:          input.Locale,
		Name:            input.Name,
		Description:     marshalJSON(input.Description),
		MetaTitle:       stringOr(input.MetaTitle, ""),
		MetaDescription: stringOr(input.MetaDescription, ""),
		MetaKeywords:    stringOr(input.MetaKeywords, ""),
	})
	if err != nil {
		return nil, err
	}
	return toCollectionModel(c), nil
}

func (r *mutationResolver) UpdateCollection(ctx context.Context, input model.UpdateCollectionInput) (*model.Collection, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(input.ID, "collection")
	if err != nil {
		return nil, err
	}
	domainInput := catalog.UpdateCollectionInput{
		ID:          id,
		Slug:        input.Slug,
		IsPublished: input.IsPublished,
		SEONoindex:  input.SeoNoindex,
	}
	if input.Metadata != nil {
		m := metadataFromInput(input.Metadata)
		domainInput.Metadata = &m
	}
	if input.PrivateMetadata != nil {
		m := metadataFromInput(input.PrivateMetadata)
		domainInput.PrivateMetadata = &m
	}
	c, err := r.CatalogSvc.UpdateCollection(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toCollectionModel(c), nil
}

func (r *mutationResolver) SetCollectionFacetOverrides(ctx context.Context, collectionID string, overrides []*model.FacetOverrideInput) (*model.Collection, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(collectionID, "collection")
	if err != nil {
		return nil, err
	}
	rows := make([]catalog.FacetOverride, 0, len(overrides))
	// List order IS the storefront order - the service re-numbers SortOrder from it
	for _, in := range overrides {
		if in == nil {
			continue
		}
		attrID, err := parseID(in.AttributeID, "attribute")
		if err != nil {
			return nil, err
		}
		rows = append(rows, catalog.FacetOverride{
			AttributeID: attrID,
			Hidden:      in.Hidden,
		})
	}
	if err := r.CatalogSvc.SetCollectionFacetOverrides(ctx, id, rows); err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.GetCollection(ctx, id, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	return toCollectionModel(c), nil
}

func (r *mutationResolver) DeleteCollection(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return false, err
	}
	cid, err := parseID(id, "collection")
	if err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteCollection(ctx, cid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UpsertCollectionContent(ctx context.Context, collectionID string, locale string, input model.UpsertCollectionContentInput) (*model.Collection, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	cid, err := parseID(collectionID, "collection")
	if err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.UpsertCollectionContent(ctx, cid, catalog.CollectionContentInput{
		Locale:          locale,
		Name:            input.Name,
		Description:     marshalJSON(input.Description),
		MetaTitle:       stringOr(input.MetaTitle, ""),
		MetaDescription: stringOr(input.MetaDescription, ""),
		MetaKeywords:    stringOr(input.MetaKeywords, ""),
	})
	if err != nil {
		return nil, err
	}
	return toCollectionModel(c), nil
}

func (r *mutationResolver) DeleteCollectionContent(ctx context.Context, collectionID string, locale string) (bool, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return false, err
	}
	cid, err := parseID(collectionID, "collection")
	if err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteCollectionContent(ctx, cid, locale); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) AddProductToCollection(ctx context.Context, collectionID string, productID string, sortOrder *int32) (*model.Collection, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	cid, err := parseID(collectionID, "collection")
	if err != nil {
		return nil, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.AddProductToCollection(ctx, cid, pid, int32Or(sortOrder, 0))
	if err != nil {
		return nil, err
	}
	return toCollectionModel(c), nil
}

func (r *mutationResolver) RemoveProductFromCollection(ctx context.Context, collectionID string, productID string) (*model.Collection, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	cid, err := parseID(collectionID, "collection")
	if err != nil {
		return nil, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.RemoveProductFromCollection(ctx, cid, pid)
	if err != nil {
		return nil, err
	}
	return toCollectionModel(c), nil
}

func (r *mutationResolver) ReorderCollectionProducts(ctx context.Context, collectionID string, productIds []string) (*model.Collection, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	cid, err := parseID(collectionID, "collection")
	if err != nil {
		return nil, err
	}
	pids := make([]uuid.UUID, len(productIds))
	for i, s := range productIds {
		pid, err := parseID(s, "product")
		if err != nil {
			return nil, err
		}
		pids[i] = pid
	}
	c, err := r.CatalogSvc.ReorderCollectionProducts(ctx, cid, pids)
	if err != nil {
		return nil, err
	}
	return toCollectionModel(c), nil
}

func (r *mutationResolver) RequestCollectionCoverUpload(ctx context.Context, input model.RequestCollectionCoverUploadInput) (*model.RequestCoverUploadPayload, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	cid, err := parseID(input.CollectionID, "collection")
	if err != nil {
		return nil, err
	}
	result, err := r.CatalogSvc.RequestCollectionCoverUpload(ctx, catalog.RequestCollectionCoverInput{
		CollectionID: cid,
		Filename:     input.Filename,
		MimeType:     input.MimeType,
		SizeBytes:    int64(input.SizeBytes),
		AltText:      stringOr(input.AltText, ""),
	})
	if err != nil {
		return nil, err
	}
	return &model.RequestCoverUploadPayload{
		Collection: toCollectionModel(result.Collection),
		UploadURL:  result.UploadURL,
		ExpiresAt:  result.ExpiresAt,
	}, nil
}

func (r *mutationResolver) ConfirmCollectionCoverUpload(ctx context.Context, collectionID string, width int32, height int32) (*model.Collection, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	cid, err := parseID(collectionID, "collection")
	if err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.ConfirmCollectionCover(ctx, cid, width, height)
	if err != nil {
		return nil, err
	}
	return toCollectionModel(c), nil
}

func (r *mutationResolver) ClearCollectionCover(ctx context.Context, collectionID string) (*model.Collection, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	cid, err := parseID(collectionID, "collection")
	if err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.ClearCollectionCover(ctx, cid)
	if err != nil {
		return nil, err
	}
	return toCollectionModel(c), nil
}

func (r *queryResolver) Collections(ctx context.Context) ([]*model.Collection, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	collections, err := r.CatalogSvc.ListCollections(ctx, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(collections))
	for i, c := range collections {
		ids[i] = c.ID
	}
	if err := r.primeCollectionContent(ctx, ids); err != nil {
		return nil, err
	}
	out := make([]*model.Collection, len(collections))
	for i, c := range collections {
		out[i] = toCollectionModel(c)
	}
	return out, nil
}

func (r *queryResolver) Collection(ctx context.Context, id string) (*model.Collection, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	cid, err := parseID(id, "collection")
	if err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.GetCollection(ctx, cid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	return toCollectionModel(c), nil
}

func (r *Resolver) Collection() CollectionResolver { return &collectionResolver{r} }

type collectionResolver struct{ *Resolver }
