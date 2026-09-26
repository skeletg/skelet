package cms

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/blockkit"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// WithTx hands back the full repository, so transaction bodies aren't narrowed
type pageStore interface {
	CountPagesByType(ctx context.Context, pageTypeID uuid.UUID) (int64, error)
	CreateRevision(ctx context.Context, pageID uuid.UUID, in PageContentInput, author *uuid.UUID) (PageRevision, error)
	DeletePage(ctx context.Context, id uuid.UUID) error
	DeletePageChannelListing(ctx context.Context, pageID, channelID uuid.UUID) error
	DeletePageContent(ctx context.Context, pageID uuid.UUID, locale string) error
	DeletePageVariantContent(ctx context.Context, pageID uuid.UUID, locale, variantKey string) error
	GetAttributeByID(ctx context.Context, id uuid.UUID) (Attribute, error)
	GetHomePageForChannel(ctx context.Context, channelID uuid.UUID) (Page, error)
	GetPageByID(ctx context.Context, id uuid.UUID) (Page, error)
	GetPageBySlug(ctx context.Context, slug string) (Page, error)
	GetPageByURLPath(ctx context.Context, urlPath string) (Page, error)
	GetPageTypeByID(ctx context.Context, id uuid.UUID) (PageType, error)
	GetPageTypeWithAttributes(ctx context.Context, id uuid.UUID) (PageType, error)
	GetPublishedPageBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID) (Page, error)
	GetPublishedPageByURLPathForChannel(ctx context.Context, urlPath string, channelID uuid.UUID) (Page, error)
	GetPublishedPageForChannel(ctx context.Context, pageID, channelID uuid.UUID) (Page, error)
	GetRevision(ctx context.Context, id uuid.UUID) (PageRevision, error)
	GoLive(ctx context.Context, id uuid.UUID, publishedAt time.Time, clearSchedule bool) (Page, error)
	ListDueScheduledPages(ctx context.Context) ([]Page, error)
	ListPageChannelListings(ctx context.Context, pageID uuid.UUID) ([]PageChannelListing, error)
	ListPageChildren(ctx context.Context, parentID uuid.UUID) ([]Page, error)
	ListPageContentByScope(ctx context.Context, pageIDs []uuid.UUID, locale string) (map[uuid.UUID]PageContent, error)
	ListPageVariantContent(ctx context.Context, pageID uuid.UUID, locale string) ([]PageVariant, error)
	ListPages(ctx context.Context, filter PageListFilter) ([]Page, error)
	ListPagesSearch(ctx context.Context, filter PageListFilter, locale, query string) ([]Page, error)
	ListRevisions(ctx context.Context, pageID uuid.UUID, locale string, limit int32) ([]PageRevision, error)
	PageSlugExists(ctx context.Context, slug string) (bool, error)
	PruneRevisions(ctx context.Context, pageID uuid.UUID, locale string, keep int32) error
	SetPagePublished(ctx context.Context, id uuid.UUID, isPublished bool, publishedAt *time.Time) (Page, error)
	SetPageRelationships(ctx context.Context, pageID, attributeID uuid.UUID, refs []PageRelationship) error
	SetPageSelections(ctx context.Context, pageID, attributeID uuid.UUID, valueIDs []uuid.UUID) error
	SetScheduledAt(ctx context.Context, pageID uuid.UUID, at *time.Time) (Page, error)
	UpdatePage(ctx context.Context, in UpdatePageInput) (Page, error)
	UpdatePageExperimentKey(ctx context.Context, id uuid.UUID, key string) error
	UpdatePagePaths(ctx context.Context, id uuid.UUID, basePath, urlPath string) error
	UpsertPageContent(ctx context.Context, pageID uuid.UUID, in PageContentInput) (PageContent, error)
	UpsertPageVariantContent(ctx context.Context, pageID uuid.UUID, locale, variantKey string, body json.RawMessage) error
	WithTx(t *sql.Tx) *postgresRepository
}

