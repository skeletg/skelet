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
	"github.com/sqlc-dev/pqtype"

	"github.com/skeletg/skelet/apps/backend/core/internal/blockkit"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

func (r *postgresRepository) ListPagesByType(ctx context.Context, pageTypeID uuid.UUID) ([]Page, error) {
	rows, err := r.queries.ListCMSPagesByType(ctx, pageTypeID)
	if err != nil {
		return nil, fmt.Errorf("list pages by type: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdatePagePaths(ctx context.Context, id uuid.UUID, basePath, urlPath string) error {
	return r.queries.UpdateCMSPagePaths(ctx, db.UpdateCMSPagePathsParams{ID: id, BasePath: basePath, UrlPath: urlPath})
}

func (r *postgresRepository) UpdatePageExperimentKey(ctx context.Context, id uuid.UUID, key string) error {
	return r.queries.UpdateCMSPageExperimentKey(ctx, db.UpdateCMSPageExperimentKeyParams{ID: id, ExperimentKey: key})
}

func (r *postgresRepository) ListPageVariantContent(ctx context.Context, pageID uuid.UUID, locale string) ([]PageVariant, error) {
	rows, err := r.queries.ListCMSPageVariantContent(ctx, db.ListCMSPageVariantContentParams{PageID: pageID, Locale: locale})
	if err != nil {
		return nil, fmt.Errorf("list page variant content: %w", err)
	}
	out := make([]PageVariant, len(rows))
	for i, row := range rows {
		var body json.RawMessage
		if row.Body.Valid {
			body = row.Body.RawMessage
		}
		out[i] = PageVariant{VariantKey: row.VariantKey, Body: body}
	}
	return out, nil
}

func (r *postgresRepository) UpsertPageVariantContent(ctx context.Context, pageID uuid.UUID, locale, variantKey string, body json.RawMessage) error {
	return r.queries.UpsertCMSPageVariantContent(ctx, db.UpsertCMSPageVariantContentParams{
		PageID: pageID, Locale: locale, VariantKey: variantKey,
		Body: pqtype.NullRawMessage{RawMessage: body, Valid: len(body) > 0},
	})
}

func (r *postgresRepository) DeletePageVariantContent(ctx context.Context, pageID uuid.UUID, locale, variantKey string) error {
	return r.queries.DeleteCMSPageVariantContent(ctx, db.DeleteCMSPageVariantContentParams{
		PageID: pageID, Locale: locale, VariantKey: variantKey,
	})
}

func (r *postgresRepository) CreatePage(ctx context.Context, in CreatePageInput) (Page, error) {
	row, err := r.queries.CreateCMSPage(ctx, db.CreateCMSPageParams{
		PageTypeID:      in.PageTypeID,
		ParentID:        sqlnull.UUID(in.ParentID),
		Slug:            in.Slug,
		BasePath:        in.Slug, // overwritten by service after base/url paths are computed
		UrlPath:         in.Slug,
		Position:        in.Position,
		SeoNoindex:      in.SEONoindex,
		Data:            emptyJSON(in.Data),
		Metadata:        metadataToJSON(in.Metadata),
		PrivateMetadata: metadataToJSON(in.PrivateMetadata),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Page{}, ErrSlugTaken
		}
		return Page{}, fmt.Errorf("create page: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) GetPageByID(ctx context.Context, id uuid.UUID) (Page, error) {
	row, err := r.queries.GetCMSPageByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get page: %w", err)
	}
	p := toPage(row)
	if p.AllContent, err = r.ListPageContent(ctx, id); err != nil {
		return Page{}, err
	}
	if p.Selections, err = r.ListPageSelections(ctx, id); err != nil {
		return Page{}, err
	}
	if p.Relationships, err = r.ListPageRelationships(ctx, id); err != nil {
		return Page{}, err
	}
	if p.ChannelListings, err = r.ListPageChannelListings(ctx, id); err != nil {
		return Page{}, err
	}
	return p, nil
}

func (r *postgresRepository) GetPageBySlug(ctx context.Context, slug string) (Page, error) {
	row, err := r.queries.GetCMSPageBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get page by slug: %w", err)
	}
	return r.GetPageByID(ctx, row.ID)
}

func (r *postgresRepository) GetPageByURLPath(ctx context.Context, urlPath string) (Page, error) {
	row, err := r.queries.GetCMSPageByURLPath(ctx, urlPath)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get page by url_path: %w", err)
	}
	return r.GetPageByID(ctx, row.ID)
}

func (r *postgresRepository) ListPages(ctx context.Context, filter PageListFilter) ([]Page, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	var pageTypeID, parentID uuid.NullUUID
	if filter.PageTypeID != nil {
		pageTypeID = uuid.NullUUID{UUID: *filter.PageTypeID, Valid: true}
	}
	if filter.ParentID != nil {
		parentID = uuid.NullUUID{UUID: *filter.ParentID, Valid: true}
	}
	// Storefront paths: when ChannelID is supplied, the join goes against cms_page_channel_listings so only pages with an active listing on
	// that store come back. cms_pages.is_published is also required (the query enforces it), so OnlyPublished is implicit
	if filter.ChannelID != nil {
		rows, err := r.queries.ListPublishedCMSPagesForChannel(ctx, db.ListPublishedCMSPagesForChannelParams{
			ChannelID:  *filter.ChannelID,
			Limit:      limit,
			Offset:     filter.Offset,
			PageTypeID: pageTypeID,
			ParentID:   parentID,
		})
		if err != nil {
			return nil, fmt.Errorf("list pages for store: %w", err)
		}
		out := make([]Page, len(rows))
		ids := make([]uuid.UUID, len(rows))
		for i, row := range rows {
			out[i] = toPage(row)
			ids[i] = row.ID
		}
		listings, err := r.ListPageChannelListingsByPageIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].ChannelListings = listings[out[i].ID]
		}
		return out, nil
	}
	rows, err := r.queries.ListCMSPages(ctx, db.ListCMSPagesParams{
		Limit:         limit,
		Offset:        filter.Offset,
		PageTypeID:    pageTypeID,
		ParentID:      parentID,
		OnlyPublished: sql.NullBool{Bool: filter.OnlyPublished, Valid: true},
		WorkflowState: sqlnull.StringNonEmpty(filter.WorkflowState),
	})
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}

