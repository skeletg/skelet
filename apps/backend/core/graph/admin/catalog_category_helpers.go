package admin

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

func toCategoryModel(c catalog.Category) *model.Category {
	ancestors := make([]*model.Category, len(c.Ancestors))
	for i, a := range c.Ancestors {
		ancestors[i] = toCategoryModel(a)
	}
	children := make([]*model.Category, len(c.Children))
	for i, ch := range c.Children {
		children[i] = toCategoryModel(ch)
	}
	// Nil unless this path loaded every locale's row - the content resolver then reads them itself, rather than
	// answering [] for a category that has translations
	var content []*model.CategoryContent
	if c.AllContent != nil {
		content = make([]*model.CategoryContent, len(c.AllContent))
		for i, cc := range c.AllContent {
			content[i] = toCategoryContentModel(cc)
		}
	}
	m := &model.Category{
		ID:              c.ID.String(),
		Slug:            c.Slug,
		URLPath:         c.URLPath,
		Position:        c.Position,
		IsPublished:     c.IsPublished,
		SeoNoindex:      c.SEONoindex,
		Ancestors:       ancestors,
		Children:        children,
		Level:           int32(len(ancestors)),
		Content:         content,
		Metadata:        toMetadataItems(c.Metadata),
		PrivateMetadata: toMetadataItems(c.PrivateMetadata),
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
	// Parent is the tail of the root->parent chain, so a caller wanting it has to pass ancestors - the category(id) resolver always does, roots pass none and get a nil parent
	if len(ancestors) > 0 {
		m.Parent = ancestors[len(ancestors)-1]
	}
	if c.CoverImage != nil {
		m.CoverImage = &model.CollectionCover{
			URL:      c.CoverImage.URL,
			AltText:  c.CoverImage.AltText,
			Width:    c.CoverImage.Width,
			Height:   c.CoverImage.Height,
			MimeType: c.CoverImage.MimeType,
			Status:   model.ProductMediaStatus(c.CoverImage.Status),
		}
	}
	if len(c.AllContent) > 0 {
		cc := c.AllContent[0]
		m.Name = &cc.Name
	}
	products := make([]*model.Product, len(c.Products))
	for i, p := range c.Products {
		products[i] = toProductBaseModel(p, nil, nil, nil, nil, nil, nil, nil)
	}
	m.Products = products

	if c.Content != nil {
		m.Name = &c.Content.Name
		if len(c.Content.ShortDescription) > 0 {
			var v any
			if err := json.Unmarshal(c.Content.ShortDescription, &v); err == nil {
				m.ShortDescription = v
			}
		}
		if len(c.Content.Description) > 0 {
			var v any
			if err := json.Unmarshal(c.Content.Description, &v); err == nil {
				m.Description = v
			}
		}
		if c.Content.MetaTitle != nil {
			m.MetaTitle = c.Content.MetaTitle
		}
		if c.Content.MetaDescription != nil {
			m.MetaDescription = c.Content.MetaDescription
		}
		locale := c.Content.Locale
		m.ContentLocale = &locale
	}
	listings := make([]*model.CategoryChannelListing, len(c.ChannelListings))
	for i, l := range c.ChannelListings {
		listings[i] = toCategoryChannelListingModel(c.ID, l)
	}
	m.Listings = listings
	return m
}

// Channel is left nil for its field resolver to fill, so a reader that only wants the flags doesn't pay a per-row store lookup
func toCategoryChannelListingModel(categoryID uuid.UUID, l catalog.CategoryChannelListing) *model.CategoryChannelListing {
	out := &model.CategoryChannelListing{
		CategoryID:  categoryID.String(),
		ChannelID:   l.ChannelID.String(),
		IsPublished: l.IsPublished,
		UpdatedAt:   l.UpdatedAt,
	}
	if l.SortOrderOverride != nil {
		out.SortOrderOverride = l.SortOrderOverride
	}
	return out
}

// Maps a category list - a tree included - after loading every node's translation rows in one query, so the
// content field reads them instead of querying per node
func (r *Resolver) renderCategoryList(ctx context.Context, cats []catalog.Category) ([]*model.Category, error) {
	var ids []uuid.UUID
	var walk func([]catalog.Category)
	walk = func(level []catalog.Category) {
		for _, c := range level {
			ids = append(ids, c.ID)
			walk(c.Children)
		}
	}
	walk(cats)
	if err := r.primeCategoryContent(ctx, ids); err != nil {
		return nil, err
	}
	out := make([]*model.Category, len(cats))
	for i, c := range cats {
		out[i] = toCategoryModel(c)
	}
	return out, nil
}
