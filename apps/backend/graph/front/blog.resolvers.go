package front

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	model1 "github.com/skeletg/skelet/apps/backend/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/internal/blog"
)

func (r *queryResolver) BlogPost(ctx context.Context, slug *string, id *string) (*model.BlogPost, error) {
	loc := r.activeLocale(ctx)
	storeID := channelIDFromContext(ctx)
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, invalidID("post")
		}
		p, err := r.BlogSvc.GetPost(ctx, uid, loc)
		if err != nil {
			return nil, err
		}
		if p.Status != blog.PostStatusPublished {
			return nil, nil
		}
		if storeID != nil && !postListedAndPublished(p, *storeID) {
			return nil, nil
		}
		return toBlogPostModel(p), nil
	case slug != nil && *slug != "":
		if storeID != nil {
			p, err := r.BlogSvc.GetPublishedPostBySlugForChannel(ctx, *slug, *storeID, loc)
			if err != nil {
				return nil, err
			}
			return toBlogPostModel(p), nil
		}
		p, err := r.BlogSvc.GetPostBySlug(ctx, *slug, loc, false)
		if err != nil {
			return nil, err
		}
		return toBlogPostModel(p), nil
	}
	return nil, selectorRequired("slug or id is required")
}

// The storefront cannot bypass the published gate - filter fields only scope to a category/tag/author by slug
func (r *queryResolver) BlogPosts(ctx context.Context, filter *model.BlogPostFilter, limit *int32, offset *int32) (*model.BlogPostList, error) {
	loc := r.activeLocale(ctx)
	f := blog.PostListFilter{
		OnlyPublished: true,
		Limit:         intDeref(limit, 50),
		Offset:        intDeref(offset, 0),
	}
	f.ChannelID = channelIDFromContext(ctx)
	if filter != nil {
		if filter.ChannelID != nil && *filter.ChannelID != "" {
			sv, err := uuid.Parse(*filter.ChannelID)
			if err != nil {
				return nil, invalidID("channel")
			}
			f.ChannelID = &sv
		}
		if filter.CategorySlug != nil && *filter.CategorySlug != "" {
			c, err := r.BlogSvc.GetCategoryBySlug(ctx, *filter.CategorySlug, loc)
			if err != nil {
				return nil, err
			}
			f.CategoryID = &c.ID
		}
		if filter.TagSlug != nil && *filter.TagSlug != "" {
			t, err := r.BlogSvc.GetTagBySlug(ctx, *filter.TagSlug, loc)
			if err != nil {
				return nil, err
			}
			f.TagID = &t.ID
		}
		if filter.AuthorSlug != nil && *filter.AuthorSlug != "" {
			a, err := r.BlogSvc.GetAuthorBySlug(ctx, *filter.AuthorSlug, loc)
			if err != nil {
				return nil, err
			}
			f.AuthorID = &a.ID
		}
	}
	posts, err := r.BlogSvc.ListPosts(ctx, f, loc)
	if err != nil {
		return nil, err
	}
	out := make([]*model.BlogPost, len(posts))
	for i, p := range posts {
		out[i] = toBlogPostModel(p)
	}
	return &model.BlogPostList{Items: out}, nil
}

// Skips the published-and-listed gates BlogPost enforces - the preview token is the authorisation
// nil on unknown or expired tokens, which the dashboard reads as "session expired, refresh"
func (r *queryResolver) PreviewBlogPost(ctx context.Context, token string) (*model.BlogPost, error) {
	if r.BlogPreviewStore == nil {
		return nil, nil
	}
	claims, err := r.BlogPreviewStore.Validate(ctx, token)
	if err != nil {
		return nil, nil
	}
	loc := claims.Locale
	if loc == "" {
		loc = r.activeLocale(ctx)
	}
	p, err := r.BlogSvc.GetPost(ctx, claims.PostID, loc)
	if err != nil {
		return nil, nil
	}
	return toBlogPostModel(p), nil
}

func (r *queryResolver) BlogCategory(ctx context.Context, slug *string, id *string) (*model.BlogCategory, error) {
	loc := r.activeLocale(ctx)
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, invalidID("category")
		}
		c, err := r.BlogSvc.GetCategory(ctx, uid, loc)
		if err != nil {
			return nil, err
		}
		return toBlogCategoryModel(c), nil
	case slug != nil && *slug != "":
		c, err := r.BlogSvc.GetCategoryBySlug(ctx, *slug, loc)
		if err != nil {
			return nil, err
		}
		return toBlogCategoryModel(c), nil
	}
	return nil, selectorRequired("slug or id is required")
}

