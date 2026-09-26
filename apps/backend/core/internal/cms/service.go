package cms

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/blockkit"
	contentpkg "github.com/skeletg/skelet/apps/backend/core/internal/content"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

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
	repo          *postgresRepository
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

// Per-entity sub-services, each embedding *core - none calls another, and the only shared method lives on *core, so there's no embedding cycle
type (
	attributeService struct{ *core } // attributes, values, groups, page-type assignment
	pageTypeService  struct{ *core }
	pageService      struct{ *core }
)

// The facade - it embeds *core directly so core's fields resolve at depth 1, unambiguously over the copies reached through each sub-service
type Service struct {
	*core
	*attributeService
	*pageTypeService
	*pageService
}

// ListPageTranslations returns every locale's content row for each page, in one query - list reads only carry the resolved locale, so the admin allContent field reads the full set here
func (s *Service) ListPageTranslations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]PageContent, error) {
	return s.repo.ListPageContentFor(ctx, ids)
}

// ListAttributeTranslations returns every locale's content row for each attribute, in one query - list reads only carry the resolved locale, so the admin allContent field reads the full set here
func (s *Service) ListAttributeTranslations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]AttributeContent, error) {
	return s.repo.ListAttributeContentFor(ctx, ids)
}

// id to title for the CMS link picker - best-effort, so a missing id or an error yields no entry and the caller falls back to the path
func (s *Service) PageTitlesByIDs(ctx context.Context, ids []uuid.UUID, locale string) map[uuid.UUID]string {
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
		core:             c,
		attributeService: &attributeService{c},
		pageTypeService:  &pageTypeService{c},
		pageService:      &pageService{c},
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

// ==================== Attributes ====================

func (s *attributeService) CreateAttribute(ctx context.Context, in CreateAttributeInput) (Attribute, error) {
	if err := validateSlug(in.Slug); err != nil {
		return Attribute{}, err
	}
	if !in.InputType.IsValid() {
		return Attribute{}, fmt.Errorf("%w: %q", ErrInvalidInputType, in.InputType)
	}
	if in.InputType.IsRelationship() {
		if in.RelationshipTarget == nil || !in.RelationshipTarget.IsValid() {
			return Attribute{}, ErrInvalidRelationship
		}
	}
	if len(in.Values) > 0 && !in.InputType.AcceptsValues() {
		return Attribute{}, ErrValuesNotApplicable
	}
	for _, v := range in.Values {
		if err := validateSlug(v.Slug); err != nil {
			return Attribute{}, fmt.Errorf("attribute value: %w", err)
		}
	}
	return s.repo.CreateAttribute(ctx, in)
}

func (s *attributeService) GetAttribute(ctx context.Context, id uuid.UUID) (Attribute, error) {
	return s.repo.GetAttributeByID(ctx, id)
}

func (s *attributeService) ListAttributes(ctx context.Context) ([]Attribute, error) {
	return s.repo.ListAttributes(ctx)
}

func (s *attributeService) UpdateAttribute(ctx context.Context, in UpdateAttributeInput) (Attribute, error) {
	return s.repo.UpdateAttribute(ctx, in)
}

func (s *attributeService) DeleteAttribute(ctx context.Context, id uuid.UUID) error {
	used, err := s.repo.ListPageTypesUsingAttribute(ctx, id)
	if err != nil {
		return err
	}
	if len(used) > 0 {
		return ErrAttributeInUse
	}
	return s.repo.DeleteAttribute(ctx, id)
}

func (s *attributeService) UpsertAttributeContent(ctx context.Context, attributeID uuid.UUID, in AttributeContentInput) (Attribute, error) {
	if strings.TrimSpace(in.Locale) == "" {
		return Attribute{}, fmt.Errorf("locale is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return Attribute{}, fmt.Errorf("name is required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, in.Locale); err != nil {
			return Attribute{}, err
		}
	}
	if _, err := s.repo.UpsertAttributeContent(ctx, attributeID, in); err != nil {
		return Attribute{}, err
	}
	return s.repo.GetAttributeByID(ctx, attributeID)
}

// ==================== Attribute Values ====================

func (s *attributeService) AddAttributeValue(ctx context.Context, attributeID uuid.UUID, in AttributeValueInput) (AttributeValue, error) {
	if err := validateSlug(in.Slug); err != nil {
		return AttributeValue{}, err
	}
	a, err := s.repo.GetAttributeByID(ctx, attributeID)
	if err != nil {
		return AttributeValue{}, err
	}
	if !a.InputType.AcceptsValues() {
		return AttributeValue{}, ErrValuesNotApplicable
	}
	return s.repo.CreateAttributeValue(ctx, attributeID, in)
}

func (s *attributeService) UpdateAttributeValue(ctx context.Context, id uuid.UUID, in AttributeValueInput) (AttributeValue, error) {
	if err := validateSlug(in.Slug); err != nil {
		return AttributeValue{}, err
	}
	return s.repo.UpdateAttributeValue(ctx, id, in)
}

func (s *attributeService) DeleteAttributeValue(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteAttributeValue(ctx, id)
}

// Returns the parent attribute with values and content, for the caller
func (s *attributeService) UpsertAttributeValueContent(ctx context.Context, valueID uuid.UUID, locale, name string) (Attribute, error) {
	if strings.TrimSpace(locale) == "" || strings.TrimSpace(name) == "" {
		return Attribute{}, fmt.Errorf("locale and name are required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, locale); err != nil {
			return Attribute{}, err
		}
	}
	v, err := s.repo.GetAttributeValueByID(ctx, valueID)
	if err != nil {
		return Attribute{}, err
	}
	if err := s.repo.UpsertAttributeValueContent(ctx, valueID, locale, name); err != nil {
		return Attribute{}, err
	}
	return s.repo.GetAttributeByID(ctx, v.AttributeID)
}

// ==================== Attribute Groups ====================

func (s *attributeService) CreateAttributeGroup(ctx context.Context, slug string, position int32) (AttributeGroup, error) {
	if err := validateSlug(slug); err != nil {
		return AttributeGroup{}, err
	}
	return s.repo.CreateAttributeGroup(ctx, slug, position)
}

func (s *attributeService) UpdateAttributeGroup(ctx context.Context, id uuid.UUID, slug string, position int32) (AttributeGroup, error) {
	if err := validateSlug(slug); err != nil {
		return AttributeGroup{}, err
	}
	return s.repo.UpdateAttributeGroup(ctx, id, slug, position)
}

func (s *attributeService) DeleteAttributeGroup(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteAttributeGroup(ctx, id)
}

func (s *attributeService) ListAttributeGroups(ctx context.Context) ([]AttributeGroup, error) {
	return s.repo.ListAttributeGroups(ctx)
}

// ==================== Page Types ====================

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

func (s *attributeService) AssignAttribute(ctx context.Context, in AssignAttributeInput) (PageType, error) {
	if _, err := s.repo.GetPageTypeByID(ctx, in.PageTypeID); err != nil {
		return PageType{}, err
	}
	if _, err := s.repo.GetAttributeByID(ctx, in.AttributeID); err != nil {
		return PageType{}, err
	}
	if err := s.repo.AssignAttributeToPageType(ctx, in); err != nil {
		return PageType{}, err
	}
	return s.repo.GetPageTypeWithAttributes(ctx, in.PageTypeID)
}

func (s *attributeService) UnassignAttribute(ctx context.Context, pageTypeID, attributeID uuid.UUID) (PageType, error) {
	if err := s.repo.UnassignAttributeFromPageType(ctx, pageTypeID, attributeID); err != nil {
		return PageType{}, err
	}
	return s.repo.GetPageTypeWithAttributes(ctx, pageTypeID)
}

// ==================== Pages ====================

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
func (s *core) propagatePageURLPaths(ctx context.Context, parentID uuid.UUID, parentBasePath string) error {
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

// resyncPagesOfType recomputes every page of a type after its root slug(s) changed. base_path is unchanged; only the
// root prefix moves, so we diff against the OLD type roots to emit the right 301s
func (s *core) resyncPagesOfType(ctx context.Context, oldPT, newPT PageType) error {
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

func (s *pageService) PublishPage(ctx context.Context, id uuid.UUID) (Page, error) {
	current, err := s.repo.GetPageByID(ctx, id)
	if err != nil {
		return Page{}, err
	}
	// Editorial gate - with require-approval on, a page can only go live from the approved or already-published state
	if s.requireApproval(ctx) &&
		current.WorkflowState != WorkflowApproved && current.WorkflowState != WorkflowPublished {
		return Page{}, ErrApprovalRequired
	}
	if s.bus != nil && !current.IsPublished {
		ing := &PagePublishing{
			PageID:     current.ID,
			PageTypeID: current.PageTypeID,
			Slug:       current.Slug,
			URLPath:    current.URLPath,
		}
		if err := s.bus.PublishSync(ctx, ing); err != nil {
			return Page{}, err
		}
	}
	publishedAt := current.PublishedAt
	if publishedAt == nil {
		now := time.Now()
		publishedAt = &now
	}
	_, err = s.repo.SetPagePublished(ctx, id, true, publishedAt)
	if err != nil {
		return Page{}, err
	}
	// Reaching live is the terminal editorial state
	_, _ = s.repo.SetPageWorkflowState(ctx, id, WorkflowPublished)
	if s.bus != nil && !current.IsPublished {
		s.bus.Publish(ctx, &PagePublished{
			PageID:     current.ID,
			PageTypeID: current.PageTypeID,
			Slug:       current.Slug,
			URLPath:    current.URLPath,
		})
	}
	return s.repo.GetPageByID(ctx, id)
}

// Whether the publish gate is enabled - defaults to false, free publish, when no config reader is wired or the lookup fails
func (s *core) requireApproval(ctx context.Context) bool {
	if s.cfg == nil {
		return false
	}
	v, err := s.cfg.GetBool(ctx, cfgRequireApproval, config.DefaultScope)
	return err == nil && v
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
// Domain events fire only after commit and only for pages that actually changed
func (s *pageService) BulkSetPublished(ctx context.Context, ids []uuid.UUID, published bool) (int, error) {
	requireApproval := s.requireApproval(ctx)
	var changed []Page
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		for _, id := range ids {
			current, err := r.GetPageByID(ctx, id)
			if err != nil {
				return err
			}
			if published {
				if current.IsPublished {
					continue
				}
				if requireApproval && current.WorkflowState != WorkflowApproved && current.WorkflowState != WorkflowPublished {
					return fmt.Errorf("page %q: %w", current.Slug, ErrApprovalRequired)
				}
				if s.bus != nil {
					if err := s.bus.PublishSync(ctx, &PagePublishing{
						PageID: current.ID, PageTypeID: current.PageTypeID, Slug: current.Slug, URLPath: current.URLPath,
					}); err != nil {
						return err
					}
				}
				publishedAt := current.PublishedAt
				if publishedAt == nil {
					now := time.Now()
					publishedAt = &now
				}
				if _, err := r.SetPagePublished(ctx, id, true, publishedAt); err != nil {
					return err
				}
				if _, err := r.SetPageWorkflowState(ctx, id, WorkflowPublished); err != nil {
					return err
				}
			} else {
				if !current.IsPublished {
					continue
				}
				if _, err := r.SetPagePublished(ctx, id, false, current.PublishedAt); err != nil {
					return err
				}
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
				s.bus.Publish(ctx, &PagePublished{PageID: p.ID, PageTypeID: p.PageTypeID, Slug: p.Slug, URLPath: p.URLPath})
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

func (s *pageService) ListPageRevisions(ctx context.Context, pageID uuid.UUID, locale string) ([]PageRevision, error) {
	return s.repo.ListRevisions(ctx, pageID, locale, 50)
}

// Re-applies a revision's content, and since that itself snapshots, a restore is reversible
func (s *pageService) RestoreRevision(ctx context.Context, revisionID uuid.UUID) (Page, error) {
	rev, err := s.repo.GetRevision(ctx, revisionID)
	if err != nil {
		return Page{}, err
	}
	return s.UpsertPageContent(ctx, rev.PageID, PageContentInput{
		Locale:          rev.Locale,
		Title:           rev.Title,
		Body:            rev.Body,
		MetaTitle:       strOrEmpty(rev.MetaTitle),
		MetaDescription: strOrEmpty(rev.MetaDescription),
		MetaKeywords:    strOrEmpty(rev.MetaKeywords),
		Data:            rev.Data,
	})
}

// Sets, or clears when at is nil, a page's future go-live time - the cms-publish job flips is_published when it arrives
func (s *pageService) SchedulePage(ctx context.Context, pageID uuid.UUID, at *time.Time) (Page, error) {
	if _, err := s.repo.SetScheduledAt(ctx, pageID, at); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, pageID)
}

// Publishes every unpublished page whose scheduled_at has arrived, clearing the schedule - run by the cms-publish scheduler job
func (s *pageService) PromoteDuePages(ctx context.Context) (int, error) {
	due, err := s.repo.ListDueScheduledPages(ctx)
	if err != nil {
		return 0, err
	}
	gate := s.requireApproval(ctx)
	n := 0
	for _, p := range due {
		// Respect the editorial gate - a scheduled page that isn't approved stays scheduled rather than erroring, and the operator must approve it
		if gate && p.WorkflowState != WorkflowApproved && p.WorkflowState != WorkflowPublished {
			log.Printf("[cms-publish] skipping due page %s: workflow_state=%s (approval required)", p.ID, p.WorkflowState)
			continue
		}
		if _, err := s.repo.SetScheduledAt(ctx, p.ID, nil); err != nil {
			return n, err
		}
		now := time.Now()
		if _, err := s.repo.SetPagePublished(ctx, p.ID, true, &now); err != nil {
			return n, err
		}
		_, _ = s.repo.SetPageWorkflowState(ctx, p.ID, WorkflowPublished)
		if s.bus != nil {
			s.bus.Publish(ctx, &PagePublished{
				PageID:     p.ID,
				PageTypeID: p.PageTypeID,
				Slug:       p.Slug,
				URLPath:    p.URLPath,
			})
		}
		n++
	}
	return n, nil
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

// ==================== Editorial workflow ====================

func (s *pageService) SubmitPageForReview(ctx context.Context, id uuid.UUID) (Page, error) {
	if _, err := s.repo.SubmitPageForReview(ctx, id, adminPtr(ctx)); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, id)
}

// Moves a page to approved, stamping the approver - the resolver gates this on cms.approve
func (s *pageService) ApprovePage(ctx context.Context, id uuid.UUID) (Page, error) {
	if _, err := s.repo.ApprovePageWorkflow(ctx, id, adminPtr(ctx)); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, id)
}

// Sends a page back to draft, recording the reason as a comment when one is given
func (s *pageService) RejectPage(ctx context.Context, id uuid.UUID, note string) (Page, error) {
	if n := strings.TrimSpace(note); n != "" {
		if by := adminPtr(ctx); by != nil {
			_, _ = s.repo.CreateComment(ctx, CommentInput{PageID: id, Body: "Rejected: " + n}, *by)
		}
	}
	if _, err := s.repo.SetPageWorkflowState(ctx, id, WorkflowDraft); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, id)
}

// Returns a page to draft from any state, e.g. to revise a published page through review again
func (s *pageService) ReopenPage(ctx context.Context, id uuid.UUID) (Page, error) {
	if _, err := s.repo.SetPageWorkflowState(ctx, id, WorkflowDraft); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, id)
}

func (s *pageService) AssignPage(ctx context.Context, id uuid.UUID, to *uuid.UUID) (Page, error) {
	if _, err := s.repo.AssignPage(ctx, id, to); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, id)
}

func (s *pageService) AddComment(ctx context.Context, in CommentInput) (PageComment, error) {
	if strings.TrimSpace(in.Body) == "" {
		return PageComment{}, fmt.Errorf("comment body is required")
	}
	by := adminPtr(ctx)
	if by == nil {
		return PageComment{}, fmt.Errorf("an authenticated admin is required to comment")
	}
	return s.repo.CreateComment(ctx, in, *by)
}

func (s *pageService) ListComments(ctx context.Context, pageID uuid.UUID) ([]PageComment, error) {
	return s.repo.ListComments(ctx, pageID)
}

func (s *pageService) ResolveComment(ctx context.Context, id uuid.UUID, resolved bool) (PageComment, error) {
	return s.repo.ResolveComment(ctx, id, resolved, adminPtr(ctx))
}

func (s *pageService) DeleteComment(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteComment(ctx, id)
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

// ==================== Validation ====================

// Enforces a write-time schema on a page's bag of custom field values, returning sanitized scalar and content data (unknown keys dropped, types coerced) plus verified selections and relationships
// nil inputs mean "no change", so update paths only validate the slices they supplied, and required-field enforcement runs only on create, when both maps are non-nil
func (s *pageService) validatePageData(
	pt PageType,
	scalarData, contentData json.RawMessage,
	selections []PageAttributeSelection,
	relationships []PageRelationship,
) (json.RawMessage, json.RawMessage, []PageAttributeSelection, []PageRelationship, error) {
	scalarMap := decodeJSONMap(scalarData)
	contentMap := decodeJSONMap(contentData)

	bySlug := make(map[string]PageTypeAttribute, len(pt.Attributes))
	byID := make(map[uuid.UUID]Attribute, len(pt.Attributes))
	for _, pta := range pt.Attributes {
		bySlug[pta.Attribute.Slug] = pta
		byID[pta.Attribute.ID] = pta.Attribute
	}

	// On create, with both maps supplied, enforce required scalar fields up front
	if scalarData != nil && contentData != nil {
		for slug, pta := range bySlug {
			req := pta.IsRequired || pta.Attribute.IsRequiredDefault
			if !req {
				continue
			}
			if pta.Attribute.InputType.AcceptsValues() || pta.Attribute.InputType.IsRelationship() {
				continue // checked below
			}
			target := scalarMap
			if pta.Attribute.IsTranslatable {
				target = contentMap
			}
			if v, ok := target[slug]; !ok || isEmpty(v) {
				return nil, nil, nil, nil, fmt.Errorf("%w: %q", ErrRequiredFieldMissing, slug)
			}
		}
	}

	// Coerce and validate every supplied scalar key, dropping unknowns
	cleanScalars := map[string]any{}
	for k, v := range scalarMap {
		pta, ok := bySlug[k]
		if !ok {
			continue
		}
		if pta.Attribute.IsTranslatable {
			// Translatable values belong in contentMap - ignore them here
			continue
		}
		coerced, err := coerceFieldValue(pta.Attribute.InputType, v)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%w: %q: %w", ErrInvalidFieldValue, k, err)
		}
		if err := validateAttributeRules(pta.Attribute, coerced); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%w: %q: %w", ErrInvalidFieldValue, k, err)
		}
		cleanScalars[k] = coerced
	}
	cleanContent := map[string]any{}
	for k, v := range contentMap {
		pta, ok := bySlug[k]
		if !ok {
			continue
		}
		if !pta.Attribute.IsTranslatable {
			continue
		}
		coerced, err := coerceFieldValue(pta.Attribute.InputType, v)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%w: %q: %w", ErrInvalidFieldValue, k, err)
		}
		if err := validateAttributeRules(pta.Attribute, coerced); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%w: %q: %w", ErrInvalidFieldValue, k, err)
		}
		cleanContent[k] = coerced
	}

	// Validate selections - the attribute must be on this page type and the value must belong to it - The repo enforces this at FK level too, but the errors here are friendlier
	for _, sl := range selections {
		attr, ok := byID[sl.AttributeID]
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("%w: attribute not on page type", ErrInvalidFieldValue)
		}
		if !attr.InputType.AcceptsValues() {
			return nil, nil, nil, nil, fmt.Errorf("%w: %q", ErrValuesNotApplicable, attr.Slug)
		}
	}

	// Validate relationships - the attribute is on the type and target_type matches its relationship_target
	for _, rl := range relationships {
		attr, ok := byID[rl.AttributeID]
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("%w: relationship attribute not on page type", ErrInvalidFieldValue)
		}
		if !attr.InputType.IsRelationship() {
			return nil, nil, nil, nil, fmt.Errorf("%w: attribute %q is not relationship-typed", ErrInvalidFieldValue, attr.Slug)
		}
		if attr.RelationshipTarget != nil && *attr.RelationshipTarget != rl.TargetType {
			return nil, nil, nil, nil, fmt.Errorf("%w: relationship target mismatch on %q", ErrInvalidFieldValue, attr.Slug)
		}
	}

	scalarOut, _ := json.Marshal(cleanScalars)
	contentOut, _ := json.Marshal(cleanContent)
	return scalarOut, contentOut, selections, relationships, nil
}

func decodeJSONMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	if m == nil {
		return map[string]any{}
	}
	return m
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

// Fills scalar and content data with each attribute's default_value for keys the page didn't supply, on create only
// Returns the inputs unchanged when nothing was seeded, preserving the required-field gate's nil-ness semantics so create-then-fill stays allowed
func applyAttributeDefaults(pt PageType, scalarData, contentData json.RawMessage) (json.RawMessage, json.RawMessage) {
	scalarMap := decodeJSONMap(scalarData)
	if scalarMap == nil {
		scalarMap = map[string]any{}
	}
	contentMap := decodeJSONMap(contentData)
	if contentMap == nil {
		contentMap = map[string]any{}
	}
	changed := false
	for _, pta := range pt.Attributes {
		a := pta.Attribute
		if len(a.DefaultValue) == 0 || a.InputType.AcceptsValues() || a.InputType.IsRelationship() {
			continue
		}
		var dv any
		if err := json.Unmarshal(a.DefaultValue, &dv); err != nil {
			continue
		}
		target := scalarMap
		if a.IsTranslatable {
			target = contentMap
		}
		if _, exists := target[a.Slug]; !exists {
			target[a.Slug] = dv
			changed = true
		}
	}
	if !changed {
		return scalarData, contentData
	}
	sb, _ := json.Marshal(scalarMap)
	cb, _ := json.Marshal(contentMap)
	return sb, cb
}

