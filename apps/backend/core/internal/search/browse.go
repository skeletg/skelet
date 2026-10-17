package search

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// OpenSearch refuses from+size beyond index.max_result_window (10k by default)
const maxResultWindow = 10000

// All fields optional - zero-valued ones are dropped from the query, and the caller clamps limit/offset
type BrowseFilter struct {
	CategoryID    *uuid.UUID
	CollectionID  *uuid.UUID
	AttributeIn   []AttributeIn // one entry per attribute slug with OR'd values
	PriceMinCents *int64
	PriceMaxCents *int64
	Kinds         []string // e.g. ["SIMPLE", "BUNDLE"] - empty = all
}

// "attr_<slug>.keyword IN (values…)" - entries AND together, values inside one entry OR together
type AttributeIn struct {
	Slug   string
	Values []string
}

// One value of one facet, counted under the current filter set MINUS that facet so picking it doesn't zero the sibling buckets - the per-facet filter agg in buildFacetAggregations does the exclusion Slug is the AttributeValue.slug for selectable attrs (stable across renames) and catalog.Slugify(value) for scalars (derived, so it shifts if a typo is fixed) Unique within a facet group but not across attributes - always pair with the parent facet's slug before round-tripping in a URL
type FacetBucket struct {
	Value string
	Slug  string
	// Mirrors AttributeValue.is_indexable or catalog_attribute_scalar_slugs.is_indexable - false for a scalar with no override row, so the operator has to opt in
	IsIndexable bool
	// Locale-resolved AttributeValueContent for selectable buckets, empty for scalars - the front resolver fills it after the OS query so the indexer needs no translation awareness
	Description string
	Count       int64
}

// Buckets for a single attribute slug - Field is the OpenSearch field (attr_color.keyword), Slug is the catalog attribute slug (color)
// Name, DefaultCollapsed and SortOrder pass through catalog.Attribute so the storefront can label the panel and pick its initial state
type FacetGroup struct {
	Field            string
	Slug             string
	Name             string
	DefaultCollapsed bool
	SortOrder        int32
	// Passes through Attribute.H1Position so the storefront knows whether an indexable filter value lands before or after the base name in the H1
	H1Position string
	// Locale-resolved AttributeContent for the facet panel header - the front resolver fills it, empty when untranslated for the active locale
	Description string
	Buckets     []FacetBucket
}

// Product hydration happens at the GraphQL resolver - returning ids keeps this package decoupled from catalog.Service for reads
type BrowseResult struct {
	IDs    []uuid.UUID
	Total  int64
	Facets []FacetGroup
}

// ApplyFacetOverrides applies one category's filter-presentation rows to a browse result: hidden facets drop, pinned facets sort by their override position ahead of every unpinned one, and unpinned facets keep their incoming relative order. Applied at the resolver seam so every engine tier (OpenSearch, Postgres, remote) gets the same treatment
func ApplyFacetOverrides(facets []FacetGroup, order map[string]int32, hidden map[string]bool) []FacetGroup {
	if len(order) == 0 && len(hidden) == 0 {
		return facets
	}
	kept := make([]FacetGroup, 0, len(facets))
	for _, fg := range facets {
		if !hidden[fg.Slug] {
			kept = append(kept, fg)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		oi, iOK := order[kept[i].Slug]
		oj, jOK := order[kept[j].Slug]
		switch {
		case iOK && jOK:
			return oi < oj
		default:
			// A pinned facet outranks an unpinned one; two unpinned keep incoming order
			return iOK && !jOK
		}
	})
	return kept
}

// A filter clause plus which facet it belongs to - per-facet aggregations skip the clause tagged with their own slug so picking a value doesn't collapse that facet's other buckets
type taggedFilter struct {
	tag    string
	clause any
}

