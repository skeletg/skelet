package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog/preview"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateBlogAuthor(ctx context.Context, input model.CreateBlogAuthorInput) (*model.BlogAuthor, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return nil, err
	}
	in, err := fromCreateBlogAuthorInput(input)
	if err != nil {
		return nil, err
	}
	a, err := r.BlogSvc.CreateAuthor(ctx, in)
	if err != nil {
		return nil, err
	}
	return toBlogAuthorModel(a), nil
}

func (r *mutationResolver) UpdateBlogAuthor(ctx context.Context, input model.UpdateBlogAuthorInput) (*model.BlogAuthor, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return nil, err
	}
	in, err := fromUpdateBlogAuthorInput(input)
	if err != nil {
		return nil, err
	}
	a, err := r.BlogSvc.UpdateAuthor(ctx, in)
	if err != nil {
		return nil, err
	}
	return toBlogAuthorModel(a), nil
}

func (r *mutationResolver) DeleteBlogAuthor(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return false, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.BlogSvc.DeleteAuthor(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UpsertBlogAuthorContent(ctx context.Context, input model.UpsertBlogAuthorContentInput) (*model.BlogAuthor, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return nil, err
	}
	authorID, err := parseID(input.AuthorID, "author")
	if err != nil {
		return nil, err
	}
	a, err := r.BlogSvc.UpsertAuthorContent(ctx, authorID, blog.AuthorContentInput{
		Locale: input.Locale,
		Name:   input.Name,
		Bio:    rawJSON(input.Bio),
	})
	if err != nil {
		return nil, err
	}
	return toBlogAuthorModel(a), nil
}

func (r *mutationResolver) DeleteBlogAuthorContent(ctx context.Context, authorID string, locale string) (bool, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return false, err
	}
	uid, err := parseID(authorID, "author")
	if err != nil {
		return false, err
	}
	if err := r.BlogSvc.DeleteAuthorContent(ctx, uid, locale); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreateBlogCategory(ctx context.Context, input model.CreateBlogCategoryInput) (*model.BlogCategory, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return nil, err
	}
	in, err := fromCreateBlogCategoryInput(input)
	if err != nil {
		return nil, err
	}
	c, err := r.BlogSvc.CreateCategory(ctx, in)
	if err != nil {
		return nil, err
	}
	return toBlogCategoryModel(c), nil
}

func (r *mutationResolver) UpdateBlogCategory(ctx context.Context, input model.UpdateBlogCategoryInput) (*model.BlogCategory, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return nil, err
	}
	in, err := fromUpdateBlogCategoryInput(input)
	if err != nil {
		return nil, err
	}
	c, err := r.BlogSvc.UpdateCategory(ctx, in)
	if err != nil {
		return nil, err
	}
	return toBlogCategoryModel(c), nil
}

func (r *mutationResolver) DeleteBlogCategory(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return false, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.BlogSvc.DeleteCategory(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UpsertBlogCategoryContent(ctx context.Context, input model.UpsertBlogCategoryContentInput) (*model.BlogCategory, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return nil, err
	}
	categoryID, err := parseID(input.CategoryID, "category")
	if err != nil {
		return nil, err
	}
	c, err := r.BlogSvc.UpsertCategoryContent(ctx, categoryID, blog.CategoryContentInput{
		Locale:          input.Locale,
		Name:            input.Name,
		Description:     rawJSON(input.Description),
		MetaTitle:       ptr.Deref(input.MetaTitle),
		MetaDescription: ptr.Deref(input.MetaDescription),
		MetaKeywords:    ptr.Deref(input.MetaKeywords),
	})
	if err != nil {
		return nil, err
	}
	return toBlogCategoryModel(c), nil
}

func (r *mutationResolver) DeleteBlogCategoryContent(ctx context.Context, categoryID string, locale string) (bool, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return false, err
	}
	uid, err := parseID(categoryID, "category")
	if err != nil {
		return false, err
	}
	if err := r.BlogSvc.DeleteCategoryContent(ctx, uid, locale); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreateBlogTag(ctx context.Context, input model.CreateBlogTagInput) (*model.BlogTag, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return nil, err
	}
	in := blog.CreateTagInput{
		Slug:            input.Slug,
		IsPublished:     boolDeref(input.IsPublished, true),
		Locale:          ptr.Deref(input.Locale),
		Name:            ptr.Deref(input.Name),
		Description:     rawJSON(input.Description),
		MetaTitle:       ptr.Deref(input.MetaTitle),
		MetaDescription: ptr.Deref(input.MetaDescription),
		MetaKeywords:    ptr.Deref(input.MetaKeywords),
	}
	t, err := r.BlogSvc.CreateTag(ctx, in)
	if err != nil {
		return nil, err
	}
	return toBlogTagModel(t), nil
}

