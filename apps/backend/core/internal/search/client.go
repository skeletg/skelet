// Package search wraps the OpenSearch client behind the storefront's product and blog post search
// OpenSearch is a soft dependency - reads return empty hits and writes log-and-continue when the cluster is down, since Postgres stays the source of truth and the reindex CLI rebuilds from it
package search

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	opensearch "github.com/opensearch-project/opensearch-go/v5"
	opensearchapi "github.com/opensearch-project/opensearch-go/v5/opensearchapi"
)

type Client struct {
	api *opensearchapi.Client
}

// ClientOptions carries what a non-dev cluster needs. Every managed OpenSearch (AWS, Aiven, Bonsai) requires basic auth over TLS, and passing only addresses meant the only way to reach one was userinfo in the URL - which then leaks into every wrapped transport error, the container logs and the log pipeline.
type ClientOptions struct {
	Username string
	Password string
	// PEM bundle for a cluster with a private CA - empty uses the system pool
	CACertPEM []byte
	// Per-request ceiling. Without one a blackholed or GC-paused cluster hangs storefront requests indefinitely instead of
	// degrading, and the client's own retries multiply the load against an already-sick cluster
	Timeout time.Duration
}

const defaultSearchTimeout = 3 * time.Second

// Dev runs with security disabled; NewClientWithOptions carries credentials for anything else
func NewClient(addresses []string) (*Client, error) {
	return NewClientWithOptions(addresses, ClientOptions{})
}

func NewClientWithOptions(addresses []string, opts ClientOptions) (*Client, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultSearchTimeout
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// DefaultTransport allows 2 idle conns per host, which churns TCP on the
	// hottest read path in the app
	transport.MaxIdleConnsPerHost = 32
	if len(opts.CACertPEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(opts.CACertPEM) {
			return nil, fmt.Errorf("opensearch client: CA bundle contains no usable certificate")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	api, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses: addresses,
			Username:  opts.Username,
			Password:  opts.Password,
			Transport: &timeoutTransport{base: transport, timeout: timeout},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("opensearch client: %w", err)
	}
	return &Client{api: api}, nil
}

// Readiness probe for main.go - nil when the cluster is reachable and reporting yellow or green
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.api.Cluster.Health(ctx, nil)
	if err != nil {
		return fmt.Errorf("opensearch ping: %w", err)
	}
	if resp.Status != "green" && resp.Status != "yellow" {
		return fmt.Errorf("opensearch unhealthy: status=%s", resp.Status)
	}
	return nil
}

// Idempotent and safe on every boot - use the reindex CLI to drop and recreate when the mapping itself changes
func (c *Client) EnsureIndex(ctx context.Context, name, mappingJSON string) error {
	existsResp, err := c.api.Indices.Exists(ctx, &opensearchapi.IndicesExistsReq{Indices: []string{name}})
	if err == nil && existsResp != nil && existsResp.StatusCode == http.StatusOK {
		return nil
	}
	_, err = c.api.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index:      name,
		BodyReader: strings.NewReader(mappingJSON),
	})
	if err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("create index %s: %w", name, err)
	}
	return nil
}

// For the reindex CLI when the mapping shape changes - never called from server boot
func (c *Client) RecreateIndex(ctx context.Context, name, mappingJSON string) error {
	_, _ = c.api.Indices.Delete(ctx, &opensearchapi.IndicesDeleteReq{Indices: []string{name}})
	_, err := c.api.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index:      name,
		BodyReader: strings.NewReader(mappingJSON),
	})
	if err != nil {
		return fmt.Errorf("create index %s: %w", name, err)
	}
	return nil
}

// id is caller-chosen and stable - we use composite ids like "<productID>:<storeID>:<locale>"
func (c *Client) IndexDoc(ctx context.Context, index, id string, doc any) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal doc: %w", err)
	}
	_, err = c.api.Document.Index(ctx, opensearchapi.IndexReq{
		Index: index,
		ID:    id,
		Body:  bytes.NewReader(body),
	})
	if err != nil {
		return fmt.Errorf("index %s/%s: %w", index, id, err)
	}
	return nil
}

// BulkDoc is one upsert inside a _bulk request
type BulkDoc struct {
	ID  string
	Doc any
}

// A batch is one request but many documents, so it can't live inside the 3s read budget - measured at ~2s for 1000 docs
// against a local cluster, and a loaded or remote one is slower
const bulkRequestTimeout = 2 * time.Minute