func (s *pageService) CreatePage(ctx context.Context, in CreatePageInput) (Page, error) {
	if err := validateSlug(in.Slug); err != nil {
		return Page{}, err
	}
	if strings.TrimSpace(in.Locale) == "" || strings.TrimSpace(in.Title) == "" {
		return Page{}, fmt.Errorf("locale and title are required")
	}
	pt, err := s.repo.GetPageTypeWithAttributes(ctx, in.PageTypeID)
	if err != nil {
		return Page{}, err
	}
	// Singleton types allow at most one page
	if pt.IsSingleton {
		n, err := s.repo.CountPagesByType(ctx, pt.ID)
		if err != nil {
			return Page{}, err
		}
		if n > 0 {
			return Page{}, ErrSingletonExists
		}
	}
	// Seed the body from the type's default block template when the new page has none - regenerate block IDs so pages don't share them
	if len(in.Body) == 0 && len(pt.DefaultBlocks) > 0 {
		if doc, ok := blockkit.ParseDocument(pt.DefaultBlocks); ok {
			if raw, err := json.Marshal(blockkit.CloneWithNewIDs(doc)); err == nil {
				in.Body = raw
			}
		}
	}
	scalarData, contentData, sels, rels, err := s.validatePageData(pt, in.Data, in.ContentData, in.Selections, in.Relationships)
	if err != nil {
		return Page{}, err
	}
	// Seed attribute defaults for keys the page didn't supply
	scalarData, contentData = applyAttributeDefaults(pt, scalarData, contentData)
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return Page{}, err
		}
	}

	basePath, err := s.pageBasePath(ctx, in.Slug, in.ParentID)
	if err != nil {
		return Page{}, err
	}
	globalPath := joinRoot(pt.RootSlug, basePath)
	// Pre-flight - reject before creating the row if any scope's path collides
	if s.urlRewrites != nil {
		for _, sp := range desiredPageURLs(basePath, pt) {
			conflict, err := s.urlRewrites.PathConflictForChannel(ctx, sp.path, entityCMSPage, uuid.Nil, sp.channelID)
			if err != nil {
				return Page{}, err
			}
			if conflict {
				return Page{}, ErrURLPathConflict
			}
		}
	}

	in.Data = scalarData
	var created Page
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		p, err := r.CreatePage(ctx, in)
		if err != nil {
			return err
		}
		if err := r.UpdatePagePaths(ctx, p.ID, basePath, globalPath); err != nil {
			return err
		}
		if _, err := r.UpsertPageContent(ctx, p.ID, PageContentInput{
			Locale:          in.Locale,
			Title:           in.Title,
			Body:            in.Body,
			MetaTitle:       in.MetaTitle,
			MetaDescription: in.MetaDescription,
			MetaKeywords:    in.MetaKeywords,
			Data:            contentData,
		}); err != nil {
			return err
		}
		// Group selections by attribute for atomic per-attribute set
		if len(sels) > 0 {
			byAttr := make(map[uuid.UUID][]uuid.UUID)
			for _, sl := range sels {
				byAttr[sl.AttributeID] = append(byAttr[sl.AttributeID], sl.ValueID)
			}
			for attrID, ids := range byAttr {
				if err := r.SetPageSelections(ctx, p.ID, attrID, ids); err != nil {
					return err
				}
			}
		}
		if len(rels) > 0 {
			byAttr := make(map[uuid.UUID][]PageRelationship)
			for _, rl := range rels {
				byAttr[rl.AttributeID] = append(byAttr[rl.AttributeID], rl)
			}
			for attrID, refs := range byAttr {
				if err := r.SetPageRelationships(ctx, p.ID, attrID, refs); err != nil {
					return err
				}
			}
		}
		p.BasePath = basePath
		p.URLPath = globalPath
		created = p
		return nil
	}); err != nil {
		return Page{}, err
	}

	if s.urlRewrites != nil {
		if _, err := s.syncPageURLs(ctx, created.ID, basePath, pt, nil); err != nil {
			return Page{}, err
		}
	}
	// Capture the initial version in the revision history (best-effort)
	var author *uuid.UUID
	if claims, ok := auth.AdminFromContext(ctx); ok {
		author = &claims.AdminID
	}
	_, _ = s.repo.CreateRevision(ctx, created.ID, PageContentInput{
		Locale: in.Locale, Title: in.Title, Body: in.Body,
		MetaTitle: in.MetaTitle, MetaDescription: in.MetaDescription, MetaKeywords: in.MetaKeywords,
		Data: in.ContentData,
	}, author)
	return s.repo.GetPageByID(ctx, created.ID)
}