type attrValidationRules struct {
	Min       *float64 `json:"min"`
	Max       *float64 `json:"max"`
	MinLength *int     `json:"minLength"`
	MaxLength *int     `json:"maxLength"`
	Regex     string   `json:"regex"`
}

// Enforces an attribute's validation JSONB against an already type-coerced value - applied on every write, and malformed rules are ignored rather than blocking saves
func validateAttributeRules(a Attribute, v any) error {
	if len(a.Validation) == 0 || v == nil {
		return nil
	}
	var r attrValidationRules
	if err := json.Unmarshal(a.Validation, &r); err != nil {
		return nil
	}
	switch a.InputType {
	case InputNumber:
		var f float64
		switch n := v.(type) {
		case float64:
			f = n
		case int:
			f = float64(n)
		case int64:
			f = float64(n)
		default:
			return nil
		}
		if r.Min != nil && f < *r.Min {
			return fmt.Errorf("must be at least %v", *r.Min)
		}
		if r.Max != nil && f > *r.Max {
			return fmt.Errorf("must be at most %v", *r.Max)
		}
	case InputText, InputTextarea, InputURL, InputRichText:
		s, ok := v.(string)
		if !ok {
			return nil
		}
		if r.MinLength != nil && len(s) < *r.MinLength {
			return fmt.Errorf("must be at least %d characters", *r.MinLength)
		}
		if r.MaxLength != nil && len(s) > *r.MaxLength {
			return fmt.Errorf("must be at most %d characters", *r.MaxLength)
		}
		if r.Regex != "" {
			if re, err := regexp.Compile(r.Regex); err == nil && !re.MatchString(s) {
				return fmt.Errorf("does not match the required pattern")
			}
		}
	}
	return nil
}

