package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

type BrowseFallbackFilter struct { // the subset of the index's browse contract Postgres can answer - relevance scoring is deliberately absent, this keeps the store shoppable when OpenSearch is down rather than replacing it
	ChannelID     uuid.UUID
	Query         string
	Locale        string // which content rows Query matches against - an empty one narrows the search back to SKU
	CategoryID    *uuid.UUID
	CollectionID  *uuid.UUID
	PriceMinCents *int64
	PriceMaxCents *int64
	Kinds         []string
	AttrValueIDs  []uuid.UUID // value ids across ALL attributes, flattened
	AttrCount     int32       // how many distinct attributes they span - a product must match at least one value in every one of them
	Sort          string      // price_asc | price_desc | sku - anything else falls back to newest
	Limit         int32
	Offset        int32
}

func (s *productService) BrowseFallback(ctx context.Context, f BrowseFallbackFilter) ([]uuid.UUID, int64, error) { // returns ids so the caller hydrates through the same path the indexed browse uses
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 24
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	params := db.BrowseProductsFallbackParams{
		ChannelID:    f.ChannelID,
		Kinds:        f.Kinds,
		Search:       strings.TrimSpace(f.Query), // trimmed like FacetCounts does, or a padded query filters the grid and not the counts
		Locale:       f.Locale,
		Sort:         f.Sort,
		Lim:          f.Limit,
		Off:          f.Offset,
		AttrValueIds: f.AttrValueIDs,
		AttrCount:    f.AttrCount,
	}
	if params.AttrValueIds == nil {
		params.AttrValueIds = []uuid.UUID{}
	}
	if f.CategoryID != nil {
		params.FilterCategory, params.CategoryID = true, *f.CategoryID
	}
	if f.CollectionID != nil {
		params.FilterCollection, params.CollectionID = true, *f.CollectionID
	}
	if f.PriceMinCents != nil {
		params.FilterMin, params.PriceMin = true, sql.NullInt64{Int64: *f.PriceMinCents, Valid: true}
	}
	if f.PriceMaxCents != nil {
		params.FilterMax, params.PriceMax = true, sql.NullInt64{Int64: *f.PriceMaxCents, Valid: true}
	}
	if params.Kinds == nil {
		params.Kinds = []string{}
	}
	rows, err := s.repo.BrowseFallback(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("browse fallback: %w", err)
	}
	ids := make([]uuid.UUID, len(rows))
	var total int64
	for i, r := range rows {
		ids[i] = r.ID
		total = r.TotalCount
	}
	return ids, total, nil
}

type SearchHit struct { // deliberately the same shape the index returns, so the engine layer maps it without a second lookup
	ProductID uuid.UUID
	SKU       string
	Name      string
	URL       string
}

// Ranked over product_content.search_tsv with an ILIKE arm for partial words - the vector is built with the unstemmed 'simple' config because the content is multi-locale and the column is an immutable generated one
func (s *productService) SearchFallback(ctx context.Context, channelID uuid.UUID, locale, query string, limit, offset int32) ([]SearchHit, int64, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []SearchHit{}, 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.repo.SearchFallback(ctx, db.SearchProductsFallbackParams{
		Q:         query,
		ChannelID: channelID,
		Locale:    locale,
		Lim:       limit,
		Off:       offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("search fallback: %w", err)
	}
	out := make([]SearchHit, len(rows))
	var total int64
	for i, r := range rows {
		out[i] = SearchHit{ProductID: r.ID, SKU: r.Sku, Name: r.Name, URL: r.Url}
		total = r.TotalCount
	}
	return out, total, nil
}

type FacetBucketCount struct { // one attribute value plus how many products carry it under the current filters
	ValueID     uuid.UUID
	Slug        string
	Name        string
	IsIndexable bool
	Count       int64
}

type FacetCountFilter struct {
	ChannelID     uuid.UUID
	Query         string
	Locale        string // must match the browse call's locale or the counts describe a different result set
	CategoryID    *uuid.UUID
	CollectionID  *uuid.UUID
	Kinds         []string
	OtherSelected map[uuid.UUID][]uuid.UUID // attribute id -> selected value ids, EXCLUDING the counted attribute - the CALLER does that exclusion, so the semantics are explicit at the call site and unit-testable
	MaxValues     int32
}

// One query per facet - at the scale this tier targets that is cheaper than it sounds (a full GROUP BY over the EAV join measures well under a millisecond), and it keeps the multi-select exclusion expressible without building SQL per request
func (s *productService) FacetCounts(ctx context.Context, targetAttributeID uuid.UUID, f FacetCountFilter) ([]FacetBucketCount, error) {
	if f.MaxValues <= 0 || f.MaxValues > 200 {
		f.MaxValues = 50
	}
	var otherValues []uuid.UUID
	otherAttrs := 0
	for _, vals := range f.OtherSelected {
		if len(vals) == 0 {
			continue
		}
		otherAttrs++
		otherValues = append(otherValues, vals...)
	}
	if otherValues == nil {
		otherValues = []uuid.UUID{}
	}
	params := db.BrowseFacetCountsFallbackParams{
		TargetAttributeID: targetAttributeID,
		OtherValueIds:     otherValues,
		OtherAttrCount:    int32(otherAttrs),
		MaxValues:         f.MaxValues,
		ChannelID:         f.ChannelID,
		Kinds:             f.Kinds,
		Search:            strings.TrimSpace(f.Query),
		Locale:            f.Locale,
	}
	if params.Kinds == nil {
		params.Kinds = []string{}
	}
	if f.CategoryID != nil {
		params.FilterCategory, params.CategoryID = true, *f.CategoryID
	}
	if f.CollectionID != nil {
		params.FilterCollection, params.CollectionID = true, *f.CollectionID
	}
	rows, err := s.repo.FacetCounts(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("facet counts: %w", err)
	}
	out := make([]FacetBucketCount, len(rows))
	for i, r := range rows {
		out[i] = FacetBucketCount{ValueID: r.ValueID, Slug: r.Slug, Name: r.Name, IsIndexable: r.IsIndexable, Count: r.ProductCount}
	}
	return out, nil
}