// Deep-copies a page into a new unpublished draft - all locale content with fresh block IDs, the scalar data bag, metadata, attribute selections, relationships, tags, and the set of channels it's listed on Live and governance state resets: the clone is never published, never a channel homepage, and starts in draft - Its slug becomes a unique "<slug>-copy[-n]" so derived URL paths don't collide, and singleton page types can't be cloned at all The A/B experiment binding and variant bodies are deliberately NOT copied, so the clone starts free of any running test
func (s *pageService) ClonePage(ctx context.Context, sourceID uuid.UUID) (Page, error) {
	src, err := s.repo.GetPageByID(ctx, sourceID)
	if err != nil {
		return Page{}, err
	}
	pt, err := s.repo.GetPageTypeWithAttributes(ctx, src.PageTypeID)
	if err != nil {
		return Page{}, err
	}
	if pt.IsSingleton {
		return Page{}, ErrSingletonExists
	}
	slug, err := s.uniqueCopySlug(ctx, src.Slug)
	if err != nil {
		return Page{}, err
	}
	// Seed CreatePage with the primary locale - the default locale's content when present, else the first row - and replicate the rest afterwards
	primary, others := splitPrimaryContent(src.AllContent, s.resolveDefaultLocale(ctx))
	if primary == nil {
		return Page{}, fmt.Errorf("source page has no content to clone")
	}
	created, err := s.CreatePage(ctx, CreatePageInput{
		PageTypeID:      src.PageTypeID,
		ParentID:        src.ParentID,
		Slug:            slug,
		Position:        src.Position,
		IsPublished:     false,
		SEONoindex:      src.SEONoindex,
		PublishedAt:     nil,
		Data:            src.Data,
		Metadata:        src.Metadata,
		PrivateMetadata: src.PrivateMetadata,
		Locale:          primary.Locale,
		Title:           primary.Title,
		Body:            cloneBody(primary.Body),
		MetaTitle:       strOrEmpty(primary.MetaTitle),
		MetaDescription: strOrEmpty(primary.MetaDescription),
		MetaKeywords:    strOrEmpty(primary.MetaKeywords),
		ContentData:     primary.Data,
		Selections:      src.Selections,
		Relationships:   src.Relationships,
	})
	if err != nil {
		return Page{}, err
	}
	for _, c := range others {
		if _, err := s.UpsertPageContent(ctx, created.ID, PageContentInput{
			Locale:          c.Locale,
			Title:           c.Title,
			Body:            cloneBody(c.Body),
			MetaTitle:       strOrEmpty(c.MetaTitle),
			MetaDescription: strOrEmpty(c.MetaDescription),
			MetaKeywords:    strOrEmpty(c.MetaKeywords),
			Data:            c.Data,
		}); err != nil {
			return Page{}, err
		}
	}
	if len(src.Tags) > 0 {
		tags := append([]string(nil), src.Tags...)
		if _, err := s.UpdatePage(ctx, UpdatePageInput{ID: created.ID, Tags: &tags}); err != nil {
			return Page{}, err
		}
	}
	// Carry which channels the page is listed on, but never published and never as the homepage, since the clone is a draft
	for _, l := range src.ChannelListings {
		if _, err := s.SetPageChannelListing(ctx, SetPageChannelListingInput{
			PageID:      created.ID,
			ChannelID:   l.ChannelID,
			IsPublished: false,
			SEONoindex:  l.SEONoindex,
			IsHomepage:  false,
		}); err != nil {
			return Page{}, err
		}
	}
	return s.repo.GetPageByID(ctx, created.ID)
}