// Normalizes a raw value to the canonical Go type for its input type, mirroring config.Service - "1" becomes true, "7" becomes 7
func coerceFieldValue(t InputType, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch t {
	case InputText, InputTextarea, InputURL:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected string, got %T", v)
		}
		return s, nil
	case InputNumber:
		switch n := v.(type) {
		case float64:
			return n, nil
		case int, int32, int64:
			return n, nil
		case string:
			var f float64
			if _, err := fmt.Sscanf(n, "%f", &f); err != nil {
				return nil, fmt.Errorf("not a number: %q", n)
			}
			return f, nil
		}
		return nil, fmt.Errorf("expected number, got %T", v)
	case InputBoolean:
		switch b := v.(type) {
		case bool:
			return b, nil
		case string:
			switch strings.ToLower(b) {
			case "true", "1", "yes", "on":
				return true, nil
			case "false", "0", "no", "off":
				return false, nil
			}
		}
		return nil, fmt.Errorf("expected boolean")
	case InputDate, InputDateTime:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected ISO-8601 date string")
		}
		return s, nil
	case InputAsset:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected asset id (uuid string)")
		}
		if _, err := uuid.Parse(s); err != nil {
			return nil, fmt.Errorf("invalid asset id: %w", err)
		}
		return s, nil
	case InputAssets:
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("expected asset id array")
		}
		ids := make([]string, 0, len(arr))
		for _, x := range arr {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("expected uuid string in array")
			}
			if _, err := uuid.Parse(s); err != nil {
				return nil, fmt.Errorf("invalid asset id: %w", err)
			}
			ids = append(ids, s)
		}
		return ids, nil
	case InputRichText, InputJSON:
		return v, nil
	}
	return v, nil
}

