package blog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
)

type tagStore interface {
	DeleteTag(ctx context.Context, id uuid.UUID) error
	DeleteTagContent(ctx context.Context, tagID uuid.UUID, locale string) error
	GetTagByID(ctx context.Context, id uuid.UUID) (Tag, error)
	GetTagBySlug(ctx context.Context, slug string) (Tag, error)
	ListTagContentFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]TagContent, error)
	ListTags(ctx context.Context) ([]Tag, error)
	UpdateTag(ctx context.Context, in UpdateTagInput) (Tag, error)
	UpsertTagContent(ctx context.Context, tagID uuid.UUID, in TagContentInput) (TagContent, error)
	WithTx(t *sql.Tx) *postgresRepository
}

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

// ListTagTranslations returns every locale's content row for each tag, in one query - list reads only carry the resolved locale, so the admin allContent field reads the full set here
func (s *tagService) ListTagTranslations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]TagContent, error) {
	return s.repo.ListTagContentFor(ctx, ids)
}
