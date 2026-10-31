package sdk

// Plugin-owned search indexes on the host's OpenSearch cluster. A plugin brings a
// mapping, an optional full-rebuild iterator, and gets back a writer for live
// upserts - the host owns the physical index (named plg-<slug>-<name>, so plugins
// can't collide with core indexes or each other), wires the index into the
// operator's reindex tooling (`skelet cli search reindex`, the search.reindex admin
// action), and batches rebuild writes into _bulk pages.
//
// Search is a SOFT dependency of the host: with no OPENSEARCH_URL the registration
// is dropped with a log line and every writer call returns an error. Plugins must
// treat the writer as best-effort - index from event subscribers, log failures, and
// rely on the reindex tooling for recovery, exactly as the core indexers do.

import (
	"context"
	"encoding/json"
)

// SearchIndexSink receives documents during a full rebuild - the host buffers them
// into bulk requests, so Add is cheap to call once per record
type SearchIndexSink interface {
	Add(ctx context.Context, docID string, doc any) error
}

// SearchIndexRegistration declares one index. MappingJSON is the full create body ({"settings":...,"mappings":...}) - the host does not
// inspect it beyond creating the index with it, so the plugin owns its analyzer and field choices outright
type SearchIndexRegistration struct {
	// Name qualifies the physical index as plg-<plugin>-<name> - lowercase letters,
	// digits and hyphens, unique within the plugin
	Name        string
	MappingJSON string
	// Reindex streams every document from the plugin's source of truth, returning how
	// many records it indexed - it runs after the host has dropped and recreated the
	// index, from the reindex CLI and the search.reindex admin action. nil means the
	// index is live-writes-only and a reindex just leaves it empty
	Reindex func(ctx context.Context, sink SearchIndexSink) (int, error)
}

// SearchHit is one matching document - Source is the raw stored doc for the plugin
// to decode into its own type
type SearchHit struct {
	ID     string
	Source json.RawMessage
}

type SearchResult struct {
	Total int64
	Hits  []SearchHit
	// Raw per-aggregation bodies when the query asked for aggs - left undecoded
	// because the plugin owns the query, so only it knows each agg's shape
	Aggregations map[string]json.RawMessage
}

// SearchDocument is one upsert inside a bulk write
type SearchDocument struct {
	ID  string
	Doc any
}

// SearchIndexWriter is the live read/write handle for one registered index, scoped to it - a plugin can only ever touch
// indexes it registered. Every method errors when search is not configured or the plugin is disabled
type SearchIndexWriter interface {
	// docID is caller-chosen and stable, so re-indexing the same record overwrites
	IndexDoc(ctx context.Context, docID string, doc any) error
	// One _bulk round trip for the whole batch - syncing N records per-doc costs N HTTP requests, the wall the core reindex was rebuilt to
	// escape. Partial failure returns an error naming the first bad doc, having already applied the rest
	IndexDocs(ctx context.Context, docs []SearchDocument) error
	// A no-op when the doc is already gone
	DeleteDoc(ctx context.Context, docID string) error
	// For clearing every doc derived from one source record - filter on a keyword
	// field the docs carry, since _id itself won't take term queries
	DeleteByField(ctx context.Context, field, value string) error
	// body is a raw OpenSearch query body - the plugin owns its mapping, so unlike the
	// storefront's Engine contract there is no domain phrasing to hide behind
	Search(ctx context.Context, body any) (SearchResult, error)
	// Makes prior writes searchable NOW instead of after the ~1s refresh interval - for read-after-write flows and the
	// plugin's own integration tests, not for sprinkling on every write (each refresh costs the cluster real work)
	Refresh(ctx context.Context) error
}

// RegisterSearchIndex declares the index and returns its writer. The writer is usable immediately - before the host
// installs it queues nothing and errors on use, matching the soft-dependency contract above
func (pl *Plugin) RegisterSearchIndex(reg SearchIndexRegistration) SearchIndexWriter {
	withHost(func(h Host) error { return h.RegisterSearchIndex(pl.slug, reg) })
	return &searchIndexWriter{slug: pl.slug, name: reg.Name}
}

type searchIndexWriter struct {
	slug string
	name string
}

func (w *searchIndexWriter) IndexDoc(ctx context.Context, docID string, doc any) error {
	h := hostNow()
	if h == nil {
		return errSearchHostMissing
	}
	return h.SearchIndexDoc(ctx, w.slug, w.name, docID, doc)
}

func (w *searchIndexWriter) IndexDocs(ctx context.Context, docs []SearchDocument) error {
	h := hostNow()
	if h == nil {
		return errSearchHostMissing
	}
	return h.SearchIndexDocs(ctx, w.slug, w.name, docs)
}

func (w *searchIndexWriter) DeleteDoc(ctx context.Context, docID string) error {
	h := hostNow()
	if h == nil {
		return errSearchHostMissing
	}
	return h.SearchDeleteDoc(ctx, w.slug, w.name, docID)
}

func (w *searchIndexWriter) DeleteByField(ctx context.Context, field, value string) error {
	h := hostNow()
	if h == nil {
		return errSearchHostMissing
	}
	return h.SearchDeleteByField(ctx, w.slug, w.name, field, value)
}

func (w *searchIndexWriter) Search(ctx context.Context, body any) (SearchResult, error) {
	h := hostNow()
	if h == nil {
		return SearchResult{}, errSearchHostMissing
	}
	return h.SearchQuery(ctx, w.slug, w.name, body)
}

func (w *searchIndexWriter) Refresh(ctx context.Context) error {
	h := hostNow()
	if h == nil {
		return errSearchHostMissing
	}
	return h.SearchRefresh(ctx, w.slug, w.name)
}

var errSearchHostMissing = Unavailable("search: host not installed yet")
