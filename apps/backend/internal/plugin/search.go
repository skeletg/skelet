package plugin

// The host side of sdk.RegisterSearchIndex. Plugins register at init(), long before
// the search tier exists, so registrations queue here and SetSearchTier replays them
// - the same lazy-wire-then-replay shape the event bus uses. With no OpenSearch
// configured the tier never arrives, queued registrations are dropped with a log
// line, and every writer call errors: search is a soft dependency for plugins
// exactly as it is for the core indexers.

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/search"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Lowercase letters, digits, hyphens - it becomes part of a physical index name
var searchIndexName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// One plugin index's physical name - the plg- prefix keeps plugins out of core
// index namespace (products_v1, blog_posts_v1) and out of each other's
func pluginIndexName(pluginSlug, name string) string {
	return "plg-" + pluginSlug + "-" + name
}

func searchIndexKey(pluginSlug, name string) string { return pluginSlug + "/" + name }

type pendingSearchIndex struct {
	slug string
	reg  sdk.SearchIndexRegistration
}

func (h *Host) RegisterSearchIndex(pluginSlug string, reg sdk.SearchIndexRegistration) error {
	if !searchIndexName.MatchString(reg.Name) {
		return fmt.Errorf("search index name %q: lowercase letters, digits and hyphens only", reg.Name)
	}
	if reg.MappingJSON == "" {
		return fmt.Errorf("search index %s/%s: MappingJSON is required", pluginSlug, reg.Name)
	}
	h.searchMu.Lock()
	defer h.searchMu.Unlock()
	key := searchIndexKey(pluginSlug, reg.Name)
	if _, dup := h.searchIndexes[key]; dup {
		return fmt.Errorf("search index %s already registered", key)
	}
	if h.searchIndexes == nil {
		h.searchIndexes = map[string]sdk.SearchIndexRegistration{}
	}
	h.searchIndexes[key] = reg
	if h.searchRegistry == nil {
		// Tier not wired yet (or never will be) - queue for SetSearchTier's replay
		h.pendingSearch = append(h.pendingSearch, pendingSearchIndex{slug: pluginSlug, reg: reg})
		return nil
	}
	return h.wireSearchIndex(pluginSlug, reg)
}

// SetSearchTier wires the live cluster client and the operator-facing registry, then replays whatever plugins queued at
// init(). A nil client means search is not configured - the pending set is dropped loudly rather than silently
func SetSearchTier(registry *search.IndexRegistry, client *search.Client) {
	h := installedHost.Load()
	if h == nil {
		panic("plugin.SetSearchTier called before plugin.Install")
	}
	h.setSearchTier(registry, client)
}

// The instance form, so the replay/drop logic is testable on a Host that was never
// installed as the process global
func (h *Host) setSearchTier(registry *search.IndexRegistry, client *search.Client) {
	h.searchMu.Lock()
	defer h.searchMu.Unlock()
	if h.searchRegistry != nil || h.searchDisabled {
		panic("plugin.SetSearchTier called twice")
	}
	if client == nil {
		for _, p := range h.pendingSearch {
			log.Printf("[plugin] search index %s/%s dropped: OPENSEARCH_URL is not set", p.slug, p.reg.Name)
		}
		h.pendingSearch = nil
		h.searchDisabled = true
		return
	}
	h.searchRegistry = registry
	h.searchClient = client
	for _, p := range h.pendingSearch {
		if err := h.wireSearchIndex(p.slug, p.reg); err != nil {
			log.Printf("[plugin] search index %s/%s: %v", p.slug, p.reg.Name, err)
		}
	}
	h.pendingSearch = nil
}

// wireSearchIndex registers the lifecycle with the operator registry and ensures the physical index exists so live
// writes have somewhere to land - caller holds searchMu
func (h *Host) wireSearchIndex(pluginSlug string, reg sdk.SearchIndexRegistration) error {
	life := &pluginIndexLifecycle{
		client:  h.searchClient,
		index:   pluginIndexName(pluginSlug, reg.Name),
		mapping: reg.MappingJSON,
		reindex: reg.Reindex,
	}
	if err := h.searchRegistry.Register(search.RegisteredIndex{
		Slug:       pluginSlug + "-" + reg.Name,
		PluginSlug: pluginSlug,
		Lifecycle:  life,
	}); err != nil {
		return err
	}
	// Boot-time EnsureAll has already run by the time plugin indexes replay, so each
	// one ensures itself - bounded, or a hung cluster would stall boot per index
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := life.EnsureIndex(ctx); err != nil {
		log.Printf("[plugin] search index %s ensure failed: %v", life.index, err)
	}
	return nil
}

// pluginIndexLifecycle adapts one registration onto search.IndexLifecycle, batching
// rebuild docs into _bulk pages the way the core product reindex does
type pluginIndexLifecycle struct {
	client  *search.Client
	index   string
	mapping string
	reindex func(ctx context.Context, sink sdk.SearchIndexSink) (int, error)
}

