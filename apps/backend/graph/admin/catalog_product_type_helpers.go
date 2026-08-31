package admin

import (
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
)

func toProductTypeModel(pt catalog.ProductType) *model.ProductType {
	attrs := make([]*model.ProductTypeAttribute, len(pt.Attributes))
	for i, a := range pt.Attributes {
		attrs[i] = toProductTypeAttributeModel(a)
	}
	groups := make([]*model.AttributeGroup, len(pt.Groups))
	for i, g := range pt.Groups {
		groups[i] = toAttributeGroupModel(g)
	}
	return &model.ProductType{
		ID:                 pt.ID.String(),
		Slug:               pt.Slug,
		Name:               pt.Name,
		IsDigital:          pt.IsDigital,
		IsShippingRequired: pt.IsShippingRequired,
		URLTemplate:        pt.URLTemplate,
		TracksInventory:    pt.TracksInventory,
		Attributes:         attrs,
		Groups:             groups,
		Metadata:           toMetadataItems(pt.Metadata),
		PrivateMetadata:    toMetadataItems(pt.PrivateMetadata),
		CreatedAt:          pt.CreatedAt,
		UpdatedAt:          pt.UpdatedAt,
	}
}

func toProductTypeAttributeModel(pta catalog.ProductTypeAttribute) *model.ProductTypeAttribute {
	m := &model.ProductTypeAttribute{
		Attribute: toAttributeModel(pta.Attribute),
		SortOrder: pta.SortOrder,
	}
	if pta.GroupID != nil {
		id := pta.GroupID.String()
		m.GroupID = &id
	}
	return m
}

func toAttributeGroupModel(g catalog.AttributeGroup) *model.AttributeGroup {
	attrs := make([]*model.ProductTypeAttribute, len(g.Attributes))
	for i, a := range g.Attributes {
		attrs[i] = toProductTypeAttributeModel(a)
	}
	return &model.AttributeGroup{
		ID:         g.ID.String(),
		Name:       g.Name,
		SortOrder:  g.SortOrder,
		Attributes: attrs,
	}
}
