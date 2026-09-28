package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
)

type postStore interface {
	DeletePost(ctx context.Context, id uuid.UUID) error
	DeletePostChannelListing(ctx context.Context, postID, channelID uuid.UUID) error
	DeletePostContent(ctx context.Context, postID uuid.UUID, locale string) error
	GetAuthorByID(ctx context.Context, id uuid.UUID) (Author, error)
	GetPostByID(ctx context.Context, id uuid.UUID) (Post, error)
	GetPostBySlug(ctx context.Context, slug string) (Post, error)
	GetPublishedPostBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID) (Post, error)
	ListAuthorContentByScope(ctx context.Context, authorIDs []uuid.UUID, locale string) (map[uuid.UUID]AuthorContent, error)
	ListAuthorsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Author, error)
	ListCategoriesByPostIDs(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID][]Category, error)
	ListDueScheduledPosts(ctx context.Context) ([]Post, error)
	ListPostCategories(ctx context.Context, postID uuid.UUID) ([]Category, error)
	ListPostChannelListings(ctx context.Context, postID uuid.UUID) ([]PostChannelListing, error)
	ListPostContent(ctx context.Context, postID uuid.UUID) ([]PostContent, error)
	ListPostContentByScope(ctx context.Context, postIDs []uuid.UUID, locale string) (map[uuid.UUID]PostContent, error)
	ListPostContentFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]PostContent, error)
	ListPostTags(ctx context.Context, postID uuid.UUID) ([]Tag, error)
	ListPosts(ctx context.Context, filter PostListFilter) ([]Post, error)
	ListTagsByPostIDs(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID][]Tag, error)
	SetPostStatus(ctx context.Context, id uuid.UUID, status PostStatus, publishedAt *time.Time) (Post, error)
	UpdatePost(ctx context.Context, in UpdatePostInput) (Post, error)
	UpsertPostChannelListing(ctx context.Context, in SetPostChannelListingInput) (PostChannelListing, error)
	UpsertPostContent(ctx context.Context, postID uuid.UUID, in PostContentInput) (PostContent, error)
	WithTx(t *sql.Tx) *postgresRepository
}

func (s *postService) CreatePost(ctx context.Context, in CreatePostInput) (Post, error) {
	if err := validateSlug(in.Slug); err != nil {
		return Post{}, err
	}
	if strings.TrimSpace(in.Locale) == "" {
		return Post{}, fmt.Errorf("locale is required")
	}
	if strings.TrimSpace(in.Title) == "" {
		return Post{}, fmt.Errorf("title is required")
	}
	if in.Status == "" {
		in.Status = PostStatusDraft
	}
	if !in.Status.IsValid() {
		return Post{}, fmt.Errorf("%w: %q", ErrInvalidStatus, in.Status)
	}
	if (in.Status == PostStatusScheduled || in.Status == PostStatusPublished) && in.PublishedAt == nil {
		return Post{}, ErrPublishedAtRequired
	}
	if in.Status == PostStatusScheduled && in.PublishedAt != nil && in.PublishedAt.Before(time.Now()) {
		return Post{}, ErrScheduledInPast
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return Post{}, err
		}
	}
	if in.AuthorID != nil {
		if _, err := s.repo.GetAuthorByID(ctx, *in.AuthorID); err != nil {
			return Post{}, err
		}
	}

	var created Post
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		p, err := r.CreatePost(ctx, in)
		if err != nil {
			return err
		}
		if _, err := r.UpsertPostContent(ctx, p.ID, PostContentInput{
			Locale:          in.Locale,
			Title:           in.Title,
			Excerpt:         in.Excerpt,
			Body:            in.Body,
			MetaTitle:       in.MetaTitle,
			MetaDescription: in.MetaDescription,
			MetaKeywords:    in.MetaKeywords,
		}); err != nil {
			return err
		}
		for i, cid := range in.CategoryIDs {
			if err := r.AddPostCategory(ctx, p.ID, cid, int32(i)); err != nil {
				return err
			}
		}
		for _, tid := range in.TagIDs {
			if err := r.AddPostTag(ctx, p.ID, tid); err != nil {
				return err
			}
		}
		created = p
		return nil
	}); err != nil {
		return Post{}, err
	}

	if s.urlRewrites != nil {
		_ = s.urlRewrites.SyncCanonical(ctx, entityPost, created.ID, "blog/"+created.Slug)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PostCreated{PostID: created.ID, Slug: created.Slug})
	}

	return s.GetPost(ctx, created.ID, in.Locale)
}

