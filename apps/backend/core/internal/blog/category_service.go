package blog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type categoryStore interface {
	DeleteCategory(ctx context.Context, id uuid.UUID) error
	DeleteCategoryContent(ctx context.Context, categoryID uuid.UUID, locale string) error
	GetCategoryByID(ctx context.Context, id uuid.UUID) (Category, error)
	GetCategoryBySlug(ctx context.Context, slug string) (Category, error)
	ListAllCategories(ctx context.Context) ([]Category, error)
	ListCategoryChildren(ctx context.Context, parentID uuid.UUID) ([]Category, error)
	ListCategoryContentByScope(ctx context.Context, categoryIDs []uuid.UUID, locale string) (map[uuid.UUID]CategoryContent, error)
	ListCategoryContentFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]CategoryContent, error)
	UpdateCategory(ctx context.Context, in UpdateCategoryInput) (Category, error)
	UpdateCategoryURLPath(ctx context.Context, id uuid.UUID, urlPath string) error
	UpsertCategoryContent(ctx context.Context, categoryID uuid.UUID, in CategoryContentInput) (CategoryContent, error)
	WithTx(t *sql.Tx) *postgresRepository
}

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

// ListCategoryTranslations returns every locale's content row for each category, in one query - list reads only carry the resolved locale, so the admin allContent field reads the full set here
func (s *categoryService) ListCategoryTranslations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]CategoryContent, error) {
	return s.repo.ListCategoryContentFor(ctx, ids)
}