func (r *mutationResolver) UpdateBlogTag(ctx context.Context, input model.UpdateBlogTagInput) (*model.BlogTag, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return nil, err
	}
	tid, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	t, err := r.BlogSvc.UpdateTag(ctx, blog.UpdateTagInput{
		ID:          tid,
		Slug:        input.Slug,
		IsPublished: input.IsPublished,
	})
	if err != nil {
		return nil, err
	}
	return toBlogTagModel(t), nil
}

func (r *mutationResolver) DeleteBlogTag(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return false, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.BlogSvc.DeleteTag(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UpsertBlogTagContent(ctx context.Context, input model.UpsertBlogTagContentInput) (*model.BlogTag, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return nil, err
	}
	tid, err := parseID(input.TagID, "tag")
	if err != nil {
		return nil, err
	}
	t, err := r.BlogSvc.UpsertTagContent(ctx, tid, blog.TagContentInput{
		Locale:          input.Locale,
		Name:            input.Name,
		Description:     rawJSON(input.Description),
		MetaTitle:       ptr.Deref(input.MetaTitle),
		MetaDescription: ptr.Deref(input.MetaDescription),
		MetaKeywords:    ptr.Deref(input.MetaKeywords),
	})
	if err != nil {
		return nil, err
	}
	return toBlogTagModel(t), nil
}

func (r *mutationResolver) DeleteBlogTagContent(ctx context.Context, tagID string, locale string) (bool, error) {
	if err := r.requireGlobal(ctx, "blog.manage"); err != nil {
		return false, err
	}
	uid, err := parseID(tagID, "tag")
	if err != nil {
		return false, err
	}
	if err := r.BlogSvc.DeleteTagContent(ctx, uid, locale); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreateBlogPost(ctx context.Context, input model.CreateBlogPostInput) (*model.BlogPost, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "blog.manage"); err != nil {
		return nil, err
	}
	in, err := fromCreateBlogPostInput(input)
	if err != nil {
		return nil, err
	}
	p, err := r.BlogSvc.CreatePost(ctx, in)
	if err != nil {
		return nil, err
	}
	return toBlogPostModel(p), nil
}

func (r *mutationResolver) UpdateBlogPost(ctx context.Context, input model.UpdateBlogPostInput) (*model.BlogPost, error) {
	in, err := fromUpdateBlogPostInput(input)
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPostChannels(ctx, "blog.manage", in.ID, ""); err != nil {
		return nil, err
	}
	p, err := r.BlogSvc.UpdatePost(ctx, in)
	if err != nil {
		return nil, err
	}
	return toBlogPostModel(p), nil
}

func (r *mutationResolver) DeleteBlogPost(ctx context.Context, id string) (bool, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.requireOnPostChannels(ctx, "blog.manage", uid, ""); err != nil {
		return false, err
	}
	if err := r.BlogSvc.DeletePost(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

// All-listed-channels gate: the post-content row is shared across every channel listing the post, so the admin must
// have blog.manage at the requested locale on every one of those channels (or a global grant)
func (r *mutationResolver) UpsertBlogPostContent(ctx context.Context, input model.UpsertBlogPostContentInput) (*model.BlogPost, error) {
	pid, err := parseID(input.PostID, "post")
	if err != nil {
		return nil, err
	}
	listings, err := r.BlogSvc.ListPostChannelListings(ctx, pid)
	if err != nil {
		return nil, err
	}
	channelIDs := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		channelIDs[i] = l.ChannelID
	}
	if err := auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, "blog.manage", channelIDs, input.Locale); err != nil {
		return nil, err
	}
	p, err := r.BlogSvc.UpsertPostContent(ctx, pid, blog.PostContentInput{
		Locale:          input.Locale,
		Title:           input.Title,
		Excerpt:         rawJSON(input.Excerpt),
		Body:            rawJSON(input.Body),
		MetaTitle:       ptr.Deref(input.MetaTitle),
		MetaDescription: ptr.Deref(input.MetaDescription),
		MetaKeywords:    ptr.Deref(input.MetaKeywords),
	})
	if err != nil {
		return nil, err
	}
	return toBlogPostModel(p), nil
}

// Same all-listed-channels gate as UpsertBlogPostContent.
func (r *mutationResolver) DeleteBlogPostContent(ctx context.Context, postID string, locale string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "blog.manage"); err != nil {
		return false, err
	}
	uid, err := parseID(postID, "post")
	if err != nil {
		return false, err
	}
	listings, err := r.BlogSvc.ListPostChannelListings(ctx, uid)
	if err != nil {
		return false, err
	}
	channelIDs := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		channelIDs[i] = l.ChannelID
	}
	if err := auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, "blog.manage", channelIDs, locale); err != nil {
		return false, err
	}
	if err := r.BlogSvc.DeletePostContent(ctx, uid, locale); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) PublishBlogPost(ctx context.Context, id string) (*model.BlogPost, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	if err := r.requireOnPostChannels(ctx, "blog.manage", uid, ""); err != nil {
		return nil, err
	}
	p, err := r.BlogSvc.PublishPost(ctx, uid)
	if err != nil {
		return nil, err
	}
	return toBlogPostModel(p), nil
}

