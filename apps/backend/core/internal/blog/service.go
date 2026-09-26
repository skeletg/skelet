package blog

import (
	"context"
	"strings"

	"github.com/google/uuid"

	contentpkg "github.com/skeletg/skelet/apps/backend/core/internal/content"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Only what more than one sub-service reaches through *core
type coreStore interface {
	ListCategoryContentByScope(ctx context.Context, categoryIDs []uuid.UUID, locale string) (map[uuid.UUID]CategoryContent, error)
	ListTagContentByScope(ctx context.Context, tagIDs []uuid.UUID, locale string) (map[uuid.UUID]TagContent, error)
}

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
	repo          coreStore
	tx            *txrunner.Manager
	locales       LocaleValidator
	defaultLocale DefaultLocaleProvider
	urlRewrites   UrlRewriteSyncer
	bus           *events.Bus
}

// No sub-service calls another - the shared methods live on *core, so there is no embedding cycle
// Each repo field shadows core's, so a sub-service can only reach its own store
type (
	authorService struct {
		*core
		repo authorStore
	}
	categoryService struct {
		*core
		repo categoryStore
	}
	tagService struct {
		*core
		repo tagStore
	}
	postService struct {
		*core
		repo postStore
	}
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
		if m, err := s.postService.repo.ListPostContentByScope(ctx, ids, locale); err == nil {
			for id, c := range m {
				out[id] = c.Title
			}
		}
	case "blog_category":
		if m, err := s.categoryService.repo.ListCategoryContentByScope(ctx, ids, locale); err == nil {
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
		authorService:   &authorService{core: c, repo: d.Repo},
		categoryService: &categoryService{core: c, repo: d.Repo},
		tagService:      &tagService{core: c, repo: d.Repo},
		postService:     &postService{core: c, repo: d.Repo},
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

// validateSlug allows lowercase letters, digits, hyphens, underscores, dots
// Matches the catalog domain's slug rules
func validateSlug(slug string) error {
	return contentpkg.ValidateSlug(slug, ErrInvalidSlug)
}
