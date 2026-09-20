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

	contentpkg "github.com/skeletg/skelet/apps/backend/core/internal/content"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type LocaleValidator interface { // optional - nil skips the locale-exists pre-check
	EnsureExists(ctx context.Context, code string) error
}

// Resolves the `general/locale/default` config - optional, and when nil content reads don't fall back across locales and writes don't enforce default-row invariants
type DefaultLocaleProvider interface {
	DefaultLocale(ctx context.Context) (string, error)
}

type UrlRewriteSyncer interface { // slim view of urlrewrite.Service
	SyncCanonical(ctx context.Context, entityType string, entityID uuid.UUID, requestPath string) error
	CreateRedirect(ctx context.Context, from string, entityType string, entityID uuid.UUID, redirectType int) error
	DeleteAllForEntity(ctx context.Context, entityType string, entityID uuid.UUID) error
}

const (
	entityPost     = "blog_post"
	entityCategory = "blog_category"
	entityTag      = "blog_tag"
	entityAuthor   = "blog_author"
)

// Deps + cross-cutting helpers shared by every sub-service - the facade Service embeds *core AND every sub-service, so method promotion keeps blogSvc.X() call sites unchanged
type core struct {
	repo          *postgresRepository
	tx            *txrunner.Manager
	locales       LocaleValidator
	defaultLocale DefaultLocaleProvider
	urlRewrites   UrlRewriteSyncer
	bus           *events.Bus
}

// No sub-service calls another - the only shared method, resolveDefaultLocale, lives on *core, so there is no embedding cycle
type (
	authorService   struct{ *core }
	categoryService struct{ *core }
	tagService      struct{ *core }
	postService     struct{ *core }
)

// *core is embedded directly so its fields/methods resolve at depth 1, unambiguously over the copies reached via each sub-service
type Service struct {
	*core
	*authorService
	*categoryService
	*tagService
	*postService
}

// Best-effort for the CMS link picker: missing ids/errors yield no entry, and tags/authors have no content-by-scope batch so the caller falls back to the path for those
func (s *Service) LinkLabelsByIDs(ctx context.Context, entityType string, ids []uuid.UUID, locale string) map[uuid.UUID]string {
	out := make(map[uuid.UUID]string)
	switch entityType {
	case "blog_post":
		if m, err := s.repo.ListPostContentByScope(ctx, ids, locale); err == nil {
			for id, c := range m {
				out[id] = c.Title
			}
		}
	case "blog_category":
		if m, err := s.repo.ListCategoryContentByScope(ctx, ids, locale); err == nil {
			for id, c := range m {
				out[id] = c.Name
			}
		}
	}
	return out
}

// All wired at construction rather than via setters - that removes the half-built window where a forgotten SetEventBus would silently stop emitting events
type Deps struct {
	Repo          *postgresRepository
	Tx            *txrunner.Manager
	Locales       LocaleValidator // nil-able, like the three below - the reindex CLI wires only what it needs
	DefaultLocale DefaultLocaleProvider
	URLRewrites   UrlRewriteSyncer
	Bus           *events.Bus
}

func NewService(d Deps) *Service {
	c := &core{
		repo:          d.Repo,
		tx:            d.Tx,
		locales:       d.Locales,
		defaultLocale: d.DefaultLocale,
		urlRewrites:   d.URLRewrites,
		bus:           d.Bus,
	}
	return &Service{
		core:            c,
		authorService:   &authorService{c},
		categoryService: &categoryService{c},
		tagService:      &tagService{c},
		postService:     &postService{c},
	}
}