func (s *postService) GetPost(ctx context.Context, id uuid.UUID, locale string) (Post, error) {
	p, err := s.repo.GetPostByID(ctx, id)
	if err != nil {
		return Post{}, err
	}
	s.fillPostContent(ctx, &p, locale)
	s.hydratePostRelations(ctx, &p, locale)
	return p, nil
}

func (s *postService) GetPostBySlug(ctx context.Context, slug, locale string, includeUnpublished bool) (Post, error) {
	p, err := s.repo.GetPostBySlug(ctx, slug)
	if err != nil {
		return Post{}, err
	}
	if !includeUnpublished && p.Status != PostStatusPublished {
		return Post{}, ErrPostNotFound
	}
	s.fillPostContent(ctx, &p, locale)
	s.hydratePostRelations(ctx, &p, locale)
	return p, nil
}

// Author + localized categories/tags for the two single-post paths - detail pages had no
// taxonomies at all, so the storefront resolved names client-side from separate queries
func (s *postService) hydratePostRelations(ctx context.Context, p *Post, locale string) {
	if p.AuthorID != nil {
		if a, err := s.repo.GetAuthorByID(ctx, *p.AuthorID); err == nil {
			s.fillAuthorContent(ctx, &a, locale)
			p.Author = &a
		}
	}
	if cats, err := s.repo.ListPostCategories(ctx, p.ID); err == nil && len(cats) > 0 {
		s.fillCategoryListContent(ctx, cats, locale)
		p.Categories = cats
	}
	if tags, err := s.repo.ListPostTags(ctx, p.ID); err == nil && len(tags) > 0 {
		s.fillTagListContent(ctx, tags, locale)
		p.Tags = tags
	}
}

func (s *postService) ListPosts(ctx context.Context, filter PostListFilter, locale string) ([]Post, error) {
	posts, err := s.repo.ListPosts(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		return posts, nil
	}
	ids := make([]uuid.UUID, len(posts))
	for i, p := range posts {
		ids[i] = p.ID
	}
	// Translations
	if locale != "" {
		batch, err := s.repo.ListPostContentByScope(ctx, ids, locale)
		if err != nil {
			return nil, err
		}
		for i, p := range posts {
			if c, ok := batch[p.ID]; ok {
				cp := c
				posts[i].Content = &cp
			}
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		var missing []uuid.UUID
		for _, p := range posts {
			if p.Content == nil {
				missing = append(missing, p.ID)
			}
		}
		if len(missing) > 0 {
			fb, err := s.repo.ListPostContentByScope(ctx, missing, def)
			if err != nil {
				return nil, err
			}
			for i, p := range posts {
				if p.Content != nil {
					continue
				}
				if c, ok := fb[p.ID]; ok {
					cp := c
					posts[i].Content = &cp
				}
			}
		}
	}
	// Categories + tags
	cats, err := s.repo.ListCategoriesByPostIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	tags, err := s.repo.ListTagsByPostIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i, p := range posts {
		if c := cats[p.ID]; c != nil {
			posts[i].Categories = c
		}
		if t := tags[p.ID]; t != nil {
			posts[i].Tags = t
		}
	}
	s.fillPostTaxonomyContent(ctx, posts, locale)
	// Authors - GetPostBySlug has always hydrated these; without the same here every list surface (blog index, taxonomy archives, related
	// rails) renders bylines as empty - Batched on the distinct ids so a page of posts costs two queries, not two per row
	authorIDs := make([]uuid.UUID, 0, len(posts))
	seenAuthor := make(map[uuid.UUID]bool, len(posts))
	for _, p := range posts {
		if p.AuthorID == nil || seenAuthor[*p.AuthorID] {
			continue
		}
		seenAuthor[*p.AuthorID] = true
		authorIDs = append(authorIDs, *p.AuthorID)
	}
	if len(authorIDs) > 0 {
		authors, err := s.repo.ListAuthorsByIDs(ctx, authorIDs)
		if err != nil {
			return nil, err
		}
		if locale != "" {
			content, err := s.repo.ListAuthorContentByScope(ctx, authorIDs, locale)
			if err != nil {
				return nil, err
			}
			for id, c := range content {
				if a, ok := authors[id]; ok {
					cp := c
					a.Content = &cp
					authors[id] = a
				}
			}
		}
		for i, p := range posts {
			if p.AuthorID == nil {
				continue
			}
			if a, ok := authors[*p.AuthorID]; ok {
				ac := a
				posts[i].Author = &ac
			}
		}
	}
	return posts, nil
}

func (s *postService) UpdatePost(ctx context.Context, in UpdatePostInput) (Post, error) {
	if in.Slug != nil {
		if err := validateSlug(*in.Slug); err != nil {
			return Post{}, err
		}
	}
	current, err := s.repo.GetPostByID(ctx, in.ID)
	if err != nil {
		return Post{}, err
	}
	if in.AuthorID != nil {
		if _, err := s.repo.GetAuthorByID(ctx, *in.AuthorID); err != nil {
			return Post{}, err
		}
	}
	updated, err := s.repo.UpdatePost(ctx, in)
	if err != nil {
		return Post{}, err
	}
	if s.urlRewrites != nil && in.Slug != nil && *in.Slug != current.Slug {
		if err := s.urlRewrites.SyncCanonical(ctx, entityPost, updated.ID, "blog/"+updated.Slug); err != nil {
			applog.From(ctx).Error("sync canonical url rewrite", "entity", entityPost, "id", updated.ID, "err", err)
		}
		if err := s.urlRewrites.CreateRedirect(ctx, "blog/"+current.Slug, entityPost, updated.ID, 301); err != nil {
			applog.From(ctx).Error("create rename redirect", "entity", entityPost, "id", updated.ID, "err", err)
		}
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PostUpdated{PostID: updated.ID, Slug: updated.Slug})
	}
	return s.GetPost(ctx, updated.ID, "")
}