// Regenerates block IDs so a clone never shares them with its source - non-block freeform bodies pass through unchanged
func cloneBody(body json.RawMessage) json.RawMessage {
	if doc, ok := blockkit.ParseDocument(body); ok {
		if raw, err := json.Marshal(blockkit.CloneWithNewIDs(doc)); err == nil {
			return raw
		}
	}
	return body
}

// Picks the locale to seed a clone with, the default when the source has a row for it and otherwise the first, and returns the remaining locales to replicate
func splitPrimaryContent(all []PageContent, defaultLocale string) (*PageContent, []PageContent) {
	if len(all) == 0 {
		return nil, nil
	}
	idx := 0
	if defaultLocale != "" {
		for i, c := range all {
			if c.Locale == defaultLocale {
				idx = i
				break
			}
		}
	}
	primary := all[idx]
	others := make([]PageContent, 0, len(all)-1)
	for i, c := range all {
		if i != idx {
			others = append(others, c)
		}
	}
	return &primary, others
}

// "<base>-copy", then "-copy-2" and so on, taking the first variant not already used so the clone's derived URL paths don't collide - falls back to a random suffix after 50
func (s *pageService) uniqueCopySlug(ctx context.Context, base string) (string, error) {
	candidate := base + "-copy"
	for i := 1; i <= 50; i++ {
		if i > 1 {
			candidate = fmt.Sprintf("%s-copy-%d", base, i)
		}
		exists, err := s.repo.PageSlugExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return fmt.Sprintf("%s-copy-%s", base, uuid.NewString()[:8]), nil
}

func (s *pageService) GetPage(ctx context.Context, id uuid.UUID, locale string) (Page, error) {
	p, err := s.repo.GetPageByID(ctx, id)
	if err != nil {
		return Page{}, err
	}
	s.fillPageContent(ctx, &p, locale)
	return p, nil
}

func (s *pageService) GetPageBySlug(ctx context.Context, slug, locale string) (Page, error) {
	p, err := s.repo.GetPageBySlug(ctx, slug)
	if err != nil {
		return Page{}, err
	}
	s.fillPageContent(ctx, &p, locale)
	return p, nil
}

func (s *pageService) GetPageByURLPath(ctx context.Context, urlPath, locale string) (Page, error) {
	p, err := s.repo.GetPageByURLPath(ctx, urlPath)
	if err != nil {
		return Page{}, err
	}
	s.fillPageContent(ctx, &p, locale)
	return p, nil
}

func (s *pageService) ListPages(ctx context.Context, filter PageListFilter, locale string) ([]Page, error) {
	// Full-text search needs a concrete locale to join content against, so fall back to the default when the caller didn't supply one
	if q := strings.TrimSpace(filter.Search); q != "" {
		searchLocale := locale
		if searchLocale == "" {
			searchLocale = s.resolveDefaultLocale(ctx)
		}
		if searchLocale == "" {
			return []Page{}, nil
		}
		pages, err := s.repo.ListPagesSearch(ctx, filter, searchLocale, q)
		if err != nil {
			return nil, err
		}
		return s.hydratePageContent(ctx, pages, searchLocale)
	}
	pages, err := s.repo.ListPages(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.hydratePageContent(ctx, pages, locale)
}

// Batch-loads each page's Content for the requested locale, falling back to the default for any still missing it - repo list queries don't hydrate AllContent the way GetPageByID does, so this loads Content directly, as blog's ListPosts does - A "" locale is a no-op
func (s *pageService) hydratePageContent(ctx context.Context, pages []Page, locale string) ([]Page, error) {
	if len(pages) == 0 || locale == "" {
		return pages, nil
	}
	ids := make([]uuid.UUID, len(pages))
	for i, p := range pages {
		ids[i] = p.ID
	}
	batch, err := s.repo.ListPageContentByScope(ctx, ids, locale)
	if err != nil {
		return nil, err
	}
	for i, p := range pages {
		if c, ok := batch[p.ID]; ok {
			cp := c
			pages[i].Content = &cp
		}
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		var missing []uuid.UUID
		for _, p := range pages {
			if p.Content == nil {
				missing = append(missing, p.ID)
			}
		}
		if len(missing) > 0 {
			fb, err := s.repo.ListPageContentByScope(ctx, missing, def)
			if err != nil {
				return nil, err
			}
			for i, p := range pages {
				if p.Content != nil {
					continue
				}
				if c, ok := fb[p.ID]; ok {
					cp := c
					pages[i].Content = &cp
				}
			}
		}
	}
	return pages, nil
}

func (s *pageService) UpdatePage(ctx context.Context, in UpdatePageInput) (Page, error) {
	if in.Slug != nil {
		if err := validateSlug(*in.Slug); err != nil {
			return Page{}, err
		}
	}
	current, err := s.repo.GetPageByID(ctx, in.ID)
	if err != nil {
		return Page{}, err
	}

	// If Data changes, validate against the page type's schema
	if in.Data != nil {
		pt, err := s.repo.GetPageTypeWithAttributes(ctx, current.PageTypeID)
		if err != nil {
			return Page{}, err
		}
		scalarData, _, _, _, err := s.validatePageData(pt, in.Data, nil, nil, nil)
		if err != nil {
			return Page{}, err
		}
		in.Data = scalarData
	}

	updated, err := s.repo.UpdatePage(ctx, in)
	if err != nil {
		return Page{}, err
	}

	if (in.Slug != nil && *in.Slug != current.Slug) ||
		(in.ParentID != nil && (current.ParentID == nil || *in.ParentID != *current.ParentID)) ||
		(in.ClearParent && current.ParentID != nil) {
		oldBasePath := current.BasePath
		newBasePath, err := s.pageBasePath(ctx, updated.Slug, updated.ParentID)
		if err != nil {
			return Page{}, err
		}
		pt, err := s.repo.GetPageTypeByID(ctx, current.PageTypeID)
		if err != nil {
			return Page{}, err
		}
		if s.urlRewrites != nil {
			if _, err := s.syncPageURLs(ctx, updated.ID, newBasePath, pt, desiredPageURLs(oldBasePath, pt)); err != nil {
				return Page{}, err
			}
		} else {
			_ = s.repo.UpdatePagePaths(ctx, updated.ID, newBasePath, joinRoot(pt.RootSlug, newBasePath))
		}
		if newBasePath != oldBasePath {
			if err := s.propagatePageURLPaths(ctx, updated.ID, newBasePath); err != nil {
				return Page{}, err
			}
		}
	}

	return s.repo.GetPageByID(ctx, updated.ID)
}

// propagatePageURLPaths recomputes descendants' paths after an ancestor's base path changed - Each child's base =
// parentBasePath + "/" + childSlug; its public URLs derive from its OWN type's root(s)
func (s *pageService) propagatePageURLPaths(ctx context.Context, parentID uuid.UUID, parentBasePath string) error {
	children, err := s.repo.ListPageChildren(ctx, parentID)
	if err != nil {
		return err
	}
	for _, ch := range children {
		newBase := parentBasePath + "/" + ch.Slug
		pt, err := s.repo.GetPageTypeByID(ctx, ch.PageTypeID)
		if err != nil {
			return err
		}
		if s.urlRewrites != nil {
			if _, err := s.syncPageURLs(ctx, ch.ID, newBase, pt, desiredPageURLs(ch.BasePath, pt)); err != nil {
				return err
			}
		} else {
			_ = s.repo.UpdatePagePaths(ctx, ch.ID, newBase, joinRoot(pt.RootSlug, newBase))
		}
		if err := s.propagatePageURLPaths(ctx, ch.ID, newBase); err != nil {
			return err
		}
	}
	return nil
}

// Every path to live goes through here - the hooks only fire for a real transition
func (s *pageService) admitPublish(ctx context.Context, p Page, gate bool) error {
	if gate && p.WorkflowState != WorkflowApproved && p.WorkflowState != WorkflowPublished {
		return ErrApprovalRequired
	}
	if s.bus != nil && !p.IsPublished {
		return s.bus.PublishSync(ctx, &PagePublishing{PageID: p.ID, PageTypeID: p.PageTypeID, Slug: p.Slug, URLPath: p.URLPath})
	}
	return nil
}

// Keeps the first go-live date
func publishDate(p Page, firstLive time.Time) time.Time {
	if p.PublishedAt != nil {
		return *p.PublishedAt
	}
	return firstLive
}

func (s *pageService) announcePublished(ctx context.Context, p Page) {
	if s.bus != nil {
		s.bus.Publish(ctx, &PagePublished{PageID: p.ID, PageTypeID: p.PageTypeID, Slug: p.Slug, URLPath: p.URLPath})
	}
}

func (s *pageService) PublishPage(ctx context.Context, id uuid.UUID) (Page, error) {
	current, err := s.repo.GetPageByID(ctx, id)
	if err != nil {
		return Page{}, err
	}
	if err := s.admitPublish(ctx, current, s.requireApproval(ctx)); err != nil {
		return Page{}, err
	}
	if _, err := s.repo.GoLive(ctx, id, publishDate(current, time.Now()), false); err != nil {
		return Page{}, err
	}
	if !current.IsPublished {
		s.announcePublished(ctx, current)
	}
	return s.repo.GetPageByID(ctx, id)
}

func (s *pageService) UnpublishPage(ctx context.Context, id uuid.UUID) (Page, error) {
	current, err := s.repo.GetPageByID(ctx, id)
	if err != nil {
		return Page{}, err
	}
	_, err = s.repo.SetPagePublished(ctx, id, false, current.PublishedAt)
	if err != nil {
		return Page{}, err
	}
	if s.bus != nil && current.IsPublished {
		s.bus.Publish(ctx, &PageUnpublished{
			PageID:     current.ID,
			PageTypeID: current.PageTypeID,
			Slug:       current.Slug,
			URLPath:    current.URLPath,
		})
	}
	return s.repo.GetPageByID(ctx, id)
}

func (s *pageService) DeletePage(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.DeletePage(ctx, id); err != nil {
		return err
	}
	if s.urlRewrites != nil {
		_ = s.urlRewrites.DeleteAllForEntity(ctx, entityCMSPage, id)
	}
	return nil
}

// Publishes or unpublishes many pages in one all-or-nothing transaction, so if any page can't transition - the approval gate rejecting it, say - nothing changes - Pages already in the target state are skipped
// Hooks are asked before the transaction opens so none waits on the batch's row locks, and events fire after commit for pages that changed
func (s *pageService) BulkSetPublished(ctx context.Context, ids []uuid.UUID, published bool) (int, error) {
	var admitted []Page
	if published {
		gate := s.requireApproval(ctx)
		seen := make(map[uuid.UUID]bool, len(ids))
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			current, err := s.repo.GetPageByID(ctx, id)
			if err != nil {
				return 0, err
			}
			if current.IsPublished {
				continue
			}
			if err := s.admitPublish(ctx, current, gate); err != nil {
				if errors.Is(err, ErrApprovalRequired) {
					return 0, fmt.Errorf("page %q: %w", current.Slug, err)
				}
				return 0, err
			}
			admitted = append(admitted, current)
		}
	}
	var changed []Page
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		changed = changed[:0]
		if published {
			now := time.Now()
			for _, p := range admitted {
				if _, err := r.GoLive(ctx, p.ID, publishDate(p, now), false); err != nil {
					return err
				}
				changed = append(changed, p)
			}
			return nil
		}
		for _, id := range ids {
			current, err := r.GetPageByID(ctx, id)
			if err != nil {
				return err
			}
			if !current.IsPublished {
				continue
			}
			if _, err := r.SetPagePublished(ctx, id, false, current.PublishedAt); err != nil {
				return err
			}
			changed = append(changed, current)
		}
		return nil
	}); err != nil {
		return 0, err
	}
	if s.bus != nil {
		for _, p := range changed {
			if published {
				s.announcePublished(ctx, p)
			} else {
				s.bus.Publish(ctx, &PageUnpublished{PageID: p.ID, PageTypeID: p.PageTypeID, Slug: p.Slug, URLPath: p.URLPath})
			}
		}
	}
	return len(changed), nil
}