// PageSlugExists reports whether any page already uses this slug (used to pick
// a unique slug when cloning)
func (r *postgresRepository) PageSlugExists(ctx context.Context, slug string) (bool, error) {
	exists, err := r.queries.CMSPageSlugExists(ctx, slug)
	if err != nil {
		return false, fmt.Errorf("page slug exists: %w", err)
	}
	return exists, nil
}

// ListPagesSearch runs the admin full-text search over a locale's content, ranked by relevance - Filters mirror
// ListPages (page type / parent / published / workflow) - Pages with no content row in the requested locale don't match
func (r *postgresRepository) ListPagesSearch(ctx context.Context, filter PageListFilter, locale, query string) ([]Page, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	var pageTypeID, parentID uuid.NullUUID
	if filter.PageTypeID != nil {
		pageTypeID = uuid.NullUUID{UUID: *filter.PageTypeID, Valid: true}
	}
	if filter.ParentID != nil {
		parentID = uuid.NullUUID{UUID: *filter.ParentID, Valid: true}
	}
	rows, err := r.queries.ListCMSPagesSearch(ctx, db.ListCMSPagesSearchParams{
		Locale:        locale,
		Q:             query,
		Limit:         limit,
		Offset:        filter.Offset,
		PageTypeID:    pageTypeID,
		ParentID:      parentID,
		OnlyPublished: sql.NullBool{Bool: filter.OnlyPublished, Valid: true},
		WorkflowState: sqlnull.StringNonEmpty(filter.WorkflowState),
	})
	if err != nil {
		return nil, fmt.Errorf("search pages: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}

func (r *postgresRepository) ListAllPages(ctx context.Context) ([]Page, error) {
	rows, err := r.queries.ListAllCMSPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all pages: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}

func (r *postgresRepository) ListPageChildren(ctx context.Context, parentID uuid.UUID) ([]Page, error) {
	rows, err := r.queries.ListCMSPageChildren(ctx, uuid.NullUUID{UUID: parentID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list page children: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdatePage(ctx context.Context, in UpdatePageInput) (Page, error) {
	current, err := r.queries.GetCMSPageByID(ctx, in.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("load page: %w", err)
	}
	parentID := current.ParentID
	if in.ClearParent {
		parentID = uuid.NullUUID{}
	} else if in.ParentID != nil {
		parentID = uuid.NullUUID{UUID: *in.ParentID, Valid: true}
	}
	slug := current.Slug
	if in.Slug != nil {
		slug = *in.Slug
	}
	position := current.Position
	if in.Position != nil {
		position = *in.Position
	}
	seoNoindex := current.SeoNoindex
	if in.SEONoindex != nil {
		seoNoindex = *in.SEONoindex
	}
	data := current.Data
	if in.Data != nil {
		data = emptyJSON(in.Data)
	}
	metadata := current.Metadata
	if in.Metadata != nil {
		metadata = metadataToJSON(in.Metadata)
	}
	privateMetadata := current.PrivateMetadata
	if in.PrivateMetadata != nil {
		privateMetadata = metadataToJSON(in.PrivateMetadata)
	}
	tags := current.Tags
	if in.Tags != nil {
		tags = normalizeTags(*in.Tags)
	}
	row, err := r.queries.UpdateCMSPage(ctx, db.UpdateCMSPageParams{
		ID:              in.ID,
		ParentID:        parentID,
		Slug:            slug,
		Position:        position,
		SeoNoindex:      seoNoindex,
		Data:            data,
		Metadata:        metadata,
		PrivateMetadata: privateMetadata,
		Tags:            tags,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Page{}, ErrSlugTaken
		}
		return Page{}, fmt.Errorf("update page: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) SetPageTags(ctx context.Context, id uuid.UUID, tags []string) error {
	if err := r.queries.SetCMSPageTags(ctx, db.SetCMSPageTagsParams{ID: id, Tags: tags}); err != nil {
		return fmt.Errorf("set page tags: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpdatePageURLPath(ctx context.Context, id uuid.UUID, urlPath string) error {
	if err := r.queries.UpdateCMSPageURLPath(ctx, db.UpdateCMSPageURLPathParams{
		ID: id, UrlPath: urlPath,
	}); err != nil {
		return fmt.Errorf("update page url_path: %w", err)
	}
	return nil
}

func (r *postgresRepository) SetPagePublished(ctx context.Context, id uuid.UUID, isPublished bool, publishedAt *time.Time) (Page, error) {
	row, err := r.queries.SetCMSPagePublished(ctx, db.SetCMSPagePublishedParams{
		ID: id, IsPublished: isPublished, PublishedAt: sqlnull.Time(publishedAt),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("set page published: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) GoLive(ctx context.Context, id uuid.UUID, publishedAt time.Time) (Page, error) {
	row, err := r.queries.GoLiveCMSPage(ctx, db.GoLiveCMSPageParams{
		ID: id, PublishedAt: sql.NullTime{Time: publishedAt, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("publish page: %w", err)
	}
	return toPage(row), nil
}

// False when the page went live through another path, or its schedule moved, after the scheduler read it
func (r *postgresRepository) GoLiveScheduled(ctx context.Context, id uuid.UUID, publishedAt, seenSchedule time.Time) (bool, error) {
	n, err := r.queries.GoLiveScheduledCMSPage(ctx, db.GoLiveScheduledCMSPageParams{
		ID: id, PublishedAt: sql.NullTime{Time: publishedAt, Valid: true}, SeenSchedule: seenSchedule,
	})
	if err != nil {
		return false, fmt.Errorf("publish scheduled page: %w", err)
	}
	return n > 0, nil
}

func (r *postgresRepository) DemoteApproved(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DemoteApprovedCMSPage(ctx, id); err != nil {
		return fmt.Errorf("void page approval: %w", err)
	}
	return nil
}

func (r *postgresRepository) PageInAncestry(ctx context.Context, pageID, candidate uuid.UUID, maxDepth int32) (bool, int32, error) {
	row, err := r.queries.CMSPageInAncestry(ctx, db.CMSPageInAncestryParams{PageID: pageID, Candidate: candidate, MaxDepth: maxDepth})
	if err != nil {
		return false, 0, fmt.Errorf("walk page ancestry: %w", err)
	}
	return row.Found, row.Depth, nil
}

func (r *postgresRepository) RelationshipTargetExists(ctx context.Context, target RelationshipTarget, id uuid.UUID) (bool, error) {
	found, err := r.queries.CMSRelationshipTargetExists(ctx, db.CMSRelationshipTargetExistsParams{TargetType: string(target), TargetID: id})
	if err != nil {
		return false, fmt.Errorf("check relationship target: %w", err)
	}
	return found, nil
}

func (r *postgresRepository) DeletePage(ctx context.Context, id uuid.UUID) (int64, error) {
	n, err := r.queries.DeleteCMSPage(ctx, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return 0, ErrPageHasChildren
		}
		return 0, fmt.Errorf("delete page: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) UpsertPageContent(ctx context.Context, pageID uuid.UUID, in PageContentInput) (PageContent, error) {
	row, err := r.queries.UpsertCMSPageContent(ctx, db.UpsertCMSPageContentParams{
		PageID:          pageID,
		Locale:          in.Locale,
		Title:           in.Title,
		Body:            pqtype.NullRawMessage{RawMessage: in.Body, Valid: len(in.Body) > 0},
		MetaTitle:       sqlnull.StringValueNonEmpty(in.MetaTitle),
		MetaDescription: sqlnull.StringValueNonEmpty(in.MetaDescription),
		MetaKeywords:    sqlnull.StringValueNonEmpty(in.MetaKeywords),
		Data:            emptyJSON(in.Data),
	})
	if err != nil {
		return PageContent{}, fmt.Errorf("upsert page content: %w", err)
	}
	// Repopulate the full-text search index for this (page, locale) - best-effort, since a content save must not fail because indexing did
	// A stale vector is corrected on the next save or by the backfill datapatch
	if err := r.queries.SetCMSPageContentSearchVector(ctx, db.SetCMSPageContentSearchVectorParams{
		PageID: pageID,
		Locale: in.Locale,
		Doc:    buildSearchText(in.Title, in.MetaTitle, in.MetaDescription, in.MetaKeywords, in.Body),
	}); err != nil {
		return toPageContent(row), nil
	}
	return toPageContent(row), nil
}

// buildSearchText assembles the indexable text for a page-content row: title + meta + the human-readable text extracted
// from the block body - Non-block freeform bodies contribute no block text
func buildSearchText(title, metaTitle, metaDescription, metaKeywords string, body json.RawMessage) string {
	parts := []string{title, metaTitle, metaDescription, metaKeywords}
	if doc, ok := blockkit.ParseDocument(body); ok {
		parts = append(parts, blockkit.PlainText(doc))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func (r *postgresRepository) GetPageContent(ctx context.Context, pageID uuid.UUID, locale string) (PageContent, error) {
	row, err := r.queries.GetCMSPageContent(ctx, db.GetCMSPageContentParams{
		PageID: pageID, Locale: locale,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PageContent{}, ErrPageContentNotFound
		}
		return PageContent{}, fmt.Errorf("get page content: %w", err)
	}
	return toPageContent(row), nil
}

func (r *postgresRepository) ListPageContent(ctx context.Context, pageID uuid.UUID) ([]PageContent, error) {
	rows, err := r.queries.ListCMSPageContent(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list page content: %w", err)
	}
	out := make([]PageContent, len(rows))
	for i, row := range rows {
		out[i] = toPageContent(row)
	}
	return out, nil
}

// ListPageContentByScope batch-loads one locale's content for many pages in a single query, keyed by page id - Used by
// ListPages so list views serve translated content without an N+1 (mirrors blog's ListPostContentByScope)
func (r *postgresRepository) ListPageContentByScope(ctx context.Context, pageIDs []uuid.UUID, locale string) (map[uuid.UUID]PageContent, error) {
	if len(pageIDs) == 0 {
		return map[uuid.UUID]PageContent{}, nil
	}
	rows, err := r.queries.ListCMSPageContentByScope(ctx, db.ListCMSPageContentByScopeParams{
		PageIds: pageIDs,
		Locale:  locale,
	})
	if err != nil {
		return nil, fmt.Errorf("list page content by scope: %w", err)
	}
	out := make(map[uuid.UUID]PageContent, len(rows))
	for _, row := range rows {
		out[row.PageID] = toPageContent(row)
	}
	return out, nil
}

func (r *postgresRepository) DeletePageContent(ctx context.Context, pageID uuid.UUID, locale string) error {
	_, err := r.queries.DeleteCMSPageContent(ctx, db.DeleteCMSPageContentParams{
		PageID: pageID, Locale: locale,
	})
	return err
}

func (r *postgresRepository) SetPageSelections(ctx context.Context, pageID, attributeID uuid.UUID, valueIDs []uuid.UUID) error {
	return r.atomic(ctx, func(q *db.Queries) error {
		if err := q.ClearCMSPageAttributeValuesForAttribute(ctx, db.ClearCMSPageAttributeValuesForAttributeParams{
			PageID: pageID, AttributeID: attributeID,
		}); err != nil {
			return err
		}
		for _, vid := range valueIDs {
			if err := q.SetCMSPageAttributeValue(ctx, db.SetCMSPageAttributeValueParams{
				PageID: pageID, AttributeID: attributeID, ValueID: vid,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *postgresRepository) ListPageSelections(ctx context.Context, pageID uuid.UUID) ([]PageAttributeSelection, error) {
	rows, err := r.queries.ListCMSPageAttributeValues(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list page selections: %w", err)
	}
	out := make([]PageAttributeSelection, len(rows))
	for i, s := range rows {
		out[i] = PageAttributeSelection{AttributeID: s.AttributeID, ValueID: s.ValueID}
	}
	return out, nil
}

func (r *postgresRepository) SetPageRelationships(ctx context.Context, pageID, attributeID uuid.UUID, refs []PageRelationship) error {
	return r.atomic(ctx, func(q *db.Queries) error {
		if err := q.ClearCMSPageRelationshipsForAttribute(ctx, db.ClearCMSPageRelationshipsForAttributeParams{
			PageID: pageID, AttributeID: attributeID,
		}); err != nil {
			return err
		}
		for _, rl := range refs {
			if err := q.SetCMSPageRelationship(ctx, db.SetCMSPageRelationshipParams{
				PageID: pageID, AttributeID: attributeID,
				TargetType: string(rl.TargetType), TargetID: rl.TargetID, Position: rl.Position,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *postgresRepository) ListPageRelationships(ctx context.Context, pageID uuid.UUID) ([]PageRelationship, error) {
	rows, err := r.queries.ListCMSPageRelationships(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list page relationships: %w", err)
	}
	out := make([]PageRelationship, len(rows))
	for i, rl := range rows {
		out[i] = PageRelationship{
			AttributeID: rl.AttributeID,
			TargetType:  RelationshipTarget(rl.TargetType),
			TargetID:    rl.TargetID,
			Position:    rl.Position,
		}
	}
	return out, nil
}

func toPage(row db.CmsPage) Page {
	var publishedAt *time.Time
	if row.PublishedAt.Valid {
		t := row.PublishedAt.Time
		publishedAt = &t
	}
	var scheduledAt *time.Time
	if row.ScheduledAt.Valid {
		t := row.ScheduledAt.Time
		scheduledAt = &t
	}
	return Page{
		ID:              row.ID,
		PageTypeID:      row.PageTypeID,
		ParentID:        sqlnull.UUIDPtr(row.ParentID),
		Slug:            row.Slug,
		BasePath:        row.BasePath,
		URLPath:         row.UrlPath,
		ExperimentKey:   row.ExperimentKey,
		Position:        row.Position,
		IsPublished:     row.IsPublished,
		SEONoindex:      row.SeoNoindex,
		PublishedAt:     publishedAt,
		ScheduledAt:     scheduledAt,
		Data:            row.Data,
		Metadata:        metadataFromJSON(row.Metadata),
		PrivateMetadata: metadataFromJSON(row.PrivateMetadata),
		Tags:            tagsOrEmpty(row.Tags),
		WorkflowState:   WorkflowState(row.WorkflowState),
		AssignedTo:      sqlnull.UUIDPtr(row.AssignedTo),
		SubmittedBy:     sqlnull.UUIDPtr(row.SubmittedBy),
		SubmittedAt:     sqlnull.TimePtr(row.SubmittedAt),
		ApprovedBy:      sqlnull.UUIDPtr(row.ApprovedBy),
		ApprovedAt:      sqlnull.TimePtr(row.ApprovedAt),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

// tagsOrEmpty normalizes a nullable TEXT[] read into a non-nil slice so the
// domain/GraphQL contract (tags is always a list) holds even for a NULL column
func tagsOrEmpty(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}

// normalizeTags cleans a tag set before persisting: each tag becomes the slug the vocabulary stores (so "Sale"/"sale" filter
// as one), empties drop, dedupe preserves first-seen order, and the set is capped to a sane ceiling - Returns a non-nil slice
func normalizeTags(in []string) []string {
	const maxTags = 50
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, t := range in {
		t = slugifyTag(t)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
		if len(out) >= maxTags {
			break
		}
	}
	return out
}

func toPageContent(row db.CmsPageContent) PageContent {
	var body json.RawMessage
	if row.Body.Valid {
		body = row.Body.RawMessage
	}
	return PageContent{
		Locale: row.Locale, Title: row.Title, Body: body,
		MetaTitle:       sqlnull.StringPtr(row.MetaTitle),
		MetaDescription: sqlnull.StringPtr(row.MetaDescription),
		MetaKeywords:    sqlnull.StringPtr(row.MetaKeywords),
		Data:            row.Data,
		UpdatedAt:       row.UpdatedAt,
	}
}

// Every locale's row per page, keyed by id - one with no rows is simply absent
func (r *postgresRepository) ListPageContentFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]PageContent, error) {
	out := make(map[uuid.UUID][]PageContent, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListCMSPageContentForPages(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("page content by ids: %w", err)
	}
	for _, row := range rows {
		out[row.PageID] = append(out[row.PageID], toPageContent(row))
	}
	return out, nil
}