func (s *core) resolveDefaultLocale(ctx context.Context) string {
	if s.defaultLocale == nil {
		return ""
	}
	code, err := s.defaultLocale.DefaultLocale(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(code)
}

// ==================== Authors ====================

func (s *authorService) CreateAuthor(ctx context.Context, in CreateAuthorInput) (Author, error) {
	if err := validateSlug(in.Slug); err != nil {
		return Author{}, err
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return Author{}, fmt.Errorf("display_name is required")
	}
	hasContent := strings.TrimSpace(in.Locale) != ""
	if hasContent {
		if strings.TrimSpace(in.Name) == "" {
			return Author{}, fmt.Errorf("name is required for initial content")
		}
		if s.locales != nil {
			if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
				return Author{}, err
			}
		}
	}

	var created Author
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		a, err := r.CreateAuthor(ctx, in)
		if err != nil {
			return err
		}
		if hasContent {
			if _, err := r.UpsertAuthorContent(ctx, a.ID, AuthorContentInput{
				Locale: in.Locale,
				Name:   in.Name,
				Bio:    in.Bio,
			}); err != nil {
				return err
			}
		}
		created = a
		return nil
	}); err != nil {
		return Author{}, err
	}

	if s.urlRewrites != nil {
		_ = s.urlRewrites.SyncCanonical(ctx, entityAuthor, created.ID, "blog/author/"+created.Slug)
	}

	return s.GetAuthor(ctx, created.ID, "")
}

func (s *authorService) GetAuthor(ctx context.Context, id uuid.UUID, locale string) (Author, error) {
	a, err := s.repo.GetAuthorByID(ctx, id)
	if err != nil {
		return Author{}, err
	}
	s.fillAuthorContent(ctx, &a, locale)
	return a, nil
}

func (s *authorService) GetAuthorBySlug(ctx context.Context, slug, locale string) (Author, error) {
	a, err := s.repo.GetAuthorBySlug(ctx, slug)
	if err != nil {
		return Author{}, err
	}
	s.fillAuthorContent(ctx, &a, locale)
	return a, nil
}