// Adds and removes tag slugs across many pages in one transaction - per page it removes the removeTags, unions the addTags, normalizes and persists
// Tag inputs are lower-cased and trimmed to match how pages store them
func (s *pageService) BulkTag(ctx context.Context, ids []uuid.UUID, addTags, removeTags []string) (int, error) {
	add := normalizeTags(addTags)
	remove := make(map[string]struct{})
	for _, t := range normalizeTags(removeTags) {
		remove[t] = struct{}{}
	}
	n := 0
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		for _, id := range ids {
			p, err := r.GetPageByID(ctx, id)
			if err != nil {
				return err
			}
			merged := make([]string, 0, len(p.Tags)+len(add))
			merged = append(merged, p.Tags...)
			merged = append(merged, add...)
			kept := merged[:0]
			for _, tg := range merged {
				if _, rm := remove[tg]; !rm {
					kept = append(kept, tg)
				}
			}
			if err := r.SetPageTags(ctx, id, normalizeTags(kept)); err != nil {
				return err
			}
			n++
		}
		return nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// Deletes many pages in one all-or-nothing transaction - a page that parents another can't be deleted under FK RESTRICT, which fails the whole batch and leaves every page intact
// URL rewrites are cleaned post-commit, mirroring single-page DeletePage
func (s *pageService) BulkDelete(ctx context.Context, ids []uuid.UUID) (int, error) {
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		for _, id := range ids {
			if err := r.DeletePage(ctx, id); err != nil {
				return fmt.Errorf("delete page %s: %w", id, err)
			}
		}
		return nil
	}); err != nil {
		return 0, err
	}
	if s.urlRewrites != nil {
		for _, id := range ids {
			_ = s.urlRewrites.DeleteAllForEntity(ctx, entityCMSPage, id)
		}
	}
	return len(ids), nil
}