func (l *pluginIndexLifecycle) EnsureIndex(ctx context.Context) error {
	return l.client.EnsureIndex(ctx, l.index, l.mapping)
}

func (l *pluginIndexLifecycle) Recreate(ctx context.Context) error {
	return l.client.RecreateIndex(ctx, l.index, l.mapping)
}

func (l *pluginIndexLifecycle) ReindexAll(ctx context.Context) (int, error) {
	if l.reindex == nil {
		return 0, nil // live-writes-only index - recreate emptied it, which is the contract
	}
	sink := &bulkSink{client: l.client, index: l.index}
	n, err := l.reindex(ctx, sink)
	if ferr := sink.flush(ctx); ferr != nil && err == nil {
		err = ferr
	}
	return n, err
}

// bulkSink buffers Add calls into the same page size the core reindex ships -
// plugins call Add per record and never think about batching
const pluginBulkPage = 500

type bulkSink struct {
	client *search.Client
	index  string
	buf    []search.BulkDoc
}

func (s *bulkSink) Add(ctx context.Context, docID string, doc any) error {
	s.buf = append(s.buf, search.BulkDoc{ID: docID, Doc: doc})
	if len(s.buf) < pluginBulkPage {
		return nil
	}
	return s.flush(ctx)
}

func (s *bulkSink) flush(ctx context.Context) error {
	if len(s.buf) == 0 {
		return nil
	}
	docs := s.buf
	s.buf = nil
	return s.client.Bulk(ctx, s.index, docs)
}

// searchWriter resolves one writer call: the index must be registered by THIS plugin (containment - no plugin writes
// another's index), search must be wired, and the plugin must be enabled, gated like event handlers are
func (h *Host) searchWriter(ctx context.Context, pluginSlug, name string) (string, error) {
	h.searchMu.Lock()
	_, known := h.searchIndexes[searchIndexKey(pluginSlug, name)]
	client := h.searchClient
	disabled := h.searchDisabled
	h.searchMu.Unlock()
	if !known {
		return "", fmt.Errorf("search index %s/%s is not registered", pluginSlug, name)
	}
	if disabled || client == nil {
		return "", sdk.Unavailable("search is not configured (OPENSEARCH_URL unset)")
	}
	if !isEnabledFromContext(ctx, pluginSlug, uuid.Nil) {
		return "", sdk.Unavailable("plugin %s is disabled", pluginSlug)
	}
	return pluginIndexName(pluginSlug, name), nil
}

func (h *Host) SearchIndexDoc(ctx context.Context, pluginSlug, name, docID string, doc any) error {
	index, err := h.searchWriter(ctx, pluginSlug, name)
	if err != nil {
		return err
	}
	return h.searchClient.IndexDoc(ctx, index, docID, doc)
}

func (h *Host) SearchIndexDocs(ctx context.Context, pluginSlug, name string, docs []sdk.SearchDocument) error {
	index, err := h.searchWriter(ctx, pluginSlug, name)
	if err != nil {
		return err
	}
	bulk := make([]search.BulkDoc, len(docs))
	for i, d := range docs {
		bulk[i] = search.BulkDoc{ID: d.ID, Doc: d.Doc}
	}
	return h.searchClient.Bulk(ctx, index, bulk)
}

func (h *Host) SearchRefresh(ctx context.Context, pluginSlug, name string) error {
	index, err := h.searchWriter(ctx, pluginSlug, name)
	if err != nil {
		return err
	}
	return h.searchClient.Refresh(ctx, index)
}

func (h *Host) SearchDeleteDoc(ctx context.Context, pluginSlug, name, docID string) error {
	index, err := h.searchWriter(ctx, pluginSlug, name)
	if err != nil {
		return err
	}
	return h.searchClient.DeleteDoc(ctx, index, docID)
}

func (h *Host) SearchDeleteByField(ctx context.Context, pluginSlug, name, field, value string) error {
	index, err := h.searchWriter(ctx, pluginSlug, name)
	if err != nil {
		return err
	}
	return h.searchClient.DeleteByField(ctx, index, field, value)
}

func (h *Host) SearchQuery(ctx context.Context, pluginSlug, name string, body any) (sdk.SearchResult, error) {
	index, err := h.searchWriter(ctx, pluginSlug, name)
	if err != nil {
		return sdk.SearchResult{}, err
	}
	total, hits, aggs, err := h.searchClient.SearchHits(ctx, index, body)
	if err != nil {
		return sdk.SearchResult{}, err
	}
	out := sdk.SearchResult{Total: total, Hits: make([]sdk.SearchHit, len(hits)), Aggregations: aggs}
	for i, hit := range hits {
		out.Hits[i] = sdk.SearchHit{ID: hit.ID, Source: hit.Source}
	}
	return out, nil
}