// Faceted browse against the product index, always scoped to the active (store, locale, is_published) triple with everything else layered on top Each filterable attribute gets ONE facet aggregation applying every active filter EXCEPT its own, so ticking a color doesn't collapse the size facet - the Algolia-style counts UX Composed with OpenSearch's filter aggregation primitive so the per-facet filter sets stay independent
func (i *ProductIndexer) BrowseProducts(
	ctx context.Context,
	channelID uuid.UUID,
	locale string,
	query string,
	filter BrowseFilter,
	sortSlug string,
	limit, offset int,
	// Same merchandising the search box gets - without it the /search page ranked differently from the typeahead the
	// shopper just clicked out of, and the operator's sales-weight / in-stock-boost knobs were dead on every PLP
	merchBoosts map[uuid.UUID]float64,
) (BrowseResult, error) {
	pageDefault := cfgIntOr(ctx, i.cfg, pathBrowsePageSizeDefault, 24)
	pageMax := cfgIntOr(ctx, i.cfg, pathBrowsePageSizeMax, 100)
	if limit <= 0 {
		limit = int(pageDefault)
	} else if int64(limit) > pageMax {
		// Clamp an over-max request to the configured ceiling rather than discarding it to the default
		limit = int(pageMax)
	}
	if offset < 0 {
		offset = 0
	}
	// from + size past max_result_window makes OpenSearch error, which the resolver soft-fails into an EMPTY page - so a deep-paged URL the
	// site itself linked to rendered "no products match". Clamp instead so the last reachable page is served
	if int64(offset)+int64(limit) > maxResultWindow {
		offset = max(int(maxResultWindow)-limit, 0)
	}
	query = strings.TrimSpace(query)

	// An external engine answers faceted browse when configured, failing open to OpenSearch below - the front resolver enriches its result exactly as it does the native one
	if i.remoteSearch != nil {
		if res, ok := i.remoteSearch.Browse(ctx, channelID, locale, query, filter, sortSlug, limit, offset); ok {
			return res, nil
		}
	}

	facetMax := int(cfgIntOr(ctx, i.cfg, pathBrowseFacetMaxValues, 50))
	oos := cfgStringOr(ctx, i.cfg, pathOutOfStockDisplay, oosShow)

	// Filterable=true means the operator marked the attribute suitable for faceted browsing - a lookup failure degrades to hits with no facets
	// IsPublic gates too: a private attribute must not surface as a facet nor be probeable through the filter input
	attrs, _ := listAttributes(ctx, i.attrs, i.catalog)
	filterable := make([]catalog.Attribute, 0, len(attrs))
	for _, a := range attrs {
		if a.Filterable && a.IsPublic {
			filterable = append(filterable, a)
		}
	}
	// Index by slug so per-bucket resolution and slug-to-display translation both read one map instead of re-walking the slice
	attrBySlug := make(map[string]catalog.Attribute, len(filterable))
	for _, a := range filterable {
		attrBySlug[a.Slug] = a
	}

	storeLocaleFilter := []any{
		map[string]any{"term": map[string]any{"channel_id": channelID.String()}},
		map[string]any{"term": map[string]any{"locale": locale}},
		map[string]any{"term": map[string]any{"is_published": true}},
	}
	// "hide" drops out-of-stock from results AND facet counts - it goes in the base filter so the main query and the per-facet aggs stay consistent
	if oos == oosHide {
		storeLocaleFilter = append(storeLocaleFilter, map[string]any{"term": map[string]any{"is_in_stock": true}})
	}

	// Each clause is tagged with the attribute slug it belongs to (or _category / _collection / _price / _kind) - per-facet aggs exclude the matching tag
	clauses := make([]taggedFilter, 0)

	if filter.CategoryID != nil {
		clauses = append(clauses, taggedFilter{
			tag:    "_category",
			clause: map[string]any{"term": map[string]any{"category_ids": filter.CategoryID.String()}},
		})
	}
	if filter.CollectionID != nil {
		clauses = append(clauses, taggedFilter{
			tag:    "_collection",
			clause: map[string]any{"term": map[string]any{"collection_ids": filter.CollectionID.String()}},
		})
	}
	if filter.PriceMinCents != nil || filter.PriceMaxCents != nil {
		rng := map[string]any{}
		if filter.PriceMinCents != nil {
			rng["gte"] = *filter.PriceMinCents
		}
		if filter.PriceMaxCents != nil {
			rng["lte"] = *filter.PriceMaxCents
		}
		clauses = append(clauses, taggedFilter{
			tag:    "_price",
			clause: map[string]any{"range": map[string]any{"price_cents": rng}},
		})
	}
	if len(filter.Kinds) > 0 {
		clauses = append(clauses, taggedFilter{
			tag:    "_kind",
			clause: map[string]any{"terms": map[string]any{"kind": filter.Kinds}},
		})
	}
	// Bound the fan-out. Each entry becomes a clause in the main query AND is re-embedded in every per-facet aggregation, so N entries against M filterable attributes is N*M bool clauses - a 256KB request body could otherwise expand into hundreds of thousands of clauses and blow max_clause_count on the cluster, which the resolver then soft-fails into an empty page
	const maxAttributeFilters = 24
	const maxValuesPerFilter = 64
	if len(filter.AttributeIn) > maxAttributeFilters {
		filter.AttributeIn = filter.AttributeIn[:maxAttributeFilters]
	}
	for _, af := range filter.AttributeIn {
		if af.Slug == "" || len(af.Values) == 0 {
			continue
		}
		if len(af.Values) > maxValuesPerFilter {
			af.Values = af.Values[:maxValuesPerFilter]
		}
		// An unknown slug would build a clause matching nothing, so a typo'd or stale filter URL rendered an empty page with
		// no facet chip to undo it. Ignoring it degrades to a wider result the shopper can actually see
		if _, known := attrBySlug[af.Slug]; !known {
			continue
		}
		// Selectable attrs filter on attr_<slug>.keyword after looking up the display name, since operator slugs may differ from slugify(name) - size "Large" can be slug "l"
		// Scalar attrs filter straight on attrslug_<slug>.keyword, the per-product URL slug the indexer wrote
		a := attrBySlug[af.Slug]
		if a.InputType.AcceptsValues() {
			field := AttributeFieldName(af.Slug) + ".keyword"
			resolved := resolveAttributeFilterValues(a, af.Values)
			clauses = append(clauses, taggedFilter{
				tag:    af.Slug,
				clause: map[string]any{"terms": map[string]any{field: resolved}},
			})
			continue
		}
		clauses = append(clauses, taggedFilter{
			tag: af.Slug,
			clause: map[string]any{
				"terms": map[string]any{AttributeSlugFieldName(af.Slug): af.Values},
			},
		})
	}

	allFilters := append([]any{}, storeLocaleFilter...)
	for _, c := range clauses {
		allFilters = append(allFilters, c.clause)
	}

	// With a query string, reuse the multi_match clause SearchProducts builds and put it in must; without one the body is filter-only and OpenSearch returns the configured sort order
	// The search clause is captured into facetBase too, so the per-facet aggs re-apply it - otherwise facets would count across the whole store instead of within the result set
	boolBody := map[string]any{"filter": allFilters}
	var facetSearchClause any
	if query != "" {
		// Reuses the ListAttributes result from above - the second call this replaced re-fetched every attribute plus every
		// value row (a 141-value attribute in the wild) once more per searched browse
		searchable := make([]catalog.Attribute, 0, len(attrs))
		for _, a := range attrs {
			if a.Searchable {
				searchable = append(searchable, a)
			}
		}
		queryAttrs := mergeWithCatalogAttributes(
			loadAttributes(ctx, i.cfg, pathProductsAttributes, defaultProductAttributes),
			searchable,
		)
		facetSearchClause = buildSearchQuery(query, queryAttrs)
		boolBody["must"] = []any{facetSearchClause}
	}
	facetBase := append([]any{}, storeLocaleFilter...)
	if facetSearchClause != nil {
		facetBase = append(facetBase, facetSearchClause)
	}

	// Wrapping in a global agg lets each per-facet filter re-apply the global set minus its own tag - inherit the top-level query instead and a color=X search shrinks the color facet to just X
	body := map[string]any{
		"from":             offset,
		"size":             limit,
		"_source":          []string{"product_id"},
		"track_total_hits": true,
		// Business ranking and merch boosts only mean anything against a scored query -
		// a filter-only browse has a constant score, so they'd be noise
		"query": browseScoredQuery(ctx, i.cfg, map[string]any{"bool": boolBody}, query, merchBoosts),
		"sort":  browseSortClause(oos, buildOSSortClause(browseSortOption(ctx, i.cfg, sortSlug, query))),
		"aggs": map[string]any{
			"facets_global": map[string]any{
				"global": map[string]any{},
				"aggs":   buildFacetAggregations(filterable, facetBase, clauses, facetMax),
			},
		},
	}

	rawResp, err := i.client.SearchRawRouted(ctx, ProductIndex, i.routing(channelID), body)
	if err != nil {
		return BrowseResult{}, err
	}

	ids := make([]uuid.UUID, 0, len(rawResp.Hits.Hits))
	seen := make(map[uuid.UUID]struct{}, len(rawResp.Hits.Hits))
	for _, h := range rawResp.Hits.Hits {
		var src struct {
			ProductID uuid.UUID `json:"product_id"`
		}
		if err := json.Unmarshal(h.Source, &src); err != nil {
			continue
		}
		// One product can have several docs (one per store/locale) - we already filter to one locale, but the dedupe is cheap and keeps hydration clean
		if _, ok := seen[src.ProductID]; ok {
			continue
		}
		seen[src.ProductID] = struct{}{}
		ids = append(ids, src.ProductID)
	}

	facets := parseFacetAggregations(filterable, rawResp.Aggregations)
	// Selectable attrs walk a.Values for the matching display name to pull AttributeValue.slug and is_indexable
	// Scalar aggregation keys are already the slug, with display value and indexability surfaced from sub-aggs - the attribute-level master switch then gates the per-bucket flag, both must be true
	for fi, fg := range facets {
		a := attrBySlug[fg.Slug]
		if a.InputType.AcceptsValues() {
			for bi, b := range fg.Buckets {
				slug, indexable := selectableBucketMeta(a, b.Value)
				facets[fi].Buckets[bi].Slug = slug
				facets[fi].Buckets[bi].IsIndexable = a.IsIndexable && indexable
			}
		} else {
			for bi := range fg.Buckets {
				facets[fi].Buckets[bi].IsIndexable = a.IsIndexable && facets[fi].Buckets[bi].IsIndexable
			}
		}
	}

	return BrowseResult{
		IDs:    ids,
		Total:  int64(rawResp.Hits.Total.Value),
		Facets: facets,
	}, nil
}

