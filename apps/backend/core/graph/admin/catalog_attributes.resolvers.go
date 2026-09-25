package admin

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Lazy on purpose - list views that don't ask for descriptions never touch catalog_attribute_content
func (r *attributeResolver) Descriptions(ctx context.Context, obj *model.Attribute) ([]*model.AttributeContent, error) {
	aid, err := parseID(obj.ID, "attribute")
	if err != nil {
		return nil, err
	}
	rows, err := r.CatalogSvc.ListAttributeContent(ctx, aid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.AttributeContent, len(rows))
	for i, row := range rows {
		out[i] = toAttributeContentModel(row)
	}
	return out, nil
}

func (r *attributeValueResolver) Descriptions(ctx context.Context, obj *model.AttributeValue) ([]*model.AttributeValueContent, error) {
	vid, err := parseID(obj.ID, "attribute value")
	if err != nil {
		return nil, err
	}
	rows, err := r.CatalogSvc.ListAttributeValueContent(ctx, vid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.AttributeValueContent, len(rows))
	for i, row := range rows {
		out[i] = toAttributeValueContentModel(row)
	}
	return out, nil
}

func (r *mutationResolver) CreateAttribute(ctx context.Context, input model.CreateAttributeInput) (*model.Attribute, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	values := make([]catalog.AttributeValueInput, len(input.Values))
	for i, v := range input.Values {
		values[i] = attributeValueInputFromGQL(v)
	}
	defaultRaw, err := jsonRawFromAny(input.DefaultValue)
	if err != nil {
		return nil, errInvalidInput("defaultValue", err)
	}
	attr, err := r.CatalogSvc.CreateAttribute(ctx, catalog.CreateAttributeInput{
		Slug:               input.Slug,
		Name:               input.Name,
		InputType:          attributeInputTypeFromGQL(input.InputType),
		RelationshipTarget: referenceTargetFromGQL(input.RelationshipTarget),
		Unit:               stringOr(input.Unit, ""),
		ValueRequired:      boolOrFalse(input.ValueRequired),
		Filterable:         boolOrFalse(input.Filterable),
		Searchable:         boolOrFalse(input.Searchable),
		IsPublic:           boolOrDefault(input.IsPublic, true),
		FacetCollapsed:     boolOrFalse(input.FacetCollapsed),
		IsIndexable:        boolOrFalse(input.IsIndexable),
		H1Position:         h1PositionFromGQLPtr(input.H1Position),
		SynapseTrack:       boolOrFalse(input.SynapseTrack),
		AutoSegment:        boolOrFalse(input.AutoSegment),
		SortOrder:          int32Or(input.SortOrder, 0),
		Values:             values,
		Metadata:           metadataFromInput(input.Metadata),
		PrivateMetadata:    metadataFromInput(input.PrivateMetadata),
		DefaultValue:       defaultRaw,
	})
	if err != nil {
		return nil, err
	}
	return toAttributeModel(attr), nil
}

func (r *mutationResolver) UpdateAttribute(ctx context.Context, input model.UpdateAttributeInput) (*model.Attribute, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(input.ID, "attribute")
	if err != nil {
		return nil, err
	}
	domainInput := catalog.UpdateAttributeInput{
		ID:             id,
		Name:           input.Name,
		Unit:           input.Unit,
		ValueRequired:  input.ValueRequired,
		Filterable:     input.Filterable,
		Searchable:     input.Searchable,
		IsPublic:       input.IsPublic,
		FacetCollapsed: input.FacetCollapsed,
		IsIndexable:    input.IsIndexable,
		SynapseTrack:   input.SynapseTrack,
		AutoSegment:    input.AutoSegment,
		SortOrder:      input.SortOrder,
	}
	if input.H1Position != nil {
		p := h1PositionFromGQL(*input.H1Position)
		domainInput.H1Position = &p
	}
	if input.Metadata != nil {
		m := metadataFromInput(input.Metadata)
		domainInput.Metadata = &m
	}
	if input.PrivateMetadata != nil {
		m := metadataFromInput(input.PrivateMetadata)
		domainInput.PrivateMetadata = &m
	}
	if input.DefaultValue != nil {
		// The wrapper input is what separates omit (nil, no change) from clear (empty RawMessage) from set - a bare nullable value can't say all three
		var raw json.RawMessage
		if input.DefaultValue.Clear == nil || !*input.DefaultValue.Clear {
			raw, err = jsonRawFromAny(input.DefaultValue.Value)
			if err != nil {
				return nil, errInvalidInput("defaultValue", err)
			}
		}
		domainInput.DefaultValue = &raw
	}
	attr, err := r.CatalogSvc.UpdateAttribute(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	loaded, err := r.CatalogSvc.GetAttribute(ctx, attr.ID) // the update returns the attribute without its values
	if err != nil {
		return nil, err
	}
	return toAttributeModel(loaded), nil
}

func (r *mutationResolver) DeleteAttribute(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return false, err
	}
	attrID, err := parseID(id, "attribute")
	if err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteAttribute(ctx, attrID); err != nil {
		return false, err
	}
	return true, nil
}

