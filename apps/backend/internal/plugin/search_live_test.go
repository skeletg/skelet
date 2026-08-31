package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/search"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The whole plugin-index seam against a real cluster - registration replay, physical index creation, writer round-trip,
// operator reindex, containment. Skips without OPENSEARCH_URL; the dev stack has one, CI without a cluster skips
func TestPluginSearchIndexLive(t *testing.T) {
	url := os.Getenv("OPENSEARCH_URL")
	if url == "" {
		t.Skip("no OPENSEARCH_URL")
	}
	client, err := search.NewClient([]string{url})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const physical = "plg-livetest-notes"
	t.Cleanup(func() { _ = client.DeleteIndex(ctx, physical) })
	_ = client.DeleteIndex(ctx, physical) // debris from a previous run

	h := &Host{}
	reg := sdk.SearchIndexRegistration{
		Name:        "notes",
		MappingJSON: `{"mappings":{"properties":{"body":{"type":"text"},"note_id":{"type":"keyword"}}}}`,
		Reindex: func(ctx context.Context, sink sdk.SearchIndexSink) (int, error) {
			for i := range 3 {
				if err := sink.Add(ctx, fmt.Sprintf("seed-%d", i), map[string]any{"body": "seeded note", "note_id": "seed"}); err != nil {
					return i, err
				}
			}
			return 3, nil
		},
	}
	if err := h.RegisterSearchIndex("livetest", reg); err != nil {
		t.Fatalf("register: %v", err)
	}

	registry := search.NewIndexRegistry()
	h.setSearchTier(registry, client) // replays the pending registration and ensures the physical index

	exists, err := client.IndexExists(ctx, physical)
	if err != nil || !exists {
		t.Fatalf("physical index after wire: exists=%v err=%v", exists, err)
	}

	// Live writer round-trip through the Host methods the sdk writer delegates to -
	// Refresh included, so a plugin's read-after-write needs no internal client access
	if err := h.SearchIndexDoc(ctx, "livetest", "notes", "n1", map[string]any{"body": "hello from the plugin", "note_id": "n1"}); err != nil {
		t.Fatalf("index doc: %v", err)
	}
	if err := h.SearchRefresh(ctx, "livetest", "notes"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	res, err := h.SearchQuery(ctx, "livetest", "notes", map[string]any{"query": map[string]any{"match": map[string]any{"body": "hello"}}})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Total != 1 || res.Hits[0].ID != "n1" {
		t.Fatalf("search = total %d hits %+v, want the one doc", res.Total, res.Hits)
	}
	var doc struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(res.Hits[0].Source, &doc); err != nil || doc.Body == "" {
		t.Fatalf("hit source did not round-trip: %v %q", err, doc.Body)
	}

	// Bulk write: one round trip for the batch
	if err := h.SearchIndexDocs(ctx, "livetest", "notes", []sdk.SearchDocument{
		{ID: "b1", Doc: map[string]any{"body": "bulk one", "note_id": "bulk"}},
		{ID: "b2", Doc: map[string]any{"body": "bulk two", "note_id": "bulk"}},
	}); err != nil {
		t.Fatalf("bulk index: %v", err)
	}
	if err := h.SearchRefresh(ctx, "livetest", "notes"); err != nil {
		t.Fatal(err)
	}
	// Aggregations ride back raw when the query asks for them
	res, err = h.SearchQuery(ctx, "livetest", "notes", map[string]any{
		"size": 0,
		"aggs": map[string]any{"by_note": map[string]any{"terms": map[string]any{"field": "note_id"}}},
	})
	if err != nil {
		t.Fatalf("agg query: %v", err)
	}
	if res.Total != 3 {
		t.Fatalf("agg query total = %d, want 3 (n1 + 2 bulk)", res.Total)
	}
	var agg struct {
		Buckets []struct {
			Key      string `json:"key"`
			DocCount int    `json:"doc_count"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal(res.Aggregations["by_note"], &agg); err != nil {
		t.Fatalf("decode aggregation: %v (raw: %s)", err, res.Aggregations["by_note"])
	}
	if len(agg.Buckets) != 2 || agg.Buckets[0].DocCount != 2 {
		t.Fatalf("aggregation buckets = %+v, want bulk=2 and n1=1", agg.Buckets)
	}

	// The operator reindex path: recreate-in-place + the plugin's Reindex sink
	results, err := registry.ReindexTargets(ctx, []string{"livetest-notes"})
	if err != nil || results[0].Err != nil || results[0].Count != 3 {
		t.Fatalf("reindex = %+v, err=%v, want 3 clean", results, err)
	}
	if err := h.SearchRefresh(ctx, "livetest", "notes"); err != nil {
		t.Fatal(err)
	}
	res, err = h.SearchQuery(ctx, "livetest", "notes", map[string]any{"query": map[string]any{"match_all": map[string]any{}}})
	if err != nil || res.Total != 3 {
		t.Fatalf("after rebuild: total=%d err=%v, want the 3 seeded docs (the live write predates the recreate)", res.Total, err)
	}

	// DeleteByField clears everything derived from one record
	if err := h.SearchDeleteByField(ctx, "livetest", "notes", "note_id", "seed"); err != nil {
		t.Fatalf("delete by field: %v", err)
	}
	if err := h.SearchRefresh(ctx, "livetest", "notes"); err != nil {
		t.Fatal(err)
	}
	res, _ = h.SearchQuery(ctx, "livetest", "notes", map[string]any{"query": map[string]any{"match_all": map[string]any{}}})
	if res.Total != 0 {
		t.Fatalf("after delete-by-field: %d docs remain", res.Total)
	}

	// Containment: another plugin's name never reaches the cluster
	if err := h.SearchIndexDoc(ctx, "someone-else", "notes", "x", nil); err == nil {
		t.Fatal("cross-plugin write accepted")
	}
}
