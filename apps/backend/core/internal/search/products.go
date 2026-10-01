package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// The suffix lets us reindex into a sibling index and flip an alias when the mapping changes incompatibly
const ProductIndex = "products_v1"

// One searchable view of a product per (store, locale) - price comes from the listing with a product-default fallback, and is_published reflects the listing, not the product
type ProductDoc struct {
	ProductID    uuid.UUID `json:"product_id"`
	ChannelID    uuid.UUID `json:"channel_id"`
	Locale       string    `json:"locale"`
	URL          string    `json:"url"`
	SKU          string    `json:"sku"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Kind         string    `json:"kind"`
	PriceCents   int64     `json:"price_cents"`
	CurrencyCode string    `json:"currency_code"`
	IsPublished  bool      `json:"is_published"`
	IsInStock    bool      `json:"is_in_stock"`
	// Units sold in the trailing 30 days on this doc's channel - stamped at index time, decays naturally as reindexes run
	Sales30d  float64   `json:"sales_30d"`
	IndexedAt time.Time `json:"indexed_at"`
}

// Assembled from the live config registry so the next Recreate picks up operator changes - name keeps a .keyword subfield because admin tooling sorts on it
// Roughly where one shard stops being comfortable
const docsPerShard = 10_000_000

func productShards(ctx context.Context, cfg *config.Service, estimatedDocs int64) int64 {
	if cfg != nil {
		if e, err := cfg.Get(ctx, pathMappingShards, config.DefaultScope); err == nil && !e.IsDefault {
			return cfgIntOr(ctx, cfg, pathMappingShards, 1)
		}
	}
	return max(1, (estimatedDocs+docsPerShard-1)/docsPerShard)
}

func buildProductMapping(ctx context.Context, cfg *config.Service, estimatedDocs int64) (string, error) {
	shards := productShards(ctx, cfg, estimatedDocs)
	replicas := cfgIntOr(ctx, cfg, pathMappingReplicas, 0)
	analyzer := cfgStringOr(ctx, cfg, pathMappingAnalyzer, analyzerStandard)
	minGram := cfgIntOr(ctx, cfg, pathMappingEdgeNgramMin, 2)
	maxGram := cfgIntOr(ctx, cfg, pathMappingEdgeNgramMax, 15)

	// Slugs are derived Go-side (buildDoc writes attrslug_* pre-slugified) - an index-side slug_normalizer would be
	// referenced by no field, and a second slugify implementation that can drift from the Go one is worse than none
	analysisSettings := map[string]any{}

	settings := map[string]any{
		"index": map[string]any{
			"number_of_shards":   shards,
			"number_of_replicas": replicas,
		},
		"analysis": analysisSettings,
	}

	nameField := map[string]any{
		"type": "text",
		"fields": map[string]any{
			"keyword": map[string]any{"type": "keyword", "ignore_above": 256},
		},
	}
	descField := map[string]any{"type": "text"}

	if analyzer == analyzerEdgeNgram {
		ng := edgeNgramAnalysisSettings(minGram, maxGram)
		analysisSettings["filter"] = ng["filter"]
		analysisSettings["analyzer"] = ng["analyzer"]
		// Index-time emits prefix tokens; query-time stays standard so keystrokes match indexed tokens without exploding into n-grams themselves
		nameField["analyzer"] = "edge_ngram_analyzer"
		nameField["search_analyzer"] = "standard"
		descField["analyzer"] = "edge_ngram_analyzer"
		descField["search_analyzer"] = "standard"
	}

	// Searches hit the analyzed text path and browse filters for selectable attrs use .keyword - scalars aggregate through the attrslug_/attrindex_ templates below instead
	attrTemplateMapping := map[string]any{
		"type": "text",
		"fields": map[string]any{
			"keyword": map[string]any{"type": "keyword", "ignore_above": 256},
		},
	}
	if analyzer == analyzerEdgeNgram {
		attrTemplateMapping["analyzer"] = "edge_ngram_analyzer"
		attrTemplateMapping["search_analyzer"] = "standard"
	}

	// Per-product URL slug for one scalar attribute - missing rows fall back to slugify(value) at index time, and browse aggregates here so one bucket is one distinct slug
	attrSlugTemplateMapping := map[string]any{
		"type":         "keyword",
		"ignore_above": 256,
	}
	// Parallel boolean carrying the operator's indexability opt-in - a browse sub-aggregation reports it per facet bucket
	attrIndexTemplateMapping := map[string]any{
		"type": "boolean",
	}

	mappings := map[string]any{
		// Without this, a scalar attribute value that happens to parse as a date gets dynamically mapped as `date`, the
		// attr_* string template never matches, and that facet silently returns empty forever - applies on the next recreate
		"date_detection": false,
		"dynamic_templates": []any{
			map[string]any{
				"attrslug_keyword": map[string]any{
					"match":              "attrslug_*",
					"match_mapping_type": "string",
					"mapping":            attrSlugTemplateMapping,
				},
			},
			map[string]any{
				"attrindex_boolean": map[string]any{
					"match":              "attrindex_*",
					"match_mapping_type": "boolean",
					"mapping":            attrIndexTemplateMapping,
				},
			},
			map[string]any{
				"attr_text": map[string]any{
					"match":              "attr_*",
					"match_mapping_type": "string",
					"mapping":            attrTemplateMapping,
				},
			},
		},
		"properties": map[string]any{
			"product_id":    map[string]any{"type": "keyword"},
			"channel_id":    map[string]any{"type": "keyword"},
			"locale":        map[string]any{"type": "keyword"},
			"url":           map[string]any{"type": "keyword"},
			"sku":           map[string]any{"type": "keyword"},
			"name":          nameField,
			"description":   descField,
			"kind":          map[string]any{"type": "keyword"},
			"price_cents":   map[string]any{"type": "long"},
			"currency_code": map[string]any{"type": "keyword"},
			"is_published":  map[string]any{"type": "boolean"},
			"is_in_stock":   map[string]any{"type": "boolean"},
			"sales_30d":     map[string]any{"type": "double"},
			"indexed_at":    map[string]any{"type": "date"},
			// The product's real age - "Newest" used indexed_at, which is just when the doc was last written
			"created_at": map[string]any{"type": "date"},
			// Keyword so browse can term-filter a specific category or collection without splitting on dashes
			"category_ids":   map[string]any{"type": "keyword"},
			"collection_ids": map[string]any{"type": "keyword"},
		},
	}
	// Past one shard every doc routes by its channel, so a storefront query reads one shard instead of all of them
	if shards > 1 {
		mappings["_routing"] = map[string]any{"required": true}
	}
	out, err := json.Marshal(map[string]any{"settings": settings, "mappings": mappings})
	if err != nil {
		return "", fmt.Errorf("marshal product mapping: %w", err)
	}
	return string(out), nil
}

// Standard tokenizer + lowercase + edge_ngram is the canonical shape for prefix typeahead - shared by the product and blog mappings
func edgeNgramAnalysisSettings(minGram, maxGram int64) map[string]any {
	return map[string]any{
		"filter": map[string]any{
			"edge_ngram_filter": map[string]any{
				"type":     "edge_ngram",
				"min_gram": minGram,
				"max_gram": maxGram,
			},
		},
		"analyzer": map[string]any{
			"edge_ngram_analyzer": map[string]any{
				"type":      "custom",
				"tokenizer": "standard",
				"filter":    []string{"lowercase", "edge_ngram_filter"},
			},
		},
	}
}

// Reads the source of truth from Postgres through catalog + channel - OpenSearch only ever sees a derived projection
type ProductIndexer struct {
	client    *Client
	catalog   ProductSource
	channel   ChannelSource
	inventory InventorySource
	cfg       *config.Service
	sales     SalesSource
	// When wired, answers SearchProducts through an external engine instead of OpenSearch - see remote.go
	remoteSearch RemoteSearchSource
	routed       atomic.Bool
}

// Writes always route by channel, a no-op on one shard, so only reads depend on this per-process flag and a stale one merely fans out
func (i *ProductIndexer) routing(channelID uuid.UUID) string {
	if i.routed.Load() {
		return channelID.String()
	}
	return ""
}

func (i *ProductIndexer) detectRouting(ctx context.Context) {
	routed, err := i.client.RoutingRequired(ctx, ProductIndex)
	if err != nil {
		log.Printf("[search] read %s routing: %v", ProductIndex, err)
		return
	}
	i.routed.Store(routed)
}

func docRouting(id string) string {
	parts := strings.Split(id, ":")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// An interface so tests can mock it without standing up the full catalog service
type ProductSource interface {
	GetProductByID(ctx context.Context, id uuid.UUID) (catalog.Product, error)
	GetProductWithAttributes(ctx context.Context, id uuid.UUID) (catalog.Product, error)
	ListProducts(ctx context.Context, locale string, rowLimit int32) ([]catalog.Product, error) // the indexer passes 0 - a capped reindex silently leaves products unsearchable
	ListProductChannelListings(ctx context.Context, productID uuid.UUID) ([]catalog.ProductChannelListing, error)
	// Feeds ReindexChannel - the channel-locale-change subscriber rebuilds every product listed on the affected channel
	ListProductChannelListingsByChannel(ctx context.Context, channelID uuid.UUID) ([]catalog.ProductChannelListing, error)
	LoadProductsContent(ctx context.Context, ids []uuid.UUID, locale string) (map[uuid.UUID]catalog.ProductContent, error)
	// The indexer keeps the Searchable union Filterable subset, which become the attr_<slug> fields
	ListAttributes(ctx context.Context) ([]catalog.Attribute, error)
	// Indexed as keyword arrays (category_ids / collection_ids) for browse filtering
	ListCategoriesByProduct(ctx context.Context, productID uuid.UUID) ([]catalog.Category, error)
	ListCollectionsByProduct(ctx context.Context, productID uuid.UUID) ([]catalog.Collection, error)
	// Global and channel rows both - buildDoc resolves channel > global > slugify(value) per doc
	ListProductAttributeSlugs(ctx context.Context, productID uuid.UUID) ([]catalog.ProductAttributeSlug, error)
	// buildDoc swaps the canonical JSONB value for the doc-locale's translation in attr_<slug>, but URL slugs stay derived from the canonical value so filter URLs are locale-stable
	ListProductAttributeLocaleValues(ctx context.Context, productID uuid.UUID) ([]catalog.ProductAttributeLocaleValue, error)

	// Batched forms, used only by ReindexAll. Upsert stays on the singular ones - it handles one product by definition,
	// and paying for a page-shaped query to fetch a single row would make every live edit slower
	ListProductsWithAttributesByIDs(ctx context.Context, ids []uuid.UUID) ([]catalog.Product, error)
	ListProductChannelListingsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]catalog.ProductChannelListing, error)
	ListCategoriesByProductIDs(ctx context.Context, productIDs []uuid.UUID, locale string) (map[uuid.UUID][]catalog.Category, error)
	ListCollectionsByProductIDs(ctx context.Context, productIDs []uuid.UUID, locale string) (map[uuid.UUID][]catalog.Collection, error)
	ListProductAttributeSlugsByProductIDs(ctx context.Context, productIDs []uuid.UUID) ([]catalog.ProductAttributeSlug, error)
	ListProductAttributeLocaleValuesByProductIDs(ctx context.Context, productIDs []uuid.UUID) ([]catalog.ProductAttributeLocaleValue, error)
}

// BatchSalesSource is the batched form of SalesSource. Optional - ReindexAll uses it when the attached source implements it and falls back
// to per-SKU otherwise, so an embedder with only the singular form still reindexes, just slower
type BatchSalesSource interface {
	SalesVelocity30dBySKUs(ctx context.Context, skus []string) (map[string]map[uuid.UUID]float64, error)
}

type ChannelSource interface {
	List(ctx context.Context) ([]channel.Channel, error)
	GetByID(ctx context.Context, id uuid.UUID) (channel.Channel, error)
}

// SalableByProductIDs returns -1 for untracked ("always in stock"), 0 for out of stock, >0 for in stock
type InventorySource interface {
	SalableByProductIDs(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]float64, error)
}

// Optional - without it sales_30d indexes as 0 and the ranking boost is a no-op
type SalesSource interface {
	SalesVelocity30d(ctx context.Context, sku string) (map[uuid.UUID]float64, error)
}

func (i *ProductIndexer) SetSalesSource(s SalesSource) { i.sales = s }

func NewProductIndexer(c *Client, cat ProductSource, sv ChannelSource, inv InventorySource, cfg *config.Service) *ProductIndexer {
	return &ProductIndexer{client: c, catalog: cat, channel: sv, inventory: inv, cfg: cfg}
}

// Called from server boot so live writes have somewhere to land before the first reindex runs
// Aliased from birth on a fresh cluster, so every rebuild is an atomic flip
func (i *ProductIndexer) EnsureIndex(ctx context.Context) error {
	mapping, err := buildProductMapping(ctx, i.cfg, 0)
	if err != nil {
		return err
	}
	if err := i.client.EnsureAliasedIndex(ctx, ProductIndex, mapping); err != nil {
		return err
	}
	i.detectRouting(ctx)
	return nil
}

// An upper bound on docs, the safe side to size shards from
func (i *ProductIndexer) estimateDocs(ctx context.Context) int64 {
	channels, err := i.channel.List(ctx)
	if err != nil {
		return 0
	}
	products, err := i.catalog.ListProducts(ctx, "", 0)
	if err != nil {
		return 0
	}
	var perProduct int64
	for _, sv := range channels {
		perProduct += int64(len(localeCodes(sv)))
	}
	return perProduct * int64(len(products))
}

// RebuildAndSwap reindexes into a fresh physical index and atomically flips the ProductIndex alias - the catalogue
// keeps serving the old docs for the whole rebuild instead of sitting empty for minutes, which recreate-in-place caused
func (i *ProductIndexer) RebuildAndSwap(ctx context.Context) (int, error) {
	mapping, err := buildProductMapping(ctx, i.cfg, i.estimateDocs(ctx))
	if err != nil {
		return 0, err
	}
	n, err := rebuildAndSwap(ctx, i.client, ProductIndex, mapping, i.reindexAllInto)
	if err == nil {
		i.routed.Store(strings.Contains(mapping, `"_routing"`))
	}
	return n, err
}

// For the bulk reindex CLI and the search.reindex admin action after a mapping change - never the live event path
func (i *ProductIndexer) Recreate(ctx context.Context) error {
	mapping, err := buildProductMapping(ctx, i.cfg, i.estimateDocs(ctx))
	if err != nil {
		return err
	}
	if err := i.client.RecreateIndex(ctx, ProductIndex, mapping); err != nil {
		return err
	}
	i.routed.Store(strings.Contains(mapping, `"_routing"`))
	return nil
}

// Reindexes one product across every (store, locale) its listings span - idempotent, so the event path can replay it freely
// GetProductWithAttributes hydrates the scalar JSONB and the selectable selections before buildDoc walks them - one extra read on an already low-frequency path
func (i *ProductIndexer) Upsert(ctx context.Context, productID uuid.UUID) error {
	product, err := i.catalog.GetProductWithAttributes(ctx, productID)
	if err != nil {
		return fmt.Errorf("load product: %w", err)
	}
	// Once per Upsert so buildDoc can map AttributeSelections back to a slug and skip non-indexed scalars
	indexedAttrs, err := i.indexedAttributes(ctx)
	if err != nil {
		return fmt.Errorf("load indexed attributes: %w", err)
	}
	// Associations are global rather than per-store, so fetch once and stamp into every doc
	categories, err := i.catalog.ListCategoriesByProduct(ctx, productID)
	if err != nil {
		return fmt.Errorf("load categories: %w", err)
	}
	collections, err := i.catalog.ListCollectionsByProduct(ctx, productID)
	if err != nil {
		return fmt.Errorf("load collections: %w", err)
	}
	categoryIDs := make([]string, len(categories))
	for i, c := range categories {
		categoryIDs[i] = c.ID.String()
	}
	collectionIDs := make([]string, len(collections))
	for i, c := range collections {
		collectionIDs[i] = c.ID.String()
	}
	listings, err := i.catalog.ListProductChannelListings(ctx, productID)
	if err != nil {
		return fmt.Errorf("load listings: %w", err)
	}
	localeValues := map[string]map[uuid.UUID]string{}
	localeRows, err := i.catalog.ListProductAttributeLocaleValues(ctx, productID)
	if err != nil {
		// Don't index a doc missing its translations on a transient blip - fail so the outbox replays the upsert
		return fmt.Errorf("load attribute locale values: %w", err)
	}
	for _, row := range localeRows {
		if localeValues[row.Locale] == nil {
			localeValues[row.Locale] = map[uuid.UUID]string{}
		}
		localeValues[row.Locale][row.AttributeID] = row.Value
	}
	// Optional and fail-soft - a reporting query hiccup must not block indexing
	salesByChannel := map[uuid.UUID]float64{}
	if i.sales != nil {
		if m, serr := i.sales.SalesVelocity30d(ctx, product.SKU); serr == nil {
			salesByChannel = m
		}
	}
	if len(listings) == 0 {
		// No store sells it - clear any docs left by a previous publish state
		return i.Delete(ctx, productID)
	}
	// Build EVERY doc before touching the index. Deleting first meant any error in the loop below - a Postgres blip inside buildDoc, one bad
	// locale - left the product with zero docs and vanished it from search and every PLP until a human noticed and reindexed by hand
	type pending struct {
		id  string
		doc any
	}
	slugRows, err := i.catalog.ListProductAttributeSlugs(ctx, productID)
	if err != nil {
		return fmt.Errorf("load attribute slug overrides: %w", err)
	}
	built := make([]pending, 0, len(listings))
	contentByLocale := map[string]catalog.ProductContent{}
	channels, err := i.channel.List(ctx)
	if err != nil {
		return fmt.Errorf("load channels: %w", err)
	}
	channelByID := make(map[uuid.UUID]channel.Channel, len(channels))
	for _, sv := range channels {
		channelByID[sv.ID] = sv
	}
	for _, lst := range listings {
		sv, ok := channelByID[lst.ChannelID]
		if !ok {
			continue // listing references a deleted store - skip rather than fail the whole upsert
		}
		// Salable depends on the channel's assigned locations, so fetch once per channel and stamp into every locale doc
		inStock := i.productInStock(ctx, product.ID, lst.ChannelID)
		locales := localeCodes(sv)
		for _, locale := range locales {
			// Channels overlap on locale far more often than not, so cache per upsert
			content, ok := contentByLocale[locale]
			if !ok {
				contents, cerr := i.catalog.LoadProductsContent(ctx, []uuid.UUID{product.ID}, locale)
				if cerr != nil {
					return fmt.Errorf("load content %s/%s: %w", product.ID, locale, cerr)
				}
				content = contents[product.ID]
				contentByLocale[locale] = content
			}
			doc, err := i.buildDoc(product, lst, sv, locale, indexedAttrs, categoryIDs, collectionIDs, localeValues[locale], content, slugRows, inStock, salesByChannel[lst.ChannelID])
			if err != nil {
				return fmt.Errorf("build doc %s/%s/%s: %w", product.ID, sv.ID, locale, err)
			}
			built = append(built, pending{id: docID(product.ID, sv.ID, locale), doc: doc})
		}
	}
	if len(built) == 0 {
		return i.Delete(ctx, productID)
	}
	// New docs and stale-doc deletes go in one _bulk - a failed stale-doc listing only leaves a harmless sibling
	existing, lerr := i.client.ListDocIDsByField(ctx, ProductIndex, "product_id", productID.String())
	if lerr != nil {
		log.Printf("[search] list existing docs for product %s: %v", productID, lerr)
	}
	fresh := make(map[string]struct{}, len(built))
	ops := make([]BulkDoc, 0, len(built)+len(existing))
	for _, b := range built {
		ops = append(ops, BulkDoc{ID: b.id, Doc: b.doc, Routing: docRouting(b.id)})
		fresh[b.id] = struct{}{}
	}
	for _, id := range existing {
		if _, ok := fresh[id]; !ok {
			ops = append(ops, BulkDoc{ID: id, Delete: true, Routing: docRouting(id)})
		}
	}
	return i.client.Bulk(ctx, ProductIndex, ops)
}

// Used when a channel is deleted so its per-locale docs can't serve stale results
func (i *ProductIndexer) DeleteChannel(ctx context.Context, channelID uuid.UUID) error {
	return i.client.DeleteByField(ctx, ProductIndex, "channel_id", channelID.String())
}

func (i *ProductIndexer) ReindexChannel(ctx context.Context, channelID uuid.UUID) (int, error) {
	sv, err := i.channel.GetByID(ctx, channelID)
	if err != nil {
		return 0, fmt.Errorf("load channel: %w", err)
	}
	locales := localeCodes(sv)
	if err := i.client.DeleteByQuery(ctx, ProductIndex, map[string]any{
		"bool": map[string]any{
			"filter":   []any{map[string]any{"term": map[string]any{"channel_id": channelID.String()}}},
			"must_not": []any{map[string]any{"terms": map[string]any{"locale": locales}}},
		},
	}); err != nil {
		return 0, err
	}
	listings, err := i.catalog.ListProductChannelListingsByChannel(ctx, channelID)
	if err != nil {
		return 0, fmt.Errorf("list channel listings: %w", err)
	}
	indexedAttrs, err := i.indexedAttributes(ctx)
	if err != nil {
		return 0, fmt.Errorf("load indexed attributes: %w", err)
	}
	only := map[uuid.UUID]channel.Channel{channelID: sv}
	done := 0
	for start := 0; start < len(listings); start += reindexPageSize {
		if ctx.Err() != nil {
			return done, ctx.Err()
		}
		end := min(start+reindexPageSize, len(listings))
		page := make([]catalog.Product, 0, end-start)
		for _, lst := range listings[start:end] {
			page = append(page, catalog.Product{ID: lst.ProductID})
		}
		n, perr := i.reindexPage(ctx, ProductIndex, page, indexedAttrs, only)
		done += n
		if perr != nil {
			return done, fmt.Errorf("reindex channel %s page %d-%d: %w", channelID, start, end, perr)
		}
	}
	return done, nil
}

func (i *ProductIndexer) Delete(ctx context.Context, productID uuid.UUID) error {
	return i.client.DeleteByField(ctx, ProductIndex, "product_id", productID.String())
}

// Slim enough for the front resolver to hydrate from Postgres, or to render directly when the projection already carries what the storefront needs
type ProductHit struct {
	ProductID uuid.UUID
	URL       string
	SKU       string
	Name      string
}

// Scoped to the (store, locale) pair and is_published only - an empty query yields nothing, browseProducts is the "list everything" path
// buildSearchQuery emits one multi_match when fields agree on matchType and fuzziness, else a bool.should fan-out so SKU can be exact while name is fuzzy
func (i *ProductIndexer) SearchProducts(ctx context.Context, query string, channelID uuid.UUID, locale string, limit, offset int, boosts map[uuid.UUID]float64) ([]ProductHit, int64, error) {
	limitDefault := cfgIntOr(ctx, i.cfg, pathLimitDefault, 20)
	limitMax := cfgIntOr(ctx, i.cfg, pathLimitMax, 100)
	if limit <= 0 {
		limit = int(limitDefault)
	} else if int64(limit) > limitMax {
		// Clamp an over-max request down instead of dropping back to the default - a ceiling means "give me the most allowed"
		limit = int(limitMax)
	}
	if offset < 0 {
		offset = 0
	}
	q := strings.TrimSpace(query)
	// Admins can require more than one char to suppress noisy single-letter typeahead lookups
	if utf8.RuneCountInString(q) < int(cfgIntOr(ctx, i.cfg, pathMinQueryLength, 1)) {
		return []ProductHit{}, 0, nil
	}

	// Fails open to the OpenSearch path below, so a down external engine degrades to native results
	if i.remoteSearch != nil {
		if hits, total, ok := i.remoteSearch.Search(ctx, q, channelID, locale, limit, offset); ok {
			return hits, total, nil
		}
	}
	// Searchable attributes auto-merge with default settings and stay overridable per field in the dashboard - a load failure falls back to core fields only rather than taking search down
	// Filterable-only attributes are in the index but sit out the search query; only browseProducts uses them
	indexed, _ := i.indexedAttributes(ctx)
	searchable := indexed[:0]
	for _, a := range indexed {
		if a.Searchable {
			searchable = append(searchable, a)
		}
	}
	attrs := mergeWithCatalogAttributes(
		loadAttributes(ctx, i.cfg, pathProductsAttributes, defaultProductAttributes),
		searchable,
	)
	oos := cfgStringOr(ctx, i.cfg, pathOutOfStockDisplay, oosShow)
	filters := []any{
		map[string]any{"term": map[string]any{"channel_id": channelID.String()}},
		map[string]any{"term": map[string]any{"locale": locale}},
		map[string]any{"term": map[string]any{"is_published": true}},
	}
	if oos == oosHide {
		filters = append(filters, map[string]any{"term": map[string]any{"is_in_stock": true}})
	}
	boolQuery := map[string]any{
		"bool": map[string]any{
			"must":   []any{buildSearchQuery(q, attrs)},
			"filter": filters,
		},
	}
	body := map[string]any{
		"from":    offset,
		"size":    limit,
		"_source": []string{"product_id", "url", "sku", "name"},
		"query":   applyMerchBoosts(applyBusinessRanking(ctx, i.cfg, boolQuery), boosts),
	}
	if oos == oosLast {
		// Push out-of-stock below in-stock, then relevance within each
		body["sort"] = []any{
			map[string]any{"is_in_stock": map[string]any{"order": "desc"}},
			"_score",
		}
	}
	hits := make([]ProductHit, 0, limit)
	total, err := i.client.SearchRouted(ctx, ProductIndex, i.routing(channelID), body, func(raw json.RawMessage) error {
		var src ProductDoc
		if err := json.Unmarshal(raw, &src); err != nil {
			return err
		}
		hits = append(hits, ProductHit{
			ProductID: src.ProductID,
			URL:       src.URL,
			SKU:       src.SKU,
			Name:      src.Name,
		})
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return hits, total, nil
}

// Cold starts and recovery from the CLI - the caller is expected to Recreate() first if the mapping changed ReindexAll rebuilds every product. It keeps going past a single bad product and always reports how far it got - aborting on the first error (and returning 0) meant one unindexable row left the whole catalogue missing with no way to tell whether 10 or 10,000 products had made it. Still one document per request; the bulk API is the next step here, and the operator-facing cost is a slow rebuild rather than a wrong one. Products per page. Each page is one round trip per data source plus one _bulk, so bigger is faster until the batch stops fitting comfortably in a request - 500 products is a few MB of NDJSON on a typical catalogue.
const reindexPageSize = 500

// ReindexAll rebuilds the whole index from Postgres, a page of products at a time.
func (i *ProductIndexer) ReindexAll(ctx context.Context) (int, error) {
	return i.reindexAllInto(ctx, ProductIndex)
}

// The target is a parameter so RebuildAndSwap can point the same machinery at a
// fresh physical index while readers stay on the alias
func (i *ProductIndexer) reindexAllInto(ctx context.Context, target string) (int, error) {
	all, err := i.catalog.ListProducts(ctx, "", 0) // 0 = every row - a capped reindex silently leaves products unsearchable
	if err != nil {
		return 0, fmt.Errorf("list products: %w", err)
	}
	// Both are per-reindex constants, hoisted out of the per-product loop -
	// ListAttributes in particular drags in every attribute's value rows
	indexedAttrs, err := i.indexedAttributes(ctx)
	if err != nil {
		return 0, fmt.Errorf("load indexed attributes: %w", err)
	}
	channels, err := i.channel.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list channels: %w", err)
	}
	channelByID := make(map[uuid.UUID]channel.Channel, len(channels))
	for _, sv := range channels {
		channelByID[sv.ID] = sv
	}

	done, failed := 0, 0
	var firstErr error
	for start := 0; start < len(all); start += reindexPageSize {
		if ctx.Err() != nil {
			// Deadline or cancellation - report progress rather than pretending it finished
			return done, fmt.Errorf("reindex stopped after %d/%d products: %w", done, len(all), ctx.Err())
		}
		end := min(start+reindexPageSize, len(all))
		n, perr := i.reindexPage(ctx, target, all[start:end], indexedAttrs, channelByID)
		done += n
		if perr != nil {
			failed += (end - start) - n
			if firstErr == nil {
				firstErr = perr
			}
			log.Printf("[search] reindex: page %d-%d partially failed: %v", start, end, perr)
		}
		log.Printf("[search] reindex: %d/%d products", done, len(all))
	}
	if firstErr != nil {
		return done, fmt.Errorf("reindexed %d/%d products, %d failed - first error: %w", done, len(all), failed, firstErr)
	}
	return done, nil
}

// reindexPage builds and ships every doc for one page of products. Returns how many products it indexed - a product
// with no listing is counted as done, since having no docs is the correct outcome for one no store sells
func (i *ProductIndexer) reindexPage(
	ctx context.Context,
	target string,
	page []catalog.Product,
	indexedAttrs []catalog.Attribute,
	channelByID map[uuid.UUID]channel.Channel,
) (int, error) {
	ids := make([]uuid.UUID, len(page))
	for n, p := range page {
		ids[n] = p.ID
	}
	// ListProducts doesn't hydrate attribute selections, and buildDoc needs them
	products, err := i.catalog.ListProductsWithAttributesByIDs(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("load products with attributes: %w", err)
	}
	skus := make([]string, len(products))
	for n, p := range products {
		skus[n] = p.SKU
	}
	listingsByProduct, err := i.catalog.ListProductChannelListingsByProductIDs(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("load listings: %w", err)
	}
	categoriesByProduct, err := i.catalog.ListCategoriesByProductIDs(ctx, ids, "")
	if err != nil {
		return 0, fmt.Errorf("load categories: %w", err)
	}
	collectionsByProduct, err := i.catalog.ListCollectionsByProductIDs(ctx, ids, "")
	if err != nil {
		return 0, fmt.Errorf("load collections: %w", err)
	}
	slugRows, err := i.catalog.ListProductAttributeSlugsByProductIDs(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("load attribute slug overrides: %w", err)
	}
	slugsByProduct := make(map[uuid.UUID][]catalog.ProductAttributeSlug, len(ids))
	for _, row := range slugRows {
		slugsByProduct[row.ProductID] = append(slugsByProduct[row.ProductID], row)
	}
	localeRows, err := i.catalog.ListProductAttributeLocaleValuesByProductIDs(ctx, ids)
	if err != nil {
		// Don't index docs missing their translations on a transient blip
		return 0, fmt.Errorf("load attribute locale values: %w", err)
	}
	localeValues := map[uuid.UUID]map[string]map[uuid.UUID]string{}
	for _, row := range localeRows {
		if localeValues[row.ProductID] == nil {
			localeValues[row.ProductID] = map[string]map[uuid.UUID]string{}
		}
		if localeValues[row.ProductID][row.Locale] == nil {
			localeValues[row.ProductID][row.Locale] = map[uuid.UUID]string{}
		}
		localeValues[row.ProductID][row.Locale][row.AttributeID] = row.Value
	}

	// Content is per locale, so load one map per locale the page's channels serve
	contentByLocale := map[string]map[uuid.UUID]catalog.ProductContent{}
	for _, sv := range channelByID {
		for _, locale := range localeCodes(sv) {
			if _, ok := contentByLocale[locale]; ok {
				continue
			}
			contents, cerr := i.catalog.LoadProductsContent(ctx, ids, locale)
			if cerr != nil {
				return 0, fmt.Errorf("load content for locale %s: %w", locale, cerr)
			}
			contentByLocale[locale] = contents
		}
	}

	// Salable is per channel, so one query per channel for the whole page
	inStockByChannel := map[uuid.UUID]map[uuid.UUID]float64{}
	if i.inventory != nil {
		for chID := range channelByID {
			salable, serr := i.inventory.SalableByProductIDs(ctx, ids, chID)
			if serr != nil {
				// Soft-degrade to in-stock, same as the single-product path
				log.Printf("[search] salable for channel %s: %v (indexing page as in-stock)", chID, serr)
				continue
			}
			inStockByChannel[chID] = salable
		}
	}

	salesBySKU := i.pageSalesVelocity(ctx, skus)

	docs := make([]BulkDoc, 0, len(page)*len(channelByID))
	indexed := 0
	for _, p := range products {
		listings := listingsByProduct[p.ID]
		if len(listings) == 0 {
			indexed++ // No store sells it - zero docs is the right answer
			continue
		}
		categoryIDs := make([]string, 0, len(categoriesByProduct[p.ID]))
		for _, c := range categoriesByProduct[p.ID] {
			categoryIDs = append(categoryIDs, c.ID.String())
		}
		collectionIDs := make([]string, 0, len(collectionsByProduct[p.ID]))
		for _, c := range collectionsByProduct[p.ID] {
			collectionIDs = append(collectionIDs, c.ID.String())
		}
		built := 0
		for _, lst := range listings {
			sv, ok := channelByID[lst.ChannelID]
			if !ok {
				continue // Listing references a deleted store
			}
			inStock := true
			if salable, ok := inStockByChannel[lst.ChannelID]; ok {
				inStock = salable[p.ID] != 0
			}
			for _, locale := range localeCodes(sv) {
				doc, derr := i.buildDoc(
					p, lst, sv, locale, indexedAttrs, categoryIDs, collectionIDs,
					localeValues[p.ID][locale], contentByLocale[locale][p.ID],
					slugsByProduct[p.ID], inStock, salesBySKU[p.SKU][lst.ChannelID],
				)
				if derr != nil {
					return indexed, fmt.Errorf("build doc %s/%s/%s: %w", p.ID, sv.ID, locale, derr)
				}
				docs = append(docs, BulkDoc{ID: docID(p.ID, sv.ID, locale), Doc: doc, Routing: sv.ID.String()})
				built++
			}
		}
		if built > 0 {
			indexed++
		}
	}
	if err := i.client.Bulk(ctx, target, docs); err != nil {
		// A partial _bulk failure has already applied every good doc - returning 0 here
		// made ReindexAll report the whole page failed and overstate the damage
		return indexed, err
	}
	return indexed, nil
}

// Uses the batched query when the attached source has one, else falls back to the
// per-SKU form so a source that only implements SalesSource still works
func (i *ProductIndexer) pageSalesVelocity(ctx context.Context, skus []string) map[string]map[uuid.UUID]float64 {
	out := map[string]map[uuid.UUID]float64{}
	if i.sales == nil {
		return out
	}
	if batch, ok := i.sales.(BatchSalesSource); ok {
		// Fail-soft - a reporting hiccup must not block indexing, it just costs ranking signal
		if m, err := batch.SalesVelocity30dBySKUs(ctx, skus); err == nil {
			return m
		} else {
			log.Printf("[search] batched sales velocity failed, falling back to per-SKU: %v", err)
		}
	}
	for _, sku := range skus {
		if m, err := i.sales.SalesVelocity30d(ctx, sku); err == nil {
			out[sku] = m
		}
	}
	return out
}

// Salable -1 (untracked) and >0 count as in stock, only a tracked 0 is out - soft-degrades to in-stock so search can't wedge on an inventory blip
func (i *ProductIndexer) productInStock(ctx context.Context, productID, channelID uuid.UUID) bool {
	if i.inventory == nil {
		return true
	}
	salable, err := i.inventory.SalableByProductIDs(ctx, []uuid.UUID{productID}, channelID)
	if err != nil {
		log.Printf("[search] salable for %s on channel %s: %v (indexing as in-stock)", productID, channelID, err)
		return true
	}
	return salable[productID] != 0
}

// Falls back to the store default locale then the product's raw fields so docs are never blank-name - returns a map so dynamic attr_<slug> fields splat alongside the static columns content and slugRows are passed in rather than fetched: buildDoc runs once per (channel, locale), so querying them here made a two-locale product pay for four round trips that the bulk reindex loads once for a whole page
func (i *ProductIndexer) buildDoc(
	p catalog.Product,
	listing catalog.ProductChannelListing,
	sv channel.Channel,
	locale string,
	indexedAttrs []catalog.Attribute,
	categoryIDs []string,
	collectionIDs []string,
	scalarLocaleValues map[uuid.UUID]string,
	content catalog.ProductContent,
	slugRows []catalog.ProductAttributeSlug,
	inStock bool,
	sales30d float64,
) (map[string]any, error) {
	name := p.SKU
	description := ""
	if content.Name != "" {
		name = content.Name
	}
	if d := stripJSON(content.ShortDescription); d != "" {
		description = d
	} else {
		description = stripJSON(content.Description)
	}
	priceCents := p.DefaultPrice
	if listing.Price != nil {
		priceCents = *listing.Price
	}
	currency := sv.CurrencyCode
	if currency == "" {
		currency = p.DefaultCurrencyCode
	}
	// A channel URL override on the listing wins over the global product url
	url := p.URL
	if listing.URL != nil && *listing.URL != "" {
		url = *listing.URL
	}
	doc := map[string]any{
		"product_id":     p.ID,
		"channel_id":     sv.ID,
		"locale":         locale,
		"url":            url,
		"sku":            p.SKU,
		"name":           name,
		"description":    description,
		"kind":           string(p.Kind),
		"price_cents":    priceCents,
		"currency_code":  currency,
		"is_published":   listing.IsPublished,
		"is_in_stock":    inStock,
		"sales_30d":      sales30d,
		"indexed_at":     time.Now().UTC(),
		"created_at":     p.CreatedAt.UTC(),
		"category_ids":   categoryIDs,
		"collection_ids": collectionIDs,
	}
	// Rows arrive in two scopes, global (channel_id NULL) and channel-narrowed - channel wins, then global, then slugify(canonical value)
	globalSlugOverrides := map[uuid.UUID]catalog.ProductAttributeSlug{}
	channelSlugOverrides := map[uuid.UUID]catalog.ProductAttributeSlug{}
	for _, row := range slugRows {
		switch {
		case row.ChannelID == nil:
			globalSlugOverrides[row.AttributeID] = row
		case *row.ChannelID == sv.ID:
			channelSlugOverrides[row.AttributeID] = row
		}
	}
	// Cache the canonical values so the attrslug_ derivation stays locale-stable while attr_<slug> swaps in the doc-locale translation
	extractedValues := make(map[uuid.UUID]string, len(indexedAttrs))
	for _, attr := range indexedAttrs {
		value := extractAttributeValue(p, attr)
		extractedValues[attr.ID] = value
		// Index as an ARRAY so .keyword carries one keyword per value - space-joining would key a multiselect bucket off the whole combined string, which never round-trips back to one value
		if attr.InputType.AcceptsValues() {
			names := selectionNames(p, attr)
			if len(names) == 0 {
				continue
			}
			doc[AttributeFieldName(attr.Slug)] = names
			continue
		}
		display := value
		if attr.InputType == catalog.InputTypePlainText {
			if v, ok := scalarLocaleValues[attr.ID]; ok && v != "" {
				display = v
			}
		}
		if display == "" {
			continue
		}
		doc[AttributeFieldName(attr.Slug)] = display
	}
	// Only the non-selectable filterable subset - selectable attrs already carry their slugs on AttributeValue
	// Indexability is attribute-level, so attrindex_<slug> is uniform across the attribute's docs, which is what the browse facet's size=1 sub-agg expects
	for _, attr := range indexedAttrs {
		if !attr.Filterable || attr.InputType.AcceptsValues() {
			continue
		}
		value := extractedValues[attr.ID]
		if value == "" {
			continue
		}
		slug := catalog.Slugify(value)
		if row, ok := globalSlugOverrides[attr.ID]; ok {
			slug = row.Slug
		}
		if row, ok := channelSlugOverrides[attr.ID]; ok {
			slug = row.Slug
		}
		doc[AttributeSlugFieldName(attr.Slug)] = slug
		doc[AttributeIndexableFieldName(attr.Slug)] = attr.IsIndexable
	}
	return doc, nil
}

func AttributeSlugFieldName(slug string) string {
	return "attrslug_" + slug
}

// A sub-aggregation source, so each bucket reports one isIndexable computed from the products that landed in that slug
func AttributeIndexableFieldName(slug string) string {
	return "attrindex_" + slug
}

// The attr_ prefix lets the dynamic_template match without colliding with core columns
func AttributeFieldName(slug string) string {
	return "attr_" + slug
}

// Always one string - scalars from the JSONB blob, selectables space-joined from the selections junction, "" when the product has no value
func extractAttributeValue(p catalog.Product, attr catalog.Attribute) string {
	if attr.InputType.AcceptsValues() {
		// The joined form feeds the searchable text field; faceting and filtering index the array form from selectionNames instead
		return strings.Join(selectionNames(p, attr), " ")
	}
	raw, ok := p.Attributes[attr.Slug]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%v", raw)
}

// An array rather than space-joined, so attr_<slug>.keyword exposes one keyword per value and facets resolve per value
func selectionNames(p catalog.Product, attr catalog.Attribute) []string {
	names := make([]string, 0, 2)
	for _, sel := range p.AttributeSelections {
		if sel.AttributeID != attr.ID {
			continue
		}
		if sel.Value.Name != "" {
			names = append(names, sel.Value.Name)
		} else if sel.Value.Value != "" {
			names = append(names, sel.Value.Value)
		}
	}
	return names
}

// Searchable union Filterable - searchable ones drive the query text field, filterable ones drive browse filters and facets, and many are both
// Private attributes sit the index out entirely so no engine (native or remote) can leak their values - like the other two flags, flipping isPublic needs a reindex to backfill
func (i *ProductIndexer) indexedAttributes(ctx context.Context) ([]catalog.Attribute, error) {
	all, err := i.catalog.ListAttributes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]catalog.Attribute, 0, len(all))
	for _, a := range all {
		if (a.Searchable || a.Filterable) && a.IsPublic {
			out = append(out, a)
		}
	}
	return out, nil
}

// Falls back to {DefaultLocale} when the junction is empty - single-locale stores often skip it
func localeCodes(sv channel.Channel) []string {
	if len(sv.Locales) == 0 {
		if sv.DefaultLocale == "" {
			return []string{"en"}
		}
		return []string{sv.DefaultLocale}
	}
	codes := make([]string, 0, len(sv.Locales))
	for _, l := range sv.Locales {
		// Retired locales don't get docs, but the channel default stays indexed even when deactivated so a store can't go completely dark by accident
		if !l.IsActive && !strings.EqualFold(l.Code, sv.DefaultLocale) {
			continue
		}
		codes = append(codes, l.Code)
	}
	if len(codes) == 0 {
		if sv.DefaultLocale != "" {
			return []string{sv.DefaultLocale}
		}
		return []string{"en"}
	}
	return codes
}

// Stable across reindexes so the indexer overwrites instead of accumulating duplicates
func docID(productID, channelID uuid.UUID, locale string) string {
	return strings.Join([]string{productID.String(), channelID.String(), locale}, ":")
}

// The WYSIWYG stores Lexical-style structured JSON - just pull the text out so the analyzer has something to tokenize
func stripJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Only string leaves - numbers and booleans are noise for search
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	var b strings.Builder
	collectText(v, &b)
	return strings.TrimSpace(b.String())
}

func collectText(v any, b *strings.Builder) {
	switch t := v.(type) {
	case string:
		if t != "" {
			b.WriteString(t)
			b.WriteByte(' ')
		}
	case []any:
		for _, item := range t {
			collectText(item, b)
		}
	case map[string]any:
		for _, item := range t {
			collectText(item, b)
		}
	}
}