// ==================== store listings ====================

// Creates or updates a per-store availability row for a page and returns it with all listings hydrated
// Mirrors blog.Service.SetPostChannelListing - admins use it to toggle a page per store without flipping the global flag
func (s *pageService) SetPageChannelListing(ctx context.Context, in SetPageChannelListingInput) (Page, error) {
	// Demote any existing homepage first when flagging this one, inside a single tx, so the partial unique index never sees two mid-flight
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		if in.IsHomepage {
			if err := r.ClearChannelHomepage(ctx, in.ChannelID, in.PageID); err != nil {
				return err
			}
		}
		_, err := r.UpsertPageChannelListing(ctx, in)
		return err
	})
	if err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, in.PageID)
}

// The channel's designated homepage - the page flagged is_homepage on its published listing - with content for the locale, or ErrPageNotFound when none is set
func (s *pageService) GetHomePageForChannel(ctx context.Context, channelID uuid.UUID, locale string) (Page, error) {
	p, err := s.repo.GetHomePageForChannel(ctx, channelID)
	if err != nil {
		return Page{}, err
	}
	s.fillPageContent(ctx, &p, locale)
	return p, nil
}

func (s *pageService) DeletePageChannelListing(ctx context.Context, pageID, channelID uuid.UUID) (Page, error) {
	if err := s.repo.DeletePageChannelListing(ctx, pageID, channelID); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, pageID)
}

