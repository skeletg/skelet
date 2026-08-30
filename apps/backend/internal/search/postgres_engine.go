package search

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/config"
)

// CatalogSearchSource is the slice of the catalog service the Postgres engine needs - an interface so search doesn't
// take a hard dependency on the concrete service and tests can stand it up cheaply
type CatalogSearchSource interface {
	BrowseFallback(ctx context.Context, f catalog.BrowseFallbackFilter) ([]uuid.UUID, int64, error)
	SearchFallback(ctx context.Context, channelID uuid.UUID, locale, query string, limit, offset int32) ([]catalog.SearchHit, int64, error)
	FacetCounts(ctx context.Context, targetAttributeID uuid.UUID, f catalog.FacetCountFilter) ([]catalog.FacetBucketCount, error)
	ListAttributes(ctx context.Context) ([]catalog.Attribute, error)
}

// PostgresEngine answers discovery straight from the catalog tables. It exists so
// the storefront has a real floor: browse is the ONLY product-listing path, so
// without a second tier a missing or downed cluster renders every category,
// collection and PLP as an empty catalogue.
//
// It is deliberately narrower than the index: relevance is ts_rank rather than a
// tuned score, merch boosts don't apply, the tsvector is 'simple' so nothing stems,
// and only selectable attributes are faceted. The point is a shoppable store, not
// parity.
type PostgresEngine struct {
	catalog CatalogSearchSource
	// nil-safe (helpers fall back to registry defaults) - without it this tier ignored the operator's limit and min-query settings while the
	// dashboard presented them as live, and on the shipped default THIS tier is the one actually serving
	cfg *config.Service
}

func NewPostgresEngine(c CatalogSearchSource, cfg *config.Service) *PostgresEngine {
	return &PostgresEngine{catalog: c, cfg: cfg}
}

func (e *PostgresEngine) Name() string { return "postgres" }

func (e *PostgresEngine) SearchProducts(ctx context.Context, query string, channelID uuid.UUID, locale string, limit, offset int, _ map[uuid.UUID]float64) ([]ProductHit, int64, error) {
	// Merch boosts are ignored - they're multiplicative factors over a Lucene score
	// with no ts_rank equivalent. Synonym expansion and redirects still apply, since
	// those run in the resolver before the engine is called.
	// Same operator knobs the index tier honors, applied here so both tiers agree on
	// what "default" and "max" mean - the SQL's own clamp stays as the last resort
	limitDefault := cfgIntOr(ctx, e.cfg, pathLimitDefault, 20)
	limitMax := cfgIntOr(ctx, e.cfg, pathLimitMax, 100)
	if limit <= 0 {
		limit = int(limitDefault)
	} else if int64(limit) > limitMax {
		limit = int(limitMax)
	}
	if utf8.RuneCountInString(strings.TrimSpace(query)) < int(cfgIntOr(ctx, e.cfg, pathMinQueryLength, 1)) {
		return []ProductHit{}, 0, nil
	}
	rows, total, err := e.catalog.SearchFallback(ctx, channelID, locale, query, int32(limit), int32(offset))
	if err != nil {
		return nil, 0, err
	}
	hits := make([]ProductHit, len(rows))
	for i, r := range rows {
		hits[i] = ProductHit{ProductID: r.ProductID, SKU: r.SKU, Name: r.Name, URL: r.URL}
	}
	return hits, total, nil
}

func (e *PostgresEngine) BrowseProducts(ctx context.Context, channelID uuid.UUID, locale, query string, filter BrowseFilter, sortSlug string, limit, offset int, _ map[uuid.UUID]float64) (BrowseResult, error) {
	// PLP page-size knobs, mirrored from the index tier - out_of_stock_display stays index-only for now: this tier's SQL
	// carries no stock signal, and honoring "hide" here would need a salable join it deliberately doesn't have
	pageDefault := cfgIntOr(ctx, e.cfg, pathBrowsePageSizeDefault, 24)
	pageMax := cfgIntOr(ctx, e.cfg, pathBrowsePageSizeMax, 100)
	if limit <= 0 {
		limit = int(pageDefault)
	} else if int64(limit) > pageMax {
		limit = int(pageMax)
	}
	// Resolve the requested slugs to value ids up front - the SQL filters and counts on ids, and a slug that matches nothing must be dropped
	// rather than turned into a clause that matches nothing (which would strand the shopper on an empty page)
	filterable, selected := e.resolveSelection(ctx, filter)

	var allValues []uuid.UUID
	for _, vals := range selected {
		allValues = append(allValues, vals...)
	}
	ids, total, err := e.catalog.BrowseFallback(ctx, catalog.BrowseFallbackFilter{
		ChannelID:     channelID,
		Query:         query,
		Locale:        locale,
		CategoryID:    filter.CategoryID,
		CollectionID:  filter.CollectionID,
		PriceMinCents: filter.PriceMinCents,
		PriceMaxCents: filter.PriceMaxCents,
		Kinds:         filter.Kinds,
		AttrValueIDs:  allValues,
		AttrCount:     int32(len(selected)),
		Sort:          fallbackSortKey(sortSlug),
		Limit:         int32(limit),
		Offset:        int32(offset),
	})
	if err != nil {
		return BrowseResult{}, err
	}
	return BrowseResult{
		IDs:    ids,
		Total:  total,
		Facets: e.buildFacets(ctx, channelID, locale, query, filter, filterable, selected),
	}, nil
}

