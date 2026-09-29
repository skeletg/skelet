package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Content is the resolver for the content field. Every locale's row, primed by the list resolvers
func (r *categoryResolver) Content(ctx context.Context, obj *model.Category) ([]*model.CategoryContent, error) {
	// The detail path loaded every row already - lists leave it nil and read the primed cache instead
	if obj.Content != nil {
		return obj.Content, nil
	}
	id, err := parseID(obj.ID, "category")
	if err != nil {
		return nil, err
	}
	rows, err := r.getCategoryContent(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CategoryContent, len(rows))
	for i, row := range rows {
		out[i] = toCategoryContentModel(row)
	}
	return out, nil
}

func (r *categoryResolver) Products(ctx context.Context, obj *model.Category) ([]*model.Product, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	cid, err := parseID(obj.ID, "category")
	if err != nil {
		return nil, err
	}
	products, err := r.CatalogSvc.ListProductsByCategory(ctx, cid, r.activeLocale(ctx), 0) // whole list - the assign picker hides members from it
	if err != nil {
		return nil, err
	}
	out := make([]*model.Product, len(products))
	for i, p := range products {
		out[i] = toProductBaseModel(p, nil, nil, nil, nil, nil, nil, nil)
	}
	return out, nil
}

// ProductCount is the resolver for the productCount field. A cheap indexed COUNT of direct memberships (descendants
// excluded) - safe to select across a category tree (e.g. picker hints), unlike the full `products` list
func (r *categoryResolver) ProductCount(ctx context.Context, obj *model.Category) (int32, error) {
	id, err := parseID(obj.ID, "category")
	if err != nil {
		return 0, err
	}
	n, err := r.CatalogSvc.CountProductsInCategory(ctx, id)
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

// Filters the rows the mapper already loaded down to the channels this admin may see - a nested per-channel row leaks the same way a list query would, and this one had not filtered since the scope rollout
// In place, so the cost is one slice copy
func (r *categoryResolver) Listings(ctx context.Context, obj *model.Category) ([]*model.CategoryChannelListing, error) {
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "catalog.view")
	if err != nil {
		return nil, err
	}
	if perm.Bypass() {
		return obj.Listings, nil
	}
	out := make([]*model.CategoryChannelListing, 0, len(obj.Listings))
	for _, l := range obj.Listings {
		cid, err := uuid.Parse(l.ChannelID)
		if err != nil {
			continue // can't prove it visible, so drop it
		}
		if perm.Allows(cid) {
			out = append(out, l)
		}
	}
	return out, nil
}