func (s *pageService) UpsertPageContent(ctx context.Context, pageID uuid.UUID, in PageContentInput) (Page, error) {
	if strings.TrimSpace(in.Locale) == "" || strings.TrimSpace(in.Title) == "" {
		return Page{}, fmt.Errorf("locale and title are required")
	}
	// When the body is a block document from the page builder, validate the block tree - freeform bodies pass through untouched
	if doc, ok := blockkit.ParseDocument(in.Body); ok {
		if err := blockkit.Validate(doc, s.customTypeChecker(ctx)); err != nil {
			return Page{}, fmt.Errorf("invalid page layout: %w", err)
		}
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return Page{}, err
		}
	}
	page, err := s.repo.GetPageByID(ctx, pageID)
	if err != nil {
		return Page{}, err
	}
	if in.Data != nil {
		pt, err := s.repo.GetPageTypeWithAttributes(ctx, page.PageTypeID)
		if err != nil {
			return Page{}, err
		}
		_, contentData, _, _, err := s.validatePageData(pt, nil, in.Data, nil, nil)
		if err != nil {
			return Page{}, err
		}
		in.Data = contentData
	}
	if _, err := s.repo.UpsertPageContent(ctx, pageID, in); err != nil {
		return Page{}, err
	}
	// Snapshot this saved version into the revision history, keeping the last 50 - best-effort, since a revision or prune failure must not fail the save
	var author *uuid.UUID
	if claims, ok := auth.AdminFromContext(ctx); ok {
		author = &claims.AdminID
	}
	if _, err := s.repo.CreateRevision(ctx, pageID, in, author); err == nil {
		_ = s.repo.PruneRevisions(ctx, pageID, in.Locale, 50)
	}
	return s.repo.GetPageByID(ctx, pageID)
}