func (s *authorService) ListAuthors(ctx context.Context, locale string) ([]Author, error) {
	authors, err := s.repo.ListAuthors(ctx)
	if err != nil {
		return nil, err
	}
	if len(authors) == 0 {
		return authors, nil
	}
	ids := make([]uuid.UUID, len(authors))
	for i, a := range authors {
		ids[i] = a.ID
	}
	if locale != "" {
		batch, err := s.repo.ListAuthorContentByScope(ctx, ids, locale)
		if err != nil {
			return nil, err
		}
		for i, a := range authors {
			if c, ok := batch[a.ID]; ok {
				cp := c
				authors[i].Content = &cp
			}
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		var missing []uuid.UUID
		for _, a := range authors {
			if a.Content == nil {
				missing = append(missing, a.ID)
			}
		}
		if len(missing) > 0 {
			fb, err := s.repo.ListAuthorContentByScope(ctx, missing, def)
			if err != nil {
				return nil, err
			}
			for i, a := range authors {
				if a.Content != nil {
					continue
				}
				if c, ok := fb[a.ID]; ok {
					cp := c
					authors[i].Content = &cp
				}
			}
		}
	}
	return authors, nil
}

func (s *authorService) UpdateAuthor(ctx context.Context, in UpdateAuthorInput) (Author, error) {
	if in.Slug != nil {
		if err := validateSlug(*in.Slug); err != nil {
			return Author{}, err
		}
	}
	current, err := s.repo.GetAuthorByID(ctx, in.ID)
	if err != nil {
		return Author{}, err
	}
	updated, err := s.repo.UpdateAuthor(ctx, in)
	if err != nil {
		return Author{}, err
	}
	if s.urlRewrites != nil && in.Slug != nil && *in.Slug != current.Slug {
		// Canonical FIRST - until it moves, this entity's own row still holds the old path and the redirect insert dies on the unique index, so the rename left no 301 at all. Errors are logged rather than dropped: a missing redirect is an invisible SEO leak otherwise
		if err := s.urlRewrites.SyncCanonical(ctx, entityAuthor, updated.ID, "blog/author/"+updated.Slug); err != nil {
			applog.From(ctx).Error("sync canonical url rewrite", "entity", entityAuthor, "id", updated.ID, "err", err)
		}
		if err := s.urlRewrites.CreateRedirect(ctx, "blog/author/"+current.Slug, entityAuthor, updated.ID, 301); err != nil {
			applog.From(ctx).Error("create rename redirect", "entity", entityAuthor, "id", updated.ID, "err", err)
		}
	}
	return s.GetAuthor(ctx, updated.ID, "")
}

func (s *authorService) DeleteAuthor(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.DeleteAuthor(ctx, id); err != nil {
		return err
	}
	if s.urlRewrites != nil {
		_ = s.urlRewrites.DeleteAllForEntity(ctx, entityAuthor, id)
	}
	return nil
}

func (s *authorService) UpsertAuthorContent(ctx context.Context, authorID uuid.UUID, in AuthorContentInput) (Author, error) {
	if strings.TrimSpace(in.Locale) == "" {
		return Author{}, fmt.Errorf("locale is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return Author{}, fmt.Errorf("name is required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return Author{}, err
		}
	}
	if _, err := s.repo.GetAuthorByID(ctx, authorID); err != nil {
		return Author{}, err
	}
	if _, err := s.repo.UpsertAuthorContent(ctx, authorID, in); err != nil {
		return Author{}, err
	}
	return s.GetAuthor(ctx, authorID, in.Locale)
}

func (s *authorService) DeleteAuthorContent(ctx context.Context, authorID uuid.UUID, locale string) error {
	return s.repo.DeleteAuthorContent(ctx, authorID, locale)
}

func (s *core) fillAuthorContent(ctx context.Context, a *Author, locale string) {
	if a == nil || locale == "" {
		return
	}
	for i := range a.AllContent {
		if a.AllContent[i].Locale == locale {
			cp := a.AllContent[i]
			a.Content = &cp
			return
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		for i := range a.AllContent {
			if a.AllContent[i].Locale == def {
				cp := a.AllContent[i]
				a.Content = &cp
				return
			}
		}
	}
}

// ==================== Categories ====================

func (s *categoryService) CreateCategory(ctx context.Context, in CreateCategoryInput) (Category, error) {
	if err := validateSlug(in.Slug); err != nil {
		return Category{}, err
	}
	if strings.TrimSpace(in.Locale) == "" {
		return Category{}, fmt.Errorf("locale is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return Category{}, fmt.Errorf("name is required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return Category{}, err
		}
	}

	urlPath := "blog/category/" + in.Slug
	if in.ParentID != nil {
		parent, err := s.repo.GetCategoryByID(ctx, *in.ParentID)
		if err != nil {
			return Category{}, fmt.Errorf("get parent category: %w", err)
		}
		urlPath = parent.URLPath + "/" + in.Slug
	}

	var created Category
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		c, err := r.CreateCategory(ctx, in)
		if err != nil {
			return err
		}
		if err := r.UpdateCategoryURLPath(ctx, c.ID, urlPath); err != nil {
			return err
		}
		if _, err := r.UpsertCategoryContent(ctx, c.ID, CategoryContentInput{
			Locale:          in.Locale,
			Name:            in.Name,
			Description:     in.Description,
			MetaTitle:       in.MetaTitle,
			MetaDescription: in.MetaDescription,
			MetaKeywords:    in.MetaKeywords,
		}); err != nil {
			return err
		}
		c.URLPath = urlPath
		created = c
		return nil
	}); err != nil {
		return Category{}, err
	}

	if s.urlRewrites != nil {
		_ = s.urlRewrites.SyncCanonical(ctx, entityCategory, created.ID, urlPath)
	}

	return s.GetCategory(ctx, created.ID, in.Locale)
}

func (s *categoryService) GetCategory(ctx context.Context, id uuid.UUID, locale string) (Category, error) {
	c, err := s.repo.GetCategoryByID(ctx, id)
	if err != nil {
		return Category{}, err
	}
	s.fillCategoryContent(ctx, &c, locale)
	return c, nil
}

func (s *categoryService) GetCategoryBySlug(ctx context.Context, slug, locale string) (Category, error) {
	c, err := s.repo.GetCategoryBySlug(ctx, slug)
	if err != nil {
		return Category{}, err
	}
	s.fillCategoryContent(ctx, &c, locale)
	return c, nil
}

func (s *categoryService) ListRootCategories(ctx context.Context, locale string) ([]Category, error) {
	all, err := s.repo.ListAllCategories(ctx)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return all, nil
	}
	s.fillCategoryListContent(ctx, all, locale)
	return buildCategoryTree(all, nil), nil
}

