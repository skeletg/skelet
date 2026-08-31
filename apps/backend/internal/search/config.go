package search

import (
	"encoding/json"

	"github.com/skeletg/skelet/apps/backend/internal/config"
)

// Runtime knobs for OpenSearch - the indexers read them at boot for mapping construction and at request time for queries, so an operator can retune from /settings without a redeploy
// The per-attribute tables are the heart of it: every searchable field carries its own {searchable, weight, matchType, fuzziness}, Algolia-style control on OpenSearch primitives - All at default scope, single cluster

const (
	pathLimitDefault      = "search/general/result_limit_default"
	pathLimitMax          = "search/general/result_limit_max"
	pathMinQueryLength    = "search/general/min_query_length"
	pathOutOfStockDisplay = "search/general/out_of_stock_display"

	// PLP pagination for browseProducts, distinct from the typeahead limits above - the default is Public so the storefront can size its grid
	pathBrowsePageSizeDefault = "search/browse/page_size_default"
	pathBrowsePageSizeMax     = "search/browse/page_size_max"
	pathBrowseFacetMaxValues  = "search/browse/facet_max_values"
	// Public: the storefront picks its pager UI from it - load_more/hybrid render ?page=N
	// cumulatively (items 1..N), so every state stays a crawlable server-rendered URL
	pathBrowsePaginationStyle = "search/browse/pagination_style"

	oosShow = "show"
	oosHide = "hide"
	oosLast = "last"

	pathProductsAttributes  = "search/products/attributes"
	pathProductsSortOptions = "search/products_sort/options"
	pathBlogAttributes      = "search/blog/attributes"

	pathMappingAnalyzer     = "search/mapping/analyzer"
	pathMappingEdgeNgramMin = "search/mapping/edge_ngram_min"
	pathMappingEdgeNgramMax = "search/mapping/edge_ngram_max"
	pathMappingShards       = "search/mapping/shards"
	pathMappingReplicas     = "search/mapping/replicas"
)

// multi_match is a standard analyzed match and supports fuzziness; bool_prefix is a last-token prefix with no fuzziness, for typeahead
// match_phrase_prefix is the stricter "exact start" form, and exact is term-level for SKU / IDs where a partial match makes no sense
const (
	matchMultiMatch       = "multi_match"
	matchBoolPrefix       = "bool_prefix"
	matchPhrasePrefix     = "match_phrase_prefix"
	matchExact            = "exact"
	analyzerStandard      = "standard"
	analyzerEdgeNgram     = "edge_ngram"
	defaultAttrMatchType  = matchMultiMatch
	defaultAttrFuzziness  = "AUTO"
	defaultAttrSearchable = true
)

// Name MUST equal a key in ProductDoc / BlogPostDoc, or the query builder silently skips the row
type Attribute struct {
	Name       string `json:"name"`
	Searchable bool   `json:"searchable"`
	Weight     int    `json:"weight"`
	MatchType  string `json:"matchType"`
	Fuzziness  string `json:"fuzziness"`
}

// Preserve pre-config behavior (name^3 sku^2 description, all fuzzy) so a fresh install matches the hardcoded queries these replaced
var defaultProductAttributes = []Attribute{
	{Name: "name", Searchable: true, Weight: 3, MatchType: matchMultiMatch, Fuzziness: "AUTO"},
	{Name: "sku", Searchable: true, Weight: 2, MatchType: matchExact, Fuzziness: ""},
	{Name: "description", Searchable: true, Weight: 1, MatchType: matchMultiMatch, Fuzziness: "AUTO"},
}