func (s *pageService) DeletePageContent(ctx context.Context, pageID uuid.UUID, locale string) error {
	return s.repo.DeletePageContent(ctx, pageID, locale)
}

// Replaces a page's select or multiselect choices for one attribute - validates that the attribute is a select type and accepts the given value IDs
func (s *pageService) SetPageSelections(ctx context.Context, pageID, attributeID uuid.UUID, valueIDs []uuid.UUID) (Page, error) {
	a, err := s.repo.GetAttributeByID(ctx, attributeID)
	if err != nil {
		return Page{}, err
	}
	if !a.InputType.AcceptsValues() {
		return Page{}, ErrValuesNotApplicable
	}
	allowed := make(map[uuid.UUID]struct{}, len(a.Values))
	for _, v := range a.Values {
		allowed[v.ID] = struct{}{}
	}
	for _, vid := range valueIDs {
		if _, ok := allowed[vid]; !ok {
			return Page{}, fmt.Errorf("%w: value %s not in attribute %s", ErrInvalidFieldValue, vid, a.Slug)
		}
	}
	if a.InputType == InputSelect && len(valueIDs) > 1 {
		return Page{}, fmt.Errorf("%w: select accepts at most one value", ErrInvalidFieldValue)
	}
	if err := s.repo.SetPageSelections(ctx, pageID, attributeID, valueIDs); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, pageID)
}