// The per-channel listing rows for a page - exposed so resolvers can gate localized-content mutations against the page's current listing channel set
func (s *pageService) ListPageChannelListings(ctx context.Context, pageID uuid.UUID) ([]PageChannelListing, error) {
	return s.repo.ListPageChannelListings(ctx, pageID)
}

// Resolves a page by id with the per-store listing enforced, so globally unpublished or unlisted pages return ErrPageNotFound - locale applies to Content as in GetPage
func (s *pageService) GetPublishedPageForChannel(ctx context.Context, pageID, channelID uuid.UUID, locale string) (Page, error) {
	p, err := s.repo.GetPublishedPageForChannel(ctx, pageID, channelID)
	if err != nil {
		return Page{}, err
	}
	s.fillPageContent(ctx, &p, locale)
	return p, nil
}

func (s *pageService) GetPublishedPageBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID, locale string) (Page, error) {
	p, err := s.repo.GetPublishedPageBySlugForChannel(ctx, slug, channelID)
	if err != nil {
		return Page{}, err
	}
	s.fillPageContent(ctx, &p, locale)
	return p, nil
}

func (s *pageService) GetPublishedPageByURLPathForChannel(ctx context.Context, urlPath string, channelID uuid.UUID, locale string) (Page, error) {
	p, err := s.repo.GetPublishedPageByURLPathForChannel(ctx, urlPath, channelID)
	if err != nil {
		return Page{}, err
	}
	s.fillPageContent(ctx, &p, locale)
	return p, nil
}