// Prepends an is_in_stock-descending tiebreaker in "last" mode so out-of-stock sinks below in-stock while the configured order holds within each group browseSortOption resolves the requested sort, downgrading _score to recency when there is no text query - every doc scores the same on a pure filter browse, so sorting by score would leave the order to the tie-break alone browseScoredQuery layers the same ranking SearchProducts applies, but only when there is a text query to score against
func browseScoredQuery(ctx context.Context, cfg *config.Service, q map[string]any, query string, boosts map[uuid.UUID]float64) map[string]any {
	if strings.TrimSpace(query) == "" {
		return q
	}
	return applyMerchBoosts(applyBusinessRanking(ctx, cfg, q), boosts)
}

func browseSortOption(ctx context.Context, cfg *config.Service, sortSlug, query string) SortOption {
	opt := resolveSortOption(sortSlug, loadSortOptions(ctx, cfg))
	if opt.Field == sortFieldScore && strings.TrimSpace(query) == "" {
		return SortOption{Slug: opt.Slug, Label: opt.Label, Field: sortFieldCreatedAt, Direction: sortDirDesc}
	}
	return opt
}

func browseSortClause(oos string, base []any) []any {
	if oos != oosLast {
		return base
	}
	return append([]any{
		map[string]any{"is_in_stock": map[string]any{"order": "desc"}},
	}, base...)
}