// Replaces the entity references for a relationship attribute - targets are validated against the attribute's declared relationship target, and position comes from list order
func (s *pageService) SetPageRelationships(ctx context.Context, pageID, attributeID uuid.UUID, refs []PageRelationship) (Page, error) {
	a, err := s.repo.GetAttributeByID(ctx, attributeID)
	if err != nil {
		return Page{}, err
	}
	if !a.InputType.IsRelationship() {
		return Page{}, fmt.Errorf("%w: attribute %s is not a relationship", ErrInvalidFieldValue, a.Slug)
	}
	out := make([]PageRelationship, len(refs))
	for i, rl := range refs {
		if a.RelationshipTarget != nil && rl.TargetType != *a.RelationshipTarget {
			return Page{}, fmt.Errorf("%w: relationship target must be %s", ErrInvalidFieldValue, *a.RelationshipTarget)
		}
		rl.AttributeID = attributeID
		rl.Position = int32(i)
		out[i] = rl
	}
	if err := s.repo.SetPageRelationships(ctx, pageID, attributeID, out); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, pageID)
}

// Binds, or with "" unbinds, a page to a Prism experiment for A/B testing
func (s *pageService) SetPageExperiment(ctx context.Context, pageID uuid.UUID, key string) (Page, error) {
	if err := s.repo.UpdatePageExperimentKey(ctx, pageID, strings.TrimSpace(key)); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, pageID)
}

// The A/B treatment bodies for a (page, locale) - the control body is PageContent.Body and isn't included
func (s *pageService) GetPageVariants(ctx context.Context, pageID uuid.UUID, locale string) ([]PageVariant, error) {
	return s.repo.ListPageVariantContent(ctx, pageID, locale)
}

// Stores, or with an empty body clears, an A/B variant's block body for a (page, locale, variantKey) - validated like a page body
func (s *pageService) UpsertPageVariantBody(ctx context.Context, pageID uuid.UUID, locale, variantKey string, body json.RawMessage) (Page, error) {
	if strings.TrimSpace(locale) == "" || strings.TrimSpace(variantKey) == "" {
		return Page{}, fmt.Errorf("locale and variantKey are required")
	}
	if len(body) == 0 {
		if err := s.repo.DeletePageVariantContent(ctx, pageID, locale, variantKey); err != nil {
			return Page{}, err
		}
		return s.repo.GetPageByID(ctx, pageID)
	}
	if doc, ok := blockkit.ParseDocument(body); ok {
		if err := blockkit.Validate(doc, s.customTypeChecker(ctx)); err != nil {
			return Page{}, err
		}
	}
	if err := s.repo.UpsertPageVariantContent(ctx, pageID, locale, variantKey, body); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, pageID)
}

// Picks Content for the requested locale, falling back to the global default - Content stays nil when neither exists
func (s *pageService) fillPageContent(ctx context.Context, p *Page, locale string) {
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