// The storefront speaks URL slugs ("price-asc") and an operator can rename them in
// config; the fallback SQL matches its own fixed vocabulary. Passing the slug
// through untranslated meant NO branch ever matched, so every sort on this tier -
// the one production runs by default - silently served created_at DESC.
// Underscores are accepted too, since the SQL's own vocabulary is spelled that way
func fallbackSortKey(slug string) string {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(slug, "-", "_"))) {
	case "price_asc":
		return "price_asc"
	case "price_desc":
		return "price_desc"
	case "sku":
		return "sku"
	default:
		// newest, relevance and anything operator-defined - the query's created_at DESC tail
		return ""
	}
}

// resolveSelection maps the request's attribute slugs and value slugs onto catalog
// ids, returning the filterable attributes and the selection keyed by attribute id
func (e *PostgresEngine) resolveSelection(ctx context.Context, filter BrowseFilter) ([]catalog.Attribute, map[uuid.UUID][]uuid.UUID) {
	attrs, err := e.catalog.ListAttributes(ctx)
	if err != nil {
		return nil, nil
	}
	filterable := make([]catalog.Attribute, 0, len(attrs))
	bySlug := make(map[string]catalog.Attribute, len(attrs))
	for _, a := range attrs {
		// Only selectable attributes have value rows to count - scalar attributes are
		// faceted through a separate per-product slug table the index reads and this
		// tier does not, so they're simply not offered here. Private attributes are
		// neither offered nor filterable - the slug lookup below is the probe gate
		if a.Filterable && a.IsPublic && a.InputType.AcceptsValues() {
			filterable = append(filterable, a)
			bySlug[a.Slug] = a
		}
	}
	selected := make(map[uuid.UUID][]uuid.UUID)
	for _, af := range filter.AttributeIn {
		a, ok := bySlug[af.Slug]
		if !ok {
			continue
		}
		idBySlug := make(map[string]uuid.UUID, len(a.Values))
		for _, v := range a.Values {
			idBySlug[v.Slug] = v.ID
		}
		var ids []uuid.UUID
		for _, raw := range af.Values {
			if id, ok := idBySlug[raw]; ok {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			selected[a.ID] = ids
		}
	}
	return filterable, selected
}

// buildFacets counts each filterable attribute separately so the target's own selections can be excluded - one query
// per facet. Any single facet failing is dropped rather than failing the page
func (e *PostgresEngine) buildFacets(
	ctx context.Context,
	channelID uuid.UUID,
	locale string,
	query string,
	filter BrowseFilter,
	filterable []catalog.Attribute,
	selected map[uuid.UUID][]uuid.UUID,
) []FacetGroup {
	out := make([]FacetGroup, 0, len(filterable))
	for _, a := range filterable {
		// Count this facet as though its own filter weren't applied - otherwise
		// ticking one colour collapses the colour panel to that single value
		others := make(map[uuid.UUID][]uuid.UUID, len(selected))
		for attrID, vals := range selected {
			if attrID != a.ID {
				others[attrID] = vals
			}
		}
		buckets, err := e.catalog.FacetCounts(ctx, a.ID, catalog.FacetCountFilter{
			ChannelID:     channelID,
			Query:         query,
			Locale:        locale,
			CategoryID:    filter.CategoryID,
			CollectionID:  filter.CollectionID,
			Kinds:         filter.Kinds,
			OtherSelected: others,
		})
		if err != nil || len(buckets) == 0 {
			continue
		}
		g := FacetGroup{
			Field:            AttributeFieldName(a.Slug) + ".keyword",
			Slug:             a.Slug,
			Name:             a.Name,
			DefaultCollapsed: a.FacetCollapsed,
			SortOrder:        a.SortOrder,
			H1Position:       string(a.H1Position),
			Buckets:          make([]FacetBucket, 0, len(buckets)),
		}
		for _, b := range buckets {
			g.Buckets = append(g.Buckets, FacetBucket{
				Value:       b.Name,
				Slug:        b.Slug,
				IsIndexable: b.IsIndexable,
				Count:       b.Count,
			})
		}
		out = append(out, g)
	}
	return out
}

// ListSortOptions omits relevance - ts_rank only means something with a query, and
// the browse path has no score to order by
func (e *PostgresEngine) ListSortOptions(context.Context) []SortOption {
	return []SortOption{
		{Slug: "newest", Label: "Newest", Field: sortFieldCreatedAt, Direction: sortDirDesc},
		{Slug: "price-asc", Label: "Price: Low to High", Field: sortFieldPrice, Direction: sortDirAsc},
		{Slug: "price-desc", Label: "Price: High to Low", Field: sortFieldPrice, Direction: sortDirDesc},
	}
}

var _ Engine = (*PostgresEngine)(nil)