// Bulk upserts docs in a single round trip. Reindexing a document at a time cost one HTTP request each and measured ~70 products/s, which put a 100k catalogue out of reach of any sane deadline; the network cost here is per batch instead.
func (c *Client) Bulk(ctx context.Context, index string, docs []BulkDoc) error {
	if len(docs) == 0 {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf) // Encode terminates each value with \n, which is the NDJSON framing _bulk wants
	for _, d := range docs {
		action := map[string]any{"index": map[string]any{"_index": index, "_id": d.ID}}
		if err := enc.Encode(action); err != nil {
			return fmt.Errorf("encode bulk action %s: %w", d.ID, err)
		}
		if err := enc.Encode(d.Doc); err != nil {
			return fmt.Errorf("marshal bulk doc %s: %w", d.ID, err)
		}
	}
	resp, err := c.api.Bulk(withRequestTimeout(ctx, bulkRequestTimeout), opensearchapi.BulkReq{Body: &buf})
	if err != nil {
		return fmt.Errorf("bulk index %s (%d docs): %w", index, len(docs), err)
	}
	if resp == nil || !resp.Errors {
		return nil
	}
	failed := 0
	firstID, firstReason := "", ""
	// v5 types each item as one struct with a slot per action rather than a map keyed by action name
	for _, item := range resp.Items {
		for _, r := range []*opensearchapi.BulkRespItem{item.Index, item.Create, item.Update, item.Delete} {
			if r == nil || r.Error == nil {
				continue
			}
			failed++
			if firstReason == "" {
				firstID, firstReason = deref(r.ID), deref(r.Error.Reason)
			}
		}
	}
	if failed == 0 {
		return nil
	}
	return fmt.Errorf("bulk index %s: %d/%d docs failed - first %s: %s", index, failed, len(docs), firstID, firstReason)
}

// _id is special metadata that won't take prefix queries, so filter on a regular keyword field (product_id / post_id) that the indexer already stamps onto every doc
func (c *Client) DeleteByField(ctx context.Context, index, field, value string) error {
	query := map[string]any{
		"query": map[string]any{
			"term": map[string]any{field: value},
		},
	}
	body, _ := json.Marshal(query)
	_, err := c.api.DeleteByQuery(ctx, &opensearchapi.DeleteByQueryReq{
		Indices:    []string{index},
		BodyReader: bytes.NewReader(body),
	})
	if err != nil {
		return fmt.Errorf("delete by %s=%s on %s: %w", field, value, index, err)
	}
	return nil
}

// The unmarshal callback avoids forcing one Doc type through every caller - products and blog posts have different docs but share the transport machinery
func (c *Client) Search(ctx context.Context, index string, body any, onHit func(raw json.RawMessage) error) (int64, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("marshal search body: %w", err)
	}
	resp, err := c.api.Search(ctx, &opensearchapi.SearchReq{
		Indices:    []string{index},
		BodyReader: bytes.NewReader(bodyBytes),
	})
	if err != nil {
		return 0, fmt.Errorf("search %s: %w", index, err)
	}
	total := totalHits(resp.Hits.Total)
	for _, hit := range resp.Hits.Hits {
		if err := onHit(hit.Source); err != nil {
			return 0, fmt.Errorf("decode hit: %w", err)
		}
	}
	return total, nil
}