func (r *mutationResolver) ScheduleBlogPost(ctx context.Context, id string, publishedAt time.Time) (*model.BlogPost, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	if err := r.requireOnPostChannels(ctx, "blog.manage", uid, ""); err != nil {
		return nil, err
	}
	p, err := r.BlogSvc.SchedulePost(ctx, uid, publishedAt)
	if err != nil {
		return nil, err
	}
	return toBlogPostModel(p), nil
}

func (r *mutationResolver) UnpublishBlogPost(ctx context.Context, id string) (*model.BlogPost, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	if err := r.requireOnPostChannels(ctx, "blog.manage", uid, ""); err != nil {
		return nil, err
	}
	p, err := r.BlogSvc.UnpublishPost(ctx, uid)
	if err != nil {
		return nil, err
	}
	return toBlogPostModel(p), nil
}

func (r *mutationResolver) ArchiveBlogPost(ctx context.Context, id string) (*model.BlogPost, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	if err := r.requireOnPostChannels(ctx, "blog.manage", uid, ""); err != nil {
		return nil, err
	}
	p, err := r.BlogSvc.ArchivePost(ctx, uid)
	if err != nil {
		return nil, err
	}
	return toBlogPostModel(p), nil
}

func (r *mutationResolver) SetBlogPostCategories(ctx context.Context, postID string, categoryIds []string) (*model.BlogPost, error) {
	pid, err := parseID(postID, "post")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPostChannels(ctx, "blog.manage", pid, ""); err != nil {
		return nil, err
	}
	cids, err := parseUUIDs(categoryIds)
	if err != nil {
		return nil, err
	}
	p, err := r.BlogSvc.SetPostCategories(ctx, pid, cids)
	if err != nil {
		return nil, err
	}
	return toBlogPostModel(p), nil
}

func (r *mutationResolver) SetBlogPostTags(ctx context.Context, postID string, tagIds []string) (*model.BlogPost, error) {
	pid, err := parseID(postID, "post")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPostChannels(ctx, "blog.manage", pid, ""); err != nil {
		return nil, err
	}
	tids, err := parseUUIDs(tagIds)
	if err != nil {
		return nil, err
	}
	p, err := r.BlogSvc.SetPostTags(ctx, pid, tids)
	if err != nil {
		return nil, err
	}
	return toBlogPostModel(p), nil
}