var defaultBlogAttributes = []Attribute{
	{Name: "title", Searchable: true, Weight: 3, MatchType: matchMultiMatch, Fuzziness: "AUTO"},
	{Name: "excerpt", Searchable: true, Weight: 2, MatchType: matchMultiMatch, Fuzziness: "AUTO"},
	{Name: "body", Searchable: true, Weight: 1, MatchType: matchMultiMatch, Fuzziness: "AUTO"},
	{Name: "author_name", Searchable: false, Weight: 1, MatchType: matchExact, Fuzziness: ""},
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

var analyzerOptions = []config.Option{
	{Value: analyzerStandard, Label: "standard"},
	{Value: analyzerEdgeNgram, Label: "edge n-gram (prefix tokens)"},
}

func init() {
	config.RegisterSection(config.Section{Slug: "search", Label: "Search", SortOrder: 4})

	config.RegisterGroup(config.Group{Section: "search", Slug: "general", Label: "General", SortOrder: 1})
	config.RegisterGroup(config.Group{Section: "search", Slug: "products", Label: "Product Search", SortOrder: 2})
	config.RegisterGroup(config.Group{Section: "search", Slug: "products_sort", Label: "Product Sort", SortOrder: 3})
	config.RegisterGroup(config.Group{Section: "search", Slug: "browse", Label: "Product Listing (PLP)", SortOrder: 4})
	config.RegisterGroup(config.Group{Section: "search", Slug: "blog", Label: "Blog Search", SortOrder: 5})
	config.RegisterGroup(config.Group{Section: "search", Slug: "mapping", Label: "Index Mapping (reindex required)", SortOrder: 6})

	config.Register(config.Definition{Path: pathLimitDefault, Type: config.TypeInt, Default: "20", Label: "Default result limit"})
	config.Register(config.Definition{Path: pathLimitMax, Type: config.TypeInt, Default: "100", Label: "Maximum result limit"})
	config.Register(config.Definition{Path: pathMinQueryLength, Type: config.TypeInt, Default: "1", Label: "Minimum search query length"})
	config.Register(config.Definition{Path: pathOutOfStockDisplay, Type: config.TypeSelect, Default: "show", Label: "Out-of-stock products in search & browse", Options: []config.Option{
		{Value: "show", Label: "Show"},
		{Value: "hide", Label: "Hide"},
		{Value: "last", Label: "Show last"},
	}})

	// page_size_default is Public so the storefront can compute pagination offsets, page_size_max is the server-side ceiling on any client-requested limit
	config.Register(config.Definition{Path: pathBrowsePageSizeDefault, Type: config.TypeInt, Default: "24", Label: "Products per page", Public: true})
	config.Register(config.Definition{Path: pathBrowsePageSizeMax, Type: config.TypeInt, Default: "100", Label: "Maximum per page"})
	config.Register(config.Definition{Path: pathBrowseFacetMaxValues, Type: config.TypeInt, Default: "50", Label: "Max filter values per facet"})
	config.Register(config.Definition{Path: pathBrowsePaginationStyle, Type: config.TypeSelect, Default: "pagination", Label: "Pagination style", Description: "How listing pages page through products. Load more keeps every state at a real ?page=N URL, so crawlers and shared links lose nothing; hybrid shows numbered pages under the button as well.", Public: true, Options: []config.Option{
		{Value: "pagination", Label: "Numbered pages"},
		{Value: "load_more", Label: "Load more button"},
		{Value: "hybrid", Label: "Load more + numbered pages"},
	}})

	// Folded into scoring at query time so tuning needs no reindex - though sales_30d itself only refreshes as docs reindex
	config.Register(config.Definition{Path: pathRankingSalesWeight, Type: config.TypeString, Default: "0.5",
		Label: "Sales-rank weight (log1p factor on 30d units; 0 = off)"})
	config.Register(config.Definition{Path: pathRankingInStockBoost, Type: config.TypeString, Default: "0",
		Label: "In-stock score boost (flat; 0 = off)"})

	// Source of truth for query construction - stored as JSON, rendered as a table editor via the SEARCH_ATTRIBUTES config type
	config.Register(config.Definition{Path: pathProductsAttributes, Type: config.TypeSearchAttributes, Default: mustJSON(defaultProductAttributes), Label: "Searchable attributes"})
	config.Register(config.Definition{Path: pathBlogAttributes, Type: config.TypeSearchAttributes, Default: mustJSON(defaultBlogAttributes), Label: "Searchable attributes"})

	config.Register(config.Definition{Path: pathProductsSortOptions, Type: config.TypeSortOptions, Default: mustJSON(defaultSortOptions), Label: "Sort options"})

	config.Register(config.Definition{Path: pathMappingAnalyzer, Type: config.TypeSelect, Default: analyzerStandard, Label: "Analyzer", Options: analyzerOptions})
	config.Register(config.Definition{Path: pathMappingEdgeNgramMin, Type: config.TypeInt, Default: "2", Label: "Edge n-gram: min length"})
	config.Register(config.Definition{Path: pathMappingEdgeNgramMax, Type: config.TypeInt, Default: "15", Label: "Edge n-gram: max length"})
	config.Register(config.Definition{Path: pathMappingShards, Type: config.TypeInt, Default: "1", Label: "Index shards"})
	config.Register(config.Definition{Path: pathMappingReplicas, Type: config.TypeInt, Default: "0", Label: "Index replicas"})
}