// One filter aggregation per filterable attribute, each wrapping the global filters MINUS its own tag so picking a color doesn't shrink the size facet
// Selectable attrs get a terms agg on attr_<slug>.keyword with slug and indexability resolved Go-side; scalars get one on attrslug_<slug> plus sub-aggs for the display value and the flag
func buildFacetAggregations(
	filterable []catalog.Attribute,
	storeLocaleFilter []any,
	clauses []taggedFilter,
	maxValues int,
) map[string]any {
	aggs := make(map[string]any, len(filterable))
	for _, a := range filterable {
		applied := append([]any{}, storeLocaleFilter...)
		for _, c := range clauses {
			if c.tag == a.Slug {
				continue
			}
			applied = append(applied, c.clause)
		}
		var valuesAgg map[string]any
		if a.InputType.AcceptsValues() {
			valuesAgg = map[string]any{
				"terms": map[string]any{
					"field": AttributeFieldName(a.Slug) + ".keyword",
					"size":  maxValues, // search/browse/facet_max_values; UI clamps further
				},
			}
		} else {
			// size=1 keeps the sub-agg cheap - the display string is uniform within a slug bucket by construction, since the indexer writes the same attr_<slug> value alongside the slug
			valuesAgg = map[string]any{
				"terms": map[string]any{
					"field": AttributeSlugFieldName(a.Slug),
					"size":  maxValues,
				},
				"aggs": map[string]any{
					"display": map[string]any{
						"terms": map[string]any{
							"field": AttributeFieldName(a.Slug) + ".keyword",
							"size":  1,
						},
					},
					"indexable": map[string]any{
						"terms": map[string]any{
							"field": AttributeIndexableFieldName(a.Slug),
							"size":  1,
						},
					},
				},
			}
		}
		aggs["facet_"+a.Slug] = map[string]any{
			"filter": map[string]any{"bool": map[string]any{"filter": applied}},
			"aggs": map[string]any{
				"values": valuesAgg,
			},
		}
	}
	return aggs
}