func (r *mutationResolver) SetBlogPostChannelListing(ctx context.Context, input model.SetBlogPostChannelListingInput) (*model.BlogPost, error) {
	postID, err := parseID(input.PostID, "post")
	if err != nil {
		return nil, err
	}
	storeID, err := parseID(input.ChannelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := r.requirePostListingChange(ctx, postID, storeID, false); err != nil {
		return nil, err
	}
	p, err := r.BlogSvc.SetPostChannelListing(ctx, blog.SetPostChannelListingInput{
		PostID:      postID,
		ChannelID:   storeID,
		IsPublished: input.IsPublished,
		SEONoindex:  boolDeref(input.SeoNoindex, false),
	})
	if err != nil {
		return nil, err
	}
	return toBlogPostModel(p), nil
}

func (r *mutationResolver) DeleteBlogPostChannelListing(ctx context.Context, postID string, channelID string) (*model.BlogPost, error) {
	pid, err := parseID(postID, "post")
	if err != nil {
		return nil, err
	}
	sid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := r.requirePostListingChange(ctx, pid, sid, true); err != nil {
		return nil, err
	}
	p, err := r.BlogSvc.DeletePostChannelListing(ctx, pid, sid)
	if err != nil {
		return nil, err
	}
	return toBlogPostModel(p), nil
}

func (r *mutationResolver) MintBlogPostPreviewToken(ctx context.Context, input model.MintBlogPostPreviewTokenInput) (*model.BlogPostPreviewToken, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "blog.manage"); err != nil {
		return nil, err
	}
	if r.BlogPreviewStore == nil {
		return nil, errNotConfigured("preview store")
	}
	postID, err := parseID(input.PostID, "post")
	if err != nil {
		return nil, err
	}
	// Access first, then existence - the other order answered "not found" for posts on channels the caller can't touch, which told a channel-scoped admin which ids exist elsewhere
	if err := r.requireOnPostChannels(ctx, "blog.manage", postID, ""); err != nil {
		return nil, err
	}
	if _, err := r.BlogSvc.GetPost(ctx, postID, ""); err != nil { // no tokens for posts that don't exist
		if errors.Is(err, blog.ErrPostNotFound) { // a post listed nowhere passes the channel check, so only a global holder learns an id is unused
			if gerr := auth.RequireGlobalPermission(ctx, r.AdminSvc, "blog.manage"); gerr != nil {
				return nil, gerr
			}
		}
		return nil, err
	}
	claims, err := auth.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	loc := ""
	if input.Locale != nil {
		loc = *input.Locale
	}
	res, err := r.BlogPreviewStore.Mint(ctx, preview.MintInput{
		PostID:  postID,
		AdminID: claims.AdminID,
		Locale:  loc,
	})
	if err != nil {
		return nil, err
	}
	return &model.BlogPostPreviewToken{Token: res.Token, ExpiresAt: res.ExpiresAt}, nil
}

func (r *queryResolver) BlogPost(ctx context.Context, slug *string, id *string, includeUnpublished *bool) (*model.BlogPost, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "blog.view"); err != nil {
		return nil, err
	}
	loc := r.activeLocale(ctx)
	includeUnpub := boolDeref(includeUnpublished, false)
	// Admin endpoint: scope is explicit, never the ambient (default) channel. includeUnpub=true returns drafts/scheduled/archived for preview; otherwise only published posts are returned, ungated by channel - per-store listing gating is a storefront concern handled on the front endpoint.
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, errInvalidID("id", err)
		}
		p, err := r.BlogSvc.GetPost(ctx, uid, loc)
		if err != nil {
			return nil, err
		}
		if !includeUnpub && p.Status != blog.PostStatusPublished {
			return nil, nil
		}
		return toBlogPostModel(p), nil
	case slug != nil && *slug != "":
		p, err := r.BlogSvc.GetPostBySlug(ctx, *slug, loc, includeUnpub)
		if err != nil {
			return nil, err
		}
		return toBlogPostModel(p), nil
	}
	return nil, fmt.Errorf("slug or id is required")
}

