package cms

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/blockkit"
)

type pageTypeStore interface {
	CountPagesByType(ctx context.Context, pageTypeID uuid.UUID) (int64, error)
	CreatePageType(ctx context.Context, in CreatePageTypeInput) (PageType, error)
	DeletePageType(ctx context.Context, id uuid.UUID) error
	GetPageTypeByID(ctx context.Context, id uuid.UUID) (PageType, error)
	GetPageTypeBySlug(ctx context.Context, slug string) (PageType, error)
	GetPageTypeWithAttributes(ctx context.Context, id uuid.UUID) (PageType, error)
	ListPageTypes(ctx context.Context) ([]PageType, error)
	ListPagesByType(ctx context.Context, pageTypeID uuid.UUID) ([]Page, error)
	SetPageTypeDefaultBlocks(ctx context.Context, id uuid.UUID, blocks json.RawMessage) error
	UpdatePageType(ctx context.Context, in UpdatePageTypeInput) (PageType, error)
	UpsertPageTypeContent(ctx context.Context, pageTypeID uuid.UUID, in PageTypeContentInput) (PageTypeContent, error)
	WithTx(t *sql.Tx) *postgresRepository
}

func (s *pageTypeService) CreatePageType(ctx context.Context, in CreatePageTypeInput) (PageType, error) {
	if err := validateSlug(in.Slug); err != nil {
		return PageType{}, err
	}
	if strings.TrimSpace(in.Locale) == "" || strings.TrimSpace(in.Name) == "" {
		return PageType{}, fmt.Errorf("%w: locale and name", ErrRequiredFieldMissing)
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return PageType{}, err
		}
	}
	return s.repo.CreatePageType(ctx, in)
}

// How many pages currently use a type, for the admin create form's singleton gating
func (s *pageTypeService) CountPagesByType(ctx context.Context, id uuid.UUID) (int64, error) {
	return s.repo.CountPagesByType(ctx, id)
}

func (s *pageTypeService) GetPageType(ctx context.Context, id uuid.UUID) (PageType, error) {
	return s.repo.GetPageTypeWithAttributes(ctx, id)
}

func (s *pageTypeService) GetPageTypeBySlug(ctx context.Context, slug string) (PageType, error) {
	return s.repo.GetPageTypeBySlug(ctx, slug)
}

func (s *pageTypeService) ListPageTypes(ctx context.Context) ([]PageType, error) {
	types, err := s.repo.ListPageTypes(ctx)
	if err != nil {
		return nil, err
	}
	// Hydrate each type's content, attributes and channel roots - the dashboard reads the page-type DETAIL from this list, since page types are low-volume
	// The bare row alone leaves the detail's attribute panel empty, so assignments never appear
	for i := range types {
		full, err := s.repo.GetPageTypeWithAttributes(ctx, types[i].ID)
		if err != nil {
			return nil, err
		}
		types[i] = full
	}
	return types, nil
}

func (s *pageTypeService) PageTypeSEODefaults(ctx context.Context) (map[uuid.UUID]PageType, error) {
	types, err := s.repo.ListPageTypes(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]PageType, len(types))
	for _, t := range types {
		out[t.ID] = t
	}
	return out, nil
}

func (s *pageTypeService) UpdatePageType(ctx context.Context, in UpdatePageTypeInput) (PageType, error) {
	return s.repo.UpdatePageType(ctx, in)
}

// Changing the global root slug re-syncs every page of the type, so their public URLs pick up the new prefix and leave 301s behind
// Every page's new paths are conflict-checked first, then the root and every page move in one tx - so a clash strands nothing and a retry redoes the whole move
func (s *pageTypeService) UpdatePageTypeStructure(ctx context.Context, in UpdatePageTypeStructureInput) (PageType, error) {
	old, err := s.repo.GetPageTypeByID(ctx, in.ID)
	if err != nil {
		return PageType{}, err
	}
	// Partial update - an omitted (nil) field keeps the current value
	merged := ResolvedPageTypeStructure{
		ID:                in.ID,
		RootSlug:          old.RootSlug,
		DefaultMetaTitle:  old.DefaultMetaTitle,
		DefaultSEONoindex: old.DefaultSEONoindex,
		IsSingleton:       old.IsSingleton,
	}
	if in.RootSlug != nil {
		merged.RootSlug = strings.Trim(strings.TrimSpace(*in.RootSlug), "/")
		if merged.RootSlug != "" {
			if err := validateSlugPath(merged.RootSlug); err != nil {
				return PageType{}, err
			}
		}
	}
	if in.DefaultMetaTitle != nil {
		merged.DefaultMetaTitle = *in.DefaultMetaTitle
	}
	if in.DefaultSEONoindex != nil {
		merged.DefaultSEONoindex = *in.DefaultSEONoindex
	}
	if in.IsSingleton != nil {
		merged.IsSingleton = *in.IsSingleton
	}
	if merged.IsSingleton && !old.IsSingleton {
		n, err := s.repo.CountPagesByType(ctx, in.ID)
		if err != nil {
			return PageType{}, err
		}
		if n > 1 {
			return PageType{}, fmt.Errorf("%w: the type already has %d pages", ErrSingletonExists, n)
		}
	}
	next := old
	next.RootSlug = merged.RootSlug
	if err := s.moveTypeURLs(ctx, old, next, func(r *postgresRepository) error {
		_, err := r.UpdatePageTypeStructure(ctx, merged)
		return err
	}); err != nil {
		return PageType{}, err
	}
	return s.repo.GetPageTypeWithAttributes(ctx, in.ID)
}

