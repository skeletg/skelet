package modelconv

import (
	"sort"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/blog"
)

func BlogPostStatus(s blog.PostStatus) model.BlogPostStatus {
	switch s {
	case blog.PostStatusScheduled:
		return model.BlogPostStatusScheduled
	case blog.PostStatusPublished:
		return model.BlogPostStatusPublished
	case blog.PostStatusArchived:
		return model.BlogPostStatusArchived
	}
	return model.BlogPostStatusDraft
}

// BlogMetadataItems converts a flat string→string map to the GraphQL metadata-item shape used across the schema. Keys
// are sorted for stable output
func BlogMetadataItems(m blog.Metadata) []*model.MetadataItem {
	if len(m) == 0 {
		return []*model.MetadataItem{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*model.MetadataItem, len(keys))
	for i, k := range keys {
		out[i] = &model.MetadataItem{Key: k, Value: m[k]}
	}
	return out
}

// ==================== Conversions: Author ====================

func BlogAuthorModel(a blog.Author) *model.BlogAuthor {
	m := &model.BlogAuthor{
		ID:          a.ID.String(),
		Slug:        a.Slug,
		DisplayName: a.DisplayName,
		Email:       a.Email,
		Metadata:    BlogMetadataItems(a.Metadata),
		AllContent:  BlogAuthorContentModels(a.AllContent),
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
	if a.Content != nil {
		m.Content = BlogAuthorContentModel(*a.Content)
	}
	return m
}

func BlogAuthorContentModel(c blog.AuthorContent) *model.BlogAuthorContent {
	return &model.BlogAuthorContent{
		Locale:    c.Locale,
		Name:      c.Name,
		Bio:       unmarshalJSON(c.Bio),
		UpdatedAt: c.UpdatedAt,
	}
}

func BlogAuthorContentModels(in []blog.AuthorContent) []*model.BlogAuthorContent {
	out := make([]*model.BlogAuthorContent, len(in))
	for i, c := range in {
		out[i] = BlogAuthorContentModel(c)
	}
	return out
}

// ==================== Conversions: Category ====================

func BlogCategoryModel(c blog.Category) *model.BlogCategory {
	m := &model.BlogCategory{
		ID:          c.ID.String(),
		Slug:        c.Slug,
		URLPath:     c.URLPath,
		Position:    c.Position,
		IsPublished: c.IsPublished,
		SeoNoindex:  c.SEONoindex,
		AllContent:  BlogCategoryContentModels(c.AllContent),
		Children:    BlogCategoryModels(c.Children),
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
	if c.Content != nil {
		m.Content = BlogCategoryContentModel(*c.Content)
	}
	return m
}

func BlogCategoryModels(cats []blog.Category) []*model.BlogCategory {
	if cats == nil {
		return []*model.BlogCategory{}
	}
	out := make([]*model.BlogCategory, len(cats))
	for i, c := range cats {
		out[i] = BlogCategoryModel(c)
	}
	return out
}

func BlogCategoryContentModel(c blog.CategoryContent) *model.BlogCategoryContent {
	return &model.BlogCategoryContent{
		Locale:          c.Locale,
		Name:            c.Name,
		Description:     unmarshalJSON(c.Description),
		MetaTitle:       c.MetaTitle,
		MetaDescription: c.MetaDescription,
		MetaKeywords:    c.MetaKeywords,
		UpdatedAt:       c.UpdatedAt,
	}
}

func BlogCategoryContentModels(in []blog.CategoryContent) []*model.BlogCategoryContent {
	out := make([]*model.BlogCategoryContent, len(in))
	for i, c := range in {
		out[i] = BlogCategoryContentModel(c)
	}
	return out
}

// ==================== Conversions: Tag ====================

func BlogTagModel(t blog.Tag) *model.BlogTag {
	m := &model.BlogTag{
		ID:          t.ID.String(),
		Slug:        t.Slug,
		IsPublished: t.IsPublished,
		AllContent:  BlogTagContentModels(t.AllContent),
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
	if t.Content != nil {
		m.Content = BlogTagContentModel(*t.Content)
	}
	return m
}

func BlogTagContentModel(c blog.TagContent) *model.BlogTagContent {
	return &model.BlogTagContent{
		Locale:          c.Locale,
		Name:            c.Name,
		Description:     unmarshalJSON(c.Description),
		MetaTitle:       c.MetaTitle,
		MetaDescription: c.MetaDescription,
		MetaKeywords:    c.MetaKeywords,
		UpdatedAt:       c.UpdatedAt,
	}
}

func BlogTagContentModels(in []blog.TagContent) []*model.BlogTagContent {
	out := make([]*model.BlogTagContent, len(in))
	for i, c := range in {
		out[i] = BlogTagContentModel(c)
	}
	return out
}

// ==================== Conversions: Post ====================

func BlogPostModel(p blog.Post) *model.BlogPost {
	m := &model.BlogPost{
		ID:                 p.ID.String(),
		Slug:               p.Slug,
		Status:             BlogPostStatus(p.Status),
		PublishedAt:        p.PublishedAt,
		SeoNoindex:         p.SEONoindex,
		ReadingTimeMinutes: p.ReadingTimeMinutes,
		Metadata:           BlogMetadataItems(p.Metadata),
		PrivateMetadata:    BlogMetadataItems(p.PrivateMetadata),
		AllContent:         BlogPostContentModels(p.AllContent),
		Categories:         BlogCategoryModels(p.Categories),
		Tags:               BlogTagsList(p.Tags),
		ChannelListings:    BlogPostChannelListings(p.ChannelListings),
		CreatedAt:          p.CreatedAt,
		UpdatedAt:          p.UpdatedAt,
	}
	if p.Content != nil {
		m.Content = BlogPostContentModel(*p.Content)
	}
	if p.Author != nil {
		m.Author = BlogAuthorModel(*p.Author)
	}
	return m
}

func BlogPostContentModel(c blog.PostContent) *model.BlogPostContent {
	return &model.BlogPostContent{
		Locale:          c.Locale,
		Title:           c.Title,
		Excerpt:         unmarshalJSON(c.Excerpt),
		Body:            unmarshalJSON(c.Body),
		MetaTitle:       c.MetaTitle,
		MetaDescription: c.MetaDescription,
		MetaKeywords:    c.MetaKeywords,
		UpdatedAt:       c.UpdatedAt,
	}
}

func BlogPostContentModels(in []blog.PostContent) []*model.BlogPostContent {
	out := make([]*model.BlogPostContent, len(in))
	for i, c := range in {
		out[i] = BlogPostContentModel(c)
	}
	return out
}

func BlogPostChannelListings(in []blog.PostChannelListing) []*model.BlogPostChannelListing {
	out := make([]*model.BlogPostChannelListing, len(in))
	for i, l := range in {
		out[i] = &model.BlogPostChannelListing{
			ChannelID:   l.ChannelID.String(),
			IsPublished: l.IsPublished,
			SeoNoindex:  l.SEONoindex,
			CreatedAt:   l.CreatedAt,
			UpdatedAt:   l.UpdatedAt,
		}
	}
	return out
}

func BlogTagsList(tags []blog.Tag) []*model.BlogTag {
	if tags == nil {
		return []*model.BlogTag{}
	}
	out := make([]*model.BlogTag, len(tags))
	for i, t := range tags {
		out[i] = BlogTagModel(t)
	}
	return out
}

// PostListedAndPublished returns true when the post has an active, published
// listing for the given channel. Used to scope storefront-visible posts.
func PostListedAndPublished(p blog.Post, storeID uuid.UUID) bool {
	for _, l := range p.ChannelListings {
		if l.ChannelID == storeID && l.IsPublished {
			return true
		}
	}
	return false
}