func (r *queryResolver) BlogPosts(ctx context.Context, filter *model.BlogPostFilter, limit *int32, offset *int32) (*model.BlogPostList, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "blog.view"); err != nil {
		return nil, err
	}
	loc := r.activeLocale(ctx)
	f := blog.PostListFilter{
		OnlyPublished: true,
		Limit:         intDeref(limit, 50),
		Offset:        intDeref(offset, 0),
	}
	// Channel scope: an explicit filter.channelId overrides; otherwise the admin's ambient scope channel, and nil = all
	// channels (Global). See scopedChannelID
	var channelArg *string
	if filter != nil {
		channelArg = filter.ChannelID
	}
	chID, err := r.scopedChannelID(ctx, channelArg)
	if err != nil {
		return nil, err
	}
	f.ChannelID = chID
	if filter != nil {
		if boolDeref(filter.IncludeUnpublished, false) {
			f.OnlyPublished = false
		}
		if filter.Status != nil {
			s := blog.PostStatus(strLowerStatus(*filter.Status))
			f.Status = &s
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
	if posts, err = r.withBlogPostTranslations(ctx, posts); err != nil {
		return nil, err
	}
	out := make([]*model.BlogPost, len(posts))
	for i, p := range posts {
		out[i] = toBlogPostModel(p)
	}
	return &model.BlogPostList{Items: out}, nil
}

func (r *queryResolver) BlogCategory(ctx context.Context, slug *string, id *string) (*model.BlogCategory, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "blog.view"); err != nil {
		return nil, err
	}
	loc := r.activeLocale(ctx)
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, errInvalidID("id", err)
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
	return nil, fmt.Errorf("slug or id is required")
}

func (r *queryResolver) BlogCategories(ctx context.Context, parentID *string) ([]*model.BlogCategory, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "blog.view"); err != nil {
		return nil, err
	}
	loc := r.activeLocale(ctx)
	var cats []blog.Category
	var err error
	if parentID != nil && *parentID != "" {
		pid, err := parseID(*parentID, "parent")
		if err != nil {
			return nil, err
		}
		cats, err2 := r.BlogSvc.ListCategoryChildren(ctx, pid, loc)
		if err2 != nil {
			return nil, err2
		}
		if cats, err2 = r.withBlogCategoryTranslations(ctx, cats); err2 != nil {
			return nil, err2
		}
		return blogCategoryModels(cats), nil
	}
	cats, err = r.BlogSvc.ListRootCategories(ctx, loc)
	if err != nil {
		return nil, err
	}
	if cats, err = r.withBlogCategoryTranslations(ctx, cats); err != nil {
		return nil, err
	}
	return blogCategoryModels(cats), nil
}

func (r *queryResolver) BlogTag(ctx context.Context, slug *string, id *string) (*model.BlogTag, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "blog.view"); err != nil {
		return nil, err
	}
	loc := r.activeLocale(ctx)
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, errInvalidID("id", err)
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
	return nil, fmt.Errorf("slug or id is required")
}

func (r *queryResolver) BlogTags(ctx context.Context) ([]*model.BlogTag, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "blog.view"); err != nil {
		return nil, err
	}
	tags, err := r.BlogSvc.ListTags(ctx, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	if tags, err = r.withBlogTagTranslations(ctx, tags); err != nil {
		return nil, err
	}
	out := make([]*model.BlogTag, len(tags))
	for i, t := range tags {
		out[i] = toBlogTagModel(t)
	}
	return out, nil
}

func (r *queryResolver) BlogAuthor(ctx context.Context, slug *string, id *string) (*model.BlogAuthor, error) {
	// Authors carry an email - the admin surface must never hand that out ungated
	if err := auth.RequirePermission(ctx, r.AdminSvc, "blog.view"); err != nil {
		return nil, err
	}
	loc := r.activeLocale(ctx)
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, errInvalidID("id", err)
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
	return nil, fmt.Errorf("slug or id is required")
}

func (r *queryResolver) BlogAuthors(ctx context.Context) ([]*model.BlogAuthor, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "blog.view"); err != nil {
		return nil, err
	}
	authors, err := r.BlogSvc.ListAuthors(ctx, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	if authors, err = r.withBlogAuthorTranslations(ctx, authors); err != nil {
		return nil, err
	}
	out := make([]*model.BlogAuthor, len(authors))
	for i, a := range authors {
		out[i] = toBlogAuthorModel(a)
	}
	return out, nil
}