// Walks the per-attribute aggregations inside the facets_global wrapper and assembles one FacetGroup each Selectable bucket keys are display values, with Slug and IsIndexable filled in later by selectableBucketMeta; scalar bucket keys are slugs and both fields are populated here Missing aggregations are silently skipped - a degraded response shouldn't error the page
func parseFacetAggregations(
	filterable []catalog.Attribute,
	raw map[string]json.RawMessage,
) []FacetGroup {
	outerBlob, ok := raw["facets_global"]
	if !ok {
		return nil
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(outerBlob, &outer); err != nil {
		return nil
	}
	out := make([]FacetGroup, 0, len(filterable))
	for _, a := range filterable {
		key := "facet_" + a.Slug
		blob, ok := outer[key]
		if !ok {
			continue
		}
		var parsed struct {
			Values struct {
				Buckets []struct {
					Key      any   `json:"key"`
					DocCount int64 `json:"doc_count"`
					Display  *struct {
						Buckets []struct {
							Key string `json:"key"`
						} `json:"buckets"`
					} `json:"display,omitempty"`
					Indexable *struct {
						Buckets []struct {
							Key any `json:"key"`
						} `json:"buckets"`
					} `json:"indexable,omitempty"`
				} `json:"buckets"`
			} `json:"values"`
		}
		if err := json.Unmarshal(blob, &parsed); err != nil {
			continue
		}
		buckets := make([]FacetBucket, 0, len(parsed.Values.Buckets))
		isSelectable := a.InputType.AcceptsValues()
		for _, b := range parsed.Values.Buckets {
			keyStr := bucketKeyToString(b.Key)
			if strings.TrimSpace(keyStr) == "" {
				continue
			}
			if isSelectable {
				buckets = append(buckets, FacetBucket{Value: keyStr, Count: b.DocCount})
				continue
			}
			// keyStr is the slug here - take the display sub-bucket's first key as the label, and let bucketKeyToBool handle the boolean, which OpenSearch reports as 1 or "true"
			display := ""
			if b.Display != nil && len(b.Display.Buckets) > 0 {
				display = b.Display.Buckets[0].Key
			}
			if display == "" {
				display = keyStr
			}
			indexable := false
			if b.Indexable != nil && len(b.Indexable.Buckets) > 0 {
				indexable = bucketKeyToBool(b.Indexable.Buckets[0].Key)
			}
			buckets = append(buckets, FacetBucket{
				Value:       display,
				Slug:        keyStr,
				IsIndexable: indexable,
				Count:       b.DocCount,
			})
		}
		if len(buckets) == 0 {
			continue
		}
		out = append(out, FacetGroup{
			Field:            AttributeFieldName(a.Slug) + ".keyword",
			Slug:             a.Slug,
			Name:             a.Name,
			DefaultCollapsed: a.FacetCollapsed,
			SortOrder:        a.SortOrder,
			H1Position:       string(a.H1Position),
			Buckets:          buckets,
		})
	}
	return out
}

// A terms-agg key may be a string, number or boolean depending on field type - booleans come back as 0/1 on some OS versions
func bucketKeyToString(key any) string {
	switch v := key.(type) {
	case string:
		return v
	case float64:
		// Non-integer floats shouldn't turn up as keyword keys, but stringify defensively
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}

func bucketKeyToBool(key any) bool {
	switch v := key.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	case float64:
		return v != 0
	default:
		return false
	}
}

// Walks the loaded a.Values for the matching display name to return the operator-blessed slug and flag
// Falls back to catalog.Slugify(value) on no match, which should only happen during a brief index lag after a rename
func selectableBucketMeta(a catalog.Attribute, value string) (string, bool) {
	for _, v := range a.Values {
		if v.Name == value {
			return v.Slug, v.IsIndexable
		}
	}
	return catalog.Slugify(value), false
}

// Maps slug-shaped inputs to the display strings stored on the doc - selectable attrs look up the AttributeValue by slug, scalars have no slug record so v1 passes through That means a scalar slug input only matches when the slug equals the indexed value exactly, e.g. a single-word value like "handmade" Unmatched input passes through unchanged, keeping admin GraphQL and CLI callers that already send display values working
func resolveAttributeFilterValues(a catalog.Attribute, in []string) []string {
	if !a.InputType.AcceptsValues() || len(a.Values) == 0 {
		return in
	}
	bySlug := make(map[string]string, len(a.Values))
	for _, v := range a.Values {
		bySlug[v.Slug] = v.Name
	}
	out := make([]string, len(in))
	for i, raw := range in {
		if name, ok := bySlug[raw]; ok {
			out[i] = name
		} else {
			out[i] = raw
		}
	}
	return out
}
