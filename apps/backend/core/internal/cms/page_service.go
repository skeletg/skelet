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
)

// WithTx hands back the full repository, so transaction bodies aren't narrowed
type pageStore interface {
	CountPagesByType(ctx context.Context, pageTypeID uuid.UUID) (int64, error)
	CreateRevision(ctx context.Context, pageID uuid.UUID, in PageContentInput, author *uuid.UUID) (PageRevision, error)
	DeletePageChannelListing(ctx context.Context, pageID, channelID uuid.UUID) error
	DeletePageContent(ctx context.Context, pageID uuid.UUID, locale string) error
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
	GoLive(ctx context.Context, id uuid.UUID, publishedAt time.Time) (Page, error)
	GoLiveScheduled(ctx context.Context, id uuid.UUID, publishedAt, seenSchedule time.Time) (bool, error)
	ListDueScheduledPages(ctx context.Context) ([]Page, error)
	ListPageChannelListings(ctx context.Context, pageID uuid.UUID) ([]PageChannelListing, error)
	ListPageContentByScope(ctx context.Context, pageIDs []uuid.UUID, locale string) (map[uuid.UUID]PageContent, error)
	ListPageContentFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]PageContent, error)
	ListPageVariantContent(ctx context.Context, pageID uuid.UUID, locale string) ([]PageVariant, error)
	ListPages(ctx context.Context, filter PageListFilter) ([]Page, error)
	ListPagesSearch(ctx context.Context, filter PageListFilter, locale, query string) ([]Page, error)
	ListRevisions(ctx context.Context, pageID uuid.UUID, locale string, limit int32) ([]PageRevision, error)
	PageInAncestry(ctx context.Context, pageID, candidate uuid.UUID, maxDepth int32) (bool, int32, error)
	PageSlugExists(ctx context.Context, slug string) (bool, error)
	PruneRevisions(ctx context.Context, pageID uuid.UUID, locale string, keep int32) error
	RelationshipTargetExists(ctx context.Context, target RelationshipTarget, id uuid.UUID) (bool, error)
	SetPagePublished(ctx context.Context, id uuid.UUID, isPublished bool, publishedAt *time.Time) (Page, error)
	SetScheduledAt(ctx context.Context, pageID uuid.UUID, at *time.Time) (Page, error)
	UpdatePageExperimentKey(ctx context.Context, id uuid.UUID, key string) error
	WithTx(t *sql.Tx) *postgresRepository
}

const maxPageDepth = 32

