package cms

import (
	"context"
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
	DeletePageTypeChannelRoot(ctx context.Context, pageTypeID, channelID uuid.UUID) error
	GetPageTypeByID(ctx context.Context, id uuid.UUID) (PageType, error)
	GetPageTypeBySlug(ctx context.Context, slug string) (PageType, error)
	GetPageTypeWithAttributes(ctx context.Context, id uuid.UUID) (PageType, error)
	ListPageTypes(ctx context.Context) ([]PageType, error)
	ListPagesByType(ctx context.Context, pageTypeID uuid.UUID) ([]Page, error)
	SetPageTypeDefaultBlocks(ctx context.Context, id uuid.UUID, blocks json.RawMessage) error
	UpdatePageType(ctx context.Context, in UpdatePageTypeInput) (PageType, error)
	UpdatePageTypeStructure(ctx context.Context, in ResolvedPageTypeStructure) (PageType, error)
	UpsertPageTypeChannelRoot(ctx context.Context, pageTypeID, channelID uuid.UUID, rootSlug string) error
	UpsertPageTypeContent(ctx context.Context, pageTypeID uuid.UUID, in PageTypeContentInput) (PageTypeContent, error)
}

func (s *pageTypeService) CreatePageType(ctx context.Context, in CreatePageTypeInput) (PageType, error) {
	if err := validateSlug(in.Slug); err != nil {
		return PageType{}, err
	}
	if strings.TrimSpace(in.Locale) == "" || strings.TrimSpace(in.Name) == "" {
		return PageType{}, fmt.Errorf("locale and name are required")
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
		if full, err := s.repo.GetPageTypeWithAttributes(ctx, types[i].ID); err == nil {
			types[i] = full
		}
	}
	return types, nil
}

func (s *pageTypeService) UpdatePageType(ctx context.Context, in UpdatePageTypeInput) (PageType, error) {
	return s.repo.UpdatePageType(ctx, in)
}

// Changing the global root slug re-syncs every page of the type, so their public URLs pick up the new prefix and leave 301s behind
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
	if _, err := s.repo.UpdatePageTypeStructure(ctx, merged); err != nil {
		return PageType{}, err
	}
	if old.RootSlug != merged.RootSlug {
		updated, err := s.repo.GetPageTypeByID(ctx, in.ID)
		if err != nil {
			return PageType{}, err
		}
		if err := s.resyncPagesOfType(ctx, old, updated); err != nil {
			return PageType{}, err
		}
	}
	return s.repo.GetPageTypeWithAttributes(ctx, in.ID)
}

// Upserts, or removes for an empty slug, a per-channel root override, then re-syncs the type's pages so that channel's URLs move
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
	if rootSlug == "" {
		if err := s.repo.DeletePageTypeChannelRoot(ctx, pageTypeID, channelID); err != nil {
			return PageType{}, err
		}
	} else if err := s.repo.UpsertPageTypeChannelRoot(ctx, pageTypeID, channelID, rootSlug); err != nil {
		return PageType{}, err
	}
	updated, err := s.repo.GetPageTypeByID(ctx, pageTypeID)
	if err != nil {
		return PageType{}, err
	}
	if err := s.resyncPagesOfType(ctx, old, updated); err != nil {
		return PageType{}, err
	}
	return s.repo.GetPageTypeWithAttributes(ctx, pageTypeID)
}

// The starter block template seeded into new pages of this type, cleared by an empty value - validated like a page body
func (s *pageTypeService) SetPageTypeDefaultBlocks(ctx context.Context, pageTypeID uuid.UUID, blocks json.RawMessage) (PageType, error) {
	if len(blocks) > 0 {
		doc, ok := blockkit.ParseDocument(blocks)
		if !ok {
			return PageType{}, fmt.Errorf("default blocks is not a valid block document")
		}
		if err := blockkit.Validate(doc, s.customTypeChecker(ctx)); err != nil {
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
		return PageType{}, fmt.Errorf("locale and name are required")
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

// resyncPagesOfType recomputes every page of a type after its root slug(s) changed. base_path is unchanged; only the
// root prefix moves, so we diff against the OLD type roots to emit the right 301s
func (s *pageTypeService) resyncPagesOfType(ctx context.Context, oldPT, newPT PageType) error {
	if s.urlRewrites == nil {
		return nil
	}
	pages, err := s.repo.ListPagesByType(ctx, newPT.ID)
	if err != nil {
		return err
	}
	for _, p := range pages {
		old := desiredPageURLs(p.BasePath, oldPT)
		if _, err := s.syncPageURLs(ctx, p.ID, p.BasePath, newPT, old); err != nil {
			return err
		}
	}
	return nil
}
