package cms

import (
	"context"
	"database/sql"
	"strings"

	"github.com/google/uuid"

	contentpkg "github.com/skeletg/skelet/apps/backend/core/internal/content"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Only what more than one sub-service reaches through *core
type coreStore interface {
	ListBlockTypes(ctx context.Context) ([]BlockType, error)
	WithTx(t *sql.Tx) *postgresRepository
}

func (s *core) txRepo(t *sql.Tx) *postgresRepository { return s.repo.WithTx(t) }

func (s *core) announceChanged(ctx context.Context, p Page, oldPaths ...string) {
	if s.bus == nil || !p.IsPublished {
		return
	}
	s.bus.Publish(ctx, &PageChanged{PageID: p.ID, Paths: oldPaths})
}

type LocaleValidator interface {
	EnsureExists(ctx context.Context, code string) error
}

// Resolves the global default content locale - content reads fall back to it when the requested locale has no row
type DefaultLocaleProvider interface {
	DefaultLocale(ctx context.Context) (string, error)
}

// UrlRewriteSyncer is the slim view of urlrewrite.Service the CMS uses to
// keep canonical entries and 301 redirects in sync on slug/path changes
type UrlRewriteSyncer interface {
	SyncCanonical(ctx context.Context, entityType string, entityID uuid.UUID, requestPath string) error
	CreateRedirect(ctx context.Context, from string, entityType string, entityID uuid.UUID, redirectType int) error
	DeleteAllForEntity(ctx context.Context, entityType string, entityID uuid.UUID) error
	// Whether requestPath is already owned by an entity OTHER than (entityType, entityID) - pass uuid.Nil on create, where any existing owner conflicts
	// Lets the CMS reject a colliding slug with a friendly error before writing a row the globally-unique request_path constraint would refuse anyway
	PathConflict(ctx context.Context, requestPath, entityType string, entityID uuid.UUID) (bool, error)
	// Channel-scoped variants, nil channelID for global - per-type root slugs give a page distinct canonical URLs per channel, and these write and clean them
	SyncCanonicalForChannel(ctx context.Context, entityType string, entityID uuid.UUID, requestPath string, channelID *uuid.UUID) error
	CreateRedirectForChannel(ctx context.Context, from string, entityType string, entityID uuid.UUID, redirectType int, channelID *uuid.UUID) error
	PathConflictForChannel(ctx context.Context, requestPath, entityType string, entityID uuid.UUID, channelID *uuid.UUID) (bool, error)
	DeleteCanonicalForChannel(ctx context.Context, entityType string, entityID uuid.UUID, channelID *uuid.UUID) error
}

const entityCMSPage = "cms_page"

// Dependencies and cross-cutting helpers shared by every cms sub-service
// Sub-services embed *core, and the facade Service embeds *core plus every sub-service, so cmsSvc.X() call sites stay unchanged through method promotion
type core struct {
	repo          coreStore
	tx            *txrunner.Manager
	locales       LocaleValidator
	defaultLocale DefaultLocaleProvider
	urlRewrites   UrlRewriteSyncer
	bus           *events.Bus
	cfg           ConfigReader
}

// ConfigReader is the slice of the config service the CMS needs (the editorial
// workflow's require-approval gate) - Nil = feature off (free publish)
type ConfigReader interface {
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
}

// cfgRequireApproval gates publishing on workflow approval when true
const cfgRequireApproval = "cms/workflow/require_approval"

// Per-entity sub-services, each embedding *core - none calls another, and the only shared methods live on *core, so there's no embedding cycle
// Each repo field shadows core's, so a sub-service can only reach its own store
type (
	attributeService struct { // attributes, values, groups, page-type assignment
		*core
		repo attributeStore
	}
	pageTypeService struct {
		*core
		repo pageTypeStore
	}
	pageService struct { // pages, their content, revisions and store listings
		*core
		repo pageStore
	}
	pageWorkflowService struct {
		*core
		repo pageWorkflowStore
	}
	blockService struct { // snippets and custom block types
		*core
		repo blockStore
	}
	tagService struct {
		*core
		repo tagStore
	}
)

// The facade - it embeds *core directly so core's fields resolve at depth 1, unambiguously over the copies reached through each sub-service
// So a bare s.repo on *Service doesn't compile - facade methods delegate
type Service struct {
	*core
	*attributeService
	*pageTypeService
	*pageService
	*pageWorkflowService
	*blockService
	*tagService
}

// id to title for the CMS link picker - best-effort, so a missing id or an error yields no entry and the caller falls back to the path
func (s *pageService) PageTitlesByIDs(ctx context.Context, ids []uuid.UUID, locale string) map[uuid.UUID]string {
	out := make(map[uuid.UUID]string)
	if m, err := s.repo.ListPageContentByScope(ctx, ids, locale); err == nil {
		for id, c := range m {
			out[id] = c.Title
		}
	}
	return out
}

// All collaborators are wired at construction rather than through setters, since none are cyclic - that removes the half-built window where a forgotten SetEventBus would silently stop emitting events
// Locales, DefaultLocale, URLRewrites and Bus may be nil; with a nil DefaultLocale, page reads return null Content when the requested locale has no row
type Deps struct {
	Repo          *postgresRepository
	Tx            *txrunner.Manager
	Locales       LocaleValidator
	DefaultLocale DefaultLocaleProvider
	URLRewrites   UrlRewriteSyncer
	Bus           *events.Bus
	Config        ConfigReader
}

func NewService(d Deps) *Service {
	c := &core{
		repo:          d.Repo,
		tx:            d.Tx,
		locales:       d.Locales,
		defaultLocale: d.DefaultLocale,
		urlRewrites:   d.URLRewrites,
		bus:           d.Bus,
		cfg:           d.Config,
	}
	return &Service{
		core:                c,
		attributeService:    &attributeService{core: c, repo: d.Repo},
		pageTypeService:     &pageTypeService{core: c, repo: d.Repo},
		pageService:         &pageService{core: c, repo: d.Repo},
		pageWorkflowService: &pageWorkflowService{core: c, repo: d.Repo},
		blockService:        &blockService{core: c, repo: d.Repo},
		tagService:          &tagService{core: c, repo: d.Repo},
	}
}

// resolveDefaultLocale returns the configured global default locale code,
// or "" when no provider is wired or the lookup fails
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

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func adminPtr(ctx context.Context) *uuid.UUID {
	if claims, ok := auth.AdminFromContext(ctx); ok {
		id := claims.AdminID
		return &id
	}
	return nil
}

func validateSlug(slug string) error {
	return contentpkg.ValidateSlug(slug, ErrInvalidSlug)
}

// Validates a possibly multi-segment URL root like "help" or "help/articles" - each "/"-separated segment must be a valid slug
func validateSlugPath(path string) error {
	for seg := range strings.SplitSeq(path, "/") {
		if err := validateSlug(seg); err != nil {
			return err
		}
	}
	return nil
}