// Cascades to every per-value flag in the same transaction
func (r *mutationResolver) SetAttributeIndexable(ctx context.Context, attributeID string, isIndexable bool) (*model.CascadeAttributeIndexableResult, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	aid, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	res, err := r.CatalogSvc.CascadeAttributeIndexable(ctx, aid, isIndexable)
	if err != nil {
		return nil, err
	}
	return &model.CascadeAttributeIndexableResult{
		Attribute:             toAttributeModel(res.Attribute),
		AffectedValuesCount:   int32(res.AffectedValuesCount),
		AffectedProductsCount: int32(res.AffectedProductsCount),
	}, nil
}

func (r *mutationResolver) SetAttributeValuesIndexable(ctx context.Context, valueIds []string, isIndexable bool) (*model.BulkAttributeValueResult, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(valueIds))
	for _, raw := range valueIds {
		id, err := parseID(raw, "attribute value")
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	n, err := r.CatalogSvc.SetAttributeValuesIndexable(ctx, ids, isIndexable)
	if err != nil {
		return nil, err
	}
	return &model.BulkAttributeValueResult{Count: int32(n)}, nil
}

func (r *mutationResolver) AddAttributeValue(ctx context.Context, attributeID string, input model.AttributeValueInput) (*model.AttributeValue, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	v, err := r.CatalogSvc.AddAttributeValue(ctx, id, attributeValueInputFromGQL(&input))
	if err != nil {
		return nil, err
	}
	return toAttributeValueModel(v), nil
}

func (r *mutationResolver) UpdateAttributeValue(ctx context.Context, id string, input model.AttributeValueInput) (*model.AttributeValue, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	valueID, err := parseID(id, "attribute value")
	if err != nil {
		return nil, err
	}
	v, err := r.CatalogSvc.UpdateAttributeValue(ctx, valueID, attributeValueInputFromGQL(&input))
	if err != nil {
		return nil, err
	}
	return toAttributeValueModel(v), nil
}

func (r *mutationResolver) DeleteAttributeValue(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return false, err
	}
	valueID, err := parseID(id, "attribute value")
	if err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteAttributeValue(ctx, valueID); err != nil {
		return false, err
	}
	return true, nil
}

// An empty description deletes the (attributeId, locale) row rather than storing a blank - the content table stays sparse
func (r *mutationResolver) SetAttributeDescription(ctx context.Context, attributeID string, locale string, description string, name *string) (*model.AttributeContent, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	aid, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	out, err := r.CatalogSvc.SetAttributeContent(ctx, catalog.UpsertAttributeContentInput{
		AttributeID: aid,
		Locale:      locale,
		Name:        stringOr(name, ""),
		Description: description,
	})
	if err != nil {
		return nil, err
	}
	return toAttributeContentModel(out), nil
}

func (r *mutationResolver) SetAttributeValueDescription(ctx context.Context, valueID string, locale string, description string, name *string) (*model.AttributeValueContent, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	vid, err := parseID(valueID, "attribute value")
	if err != nil {
		return nil, err
	}
	out, err := r.CatalogSvc.SetAttributeValueContent(ctx, catalog.UpsertAttributeValueContentInput{
		ValueID:     vid,
		Locale:      locale,
		Name:        stringOr(name, ""),
		Description: description,
	})
	if err != nil {
		return nil, err
	}
	return toAttributeValueContentModel(out), nil
}

func (r *mutationResolver) SetProductAttributeSlug(ctx context.Context, input model.SetProductAttributeSlugInput) (*model.ProductAttributeSlug, error) {
	productID, err := parseID(input.ProductID, "product")
	if err != nil {
		return nil, err
	}
	attrID, err := parseID(input.AttributeID, "attribute")
	if err != nil {
		return nil, err
	}
	in := catalog.UpsertProductAttributeSlugInput{
		ProductID:   productID,
		AttributeID: attrID,
		Slug:        input.Slug,
	}
	if input.ChannelID != nil {
		cid, err := parseID(*input.ChannelID, "channel")
		if err != nil {
			return nil, err
		}
		in.ChannelID = &cid
	}
	if err := r.requireProductSlugScope(ctx, productID, in.ChannelID); err != nil {
		return nil, err
	}
	out, err := r.CatalogSvc.SetProductAttributeSlug(ctx, in)
	if err != nil {
		return nil, err
	}
	return toProductAttributeSlugModel(out), nil
}

