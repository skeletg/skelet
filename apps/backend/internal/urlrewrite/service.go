package urlrewrite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/cache"
	"github.com/skeletg/skelet/apps/backend/internal/events"
)

type Service struct {
	repo  Repository
	bus   *events.Bus
	cache *cache.Cache
}

// 60 min is long because reads (every PDP / PLP catch-all hit) dwarf writes - any admin write drops the affected path keys immediately, so the TTL only matters for orphans
const resolveCacheTTL = 60 * time.Minute

// Keeps "404, no rewrite" out of Postgres too, since typo URLs and bots otherwise hammer the repo - shorter than the hit TTL so a fresh rewrite shows up without operator invalidation
const resolveNotFoundCacheTTL = 5 * time.Minute

// The sentinel stored at a path that resolved to nothing, so Get can tell "not cached" from "cached negative"
type negativeMarker struct {
	NotFound bool `json:"not_found"`
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetEventBus(b *events.Bus) { s.bus = b }

// Optional - nil disables caching and falls through to Postgres
func (s *Service) SetCache(c *cache.Cache) { s.cache = c }

// Keyed by the LOOKUP scope (the requesting channel) rather than the resolved row's, so a per-channel override and the global default for one path don't collide
// The PATH comes first so every scope for one path shares a prefix - that is what lets a global write
// invalidate the per-channel keys too, with a pattern that can't spill onto a neighbouring path
// global path:<path>:g, channel path:<path>:c:<channelID>
func cacheKey(channelID *uuid.UUID, path string) string {
	if channelID == nil {
		return "path:" + path + ":g"
	}
	return "path:" + path + ":c:" + channelID.String()
}

// Redis glob metacharacters in a stored path would make the invalidation pattern match the wrong keys
var cachePatternEscaper = strings.NewReplacer("*", "\\*", "?", "\\?", "[", "\\[", "]", "\\]", "\\", "\\\\")

// Called after any write that touches a rewrite's request_path - nil channel means global
// Storefront traffic always resolves WITH a channel, so a global write has to drop the per-channel
// keys as well: dropping only the global one left every channel serving the pre-rename answer for
// up to the full hit TTL, with no admin surface to clear it
func (s *Service) invalidatePath(ctx context.Context, path string, channelID *uuid.UUID) {
	if s.cache == nil || path == "" {
		return
	}
	if channelID != nil {
		// A channel-scoped row only shadows that one channel's lookups
		s.cache.Del(ctx, cacheKey(channelID, path))
		return
	}
	s.cache.DelPattern(ctx, "path:"+cachePatternEscaper.Replace(path)+":*")
}

func (s *Service) WithTx(tx *sql.Tx) *Service {
	// Publishes to the parent's bus, so events fire after the outer tx commits as long as callers respect the publish-after-commit convention
	return &Service{repo: s.repo.WithTx(tx), bus: s.bus, cache: s.cache}
}

func (s *Service) publishChanged(ctx context.Context, rw UrlRewrite, deleted bool) {
	// Every mutation that inserts or updates a rewrite routes through here, so the cache stays consistent without each writer remembering to invalidate
	s.invalidatePath(ctx, rw.RequestPath, rw.ChannelID)
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, &Changed{
		ID:         rw.ID,
		Path:       rw.RequestPath,
		EntityType: string(rw.EntityType),
		EntityID:   rw.EntityID,
		ChannelID:  rw.ChannelID,
		Deleted:    deleted,
	})
}

func (s *Service) ResolveUrl(ctx context.Context, path string) (UrlRewrite, error) {
	return s.ResolveURLForChannel(ctx, path, nil)
}

// A channel-specific row wins over the global default for the same path - pass nil for pure-global, and results cache per lookup scope
func (s *Service) ResolveURLForChannel(ctx context.Context, path string, channelID *uuid.UUID) (UrlRewrite, error) {
	// Cached negatives surface as ErrNotFound so the catch-all route sees the shape it always did - bots and typos would otherwise hit Postgres on every random URL
	type cached struct {
		Rewrite  UrlRewrite      `json:"rewrite"`
		Negative *negativeMarker `json:"negative,omitempty"`
	}
	key := cacheKey(channelID, path)
	if hit, ok := cache.GetJSON[cached](ctx, s.cache, key); ok {
		if hit.Negative != nil && hit.Negative.NotFound {
			return UrlRewrite{}, ErrNotFound
		}
		return hit.Rewrite, nil
	}
	rw, err := s.repo.GetByRequestPath(ctx, path, channelID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.cache.SetJSON(ctx, key,
				cached{Negative: &negativeMarker{NotFound: true}},
				resolveNotFoundCacheTTL,
			)
		}
		return UrlRewrite{}, err
	}
	s.cache.SetJSON(ctx, key, cached{Rewrite: rw}, resolveCacheTTL)
	return rw, nil
}