func (r *queryResolver) BlogCategories(ctx context.Context, parentID *string) ([]*model.BlogCategory, error) {
	loc := r.activeLocale(ctx)
	if parentID != nil && *parentID != "" {
		pid, err := uuid.Parse(*parentID)
		if err != nil {
			return nil, invalidID("parent")
		}
		cats, err := r.BlogSvc.ListCategoryChildren(ctx, pid, loc)
		if err != nil {
			return nil, err
		}
		return blogCategoryModels(cats), nil
	}
	cats, err := r.BlogSvc.ListRootCategories(ctx, loc)
	if err != nil {
		return nil, err
	}
	return blogCategoryModels(cats), nil
}

func (r *queryResolver) BlogTag(ctx context.Context, slug *string, id *string) (*model.BlogTag, error) {
	loc := r.activeLocale(ctx)
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, invalidID("tag")
		}
		t, err := r.BlogSvc.GetTag(ctx, uid, loc)
		if err != nil {
			return nil, err
		}
		return toBlogTagModel(t), nil
	case slug != nil && *slug != "":
		t, err := r.BlogSvc.GetTagBySlug(ctx, *slug, loc)
		if err != nil {
			return nil, err
		}
		return toBlogTagModel(t), nil
	}
	return nil, selectorRequired("slug or id is required")
}

func (r *queryResolver) BlogTags(ctx context.Context) ([]*model.BlogTag, error) {
	tags, err := r.BlogSvc.ListTags(ctx, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]*model.BlogTag, len(tags))
	for i, t := range tags {
		out[i] = toBlogTagModel(t)
	}
	return out, nil
}

func (r *queryResolver) BlogAuthor(ctx context.Context, slug *string, id *string) (*model.BlogAuthor, error) {
	loc := r.activeLocale(ctx)
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, invalidID("author")
		}
		a, err := r.BlogSvc.GetAuthor(ctx, uid, loc)
		if err != nil {
			return nil, err
		}
		return toBlogAuthorModel(a), nil
	case slug != nil && *slug != "":
		a, err := r.BlogSvc.GetAuthorBySlug(ctx, *slug, loc)
		if err != nil {
			return nil, err
		}
		return toBlogAuthorModel(a), nil
	}
	return nil, selectorRequired("slug or id is required")
}

func (r *queryResolver) BlogAuthors(ctx context.Context) ([]*model.BlogAuthor, error) {
	authors, err := r.BlogSvc.ListAuthors(ctx, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]*model.BlogAuthor, len(authors))
	for i, a := range authors {
		out[i] = toBlogAuthorModel(a)
	}
	return out, nil
}

// Mirrors SearchProducts on the catalog side - empty result on a missing index or missing store context, and a soft-fail when the cluster is unreachable so the storefront keeps rendering
func (r *queryResolver) SearchBlogPosts(ctx context.Context, query string, limit *int32, offset *int32) (*model1.BlogPostSearchResult, error) {
	empty := &model1.BlogPostSearchResult{Items: []*model1.BlogPostSearchHit{}, TotalCount: 0}
	if r.BlogPostSearch == nil {
		return empty, nil
	}
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return empty, nil
	}
	lim, off := 20, 0
	if limit != nil {
		lim = int(*limit)
	}
	if offset != nil {
		off = int(*offset)
	}
	hits, total, err := r.BlogPostSearch.SearchBlogPosts(ctx, query, *storeID, r.activeLocale(ctx), lim, off)
	if err != nil {
		return empty, nil
	}
	items := make([]*model1.BlogPostSearchHit, len(hits))
	for i, h := range hits {
		var title, excerpt, author *string
		if h.Title != "" {
			t := h.Title
			title = &t
		}
		if h.Excerpt != "" {
			e := h.Excerpt
			excerpt = &e
		}
		if h.AuthorName != "" {
			a := h.AuthorName
			author = &a
		}
		items[i] = &model1.BlogPostSearchHit{
			ID:          h.PostID.String(),
			Slug:        h.Slug,
			Title:       title,
			Excerpt:     excerpt,
			PublishedAt: h.PublishedAt,
			AuthorName:  author,
		}
	}
	return &model1.BlogPostSearchResult{Items: items, TotalCount: int32(total)}, nil
}
