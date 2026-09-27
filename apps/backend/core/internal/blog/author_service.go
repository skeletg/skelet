package blog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
)

type authorStore interface {
	DeleteAuthor(ctx context.Context, id uuid.UUID) error
	DeleteAuthorContent(ctx context.Context, authorID uuid.UUID, locale string) error
	GetAuthorByID(ctx context.Context, id uuid.UUID) (Author, error)
	GetAuthorBySlug(ctx context.Context, slug string) (Author, error)
	ListAuthorContentByScope(ctx context.Context, authorIDs []uuid.UUID, locale string) (map[uuid.UUID]AuthorContent, error)
	ListAuthorContentFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]AuthorContent, error)
	ListAuthors(ctx context.Context) ([]Author, error)
	UpdateAuthor(ctx context.Context, in UpdateAuthorInput) (Author, error)
	UpsertAuthorContent(ctx context.Context, authorID uuid.UUID, in AuthorContentInput) (AuthorContent, error)
	WithTx(t *sql.Tx) *postgresRepository
}

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

// ListAuthorTranslations returns every locale's content row for each author, in one query - list reads only carry the resolved locale, so the admin allContent field reads the full set here
func (s *authorService) ListAuthorTranslations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]AuthorContent, error) {
	return s.repo.ListAuthorContentFor(ctx, ids)
}