// Google's per-file limit is 50k URLs, so callers never legitimately need more
const sitemapMaxPage = 50000

func (s *Service) SitemapEntries(ctx context.Context, entityType EntityType, channelID uuid.UUID, limit, offset int32) ([]SitemapEntry, error) {
	if limit <= 0 || limit > sitemapMaxPage {
		limit = sitemapMaxPage
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.SitemapEntries(ctx, entityType, channelID, limit, offset)
}

func (s *Service) SitemapCounts(ctx context.Context, channelID uuid.UUID) (products, categories, collections int64, err error) {
	if products, err = s.repo.SitemapCount(ctx, EntityTypeProduct, channelID); err != nil {
		return 0, 0, 0, err
	}
	if categories, err = s.repo.SitemapCount(ctx, EntityTypeCategory, channelID); err != nil {
		return 0, 0, 0, err
	}
	if collections, err = s.repo.SitemapCount(ctx, EntityTypeCollection, channelID); err != nil {
		return 0, 0, 0, err
	}
	return products, categories, collections, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (UrlRewrite, error) {
	return s.repo.GetByID(ctx, id)
}

// GLOBAL scope - a wrapper over the channel form
func (s *Service) PathConflict(ctx context.Context, requestPath, entityType string, entityID uuid.UUID) (bool, error) {
	return s.PathConflictForChannel(ctx, requestPath, entityType, entityID, nil)
}

// Scope-exact: when writing a channel row, a global row at the same path is NOT a conflict, since the override shadows it
// Pass uuid.Nil for entityID on create
func (s *Service) PathConflictForChannel(ctx context.Context, requestPath, entityType string, entityID uuid.UUID, channelID *uuid.UUID) (bool, error) {
	rw, err := s.repo.GetByRequestPath(ctx, requestPath, channelID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	// A resolved GLOBAL row means no channel row exists yet, so the override is free to create and will shadow it
	if channelID != nil && rw.ChannelID == nil {
		return false, nil
	}
	// Its own canonical row is no conflict - anyone else holding the path in this scope is
	if rw.EntityType == EntityType(entityType) && rw.EntityID == entityID {
		return false, nil
	}
	return true, nil
}

// The route resolver uses this to point a redirect-type rewrite with no explicit target_path - autogenerated rename redirects never carry one - at the entity's live URL
// Uncached, since only the cold redirect path calls it; canonical hits and bots go through ResolveUrl
func (s *Service) CanonicalPath(ctx context.Context, entityType EntityType, entityID uuid.UUID) (string, bool) {
	return s.CanonicalPathForChannel(ctx, entityType, entityID, nil)
}

// Prefers the channel-specific canonical and falls back to the global one, so a per-channel redirect targets that channel's URL
func (s *Service) CanonicalPathForChannel(ctx context.Context, entityType EntityType, entityID uuid.UUID, channelID *uuid.UUID) (string, bool) {
	rows, err := s.repo.ListByEntity(ctx, entityType, entityID)
	if err != nil {
		return "", false
	}
	var global string
	var haveGlobal bool
	for _, rw := range rows {
		if rw.RedirectType != RedirectTypeCanonical {
			continue
		}
		if channelID != nil && rw.ChannelID != nil && *rw.ChannelID == *channelID {
			return rw.RequestPath, true // channel-specific canonical wins
		}
		if rw.ChannelID == nil {
			global, haveGlobal = rw.RequestPath, true
		}
	}
	return global, haveGlobal
}

// Every canonical URL an entity is reachable at, one per channel plus the global fallback, each with the leading slash the storefront's revalidatePath expects (rows are stored without one)
// Cache invalidation wants all of them: CanonicalPathForChannel answers "where does this redirect go", but a purge that skips a channel's URL leaves that storefront serving a stale page until its timer runs out
func (s *Service) CanonicalPaths(ctx context.Context, entityType EntityType, entityID uuid.UUID) []string {
	rows, err := s.repo.ListByEntity(ctx, entityType, entityID)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, rw := range rows {
		if rw.RedirectType != RedirectTypeCanonical || rw.RequestPath == "" {
			continue
		}
		p := EnsureLeadingSlash(rw.RequestPath)
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// revalidatePath treats the argument as a literal route, so a missing slash silently matches nothing
func EnsureLeadingSlash(path string) string {
	if path == "" || strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]UrlRewrite, int64, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 500 {
		filter.Limit = 500
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	filter.Search = strings.TrimSpace(filter.Search)
	return s.repo.List(ctx, filter)
}

// Canonical (default-URL) rows matching search, empty for all, capped at limit - backs the CMS internal-link picker, which passes "" for entityType, and the catalog reference-attribute picker, which scopes to its target type
func (s *Service) SearchCanonical(ctx context.Context, search, entityType string, limit int32) ([]UrlRewrite, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.SearchCanonical(ctx, search, entityType, limit)
}

func (s *Service) ListByEntity(ctx context.Context, entityType EntityType, entityID uuid.UUID) ([]UrlRewrite, error) {
	return s.repo.ListByEntity(ctx, entityType, entityID)
}

// Folds N item URLs into one query per target type instead of N round-trips - currently the menu storefront resolver
func (s *Service) ListByEntityIDs(ctx context.Context, entityType EntityType, entityIDs []uuid.UUID) ([]UrlRewrite, error) {
	if len(entityIDs) == 0 {
		return nil, nil
	}
	return s.repo.ListByEntityIDs(ctx, entityType, entityIDs)
}

// Implements the cms / catalog / blog UrlRewriteSyncer interfaces, unchanged arity
func (s *Service) SyncCanonical(ctx context.Context, entityType string, entityID uuid.UUID, requestPath string) error {
	return s.SyncCanonicalForChannel(ctx, entityType, entityID, requestPath, nil)
}

// Safe on create (inserts) and on slug change (updates request_path)
func (s *Service) SyncCanonicalForChannel(ctx context.Context, entityType string, entityID uuid.UUID, requestPath string, channelID *uuid.UUID) error {
	// If this entity left a stale redirect on the target path in this scope - renamed away and now renamed back - drop it so the canonical can reclaim the path
	// Otherwise the per-scope unique request_path constraint blocks the move and the canonical silently stays put - Only this entity's OWN redirect is removed
	if existing, err := s.repo.GetByRequestPath(ctx, requestPath, channelID); err == nil {
		if existing.RedirectType != RedirectTypeCanonical &&
			existing.EntityType == EntityType(entityType) && existing.EntityID == entityID &&
			sameChannel(existing.ChannelID, channelID) {
			if err := s.repo.Delete(ctx, existing.ID); err != nil {
				return err
			}
			s.invalidatePath(ctx, requestPath, channelID)
		}
	}
	rw, err := s.repo.UpsertCanonical(ctx, EntityType(entityType), entityID, requestPath, channelID)
	if err != nil {
		return err
	}
	s.publishChanged(ctx, rw, false)
	return nil
}

// Marked autogenerated so the retention reaper can prune them - callers are the catalog URL-move paths for product, category and collection renames
func (s *Service) CreateRedirect(ctx context.Context, from string, entityType string, entityID uuid.UUID, redirectType int) error {
	return s.createRedirect(ctx, from, entityType, entityID, redirectType, nil, true)
}

func (s *Service) CreateRedirectForChannel(ctx context.Context, from string, entityType string, entityID uuid.UUID, redirectType int, channelID *uuid.UUID) error {
	return s.createRedirect(ctx, from, entityType, entityID, redirectType, channelID, true)
}

// Operator-authored, so unlike the rename redirects these are never touched by the retention reaper
func (s *Service) CreateManualRedirect(ctx context.Context, from string, entityType string, entityID uuid.UUID, redirectType int) error {
	return s.createRedirect(ctx, from, entityType, entityID, redirectType, nil, false)
}

func (s *Service) CreateManualRedirectForChannel(ctx context.Context, from string, entityType string, entityID uuid.UUID, redirectType int, channelID *uuid.UUID) error {
	return s.createRedirect(ctx, from, entityType, entityID, redirectType, channelID, false)
}

func (s *Service) createRedirect(ctx context.Context, from string, entityType string, entityID uuid.UUID, redirectType int, channelID *uuid.UUID, autogenerated bool) error {
	rt := RedirectType(redirectType)
	if !rt.IsValid() || rt == RedirectTypeCanonical {
		return fmt.Errorf("%w: %d", ErrInvalidRedirectType, redirectType)
	}
	rw, err := s.repo.Create(ctx, CreateInput{
		EntityType:      EntityType(entityType),
		EntityID:        entityID,
		RequestPath:     from,
		RedirectType:    rt,
		ChannelID:       channelID,
		IsAutogenerated: autogenerated,
	})
	if err != nil {
		return err
	}
	s.publishChanged(ctx, rw, false)
	return nil
}

// Operator-authored redirects and canonicals are never removed
func (s *Service) PruneStaleRedirects(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, nil
	}
	return s.repo.PruneAutogeneratedRedirects(ctx, time.Now().Add(-olderThan))
}

func sameChannel(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *Service) Create(ctx context.Context, input CreateInput) (UrlRewrite, error) {
	if !input.RedirectType.IsValid() {
		return UrlRewrite{}, ErrInvalidRedirectType
	}
	var (
		rw  UrlRewrite
		err error
	)
	if input.RedirectType == RedirectTypeCanonical {
		rw, err = s.repo.UpsertCanonical(ctx, input.EntityType, input.EntityID, input.RequestPath, input.ChannelID)
	} else {
		rw, err = s.repo.Create(ctx, input)
	}
	if err != nil {
		return UrlRewrite{}, err
	}
	s.publishChanged(ctx, rw, false)
	return rw, nil
}

func (s *Service) Update(ctx context.Context, input UpdateInput) (UrlRewrite, error) {
	existing, err := s.repo.GetByID(ctx, input.ID)
	if err != nil {
		return UrlRewrite{}, err
	}
	if existing.IsAutogenerated && existing.RedirectType == RedirectTypeCanonical {
		return UrlRewrite{}, ErrCannotDeleteAutogenerated
	}
	if input.RedirectType != nil && !input.RedirectType.IsValid() {
		return UrlRewrite{}, ErrInvalidRedirectType
	}
	rw, err := s.repo.Update(ctx, input)
	if err != nil {
		return UrlRewrite{}, err
	}
	// The OLD path would otherwise keep serving the stale rewrite until its TTL expires
	if existing.RequestPath != rw.RequestPath {
		s.invalidatePath(ctx, existing.RequestPath, existing.ChannelID)
	}
	s.publishChanged(ctx, rw, false)
	return rw, nil
}

// Drops every rewrite row for one entity in a single query and fires per-path invalidation, so url_rewrites doesn't accumulate orphans pointing at dead ids
// Best-effort by convention - the domain Delete methods log and continue, treating a stale rewrite as a downstream problem rather than a reason to abort the delete
func (s *Service) DeleteAllForEntity(ctx context.Context, entityType string, entityID uuid.UUID) error {
	deleted, err := s.repo.DeleteAllForEntity(ctx, EntityType(entityType), entityID)
	if err != nil {
		return err
	}
	for _, rw := range deleted {
		s.publishChanged(ctx, rw, true)
	}
	return nil
}

// channelID must be non-nil, the global canonical is never dropped here - used when a per-channel root override is removed so the page falls back to the global canonical
func (s *Service) DeleteCanonicalForChannel(ctx context.Context, entityType string, entityID uuid.UUID, channelID *uuid.UUID) error {
	if channelID == nil {
		return nil
	}
	rows, err := s.repo.ListByEntity(ctx, EntityType(entityType), entityID)
	if err != nil {
		return err
	}
	for _, rw := range rows {
		if rw.RedirectType == RedirectTypeCanonical && sameChannel(rw.ChannelID, channelID) {
			if err := s.repo.Delete(ctx, rw.ID); err != nil {
				return err
			}
			s.publishChanged(ctx, rw, true)
		}
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.IsAutogenerated && existing.RedirectType == RedirectTypeCanonical {
		return ErrCannotDeleteAutogenerated
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.publishChanged(ctx, existing, true)
	return nil
}