// ==================== helpers ====================

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

// ==================== Reusable blocks: snippets + custom block types ====================

// CreateSnippet / GetSnippet / ListSnippets / UpdateSnippet / DeleteSnippet - a reusable section, a saved block subtree
// `blocks` is a { version, blocks } Document JSON, the same shape a page body carries
func (s *Service) CreateSnippet(ctx context.Context, slug, name string, blocks json.RawMessage) (Snippet, error) {
	if err := validateSlug(slug); err != nil {
		return Snippet{}, err
	}
	return s.repo.CreateSnippet(ctx, slug, name, blocks)
}

func (s *Service) GetSnippet(ctx context.Context, id uuid.UUID) (Snippet, error) {
	return s.repo.GetSnippetByID(ctx, id)
}

func (s *Service) ListSnippets(ctx context.Context) ([]Snippet, error) {
	return s.repo.ListSnippets(ctx)
}

func (s *Service) UpdateSnippet(ctx context.Context, id uuid.UUID, name string, blocks json.RawMessage) (Snippet, error) {
	return s.repo.UpdateSnippet(ctx, id, name, blocks)
}

func (s *Service) DeleteSnippet(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteSnippet(ctx, id)
}

// Resolves a snippet slug to its block subtree, for the front SnippetResolver expanding `ref` blocks server-side - ok=false when the slug is unknown or its body isn't a block document
func (s *Service) GetSnippetBlocks(ctx context.Context, slug string) ([]blockkit.Block, bool) {
	sn, err := s.repo.GetSnippetBySlug(ctx, slug)
	if err != nil {
		return nil, false
	}
	doc, ok := blockkit.ParseDocument(sn.Blocks)
	if !ok {
		return nil, false
	}
	return doc.Blocks, true
}