func (s *categoryService) ListCategoryChildren(ctx context.Context, parentID uuid.UUID, locale string) ([]Category, error) {
	children, err := s.repo.ListCategoryChildren(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if len(children) == 0 {
		return children, nil
	}
	s.fillCategoryListContent(ctx, children, locale)
	return children, nil
}

func (s *categoryService) UpdateCategory(ctx context.Context, in UpdateCategoryInput) (Category, error) {
	if in.Slug != nil {
		if err := validateSlug(*in.Slug); err != nil {
			return Category{}, err
		}
	}
	current, err := s.repo.GetCategoryByID(ctx, in.ID)
	if err != nil {
		return Category{}, err
	}
	oldURLPath := current.URLPath

	updated, err := s.repo.UpdateCategory(ctx, in)
	if err != nil {
		return Category{}, err
	}

	pathChanged := false
	// Recompute url_path on slug change OR parent change
	if (in.Slug != nil && *in.Slug != current.Slug) ||
		(in.ParentID != nil && (current.ParentID == nil || *in.ParentID != *current.ParentID)) ||
		(in.ClearParent && current.ParentID != nil) {
		newURLPath := "blog/category/" + updated.Slug
		if updated.ParentID != nil {
			parent, err := s.repo.GetCategoryByID(ctx, *updated.ParentID)
			if err != nil {
				return Category{}, fmt.Errorf("get parent: %w", err)
			}
			newURLPath = parent.URLPath + "/" + updated.Slug
		}
		if err := s.repo.UpdateCategoryURLPath(ctx, updated.ID, newURLPath); err != nil {
			return Category{}, err
		}
		updated.URLPath = newURLPath
		pathChanged = newURLPath != oldURLPath
		if s.urlRewrites != nil && pathChanged {
			_ = s.urlRewrites.SyncCanonical(ctx, entityCategory, updated.ID, newURLPath)
			_ = s.urlRewrites.CreateRedirect(ctx, oldURLPath, entityCategory, updated.ID, 301)
		}
		if pathChanged {
			if err := s.propagateCategoryURLPaths(ctx, updated.ID, newURLPath); err != nil {
				return Category{}, err
			}
		}
	}

	return s.GetCategory(ctx, updated.ID, "")
}

func (s *categoryService) propagateCategoryURLPaths(ctx context.Context, parentID uuid.UUID, parentURLPath string) error {
	children, err := s.repo.ListCategoryChildren(ctx, parentID)
	if err != nil {
		return err
	}
	for _, ch := range children {
		newPath := parentURLPath + "/" + ch.Slug
		oldPath := ch.URLPath
		if err := s.repo.UpdateCategoryURLPath(ctx, ch.ID, newPath); err != nil {
			return err
		}
		if s.urlRewrites != nil {
			_ = s.urlRewrites.SyncCanonical(ctx, entityCategory, ch.ID, newPath)
			_ = s.urlRewrites.CreateRedirect(ctx, oldPath, entityCategory, ch.ID, 301)
		}
		if err := s.propagateCategoryURLPaths(ctx, ch.ID, newPath); err != nil {
			return err
		}
	}
	return nil
}

func (s *categoryService) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	children, err := s.repo.ListCategoryChildren(ctx, id)
	if err != nil {
		return err
	}
	if len(children) > 0 {
		return ErrCategoryHasChildren
	}
	if err := s.repo.DeleteCategory(ctx, id); err != nil {
		return err
	}
	if s.urlRewrites != nil {
		_ = s.urlRewrites.DeleteAllForEntity(ctx, entityCategory, id)
	}
	return nil
}

func (s *categoryService) UpsertCategoryContent(ctx context.Context, categoryID uuid.UUID, in CategoryContentInput) (Category, error) {
	if strings.TrimSpace(in.Locale) == "" {
		return Category{}, fmt.Errorf("locale is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return Category{}, fmt.Errorf("name is required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return Category{}, err
		}
	}
	if _, err := s.repo.GetCategoryByID(ctx, categoryID); err != nil {
		return Category{}, err
	}
	if _, err := s.repo.UpsertCategoryContent(ctx, categoryID, in); err != nil {
		return Category{}, err
	}
	return s.GetCategory(ctx, categoryID, in.Locale)
}

