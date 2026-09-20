package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/search"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// fakeCluster records every _bulk request the sink ships and answers like a healthy OpenSearch, so the paging behaviour is observed on the wire rather than inferred
type fakeCluster struct {
	mu       sync.Mutex
	bulks    [][]string // per _bulk request, the action lines
	requests []string   // every method+path, for asserting nothing unexpected fired
}

func (f *fakeCluster) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/_bulk") || r.URL.Path == "/_bulk" {
			var actions []string
			sc := bufio.NewScanner(r.Body)
			sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
			odd := false
			for sc.Scan() {
				if !odd {
					actions = append(actions, sc.Text())
				}
				odd = !odd
			}
			f.bulks = append(f.bulks, actions)
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"took":1,"errors":false,"items":[]}`))
	})
}

func (f *fakeCluster) bulkSizes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int, len(f.bulks))
	for i, b := range f.bulks {
		out[i] = len(b)
	}
	return out
}

func clusterAndLifecycle(t *testing.T, docs int) (*fakeCluster, *pluginIndexLifecycle) {
	t.Helper()
	f := &fakeCluster{}
	ts := httptest.NewServer(f.handler())
	t.Cleanup(ts.Close)
	client, err := search.NewClient([]string{ts.URL})
	if err != nil {
		t.Fatalf("client against the fake cluster: %v", err)
	}
	life := &pluginIndexLifecycle{
		client:  client,
		index:   pluginIndexName("acme", "faq"),
		mapping: `{"mappings":{}}`,
		reindex: func(ctx context.Context, sink sdk.SearchIndexSink) (int, error) {
			for i := range docs {
				if err := sink.Add(ctx, "doc-"+strconv.Itoa(i), map[string]any{"n": i}); err != nil {
					return i, err
				}
			}
			return docs, nil
		},
	}
	return f, life
}

func TestReindexPagesInto500DocBulks(t *testing.T) { // the whole point of the sink - per-doc writes are the wall the core reindex was rebuilt to escape
	f, life := clusterAndLifecycle(t, 1250)

	n, err := life.ReindexAll(context.Background())
	if err != nil {
		t.Fatalf("ReindexAll: %v", err)
	}
	if n != 1250 {
		t.Errorf("count = %d, want the reindex func's 1250 reported", n)
	}
	sizes := f.bulkSizes()
	if len(sizes) != 3 || sizes[0] != 500 || sizes[1] != 500 || sizes[2] != 250 {
		t.Errorf("bulk pages = %v, want [500 500 250] - full pages plus the flushed tail", sizes)
	}
}

func TestReindexFlushesThePartialTail(t *testing.T) { // 10 docs never reach the page size, and losing them would mean every small plugin index reindexes to empty
	f, life := clusterAndLifecycle(t, 10)

	if _, err := life.ReindexAll(context.Background()); err != nil {
		t.Fatalf("ReindexAll: %v", err)
	}
	sizes := f.bulkSizes()
	if len(sizes) != 1 || sizes[0] != 10 {
		t.Errorf("bulk pages = %v, want the one 10-doc tail flush", sizes)
	}
}

func TestReindexTargetsThePluginsPhysicalIndex(t *testing.T) { // the plg- prefix is the containment - a doc landing in a core index would be served to storefront shoppers
	f, life := clusterAndLifecycle(t, 3)

	if _, err := life.ReindexAll(context.Background()); err != nil {
		t.Fatalf("ReindexAll: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bulks) != 1 {
		t.Fatalf("%d bulk requests, want 1", len(f.bulks))
	}
	for _, action := range f.bulks[0] {
		var a map[string]map[string]any
		if err := json.Unmarshal([]byte(action), &a); err != nil {
			t.Fatalf("action line %q: %v", action, err)
		}
		if idx := a["index"]["_index"]; idx != "plg-acme-faq" {
			t.Errorf("doc addressed to %v, want plg-acme-faq", idx)
		}
	}
}

func TestReindexAllWithNoReindexFunc(t *testing.T) { // live-writes-only indexes reindex to empty by contract, with zero cluster traffic
	f := &fakeCluster{}
	ts := httptest.NewServer(f.handler())
	t.Cleanup(ts.Close)
	client, err := search.NewClient([]string{ts.URL})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	life := &pluginIndexLifecycle{client: client, index: "plg-acme-log", mapping: "{}"}

	n, err := life.ReindexAll(context.Background())
	if err != nil || n != 0 {
		t.Errorf("ReindexAll = (%d, %v), want (0, nil)", n, err)
	}
	f.mu.Lock()
	reqs := len(f.requests)
	f.mu.Unlock()
	if reqs != 0 {
		t.Errorf("%d cluster requests for a nil-reindex rebuild, want none", reqs)
	}
}

func TestReindexZeroDocsSendsNothing(t *testing.T) { // an empty source must not ship an empty _bulk, which OpenSearch 400s on
	f, life := clusterAndLifecycle(t, 0)
	if _, err := life.ReindexAll(context.Background()); err != nil {
		t.Fatalf("ReindexAll: %v", err)
	}
	if sizes := f.bulkSizes(); len(sizes) != 0 {
		t.Errorf("bulk pages = %v, want none", sizes)
	}
}

func TestSearchIndexNameBoundaries(t *testing.T) { // the name goes into a physical index name verbatim, so the regex is the only thing between a plugin and an invalid or colliding index
	for name, ok := range map[string]bool{
		"faq":                   true,
		"a":                     true,
		"faq-2":                 true,
		"0day":                  true,
		strings.Repeat("a", 64): true,
		strings.Repeat("a", 65): false,
		"":                      false,
		"FAQ":                   false,
		"faq_2":                 false,
		"-faq":                  false,
		"faq.v2":                false,
		"plg-escape":            true, // the prefix is added by the host, a plugin naming one plg- just gets plg-<slug>-plg-escape
	} {
		if got := searchIndexName.MatchString(name); got != ok {
			t.Errorf("searchIndexName(%q) = %v, want %v", name, got, ok)
		}
	}
}

func TestPluginIndexNameContainment(t *testing.T) {
	if got := pluginIndexName("acme", "faq"); got != "plg-acme-faq" {
		t.Errorf("physical name = %q, want plg-acme-faq", got)
	}
	// two plugins registering the same logical name must land in different physical indexes
	if pluginIndexName("acme", "faq") == pluginIndexName("other", "faq") {
		t.Error("two plugins share a physical index")
	}
	if !strings.HasPrefix(pluginIndexName("x", "y"), "plg-") {
		t.Error("a plugin index escaped the plg- namespace")
	}
}
