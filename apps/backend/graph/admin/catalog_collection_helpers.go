package admin

import (
	"encoding/json"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/catalog"
)

func toCollectionModel(c catalog.Collection) *model.Collection {
	m := &model.Collection{
		ID:              c.ID.String(),
		Slug:            c.Slug,
		IsPublished:     c.IsPublished,
		SeoNoindex:      c.SEONoindex,
		Content:         toCollectionContentModels(c.AllContent),
		CoverImage:      toCollectionCoverModel(c.CoverImage),
		Products:        []*model.Product{},
		Metadata:        toMetadataItems(c.Metadata),
		PrivateMetadata: toMetadataItems(c.PrivateMetadata),
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
	if c.Content != nil {
		m.Name = &c.Content.Name
		if len(c.Content.Description) > 0 {
			var v any
			_ = json.Unmarshal(c.Content.Description, &v)
			m.Description = v
		}
		m.MetaTitle = c.Content.MetaTitle
		m.MetaDescription = c.Content.MetaDescription
		m.MetaKeywords = c.Content.MetaKeywords
		locale := c.Content.Locale
		m.ContentLocale = &locale
	}
	// Products in a collection are loaded and set by resolvers (not in this mapper).
	return m
}

func toCollectionModels(collections []catalog.Collection) []*model.Collection {
	out := make([]*model.Collection, len(collections))
	for i, c := range collections {
		out[i] = toCollectionModel(c)
	}
	return out
}