// CreateBlockType / GetBlockType / ListBlockTypes / UpdateBlockType / DeleteBlockType - admin-defined custom block types, a field schema plus an HTML template
func (s *Service) CreateBlockType(ctx context.Context, key, name, icon string, fields json.RawMessage, html string) (BlockType, error) {
	return s.repo.CreateBlockType(ctx, key, name, icon, fields, html)
}

func (s *Service) GetBlockType(ctx context.Context, id uuid.UUID) (BlockType, error) {
	return s.repo.GetBlockTypeByID(ctx, id)
}

func (s *Service) ListBlockTypes(ctx context.Context) ([]BlockType, error) {
	return s.repo.ListBlockTypes(ctx)
}

func (s *Service) UpdateBlockType(ctx context.Context, id uuid.UUID, name, icon string, fields json.RawMessage, html string) (BlockType, error) {
	return s.repo.UpdateBlockType(ctx, id, name, icon, fields, html)
}

func (s *Service) DeleteBlockType(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteBlockType(ctx, id)
}

// Builds a checker recognizing the admin's custom block-type keys, so a page using one validates on save - best-effort, since a load error yields a checker that rejects all custom types
// On *core so pageService.UpsertPageContent can reach it
func (s *core) customTypeChecker(ctx context.Context) blockkit.CustomTypeChecker {
	types, err := s.repo.ListBlockTypes(ctx)
	known := make(map[string]bool, len(types))
	if err == nil {
		for _, t := range types {
			known[t.Key] = true
		}
	}
	return func(typeKey string) bool { return known[typeKey] }
}

// Looks up a custom block type by key, for the front CustomRenderer, which resolves a custom block lazily and only when present
func (s *Service) GetBlockTypeByKey(ctx context.Context, key string) (BlockType, bool) {
	bt, err := s.repo.GetBlockTypeByKey(ctx, key)
	if err != nil {
		return BlockType{}, false
	}
	return bt, true
}

// ==================== Tag library ====================

var tagSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Derives the stable slug pages actually store from a display name
func slugifyTag(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = tagSlugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 64 {
		s = s[:64]
	}
	return strings.Trim(s, "-")
}

func (s *core) ListTags(ctx context.Context) ([]Tag, error) { return s.repo.ListTags(ctx) }

func (s *core) TagUsageCounts(ctx context.Context) (map[string]int, error) {
	return s.repo.TagUsageCounts(ctx)
}

// Adds a tag, or updates the display name on a slug collision
func (s *core) CreateTag(ctx context.Context, name string) (Tag, error) {
	slug := slugifyTag(name)
	if slug == "" {
		return Tag{}, fmt.Errorf("tag name must contain a letter or number")
	}
	return s.repo.UpsertTag(ctx, slug, strings.TrimSpace(name))
}

// Changes a tag's display name - the slug pages store stays stable
func (s *core) RenameTag(ctx context.Context, slug, name string) (Tag, error) {
	display := strings.TrimSpace(name)
	if display == "" {
		return Tag{}, fmt.Errorf("tag name is required")
	}
	return s.repo.RenameTag(ctx, slug, display)
}

// Removes a tag from the vocabulary and from every page using it
func (s *core) DeleteTag(ctx context.Context, slug string) error {
	return s.repo.DeleteTag(ctx, slug)
}