func (s *categoryService) DeleteCategoryContent(ctx context.Context, categoryID uuid.UUID, locale string) error {
	return s.repo.DeleteCategoryContent(ctx, categoryID, locale)
}

func (s *core) fillCategoryContent(ctx context.Context, c *Category, locale string) {
	if c == nil || locale == "" {
		return
	}
	for i := range c.AllContent {
		if c.AllContent[i].Locale == locale {
			cp := c.AllContent[i]
			c.Content = &cp
			return
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		for i := range c.AllContent {
			if c.AllContent[i].Locale == def {
				cp := c.AllContent[i]
				c.Content = &cp
				return
			}
		}
	}
}

func (s *core) fillCategoryListContent(ctx context.Context, cats []Category, locale string) {
	if len(cats) == 0 {
		return
	}
	ids := make([]uuid.UUID, len(cats))
	for i, c := range cats {
		ids[i] = c.ID
	}
	if locale != "" {
		batch, err := s.repo.ListCategoryContentByScope(ctx, ids, locale)
		if err == nil {
			for i, c := range cats {
				if cc, ok := batch[c.ID]; ok {
					cp := cc
					cats[i].Content = &cp
				}
			}
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		var missing []uuid.UUID
		for _, c := range cats {
			if c.Content == nil {
				missing = append(missing, c.ID)
			}
		}
		if len(missing) > 0 {
			fb, err := s.repo.ListCategoryContentByScope(ctx, missing, def)
			if err == nil {
				for i, c := range cats {
					if c.Content != nil {
						continue
					}
					if cc, ok := fb[c.ID]; ok {
						cp := cc
						cats[i].Content = &cp
					}
				}
			}
		}
	}
}

// Batched translation fill for the categories/tags hanging off a page of posts - without
// it archives rendered raw slugs while every other surface localized
func (s *postService) fillPostTaxonomyContent(ctx context.Context, posts []Post, locale string) {
	var cats []Category
	var tags []Tag
	seenCat := map[uuid.UUID]bool{}
	seenTag := map[uuid.UUID]bool{}
	for _, p := range posts {
		for _, c := range p.Categories {
			if !seenCat[c.ID] {
				seenCat[c.ID] = true
				cats = append(cats, c)
			}
		}
		for _, t := range p.Tags {
			if !seenTag[t.ID] {
				seenTag[t.ID] = true
				tags = append(tags, t)
			}
		}
	}
	s.fillCategoryListContent(ctx, cats, locale)
	s.fillTagListContent(ctx, tags, locale)
	catContent := make(map[uuid.UUID]*CategoryContent, len(cats))
	for i := range cats {
		if cats[i].Content != nil {
			catContent[cats[i].ID] = cats[i].Content
		}
	}
	tagContent := make(map[uuid.UUID]*TagContent, len(tags))
	for i := range tags {
		if tags[i].Content != nil {
			tagContent[tags[i].ID] = tags[i].Content
		}
	}
	for i := range posts {
		for j := range posts[i].Categories {
			if c := catContent[posts[i].Categories[j].ID]; c != nil {
				cp := *c
				posts[i].Categories[j].Content = &cp
			}
		}
		for j := range posts[i].Tags {
			if c := tagContent[posts[i].Tags[j].ID]; c != nil {
				cp := *c
				posts[i].Tags[j].Content = &cp
			}
		}
	}
}

func buildCategoryTree(all []Category, parentID *uuid.UUID) []Category {
	var result []Category
	for _, c := range all {
		match := (parentID == nil && c.ParentID == nil) ||
			(parentID != nil && c.ParentID != nil && *c.ParentID == *parentID)
		if !match {
			continue
		}
		id := c.ID
		c.Children = buildCategoryTree(all, &id)
		result = append(result, c)
	}
	return result
}

// ==================== Tags ====================

func (s *tagService) CreateTag(ctx context.Context, in CreateTagInput) (Tag, error) {
	if err := validateSlug(in.Slug); err != nil {
		return Tag{}, err
	}
	hasContent := strings.TrimSpace(in.Locale) != ""
	if hasContent {
		if strings.TrimSpace(in.Name) == "" {
			return Tag{}, fmt.Errorf("name is required for initial content")
		}
		if s.locales != nil {
			if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
				return Tag{}, err
			}
		}
	}

	var created Tag
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		t2, err := r.CreateTag(ctx, in)
		if err != nil {
			return err
		}
		if hasContent {
			if _, err := r.UpsertTagContent(ctx, t2.ID, TagContentInput{
				Locale:          in.Locale,
				Name:            in.Name,
				Description:     in.Description,
				MetaTitle:       in.MetaTitle,
				MetaDescription: in.MetaDescription,
				MetaKeywords:    in.MetaKeywords,
			}); err != nil {
				return err
			}
		}
		created = t2
		return nil
	}); err != nil {
		return Tag{}, err
	}

	if s.urlRewrites != nil {
		_ = s.urlRewrites.SyncCanonical(ctx, entityTag, created.ID, "blog/tag/"+created.Slug)
	}

	return s.GetTag(ctx, created.ID, in.Locale)
}