// Upserts, or removes for an empty slug, a per-channel root override, moving that channel's page URLs with it in the same tx
func (s *pageTypeService) SetPageTypeChannelRoot(ctx context.Context, pageTypeID, channelID uuid.UUID, rootSlug string) (PageType, error) {
	rootSlug = strings.Trim(strings.TrimSpace(rootSlug), "/")
	if rootSlug != "" {
		if err := validateSlugPath(rootSlug); err != nil {
			return PageType{}, err
		}
	}
	old, err := s.repo.GetPageTypeByID(ctx, pageTypeID)
	if err != nil {
		return PageType{}, err
	}
	next := old
	next.ChannelRoots = make([]PageTypeChannelRoot, 0, len(old.ChannelRoots)+1)
	for _, cr := range old.ChannelRoots {
		if cr.ChannelID != channelID {
			next.ChannelRoots = append(next.ChannelRoots, cr)
		}
	}
	if rootSlug != "" {
		next.ChannelRoots = append(next.ChannelRoots, PageTypeChannelRoot{ChannelID: channelID, RootSlug: rootSlug})
	}
	if err := s.moveTypeURLs(ctx, old, next, func(r *postgresRepository) error {
		if rootSlug == "" {
			return r.DeletePageTypeChannelRoot(ctx, pageTypeID, channelID)
		}
		return r.UpsertPageTypeChannelRoot(ctx, pageTypeID, channelID, rootSlug)
	}); err != nil {
		return PageType{}, err
	}
	return s.repo.GetPageTypeWithAttributes(ctx, pageTypeID)
}

// The starter block template seeded into new pages of this type, cleared by an empty value - validated like a page body
func (s *pageTypeService) SetPageTypeDefaultBlocks(ctx context.Context, pageTypeID uuid.UUID, blocks json.RawMessage) (PageType, error) {
	if len(blocks) > 0 {
		doc, ok := blockkit.ParseDocument(blocks)
		if !ok {
			return PageType{}, fmt.Errorf("%w: default blocks", blockkit.ErrInvalidDocument)
		}
		checker, err := s.customTypeChecker(ctx)
		if err != nil {
			return PageType{}, err
		}
		if err := blockkit.Validate(doc, checker); err != nil {
			return PageType{}, err
		}
	}
	if err := s.repo.SetPageTypeDefaultBlocks(ctx, pageTypeID, blocks); err != nil {
		return PageType{}, err
	}
	return s.repo.GetPageTypeWithAttributes(ctx, pageTypeID)
}

func (s *pageTypeService) DeletePageType(ctx context.Context, id uuid.UUID) error {
	pt, err := s.repo.GetPageTypeByID(ctx, id)
	if err != nil {
		return err
	}
	if pt.IsSystem {
		return ErrPageTypeIsSystem
	}
	n, err := s.repo.CountPagesByType(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrPageTypeHasPages
	}
	return s.repo.DeletePageType(ctx, id)
}

func (s *pageTypeService) UpsertPageTypeContent(ctx context.Context, pageTypeID uuid.UUID, in PageTypeContentInput) (PageType, error) {
	if strings.TrimSpace(in.Locale) == "" || strings.TrimSpace(in.Name) == "" {
		return PageType{}, fmt.Errorf("%w: locale and name", ErrRequiredFieldMissing)
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return PageType{}, err
		}
	}
	if _, err := s.repo.UpsertPageTypeContent(ctx, pageTypeID, in); err != nil {
		return PageType{}, err
	}
	return s.repo.GetPageTypeWithAttributes(ctx, pageTypeID)
}

// Applies write, then moves every page whose stored URL isn't where the new roots put it in the same tx - a stale url_path from an unfinished resync is caught too
func (s *pageTypeService) moveTypeURLs(ctx context.Context, oldPT, newPT PageType, write func(r *postgresRepository) error) error {
	pages, err := s.repo.ListPagesByType(ctx, newPT.ID)
	if err != nil {
		return err
	}
	var stale []Page
	for _, p := range pages {
		if p.URLPath != joinRoot(newPT.RootSlug, p.BasePath) || !sameScopedPaths(desiredPageURLs(p.BasePath, oldPT), desiredPageURLs(p.BasePath, newPT)) {
			stale = append(stale, p)
		}
	}
	if s.urlRewrites != nil {
		for _, p := range stale {
			if err := checkPageURLs(ctx, s.urlRewrites, p.ID, desiredPageURLs(p.BasePath, newPT)); err != nil {
				return fmt.Errorf("page %q: %w", p.Slug, err)
			}
		}
	}
	if err := s.inURLTx(ctx, func(r *postgresRepository, rw UrlRewriteSyncer) error {
		if err := write(r); err != nil {
			return err
		}
		for _, p := range stale {
			old := desiredPageURLs(p.BasePath, oldPT)
			old[0].path = p.URLPath
			if err := syncPageURLs(ctx, r, rw, p.ID, p.BasePath, newPT, old); err != nil {
				return fmt.Errorf("page %q: %w", p.Slug, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, p := range stale {
		s.announceChanged(ctx, p, publicPaths(p.BasePath, oldPT)...)
	}
	return nil
}

func sameScopedPaths(a, b []scopedPath) bool {
	if len(a) != len(b) {
		return false
	}
	byKey := make(map[string]string, len(a))
	for _, sp := range a {
		byKey[channelKey(sp.channelID)] = sp.path
	}
	for _, sp := range b {
		if p, ok := byKey[channelKey(sp.channelID)]; !ok || p != sp.path {
			return false
		}
	}
	return true
}