func (r *mutationResolver) DeleteProductAttributeSlug(ctx context.Context, productID string, attributeID string, channelID *string) (bool, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return false, err
	}
	aid, err := parseID(attributeID, "attribute")
	if err != nil {
		return false, err
	}
	var cid *uuid.UUID
	if channelID != nil {
		v, err := parseID(*channelID, "channel")
		if err != nil {
			return false, err
		}
		cid = &v
	}
	if err := r.requireProductSlugScope(ctx, pid, cid); err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteProductAttributeSlug(ctx, pid, aid, cid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetAttributeValueChannelSlug(ctx context.Context, valueID string, channelID string, slug string) (*model.AttributeValueChannelSlug, error) {
	vid, err := parseID(valueID, "attribute value")
	if err != nil {
		return nil, err
	}
	cid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "catalog.manage", cid, nil); err != nil {
		return nil, err
	}
	out, err := r.CatalogSvc.SetAttributeValueChannelSlug(ctx, vid, cid, slug)
	if err != nil {
		return nil, err
	}
	return toAttributeValueChannelSlugModel(out), nil
}

func (r *mutationResolver) DeleteAttributeValueChannelSlug(ctx context.Context, valueID string, channelID string) (bool, error) {
	vid, err := parseID(valueID, "attribute value")
	if err != nil {
		return false, err
	}
	cid, err := parseID(channelID, "channel")
	if err != nil {
		return false, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "catalog.manage", cid, nil); err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteAttributeValueChannelSlug(ctx, vid, cid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetProductAttributeLocaleValue(ctx context.Context, productID string, attributeID string, locale string, value string) (*model.ProductAttributeLocaleValue, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	aid, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnProductChannelsInLocale(ctx, pid, locale); err != nil {
		return nil, err
	}
	out, err := r.CatalogSvc.SetProductAttributeLocaleValue(ctx, pid, aid, locale, value)
	if err != nil {
		return nil, err
	}
	return toProductAttributeLocaleValueModel(out), nil
}

func (r *queryResolver) Attributes(ctx context.Context) ([]*model.Attribute, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	attrs, err := r.CatalogSvc.ListAttributes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Attribute, len(attrs))
	for i, a := range attrs {
		out[i] = toAttributeModel(a)
	}
	return out, nil
}

func (r *queryResolver) Attribute(ctx context.Context, id string) (*model.Attribute, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	attrID, err := parseID(id, "attribute")
	if err != nil {
		return nil, err
	}
	attr, err := r.CatalogSvc.GetAttribute(ctx, attrID)
	if err != nil {
		return nil, err
	}
	return toAttributeModel(attr), nil
}

func (r *queryResolver) ReferenceCandidates(ctx context.Context, target model.AttributeReferenceTarget, search *string, limit *int32) ([]*model.Linkable, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	s := ""
	if search != nil {
		s = *search
	}
	lim := int32(20)
	if limit != nil && *limit > 0 {
		lim = *limit
	}
	entityType := string(referenceTargetFromGQL(&target)) // filtered server-side, or a product-target picker gets its hits crowded out of the window by categories and collections
	rows, err := r.UrlRewriteSvc.SearchCanonical(ctx, s, entityType, lim)
	if err != nil {
		return nil, err
	}
	return r.buildLinkables(ctx, rows), nil
}

// Empty when nothing is customised - the indexer then falls back to slugify + is_indexable=false at index time
func (r *queryResolver) ProductAttributeSlugs(ctx context.Context, productID string) ([]*model.ProductAttributeSlug, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	rows, err := r.CatalogSvc.ListProductAttributeSlugs(ctx, pid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ProductAttributeSlug, len(rows))
	for i, row := range rows {
		out[i] = toProductAttributeSlugModel(row)
	}
	return out, nil
}

func (r *queryResolver) ProductAttributeLocaleValues(ctx context.Context, productID string) ([]*model.ProductAttributeLocaleValue, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	rows, err := r.CatalogSvc.ListProductAttributeLocaleValues(ctx, pid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ProductAttributeLocaleValue, len(rows))
	for i, row := range rows {
		out[i] = toProductAttributeLocaleValueModel(row)
	}
	return out, nil
}

func (r *queryResolver) AttributeValueChannelSlugs(ctx context.Context, valueID string) ([]*model.AttributeValueChannelSlug, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	vid, err := parseID(valueID, "attribute value")
	if err != nil {
		return nil, err
	}
	rows, err := r.CatalogSvc.ListAttributeValueChannelSlugs(ctx, vid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.AttributeValueChannelSlug, len(rows))
	for i, row := range rows {
		out[i] = toAttributeValueChannelSlugModel(row)
	}
	return out, nil
}

func (r *Resolver) Attribute() AttributeResolver { return &attributeResolver{r} }

func (r *Resolver) AttributeValue() AttributeValueResolver { return &attributeValueResolver{r} }

type (
	attributeResolver      struct{ *Resolver }
	attributeValueResolver struct{ *Resolver }
)