func (s *tagService) GetTag(ctx context.Context, id uuid.UUID, locale string) (Tag, error) {
	t, err := s.repo.GetTagByID(ctx, id)
	if err != nil {
		return Tag{}, err
	}
	s.fillTagContent(ctx, &t, locale)
	return t, nil
}

func (s *tagService) GetTagBySlug(ctx context.Context, slug, locale string) (Tag, error) {
	t, err := s.repo.GetTagBySlug(ctx, slug)
	if err != nil {
		return Tag{}, err
	}
	s.fillTagContent(ctx, &t, locale)
	return t, nil
}

func (s *tagService) ListTags(ctx context.Context, locale string) ([]Tag, error) {
	tags, err := s.repo.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	s.fillTagListContent(ctx, tags, locale)
	return tags, nil
}

func (s *tagService) UpdateTag(ctx context.Context, in UpdateTagInput) (Tag, error) {
	if in.Slug != nil {
		if err := validateSlug(*in.Slug); err != nil {
			return Tag{}, err
		}
	}
	current, err := s.repo.GetTagByID(ctx, in.ID)
	if err != nil {
		return Tag{}, err
	}
	updated, err := s.repo.UpdateTag(ctx, in)
	if err != nil {
		return Tag{}, err
	}
	if s.urlRewrites != nil && in.Slug != nil && *in.Slug != current.Slug {
		if err := s.urlRewrites.SyncCanonical(ctx, entityTag, updated.ID, "blog/tag/"+updated.Slug); err != nil {
			applog.From(ctx).Error("sync canonical url rewrite", "entity", entityTag, "id", updated.ID, "err", err)
		}
		if err := s.urlRewrites.CreateRedirect(ctx, "blog/tag/"+current.Slug, entityTag, updated.ID, 301); err != nil {
			applog.From(ctx).Error("create rename redirect", "entity", entityTag, "id", updated.ID, "err", err)
		}
	}
	return s.GetTag(ctx, updated.ID, "")
}

func (s *tagService) DeleteTag(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.DeleteTag(ctx, id); err != nil {
		return err
	}
	if s.urlRewrites != nil {
		_ = s.urlRewrites.DeleteAllForEntity(ctx, entityTag, id)
	}
	return nil
}

func (s *tagService) UpsertTagContent(ctx context.Context, tagID uuid.UUID, in TagContentInput) (Tag, error) {
	if strings.TrimSpace(in.Locale) == "" {
		return Tag{}, fmt.Errorf("locale is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return Tag{}, fmt.Errorf("name is required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return Tag{}, err
		}
	}
	if _, err := s.repo.GetTagByID(ctx, tagID); err != nil {
		return Tag{}, err
	}
	if _, err := s.repo.UpsertTagContent(ctx, tagID, in); err != nil {
		return Tag{}, err
	}
	return s.GetTag(ctx, tagID, in.Locale)
}

func (s *tagService) DeleteTagContent(ctx context.Context, tagID uuid.UUID, locale string) error {
	return s.repo.DeleteTagContent(ctx, tagID, locale)
}