func (r *categoryResolver) FacetOverrides(ctx context.Context, obj *model.Category) ([]*model.FacetOverride, error) {
	id, err := parseID(obj.ID, "category")
	if err != nil {
		return nil, err
	}
	rows, err := r.CatalogSvc.ListCategoryFacetOverrides(ctx, id)
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

func (r *categoryChannelListingResolver) Channel(ctx context.Context, obj *model.CategoryChannelListing) (*model.Channel, error) {
	chID, err := parseID(obj.ChannelID, "channel")
	if err != nil {
		return nil, err
	}
	ch, err := r.ChannelSvc.GetByID(ctx, chID)
	if err != nil {
		return nil, err
	}
	return toChannelModel(ch), nil
}

func (r *mutationResolver) CreateCategory(ctx context.Context, input model.CreateCategoryInput) (*model.Category, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return nil, err
	}
	domainInput := catalog.CreateCategoryInput{
		Slug:             input.Slug,
		Position:         int32Or(input.Position, 0),
		IsPublished:      boolOrFalse(input.IsPublished),
		SEONoindex:       boolOrFalse(input.SeoNoindex),
		Metadata:         metadataFromInput(input.Metadata),
		PrivateMetadata:  metadataFromInput(input.PrivateMetadata),
		Locale:           input.Locale,
		Name:             input.Name,
		ShortDescription: marshalJSON(input.ShortDescription),
		Description:      marshalJSON(input.Description),
		MetaTitle:        stringOr(input.MetaTitle, ""),
		MetaDescription:  stringOr(input.MetaDescription, ""),
		MetaKeywords:     stringOr(input.MetaKeywords, ""),
	}
	if input.ParentID != nil {
		id, err := parseID(*input.ParentID, "parent category")
		if err != nil {
			return nil, err
		}
		domainInput.ParentID = &id
	}
	c, err := r.CatalogSvc.CreateCategory(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toCategoryModel(c), nil
}

func (r *mutationResolver) UpdateCategory(ctx context.Context, input model.UpdateCategoryInput) (*model.Category, error) {
	id, err := parseID(input.ID, "category")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnCategoryChannels(ctx, "catalog.manage", id); err != nil {
		return nil, err
	}
	domainInput := catalog.UpdateCategoryInput{
		ID:          id,
		Slug:        input.Slug,
		Position:    input.Position,
		IsPublished: input.IsPublished,
		SEONoindex:  input.SeoNoindex,
		ClearParent: boolOrFalse(input.ClearParent),
	}
	if input.ParentID != nil {
		pid, err := parseID(*input.ParentID, "parent category")
		if err != nil {
			return nil, err
		}
		domainInput.ParentID = &pid
	}
	if input.Metadata != nil {
		m := metadataFromInput(input.Metadata)
		domainInput.Metadata = &m
	}
	if input.PrivateMetadata != nil {
		m := metadataFromInput(input.PrivateMetadata)
		domainInput.PrivateMetadata = &m
	}
	c, err := r.CatalogSvc.UpdateCategory(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toCategoryModel(c), nil
}

func (r *mutationResolver) SetCategoryFacetOverrides(ctx context.Context, categoryID string, overrides []*model.FacetOverrideInput) (*model.Category, error) {
	id, err := parseID(categoryID, "category")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnCategoryChannels(ctx, "catalog.manage", id); err != nil {
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
	if err := r.CatalogSvc.SetCategoryFacetOverrides(ctx, id, rows); err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.GetCategory(ctx, id, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	return toCategoryModel(c), nil
}

func (r *mutationResolver) DeleteCategory(ctx context.Context, id string) (bool, error) {
	cid, err := parseID(id, "category")
	if err != nil {
		return false, err
	}
	if err := r.requireOnCategoryChannels(ctx, "catalog.manage", cid); err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteCategory(ctx, cid); err != nil {
		return false, err
	}
	return true, nil
}

// Same all-listed-channels gate as products - the category's content
// row is shared across every channel listing the category.
func (r *mutationResolver) UpsertCategoryContent(ctx context.Context, categoryID string, input model.UpsertCategoryContentInput) (*model.Category, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return nil, err
	}
	cid, err := parseID(categoryID, "category")
	if err != nil {
		return nil, err
	}
	listings, err := r.CatalogSvc.ListCategoryChannelListings(ctx, cid)
	if err != nil {
		return nil, err
	}
	channelIDs := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		channelIDs[i] = l.ChannelID
	}
	if err := auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, "catalog.manage", channelIDs, input.Locale); err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.UpsertCategoryContent(ctx, cid, catalog.CategoryContentInput{
		Locale:           input.Locale,
		Name:             input.Name,
		ShortDescription: marshalJSON(input.ShortDescription),
		Description:      marshalJSON(input.Description),
		MetaTitle:        stringOr(input.MetaTitle, ""),
		MetaDescription:  stringOr(input.MetaDescription, ""),
		MetaKeywords:     stringOr(input.MetaKeywords, ""),
	})
	if err != nil {
		return nil, err
	}
	return toCategoryModel(c), nil
}

// Same all-listed-channels gate as UpsertCategoryContent.
func (r *mutationResolver) DeleteCategoryContent(ctx context.Context, categoryID string, locale string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return false, err
	}
	cid, err := parseID(categoryID, "category")
	if err != nil {
		return false, err
	}
	listings, err := r.CatalogSvc.ListCategoryChannelListings(ctx, cid)
	if err != nil {
		return false, err
	}
	channelIDs := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		channelIDs[i] = l.ChannelID
	}
	if err := auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, "catalog.manage", channelIDs, locale); err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteCategoryContent(ctx, cid, locale); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) AddProductToCategory(ctx context.Context, productID string, categoryID string, position *int32, isPrimary *bool) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	cid, err := parseID(categoryID, "category")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnCategoryAndProductChannels(ctx, cid, pid); err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.AddProductToCategory(ctx, pid, cid, int32Or(position, 0), boolOrFalse(isPrimary))
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

func (r *mutationResolver) RemoveProductFromCategory(ctx context.Context, productID string, categoryID string) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	cid, err := parseID(categoryID, "category")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnCategoryAndProductChannels(ctx, cid, pid); err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.RemoveProductFromCategory(ctx, pid, cid)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

func (r *mutationResolver) SetProductPrimaryCategory(ctx context.Context, productID string, categoryID string) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	cid, err := parseID(categoryID, "category")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnCategoryAndProductChannels(ctx, cid, pid); err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.SetProductPrimaryCategory(ctx, pid, cid)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

func (r *mutationResolver) RequestCategoryCoverUpload(ctx context.Context, input model.RequestCategoryCoverUploadInput) (*model.RequestCoverUploadPayload, error) {
	cid, err := parseID(input.CategoryID, "category")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnCategoryChannels(ctx, "catalog.manage", cid); err != nil {
		return nil, err
	}
	result, err := r.CatalogSvc.RequestCategoryCoverUpload(ctx, catalog.RequestCategoryCoverInput{
		CategoryID: cid,
		Filename:   input.Filename,
		MimeType:   input.MimeType,
		SizeBytes:  int64(input.SizeBytes),
		AltText:    stringOr(input.AltText, ""),
	})
	if err != nil {
		return nil, err
	}
	return &model.RequestCoverUploadPayload{
		UploadURL: result.UploadURL,
		ExpiresAt: result.ExpiresAt,
	}, nil
}