// publishedAt is stamped now() when not already set
func (s *postService) PublishPost(ctx context.Context, id uuid.UUID) (Post, error) {
	current, err := s.repo.GetPostByID(ctx, id)
	if err != nil {
		return Post{}, err
	}
	// Sync pre-commit hook for editorial review / lockdown windows
	if s.bus != nil && current.Status != PostStatusPublished {
		ing := &PostPublishing{
			PostID:   current.ID,
			Slug:     current.Slug,
			AuthorID: current.AuthorID,
		}
		if err := s.bus.PublishSync(ctx, ing); err != nil {
			return Post{}, err
		}
	}
	publishedAt := current.PublishedAt
	if publishedAt == nil {
		now := time.Now()
		publishedAt = &now
	}
	_, err = s.repo.SetPostStatus(ctx, id, PostStatusPublished, publishedAt)
	if err != nil {
		return Post{}, err
	}
	if s.bus != nil && current.Status != PostStatusPublished {
		// Only emit on actual transitions - re-publishing an already-published post is a noop downstream-wise
		s.bus.Publish(ctx, &PostPublished{
			PostID:   current.ID,
			Slug:     current.Slug,
			AuthorID: current.AuthorID,
		})
	}
	return s.GetPost(ctx, id, "")
}

// SchedulePost moves the post to `scheduled` with the given publishedAt
func (s *postService) SchedulePost(ctx context.Context, id uuid.UUID, publishedAt time.Time) (Post, error) {
	if publishedAt.Before(time.Now()) {
		return Post{}, ErrScheduledInPast
	}
	if _, err := s.repo.SetPostStatus(ctx, id, PostStatusScheduled, &publishedAt); err != nil {
		return Post{}, err
	}
	return s.GetPost(ctx, id, "")
}

// publishedAt is preserved through archiving
func (s *postService) ArchivePost(ctx context.Context, id uuid.UUID) (Post, error) {
	current, err := s.repo.GetPostByID(ctx, id)
	if err != nil {
		return Post{}, err
	}
	if _, err := s.repo.SetPostStatus(ctx, id, PostStatusArchived, current.PublishedAt); err != nil {
		return Post{}, err
	}
	if s.bus != nil && current.Status == PostStatusPublished {
		s.bus.Publish(ctx, &PostUnpublished{
			PostID:    current.ID,
			Slug:      current.Slug,
			NewStatus: PostStatusArchived,
		})
	}
	return s.GetPost(ctx, id, "")
}

// publishedAt is cleared so the next publish stamps `now`
func (s *postService) UnpublishPost(ctx context.Context, id uuid.UUID) (Post, error) {
	current, err := s.repo.GetPostByID(ctx, id)
	if err != nil {
		return Post{}, err
	}
	if _, err := s.repo.SetPostStatus(ctx, id, PostStatusDraft, nil); err != nil {
		return Post{}, err
	}
	if s.bus != nil && current.Status == PostStatusPublished {
		s.bus.Publish(ctx, &PostUnpublished{
			PostID:    current.ID,
			Slug:      current.Slug,
			NewStatus: PostStatusDraft,
		})
	}
	return s.GetPost(ctx, id, "")
}