// The tag twin of fillCategoryListContent - locale first, default-locale fallback for the
// rest, both batched
func (s *core) fillTagListContent(ctx context.Context, tags []Tag, locale string) {
	if len(tags) == 0 {
		return
	}
	ids := make([]uuid.UUID, len(tags))
	for i, t := range tags {
		ids[i] = t.ID
	}
	if locale != "" {
		batch, err := s.repo.ListTagContentByScope(ctx, ids, locale)
		if err == nil {
			for i, t := range tags {
				if c, ok := batch[t.ID]; ok {
					cp := c
					tags[i].Content = &cp
				}
			}
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		var missing []uuid.UUID
		for _, t := range tags {
			if t.Content == nil {
				missing = append(missing, t.ID)
			}
		}
		if len(missing) > 0 {
			fb, err := s.repo.ListTagContentByScope(ctx, missing, def)
			if err == nil {
				for i, t := range tags {
					if t.Content != nil {
						continue
					}
					if c, ok := fb[t.ID]; ok {
						cp := c
						tags[i].Content = &cp
					}
				}
			}
		}
	}
}

func (s *core) fillTagContent(ctx context.Context, t *Tag, locale string) {
	if t == nil || locale == "" {
		return
	}
	for i := range t.AllContent {
		if t.AllContent[i].Locale == locale {
			cp := t.AllContent[i]
			t.Content = &cp
			return
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		for i := range t.AllContent {
			if t.AllContent[i].Locale == def {
				cp := t.AllContent[i]
				t.Content = &cp
				return
			}
		}
	}
}

// ==================== Posts ====================

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

// ==================== Store listings ====================

// SetPostChannelListing creates or updates a per-store availability row for a post - Combined with the post's global
// status, this gates whether storefront callers see the post on a given store
func (s *postService) SetPostChannelListing(ctx context.Context, in SetPostChannelListingInput) (Post, error) {
	if _, err := s.repo.GetPostByID(ctx, in.PostID); err != nil {
		return Post{}, err
	}
	listing, err := s.repo.UpsertPostChannelListing(ctx, in)
	if err != nil {
		return Post{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PostListingChanged{
			PostID:      in.PostID,
			ChannelID:   in.ChannelID,
			IsPublished: listing.IsPublished,
			Cleared:     false,
		})
	}
	return s.GetPost(ctx, in.PostID, "")
}

func (s *postService) DeletePostChannelListing(ctx context.Context, postID, channelID uuid.UUID) (Post, error) {
	if err := s.repo.DeletePostChannelListing(ctx, postID, channelID); err != nil {
		return Post{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PostListingChanged{
			PostID:      postID,
			ChannelID:   channelID,
			IsPublished: false,
			Cleared:     true,
		})
	}
	return s.GetPost(ctx, postID, "")
}

// ListPostChannelListings returns the per-channel listing rows for a post - Exposed so resolvers can gate
// localized-content mutations against the post's current listing channel set (auth RequireAllListedChannelsPermission)
func (s *postService) ListPostChannelListings(ctx context.Context, postID uuid.UUID) ([]PostChannelListing, error) {
	return s.repo.ListPostChannelListings(ctx, postID)
}

// GetPublishedPostBySlugForChannel returns a post only if it is globally published and listed on the given store with
// is_published=true - Used by storefront resolvers and feed/sitemap handlers
func (s *postService) GetPublishedPostBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID, locale string) (Post, error) {
	p, err := s.repo.GetPublishedPostBySlugForChannel(ctx, slug, channelID)
	if err != nil {
		return Post{}, err
	}
	s.fillPostContent(ctx, &p, locale)
	if p.AuthorID != nil {
		if a, err := s.repo.GetAuthorByID(ctx, *p.AuthorID); err == nil {
			s.fillAuthorContent(ctx, &a, locale)
			p.Author = &a
		}
	}
	return p, nil
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

// ==================== helpers ====================

// validateSlug allows lowercase letters, digits, hyphens, underscores, dots
// Matches the catalog domain's slug rules
func validateSlug(slug string) error {
	return contentpkg.ValidateSlug(slug, ErrInvalidSlug)
}

func (s *Service) PromoteDuePosts(ctx context.Context) (int, error) { // flips every scheduled post whose publishedAt has passed to status=`published`, returning the count
	due, err := s.repo.ListDueScheduledPosts(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, p := range due {
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