func (s *pageService) CreatePage(ctx context.Context, in CreatePageInput) (Page, error) {
	if err := validateSlug(in.Slug); err != nil {
		return Page{}, err
	}
	if strings.TrimSpace(in.Locale) == "" || strings.TrimSpace(in.Title) == "" {
		return Page{}, fmt.Errorf("%w: locale and title", ErrRequiredFieldMissing)
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
	if err := s.checkBody(ctx, in.Body); err != nil {
		return Page{}, err
	}
	scalarData, contentData, sels, rels, err := s.validatePageData(pt, in.Data, in.ContentData, in.Selections, in.Relationships)
	if err != nil {
		return Page{}, err
	}
	if err := s.checkRelationshipTargets(ctx, rels); err != nil {
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
	// Pre-flight - reject before creating the row if any scope's path collides
	if s.urlRewrites != nil {
		if err := checkPageURLs(ctx, s.urlRewrites, uuid.Nil, desiredPageURLs(basePath, pt)); err != nil {
			return Page{}, err
		}
	}

	in.Data = scalarData
	var created Page
	if err := s.inURLTx(ctx, func(r *postgresRepository, rw UrlRewriteSyncer) error {
		p, err := r.CreatePage(ctx, in)
		if err != nil {
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
		created = p
		return syncPageURLs(ctx, r, rw, p.ID, basePath, pt, nil)
	}); err != nil {
		return Page{}, err
	}

	// Capture the initial version in the revision history (best-effort)
	_, _ = s.repo.CreateRevision(ctx, created.ID, PageContentInput{
		Locale: in.Locale, Title: in.Title, Body: in.Body,
		MetaTitle: in.MetaTitle, MetaDescription: in.MetaDescription, MetaKeywords: in.MetaKeywords,
		Data: in.ContentData,
	}, adminPtr(ctx))
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
		return Page{}, fmt.Errorf("%w: the source page has none to clone", ErrPageContentNotFound)
	}
	created, err := s.CreatePage(ctx, CreatePageInput{
		PageTypeID:      src.PageTypeID,
		ParentID:        src.ParentID,
		Slug:            slug,
		Position:        src.Position,
		SEONoindex:      src.SEONoindex,
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

	parent := current.ParentID
	if in.ClearParent {
		parent = nil
	} else if in.ParentID != nil {
		if err := s.checkParent(ctx, in.ID, *in.ParentID); err != nil {
			return Page{}, err
		}
		parent = in.ParentID
	}
	slug := current.Slug
	if in.Slug != nil {
		slug = *in.Slug
	}
	moved := slug != current.Slug || !sameParent(parent, current.ParentID)

	var (
		pt      PageType
		newBase string
	)
	if moved {
		if newBase, err = s.pageBasePath(ctx, slug, parent); err != nil {
			return Page{}, err
		}
		if pt, err = s.repo.GetPageTypeByID(ctx, current.PageTypeID); err != nil {
			return Page{}, err
		}
		// Pre-flight the page's own new paths before touching anything - descendants are checked inside the tx, which a clash rolls back
		if s.urlRewrites != nil {
			if err := checkPageURLs(ctx, s.urlRewrites, in.ID, desiredPageURLs(newBase, pt)); err != nil {
				return Page{}, err
			}
		}
	}

	var descendants []Page
	if err := s.inURLTx(ctx, func(r *postgresRepository, rw UrlRewriteSyncer) error {
		if _, err := r.UpdatePage(ctx, in); err != nil {
			return err
		}
		if in.Data != nil {
			if err := r.DemoteApproved(ctx, in.ID); err != nil {
				return err
			}
		}
		if !moved {
			return nil
		}
		if err := syncPageURLs(ctx, r, rw, in.ID, newBase, pt, desiredPageURLs(current.BasePath, pt)); err != nil {
			return err
		}
		if newBase == current.BasePath {
			return nil
		}
		return s.propagatePageURLPaths(ctx, r, rw, in.ID, newBase, 1, &descendants)
	}); err != nil {
		return Page{}, err
	}

	var oldPaths []string
	if moved {
		oldPaths = publicPaths(current.BasePath, pt)
	}
	s.announceChanged(ctx, current, oldPaths...)
	for _, d := range descendants {
		s.announceChanged(ctx, d, d.URLPath)
	}
	return s.repo.GetPageByID(ctx, in.ID)
}

func sameParent(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *pageService) checkParent(ctx context.Context, pageID, parentID uuid.UUID) error {
	if parentID == pageID {
		return ErrPageCycle
	}
	found, depth, err := s.repo.PageInAncestry(ctx, pageID, parentID, maxPageDepth+1)
	if err != nil {
		return err
	}
	if found {
		return ErrPageCycle
	}
	if depth == 0 {
		return ErrPageNotFound
	}
	if depth >= maxPageDepth {
		return ErrPageTooDeep
	}
	return nil
}

// propagatePageURLPaths recomputes descendants' paths after an ancestor's base path changed - Each child's base =
// parentBasePath + "/" + childSlug; its public URLs derive from its OWN type's root(s) - moved collects each descendant as it was
func (s *pageService) propagatePageURLPaths(ctx context.Context, r *postgresRepository, rw UrlRewriteSyncer, parentID uuid.UUID, parentBasePath string, depth int, moved *[]Page) error {
	if depth > maxPageDepth {
		return ErrPageTooDeep
	}
	children, err := r.ListPageChildren(ctx, parentID)
	if err != nil {
		return err
	}
	for _, ch := range children {
		newBase := parentBasePath + "/" + ch.Slug
		pt, err := r.GetPageTypeByID(ctx, ch.PageTypeID)
		if err != nil {
			return err
		}
		if err := syncPageURLs(ctx, r, rw, ch.ID, newBase, pt, desiredPageURLs(ch.BasePath, pt)); err != nil {
			return err
		}
		*moved = append(*moved, ch)
		if err := s.propagatePageURLPaths(ctx, r, rw, ch.ID, newBase, depth+1, moved); err != nil {
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

func (s *pageService) announceUnpublished(ctx context.Context, p Page) {
	if s.bus != nil {
		s.bus.Publish(ctx, &PageUnpublished{PageID: p.ID, PageTypeID: p.PageTypeID, Slug: p.Slug, URLPath: p.URLPath})
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
	if _, err := s.repo.GoLive(ctx, id, publishDate(current, time.Now())); err != nil {
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
	if _, err := s.repo.SetPagePublished(ctx, id, false, current.PublishedAt); err != nil {
		return Page{}, err
	}
	if current.IsPublished {
		s.announceUnpublished(ctx, current)
	}
	return s.repo.GetPageByID(ctx, id)
}

func (s *pageService) DeletePage(ctx context.Context, id uuid.UUID) error {
	n, err := s.BulkDelete(ctx, []uuid.UUID{id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrPageNotFound
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
				if _, err := r.GoLive(ctx, p.ID, publishDate(p, now)); err != nil {
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
	for _, p := range changed {
		if published {
			s.announcePublished(ctx, p)
		} else {
			s.announceUnpublished(ctx, p)
		}
	}
	return len(changed), nil
}

// Adds and removes tag slugs across many pages in one transaction - per page it removes the removeTags, unions the addTags, normalizes and persists
// Tag inputs are slugified to match how pages store them
func (s *pageService) BulkTag(ctx context.Context, ids []uuid.UUID, addTags, removeTags []string) (int, error) {
	add := normalizeTags(addTags)
	remove := make(map[string]struct{})
	for _, t := range normalizeTags(removeTags) {
		remove[t] = struct{}{}
	}
	var tagged []Page
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		tagged = tagged[:0]
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
			tagged = append(tagged, p)
		}
		return nil
	}); err != nil {
		return 0, err
	}
	for _, p := range tagged {
		s.announceChanged(ctx, p)
	}
	return len(tagged), nil
}

// Deletes many pages in one all-or-nothing transaction, their URL rewrites with them - a page that parents another can't be deleted under FK RESTRICT, which fails the whole batch and leaves every page intact
func (s *pageService) BulkDelete(ctx context.Context, ids []uuid.UUID) (int, error) {
	var gone []Page
	types := map[uuid.UUID]PageType{}
	for _, id := range ids {
		p, err := s.repo.GetPageByID(ctx, id)
		if errors.Is(err, ErrPageNotFound) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if _, ok := types[p.PageTypeID]; !ok {
			pt, err := s.repo.GetPageTypeByID(ctx, p.PageTypeID)
			if err != nil {
				return 0, err
			}
			types[p.PageTypeID] = pt
		}
		gone = append(gone, p)
	}
	var total int64
	if err := s.inURLTx(ctx, func(r *postgresRepository, rw UrlRewriteSyncer) error {
		total = 0
		for _, id := range ids {
			n, err := r.DeletePage(ctx, id)
			if err != nil {
				return fmt.Errorf("delete page %s: %w", id, err)
			}
			total += n
			if rw != nil && n > 0 {
				if err := rw.DeleteAllForEntity(ctx, entityCMSPage, id); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return 0, err
	}
	for _, p := range gone {
		s.announceChanged(ctx, p, publicPaths(p.BasePath, types[p.PageTypeID])...)
	}
	return int(total), nil
}

func (s *pageService) UpsertPageContent(ctx context.Context, pageID uuid.UUID, in PageContentInput) (Page, error) {
	if strings.TrimSpace(in.Locale) == "" || strings.TrimSpace(in.Title) == "" {
		return Page{}, fmt.Errorf("%w: locale and title", ErrRequiredFieldMissing)
	}
	if err := s.checkBody(ctx, in.Body); err != nil {
		return Page{}, err
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
	if err := s.editContent(ctx, pageID, func(r *postgresRepository) error {
		_, err := r.UpsertPageContent(ctx, pageID, in)
		return err
	}); err != nil {
		return Page{}, err
	}
	// Snapshot this saved version into the revision history, keeping the last 50 - best-effort, since a revision or prune failure must not fail the save
	if _, err := s.repo.CreateRevision(ctx, pageID, in, adminPtr(ctx)); err == nil {
		_ = s.repo.PruneRevisions(ctx, pageID, in.Locale, 50)
	}
	s.announceChanged(ctx, page)
	return s.repo.GetPageByID(ctx, pageID)
}

// A content or data write plus the approval it voids, in one tx - an approved page that changes must be reviewed again
func (s *pageService) editContent(ctx context.Context, pageID uuid.UUID, write func(r *postgresRepository) error) error {
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		if err := write(r); err != nil {
			return err
		}
		return r.DemoteApproved(ctx, pageID)
	})
}

func (s *pageService) DeletePageContent(ctx context.Context, pageID uuid.UUID, locale string) error {
	page, err := s.repo.GetPageByID(ctx, pageID)
	if err != nil {
		return err
	}
	if err := s.editContent(ctx, pageID, func(r *postgresRepository) error {
		return r.DeletePageContent(ctx, pageID, locale)
	}); err != nil {
		return err
	}
	s.announceChanged(ctx, page)
	return nil
}

func (s *pageService) pageAttribute(ctx context.Context, page Page, attributeID uuid.UUID) (PageType, Attribute, error) {
	pt, err := s.repo.GetPageTypeWithAttributes(ctx, page.PageTypeID)
	if err != nil {
		return PageType{}, Attribute{}, err
	}
	for _, pta := range pt.Attributes {
		if pta.Attribute.ID == attributeID {
			return pt, pta.Attribute, nil
		}
	}
	return PageType{}, Attribute{}, fmt.Errorf("%w: attribute not on page type", ErrInvalidFieldValue)
}

// Replaces a page's select or multiselect choices for one attribute - the attribute must be on the page's type, a select type, and own every value
func (s *pageService) SetPageSelections(ctx context.Context, pageID, attributeID uuid.UUID, valueIDs []uuid.UUID) (Page, error) {
	page, err := s.repo.GetPageByID(ctx, pageID)
	if err != nil {
		return Page{}, err
	}
	pt, _, err := s.pageAttribute(ctx, page, attributeID)
	if err != nil {
		return Page{}, err
	}
	sels := make([]PageAttributeSelection, len(valueIDs))
	for i, vid := range valueIDs {
		sels[i] = PageAttributeSelection{AttributeID: attributeID, ValueID: vid}
	}
	if _, _, _, _, err := s.validatePageData(pt, nil, nil, sels, nil); err != nil {
		return Page{}, err
	}
	if err := s.editContent(ctx, pageID, func(r *postgresRepository) error {
		return r.SetPageSelections(ctx, pageID, attributeID, valueIDs)
	}); err != nil {
		return Page{}, err
	}
	s.announceChanged(ctx, page)
	return s.repo.GetPageByID(ctx, pageID)
}

// Replaces the entity references for a relationship attribute - targets are validated against the attribute's declared relationship target and must exist, and position comes from list order
func (s *pageService) SetPageRelationships(ctx context.Context, pageID, attributeID uuid.UUID, refs []PageRelationship) (Page, error) {
	page, err := s.repo.GetPageByID(ctx, pageID)
	if err != nil {
		return Page{}, err
	}
	pt, a, err := s.pageAttribute(ctx, page, attributeID)
	if err != nil {
		return Page{}, err
	}
	if !a.InputType.IsRelationship() {
		return Page{}, fmt.Errorf("%w: attribute %s is not a relationship", ErrInvalidFieldValue, a.Slug)
	}
	out := make([]PageRelationship, len(refs))
	for i, rl := range refs {
		rl.AttributeID = attributeID
		rl.Position = int32(i)
		out[i] = rl
	}
	if _, _, _, _, err := s.validatePageData(pt, nil, nil, nil, out); err != nil {
		return Page{}, err
	}
	if err := s.checkRelationshipTargets(ctx, out); err != nil {
		return Page{}, err
	}
	if err := s.editContent(ctx, pageID, func(r *postgresRepository) error {
		return r.SetPageRelationships(ctx, pageID, attributeID, out)
	}); err != nil {
		return Page{}, err
	}
	s.announceChanged(ctx, page)
	return s.repo.GetPageByID(ctx, pageID)
}

func (s *pageService) checkRelationshipTargets(ctx context.Context, refs []PageRelationship) error {
	for _, rl := range refs {
		ok, err := s.repo.RelationshipTargetExists(ctx, rl.TargetType, rl.TargetID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: %s %s does not exist", ErrInvalidFieldValue, rl.TargetType, rl.TargetID)
		}
	}
	return nil
}

// Binds, or with "" unbinds, a page to a Prism experiment for A/B testing
func (s *pageService) SetPageExperiment(ctx context.Context, pageID uuid.UUID, key string) (Page, error) {
	page, err := s.repo.GetPageByID(ctx, pageID)
	if err != nil {
		return Page{}, err
	}
	if err := s.repo.UpdatePageExperimentKey(ctx, pageID, strings.TrimSpace(key)); err != nil {
		return Page{}, err
	}
	s.announceChanged(ctx, page)
	return s.repo.GetPageByID(ctx, pageID)
}

// The A/B treatment bodies for a (page, locale) - the control body is PageContent.Body and isn't included
func (s *pageService) GetPageVariants(ctx context.Context, pageID uuid.UUID, locale string) ([]PageVariant, error) {
	return s.repo.ListPageVariantContent(ctx, pageID, locale)
}

// Stores, or with an empty body clears, an A/B variant's block body for a (page, locale, variantKey) - validated like a page body
func (s *pageService) UpsertPageVariantBody(ctx context.Context, pageID uuid.UUID, locale, variantKey string, body json.RawMessage) (Page, error) {
	if strings.TrimSpace(locale) == "" || strings.TrimSpace(variantKey) == "" {
		return Page{}, fmt.Errorf("%w: locale and variantKey", ErrRequiredFieldMissing)
	}
	page, err := s.repo.GetPageByID(ctx, pageID)
	if err != nil {
		return Page{}, err
	}
	if len(body) > 0 {
		if err := s.checkBody(ctx, body); err != nil {
			return Page{}, err
		}
	}
	if err := s.editContent(ctx, pageID, func(r *postgresRepository) error {
		if len(body) == 0 {
			return r.DeletePageVariantContent(ctx, pageID, locale, variantKey)
		}
		return r.UpsertPageVariantContent(ctx, pageID, locale, variantKey, body)
	}); err != nil {
		return Page{}, err
	}
	s.announceChanged(ctx, page)
	return s.repo.GetPageByID(ctx, pageID)
}

// The write-time gate every stored block body passes - freeform content is fine, but a body claiming to be a block document must parse and validate
func (s *core) checkBody(ctx context.Context, body json.RawMessage) error {
	if len(body) == 0 {
		return nil
	}
	checker, err := s.customTypeChecker(ctx)
	if err != nil {
		return err
	}
	return blockkit.CheckBody(body, checker)
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

// ListPageTranslations returns every locale's content row for each page, in one query - list reads only carry the resolved locale, so the admin allContent field reads the full set here
func (s *pageService) ListPageTranslations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]PageContent, error) {
	return s.repo.ListPageContentFor(ctx, ids)
}