func (s *postService) DeletePost(ctx context.Context, id uuid.UUID) error {
	// Snapshot slug before delete so the event carries it
	var slug string
	if p, err := s.repo.GetPostByID(ctx, id); err == nil {
		slug = p.Slug
	}
	if err := s.repo.DeletePost(ctx, id); err != nil {
		return err
	}
	if s.urlRewrites != nil {
		_ = s.urlRewrites.DeleteAllForEntity(ctx, entityPost, id)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PostDeleted{PostID: id, Slug: slug})
	}
	return nil
}

func (s *postService) UpsertPostContent(ctx context.Context, postID uuid.UUID, in PostContentInput) (Post, error) {
	if strings.TrimSpace(in.Locale) == "" {
		return Post{}, fmt.Errorf("locale is required")
	}
	if strings.TrimSpace(in.Title) == "" {
		return Post{}, fmt.Errorf("title is required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return Post{}, err
		}
	}
	if _, err := s.repo.GetPostByID(ctx, postID); err != nil {
		return Post{}, err
	}
	if _, err := s.repo.UpsertPostContent(ctx, postID, in); err != nil {
		return Post{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PostContentChanged{PostID: postID, Locale: in.Locale})
	}
	return s.GetPost(ctx, postID, in.Locale)
}

func (s *postService) DeletePostContent(ctx context.Context, postID uuid.UUID, locale string) error {
	if def := s.resolveDefaultLocale(ctx); def != "" && locale == def {
		all, err := s.repo.ListPostContent(ctx, postID)
		if err != nil {
			return err
		}
		for _, c := range all {
			if c.Locale != def {
				return errors.New("cannot delete default-locale content while translations exist")
			}
		}
	}
	if err := s.repo.DeletePostContent(ctx, postID, locale); err != nil {
		return err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PostContentChanged{PostID: postID, Locale: locale})
	}
	return nil
}

// SetPostCategories replaces a post's categories with the supplied list, in order
func (s *postService) SetPostCategories(ctx context.Context, postID uuid.UUID, categoryIDs []uuid.UUID) (Post, error) {
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		if err := r.ClearPostCategories(ctx, postID); err != nil {
			return err
		}
		for i, cid := range categoryIDs {
			if err := r.AddPostCategory(ctx, postID, cid, int32(i)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return Post{}, err
	}
	return s.GetPost(ctx, postID, "")
}

// SetPostTags replaces a post's tags with the supplied list
func (s *postService) SetPostTags(ctx context.Context, postID uuid.UUID, tagIDs []uuid.UUID) (Post, error) {
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		if err := r.ClearPostTags(ctx, postID); err != nil {
			return err
		}
		for _, tid := range tagIDs {
			if err := r.AddPostTag(ctx, postID, tid); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return Post{}, err
	}
	return s.GetPost(ctx, postID, "")
}

func (s *core) fillPostContent(ctx context.Context, p *Post, locale string) {
	if p == nil || locale == "" {
		return
	}
	for i := range p.AllContent {
		if p.AllContent[i].Locale == locale {
			cp := p.AllContent[i]
			p.Content = &cp
			return
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		for i := range p.AllContent {
			if p.AllContent[i].Locale == def {
				cp := p.AllContent[i]
				p.Content = &cp
				return
			}
		}
	}
}

func (s *postService) PromoteDuePosts(ctx context.Context) (int, error) { // flips every scheduled post whose publishedAt has passed to status=`published`, returning the count
	due, err := s.repo.ListDueScheduledPosts(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, p := range due {
		// A vetoed post stays scheduled and gets asked again next run
		if s.bus != nil {
			if err := s.bus.PublishSync(ctx, &PostPublishing{PostID: p.ID, Slug: p.Slug, AuthorID: p.AuthorID}); err != nil {
				log.Printf("[blog] promote post %s: publishing hook refused: %v", p.ID, err)
				continue
			}
		}
		if _, err := s.repo.SetPostStatus(ctx, p.ID, PostStatusPublished, p.PublishedAt); err != nil {
			log.Printf("[blog] promote post %s: %v", p.ID, err)
			continue
		}
		count++
		if s.bus != nil {
			s.bus.Publish(ctx, &PostPublished{
				PostID:   p.ID,
				Slug:     p.Slug,
				AuthorID: p.AuthorID,
			})
		}
	}
	return count, nil
}

// ListPostTranslations returns every locale's content row for each post, in one query - list reads only carry the resolved locale, so the admin allContent field reads the full set here
func (s *postService) ListPostTranslations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]PostContent, error) {
	return s.repo.ListPostContentFor(ctx, ids)
}