func (r *mutationResolver) ConfirmCategoryCoverUpload(ctx context.Context, categoryID string, width int32, height int32) (*model.Category, error) {
	cid, err := parseID(categoryID, "category")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnCategoryChannels(ctx, "catalog.manage", cid); err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.ConfirmCategoryCover(ctx, cid, width, height)
	if err != nil {
		return nil, err
	}
	return toCategoryModel(c), nil
}

func (r *mutationResolver) ClearCategoryCover(ctx context.Context, categoryID string) (*model.Category, error) {
	cid, err := parseID(categoryID, "category")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnCategoryChannels(ctx, "catalog.manage", cid); err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.ClearCategoryCover(ctx, cid)
	if err != nil {
		return nil, err
	}
	return toCategoryModel(c), nil
}

// SetCategoryListing creates or updates the (category, store) listing row. Storefronts only render categories listed
// AND published on the active store, so this is the gate. Requires catalog.manage
func (r *mutationResolver) SetCategoryListing(ctx context.Context, input model.SetCategoryListingInput) (*model.Category, error) {
	categoryID, err := parseID(input.CategoryID, "category")
	if err != nil {
		return nil, err
	}
	channelID, err := parseID(input.ChannelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := r.requireCategoryListingChange(ctx, categoryID, channelID, false); err != nil {
		return nil, err
	}
	domainInput := catalog.SetCategoryChannelListingInput{
		CategoryID:  categoryID,
		ChannelID:   channelID,
		IsPublished: input.IsPublished,
	}
	if input.SortOrderOverride != nil {
		v := int32(*input.SortOrderOverride)
		domainInput.SortOrderOverride = &v
	}
	c, err := r.CatalogSvc.SetCategoryChannelListing(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toCategoryModel(c), nil
}

// ClearCategoryListing removes the (category, store) listing row. With
// no row, the category is hidden on that store. Requires catalog.manage.
func (r *mutationResolver) ClearCategoryListing(ctx context.Context, categoryID string, channelID string) (*model.Category, error) {
	cid, err := parseID(categoryID, "category")
	if err != nil {
		return nil, err
	}
	svid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := r.requireCategoryListingChange(ctx, cid, svid, true); err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.ClearCategoryChannelListing(ctx, cid, svid)
	if err != nil {
		return nil, err
	}
	return toCategoryModel(c), nil
}

// RootCategories is the resolver for the rootCategories field. When channelId is set the result is restricted to categories listed AND
// published on that channel - header-driven admin uses this in CHANNEL mode so the tree matches what the storefront serves
func (r *queryResolver) RootCategories(ctx context.Context, channelID *string) ([]*model.Category, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	chID, err := r.scopedChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if chID != nil {
		cats, err := r.CatalogSvc.ListRootCategoriesListedOnChannel(ctx, *chID, r.activeLocale(ctx))
		if err != nil {
			return nil, err
		}
		return r.renderCategoryList(ctx, cats)
	}
	cats, err := r.CatalogSvc.ListRootCategories(ctx, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	return r.renderCategoryList(ctx, cats)
}

// Filters by channelId when present.
func (r *queryResolver) CategoryChildren(ctx context.Context, parentID string, channelID *string) ([]*model.Category, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	pid, err := parseID(parentID, "category")
	if err != nil {
		return nil, err
	}
	chID, err := r.scopedChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if chID != nil {
		cats, err := r.CatalogSvc.ListCategoryChildrenListedOnChannel(ctx, *chID, pid, r.activeLocale(ctx))
		if err != nil {
			return nil, err
		}
		return r.renderCategoryList(ctx, cats)
	}
	cats, err := r.CatalogSvc.ListCategoryChildren(ctx, pid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	return r.renderCategoryList(ctx, cats)
}

func (r *queryResolver) Category(ctx context.Context, id string) (*model.Category, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	cid, err := parseID(id, "category")
	if err != nil {
		return nil, err
	}
	c, err := r.CatalogSvc.GetCategory(ctx, cid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	ancestors, err := r.CatalogSvc.GetCategoryAncestors(ctx, cid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	children, err := r.CatalogSvc.ListCategoryChildren(ctx, cid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	c.Ancestors = ancestors
	c.Children = children
	return toCategoryModel(c), nil
}

func (r *Resolver) Category() CategoryResolver { return &categoryResolver{r} }

func (r *Resolver) CategoryChannelListing() CategoryChannelListingResolver {
	return &categoryChannelListingResolver{r}
}

type (
	categoryResolver               struct{ *Resolver }
	categoryChannelListingResolver struct{ *Resolver }
)