// Client.Search drops aggregations on the floor, so the browse path needs this - the JSON round-trip is unfortunate but the SDK won't expose the raw body otherwise
type SearchRawResponse struct {
	Hits struct {
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
		Hits []struct {
			Source json.RawMessage `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
	// Left raw so each caller decodes the agg shapes it expects without the client knowing every aggregation type
	Aggregations map[string]json.RawMessage `json:"aggregations"`
}

func (c *Client) SearchRaw(ctx context.Context, index string, body any) (*SearchRawResponse, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal search body: %w", err)
	}
	resp, err := c.api.Search(ctx, &opensearchapi.SearchReq{
		Indices:    []string{index},
		BodyReader: bytes.NewReader(bodyBytes),
	})
	if err != nil {
		return nil, fmt.Errorf("search %s: %w", index, err)
	}
	// The SDK hands back a parsed envelope with the aggregations still raw inside - re-marshalling is the simplest way to preserve them
	envelope, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("re-encode opensearch response: %w", err)
	}
	var out SearchRawResponse
	if err := json.Unmarshal(envelope, &out); err != nil {
		return nil, fmt.Errorf("decode opensearch response: %w", err)
	}
	return &out, nil
}

// OpenSearch refreshes every ~1s by default - tests and the reindex CLI call this so a follow-up search sees new docs without sleeping
func (c *Client) Refresh(ctx context.Context, index string) error {
	_, err := c.api.Indices.Refresh(ctx, &opensearchapi.IndicesRefreshReq{Indices: []string{index}})
	if err != nil {
		return fmt.Errorf("refresh %s: %w", index, err)
	}
	return nil
}

// v5 returns optional strings as pointers and the hit total as a union - these keep the call sites flat
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func totalHits(t *opensearchapi.SearchHitsMetadataTotal) int64 {
	if t == nil {
		return 0
	}
	if th, err := t.TotalHits(); err == nil {
		return th.Value
	}
	n, err := t.Int64()
	if err != nil {
		return 0
	}
	return n
}

// Two callers can race to create the same index - treat resource_already_exists_exception as success
func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "resource_already_exists_exception")
}

// ListDocIDsByField returns the ids of every doc matching one term. Used by the indexer to find leftover docs after a rebuild, so stale
// siblings can be removed WITHOUT a delete-first pass that would blank the product on any mid-rebuild error
func (c *Client) ListDocIDsByField(ctx context.Context, index, field, value string) ([]string, error) {
	body := map[string]any{
		"query":   map[string]any{"term": map[string]any{field: value}},
		"_source": false,
		"size":    1000,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := c.api.Search(ctx, &opensearchapi.SearchReq{
		Indices:    []string{index},
		BodyReader: bytes.NewReader(raw),
	})
	if err != nil {
		return nil, fmt.Errorf("list doc ids in %s: %w", index, err)
	}
	out := make([]string, 0, len(resp.Hits.Hits))
	for _, h := range resp.Hits.Hits {
		out = append(out, deref(h.ID))
	}
	return out, nil
}

// DeleteDoc removes one document by id - a no-op when it's already gone
func (c *Client) DeleteDoc(ctx context.Context, index, id string) error {
	if _, err := c.api.Document.Delete(ctx, opensearchapi.DeleteReq{Index: index, ID: id}); err != nil {
		return fmt.Errorf("delete doc %s/%s: %w", index, id, err)
	}
	return nil
}

// timeoutTransport bounds every request the OpenSearch client makes. The library retries 3 times by default, so an
// unbounded per-attempt wait turns one slow cluster into a 4x pile-up of blocked storefront goroutines
type timeoutTransport struct {
	base    http.RoundTripper
	timeout time.Duration
}

// Lets a write path opt out of the read budget without loosening it for everyone -
// indexing a batch legitimately takes longer than any storefront query should
type requestTimeoutKey struct{}

func withRequestTimeout(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, requestTimeoutKey{}, d)
}

func (t *timeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	timeout := t.timeout
	if d, ok := req.Context().Value(requestTimeoutKey{}).(time.Duration); ok && d > 0 {
		timeout = d
	}
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	resp, err := t.base.RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	// The body is still streaming - cancel when it closes, not now
	resp.Body = &cancelOnCloseBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

// RawHit is one hit with its id - SearchRawResponse drops ids on purpose (the
// storefront never needs them), but plugin-owned indexes address docs by id
type RawHit struct {
	ID     string
	Source json.RawMessage
}

// SearchHits runs a raw query and returns ids alongside sources - the plugin
// surface's read path, where the caller owns the mapping and decodes its own docs
func (c *Client) SearchHits(ctx context.Context, index string, body any) (int64, []RawHit, map[string]json.RawMessage, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("marshal search body: %w", err)
	}
	resp, err := c.api.Search(ctx, &opensearchapi.SearchReq{
		Indices:    []string{index},
		BodyReader: bytes.NewReader(bodyBytes),
	})
	if err != nil {
		return 0, nil, nil, fmt.Errorf("search %s: %w", index, err)
	}
	hits := make([]RawHit, 0, len(resp.Hits.Hits))
	for _, h := range resp.Hits.Hits {
		hits = append(hits, RawHit{ID: deref(h.ID), Source: h.Source})
	}
	// Aggregations ride along raw for the same reason SearchRawResponse keeps them
	// raw - only the query's author knows each agg's shape
	var aggs map[string]json.RawMessage
	if len(resp.Aggregations) > 0 {
		envelope, merr := json.Marshal(resp)
		if merr == nil {
			var out struct {
				Aggregations map[string]json.RawMessage `json:"aggregations"`
			}
			if json.Unmarshal(envelope, &out) == nil {
				aggs = out.Aggregations
			}
		}
	}
	return totalHits(resp.Hits.Total), hits, aggs, nil
}

// --- alias-flip rebuild support ---

// ResolveAlias returns the physical indexes behind an alias - nil when the name is
// unknown or names a concrete index rather than an alias
func (c *Client) ResolveAlias(ctx context.Context, alias string) ([]string, error) {
	resp, err := c.api.Indices.GetAlias(ctx, &opensearchapi.IndicesGetAliasReq{Name: []string{alias}})
	if err != nil {
		if resp != nil && resp.Inspect().Response != nil && resp.Inspect().Response.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		// A 404 also arrives as a typed API error rather than on the response - matching the error TEXT
		// for "404"/"missing" swallowed unrelated failures whose message happened to contain either
		var apiErr *opensearch.StructError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve alias %s: %w", alias, err)
	}
	indices := resp.Entries
	out := make([]string, 0, len(indices))
	for index, meta := range indices {
		// The API resolves the name as an index EXPRESSION: asking about a concrete index returns it with an empty aliases
		// map - counting that as "the alias points here" would make the alias migration skip and the swap 400
		if _, carries := meta.Aliases[alias]; carries {
			out = append(out, index)
		}
	}
	return out, nil
}

// IndexExists answers for concrete indexes AND aliases - OpenSearch's HEAD treats
// both as "the name resolves"
func (c *Client) IndexExists(ctx context.Context, name string) (bool, error) {
	resp, err := c.api.Indices.Exists(ctx, &opensearchapi.IndicesExistsReq{Indices: []string{name}})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, fmt.Errorf("index exists %s: %w", name, err)
	}
	return resp != nil && resp.StatusCode == http.StatusOK, nil
}

// EnsureAliasedIndex is EnsureIndex for the blue-green world: a missing name is created as a DETERMINISTIC physical (<alias>-000) carrying the alias from birth, so two servers racing at boot collide on resource_already_exists instead of leaving the alias pointing at two indexes - which would fail every write
func (c *Client) EnsureAliasedIndex(ctx context.Context, alias, mappingJSON string) error {
	exists, err := c.IndexExists(ctx, alias)
	if err != nil {
		return err
	}
	if exists {
		return nil // an alias, or a concrete index under that name - the first rebuild converts it
	}
	body, err := withAliasInBody(mappingJSON, alias)
	if err != nil {
		return err
	}
	_, err = c.api.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index:      alias + "-000",
		BodyReader: strings.NewReader(body),
	})
	if err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("create aliased index %s: %w", alias+"-000", err)
	}
	return nil
}

// SwapAlias points the alias at addIndex and removes it from every index in
// removeFrom, in ONE atomic _aliases call - readers never see a gap
func (c *Client) SwapAlias(ctx context.Context, alias, addIndex string, removeFrom []string) error {
	actions := make([]any, 0, len(removeFrom)+1)
	for _, old := range removeFrom {
		actions = append(actions, map[string]any{"remove": map[string]any{"index": old, "alias": alias}})
	}
	actions = append(actions, map[string]any{"add": map[string]any{"index": addIndex, "alias": alias}})
	raw, err := json.Marshal(map[string]any{"actions": actions})
	if err != nil {
		return fmt.Errorf("marshal alias actions: %w", err)
	}
	if _, err := c.api.Indices.UpdateAliases(ctx, &opensearchapi.IndicesUpdateAliasesReq{BodyReader: bytes.NewReader(raw)}); err != nil {
		return fmt.Errorf("swap alias %s -> %s: %w", alias, addIndex, err)
	}
	return nil
}

func (c *Client) DeleteIndex(ctx context.Context, name string) error {
	if _, err := c.api.Indices.Delete(ctx, &opensearchapi.IndicesDeleteReq{Indices: []string{name}}); err != nil {
		return fmt.Errorf("delete index %s: %w", name, err)
	}
	return nil
}

// CreateIndex makes a bare physical index - the rebuild target before its swap
func (c *Client) CreateIndex(ctx context.Context, name, mappingJSON string) error {
	_, err := c.api.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index:      name,
		BodyReader: strings.NewReader(mappingJSON),
	})
	if err != nil {
		return fmt.Errorf("create index %s: %w", name, err)
	}
	return nil
}

// withAliasInBody injects "aliases":{alias:{}} into a create body so index and
// alias appear in one atomic call
func withAliasInBody(mappingJSON, alias string) (string, error) {
	var body map[string]any
	if err := json.Unmarshal([]byte(mappingJSON), &body); err != nil {
		return "", fmt.Errorf("parse mapping for alias injection: %w", err)
	}
	body["aliases"] = map[string]any{alias: map[string]any{}}
	out, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
