package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateProductType(ctx context.Context, input model.CreateProductTypeInput) (*model.ProductType, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	pt, err := r.CatalogSvc.CreateProductType(ctx, catalog.CreateProductTypeInput{
		Slug:               input.Slug,
		Name:               input.Name,
		IsDigital:          boolOrFalse(input.IsDigital),
		IsShippingRequired: boolOrDefault(input.IsShippingRequired, true),
		URLTemplate:        ptr.Deref(input.URLTemplate),
		TracksInventory:    boolOrFalse(input.TracksInventory),
		Metadata:           metadataFromInput(input.Metadata),
		PrivateMetadata:    metadataFromInput(input.PrivateMetadata),
	})
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *mutationResolver) UpdateProductType(ctx context.Context, input model.UpdateProductTypeInput) (*model.ProductType, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(input.ID, "product type")
	if err != nil {
		return nil, err
	}
	domainInput := catalog.UpdateProductTypeInput{
		ID:                 id,
		Name:               input.Name,
		IsDigital:          input.IsDigital,
		IsShippingRequired: input.IsShippingRequired,
		URLTemplate:        input.URLTemplate,
		TracksInventory:    input.TracksInventory,
	}
	if input.Metadata != nil {
		m := metadataFromInput(input.Metadata)
		domainInput.Metadata = &m
	}
	if input.PrivateMetadata != nil {
		m := metadataFromInput(input.PrivateMetadata)
		domainInput.PrivateMetadata = &m
	}
	pt, err := r.CatalogSvc.UpdateProductType(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *mutationResolver) DeleteProductType(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return false, err
	}
	ptID, err := parseID(id, "product type")
	if err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteProductType(ctx, ptID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetProductTypeURLPathTemplate(ctx context.Context, productTypeID string, template string) (*model.ProductType, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	ptID, err := parseID(productTypeID, "product type")
	if err != nil {
		return nil, err
	}
	pt, err := r.CatalogSvc.SetProductTypeURLPathTemplate(ctx, ptID, template)
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *mutationResolver) AssignAttribute(ctx context.Context, productTypeID string, attributeID string, sortOrder *int32, groupID *string) (*model.ProductType, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	ptID, err := parseID(productTypeID, "product type")
	if err != nil {
		return nil, err
	}
	attrID, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	input := catalog.AssignAttributeInput{
		ProductTypeID: ptID,
		AttributeID:   attrID,
		SortOrder:     int32Or(sortOrder, 0),
	}
	if groupID != nil {
		gid, err := parseID(*groupID, "group")
		if err != nil {
			return nil, err
		}
		input.GroupID = &gid
	}
	pt, err := r.CatalogSvc.AssignAttribute(ctx, input)
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *mutationResolver) UnassignAttribute(ctx context.Context, productTypeID string, attributeID string) (*model.ProductType, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	ptID, err := parseID(productTypeID, "product type")
	if err != nil {
		return nil, err
	}
	attrID, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	pt, err := r.CatalogSvc.UnassignAttribute(ctx, ptID, attrID)
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *mutationResolver) CreateAttributeGroup(ctx context.Context, productTypeID string, name string, sortOrder *int32) (*model.ProductType, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	ptID, err := parseID(productTypeID, "product type")
	if err != nil {
		return nil, err
	}
	pt, err := r.CatalogSvc.CreateAttributeGroup(ctx, catalog.CreateAttributeGroupInput{
		ProductTypeID: ptID,
		Name:          name,
		SortOrder:     int32Or(sortOrder, 0),
	})
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *mutationResolver) UpdateAttributeGroup(ctx context.Context, id string, name *string, sortOrder *int32) (*model.ProductType, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	gid, err := parseID(id, "group")
	if err != nil {
		return nil, err
	}
	pt, err := r.CatalogSvc.UpdateAttributeGroup(ctx, catalog.UpdateAttributeGroupInput{
		ID:        gid,
		Name:      name,
		SortOrder: sortOrder,
	})
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *mutationResolver) DeleteAttributeGroup(ctx context.Context, id string) (*model.ProductType, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	gid, err := parseID(id, "group")
	if err != nil {
		return nil, err
	}
	pt, err := r.CatalogSvc.DeleteAttributeGroup(ctx, gid)
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *mutationResolver) MoveAttributeToGroup(ctx context.Context, productTypeID string, attributeID string, groupID *string) (*model.ProductType, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	ptID, err := parseID(productTypeID, "product type")
	if err != nil {
		return nil, err
	}
	attrID, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	var gid *uuid.UUID
	if groupID != nil {
		id, err := parseID(*groupID, "group")
		if err != nil {
			return nil, err
		}
		gid = &id
	}
	pt, err := r.CatalogSvc.MoveAttributeToGroup(ctx, ptID, attrID, gid)
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *queryResolver) ProductTypes(ctx context.Context) ([]*model.ProductType, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	pts, err := r.CatalogSvc.ListProductTypes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ProductType, len(pts))
	for i, pt := range pts {
		out[i] = toProductTypeModel(pt)
	}
	return out, nil
}

func (r *queryResolver) ProductType(ctx context.Context, id string) (*model.ProductType, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	ptID, err := parseID(id, "product type")
	if err != nil {
		return nil, err
	}
	pt, err := r.CatalogSvc.GetProductType(ctx, ptID)
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *Resolver) ProductType() ProductTypeResolver { return &productTypeResolver{r} }

type productTypeResolver struct{ *Resolver }
