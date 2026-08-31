package search

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/skeletg/skelet/apps/backend/internal/config"
)

// One row of the operator-curated sort list on browse pages, mapping a stable URL slug onto an OpenSearch field and direction - it lives in config so the dashboard can reorder or add options without a deploy
// "_score" only means anything when the browse query has a search term, and falls back to a stable product_id tie-breaker otherwise
// attr_<slug>.keyword sorts on a filterable attribute, but numerics sort lexicographically since the indexer stores them as strings - a proper attrnum_<slug> long subfield is deferred
type SortOption struct {
	Slug      string `json:"slug"`
	Label     string `json:"label"`
	Field     string `json:"field"`
	Direction string `json:"direction"` // "asc" | "desc"
	// Operator opt-in for SEO - only indexable sort URLs render as <a href> for Googlebot and stay out of noindex
	// The first option's canonical URL drops the /s/<slug> segment entirely, so leave this false on the defaults and let operators flip the price/newest variants they actually want indexed
	IsIndexable bool `json:"isIndexable"`
}

const (
	sortFieldScore     = "_score"
	sortFieldPrice     = "price_cents"
	sortFieldName      = "name.keyword"
	sortFieldIndexedAt = "indexed_at"
	// created_at is the product's real age - indexed_at is when the doc was last written, so sorting on indexed_at would make a stock tweak on a 3-year-old SKU the "newest" product
	sortFieldCreatedAt = "created_at"

	sortDirAsc  = "asc"
	sortDirDesc = "desc"
)

// "Relevance" must sort by _score or it isn't relevance - it was indexed_at DESC,
// so results were ordered by whichever product was reindexed most recently and
// editing any product's description pushed it to position 1 of every listing.
// _score is meaningless without a text query; browseSortClause drops it then and
// falls through to the tie-break, which is why "newest" stays a separate option.
var defaultSortOptions = []SortOption{
	{Slug: "relevance", Label: "Relevance", Field: sortFieldScore, Direction: sortDirDesc},
	{Slug: "price-asc", Label: "Price: Low to High", Field: sortFieldPrice, Direction: sortDirAsc},
	{Slug: "price-desc", Label: "Price: High to Low", Field: sortFieldPrice, Direction: sortDirDesc},
	{Slug: "newest", Label: "Newest", Field: sortFieldCreatedAt, Direction: sortDirDesc},
}

func loadSortOptions(ctx context.Context, cfg *config.Service) []SortOption {
	if cfg == nil {
		return defaultSortOptions
	}
	raw, err := cfg.GetString(ctx, pathProductsSortOptions, config.DefaultScope)
	if err != nil || raw == "" {
		return defaultSortOptions
	}
	var out []SortOption
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return defaultSortOptions
	}
	if len(out) == 0 {
		return defaultSortOptions
	}
	return out
}

func resolveSortOption(slug string, opts []SortOption) SortOption {
	slug = strings.TrimSpace(slug)
	for _, o := range opts {
		if o.Slug == slug {
			return o
		}
	}
	return opts[0]
}

func (i *ProductIndexer) ListSortOptions(ctx context.Context) []SortOption {
	return loadSortOptions(ctx, i.cfg)
}

// Always appends product_id as a stable tie-breaker so deep pagination doesn't shuffle equal-keyed items between pages
// unmapped_type keeps the query alive when sorting on an attr_<slug> that isn't in the index yet - a new filterable attribute nobody has reindexed for - where OpenSearch would otherwise 400
func buildOSSortClause(opt SortOption) []any {
	dir := opt.Direction
	if dir != sortDirAsc && dir != sortDirDesc {
		dir = sortDirDesc
	}
	primary := map[string]any{
		opt.Field: map[string]any{
			"order":         dir,
			"missing":       "_last",
			"unmapped_type": "keyword",
		},
	}
	// _score rejects the missing/unmapped sub-keys with "[_score] unknown field [missing]" - use the bare short form
	if opt.Field == sortFieldScore {
		primary = map[string]any{opt.Field: map[string]any{"order": dir}}
	}
	return []any{
		primary,
		map[string]any{"product_id": "asc"},
	}
}
