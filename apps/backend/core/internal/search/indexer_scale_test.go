package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
)

type indexCatalog struct {
	ProductSource
	products map[uuid.UUID]catalog.Product
	listings []catalog.ProductChannelListing
}

func (f *indexCatalog) GetProductWithAttributes(_ context.Context, id uuid.UUID) (catalog.Product, error) {
	return f.products[id], nil
}
func (f *indexCatalog) ListAttributes(context.Context) ([]catalog.Attribute, error) { return nil, nil }
func (f *indexCatalog) ListProducts(context.Context, string, int32) ([]catalog.Product, error) {
	out := make([]catalog.Product, 0, len(f.products))
	for _, p := range f.products {
		out = append(out, p)
	}
	return out, nil
}
func (f *indexCatalog) ListCategoriesByProduct(context.Context, uuid.UUID) ([]catalog.Category, error) {
	return nil, nil
}
func (f *indexCatalog) ListCollectionsByProduct(context.Context, uuid.UUID) ([]catalog.Collection, error) {
	return nil, nil
}
func (f *indexCatalog) ListProductChannelListings(_ context.Context, id uuid.UUID) ([]catalog.ProductChannelListing, error) {
	var out []catalog.ProductChannelListing
	for _, l := range f.listings {
		if l.ProductID == id {
			out = append(out, l)
		}
	}
	return out, nil
}
func (f *indexCatalog) ListProductChannelListingsByChannel(_ context.Context, ch uuid.UUID) ([]catalog.ProductChannelListing, error) {
	var out []catalog.ProductChannelListing
	for _, l := range f.listings {
		if l.ChannelID == ch {
			out = append(out, l)
		}
	}
	return out, nil
}
func (f *indexCatalog) ListProductChannelListingsByProductIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID][]catalog.ProductChannelListing, error) {
	out := map[uuid.UUID][]catalog.ProductChannelListing{}
	for _, id := range ids {
		out[id], _ = f.ListProductChannelListings(context.Background(), id)
	}
	return out, nil
}
func (f *indexCatalog) ListProductAttributeLocaleValues(context.Context, uuid.UUID) ([]catalog.ProductAttributeLocaleValue, error) {
	return nil, nil
}
func (f *indexCatalog) ListProductAttributeSlugs(context.Context, uuid.UUID) ([]catalog.ProductAttributeSlug, error) {
	return nil, nil
}
func (f *indexCatalog) LoadProductsContent(context.Context, []uuid.UUID, string) (map[uuid.UUID]catalog.ProductContent, error) {
	return nil, nil
}
func (f *indexCatalog) ListProductsWithAttributesByIDs(_ context.Context, ids []uuid.UUID) ([]catalog.Product, error) {
	out := make([]catalog.Product, 0, len(ids))
	for _, id := range ids {
		out = append(out, f.products[id])
	}
	return out, nil
}
func (f *indexCatalog) ListCategoriesByProductIDs(context.Context, []uuid.UUID, string) (map[uuid.UUID][]catalog.Category, error) {
	return nil, nil
}
func (f *indexCatalog) ListCollectionsByProductIDs(context.Context, []uuid.UUID, string) (map[uuid.UUID][]catalog.Collection, error) {
	return nil, nil
}
func (f *indexCatalog) ListProductAttributeSlugsByProductIDs(context.Context, []uuid.UUID) ([]catalog.ProductAttributeSlug, error) {
	return nil, nil
}
func (f *indexCatalog) ListProductAttributeLocaleValuesByProductIDs(context.Context, []uuid.UUID) ([]catalog.ProductAttributeLocaleValue, error) {
	return nil, nil
}

type fakeChannels struct {
	all            []channel.Channel
	gets, listings int
}

func (f *fakeChannels) List(context.Context) ([]channel.Channel, error) {
	f.listings++
	return f.all, nil
}
func (f *fakeChannels) GetByID(_ context.Context, id uuid.UUID) (channel.Channel, error) {
	f.gets++
	for _, c := range f.all {
		if c.ID == id {
			return c, nil
		}
	}
	return channel.Channel{}, channel.ErrChannelNotFound
}

// recordingCluster answers like OpenSearch and keeps every request it saw
type recordingCluster struct {
	mu       sync.Mutex
	requests []string
	bulk     []map[string]any
	existing []string
	aliases  map[string][]string
}

func (c *recordingCluster) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	req := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		req += "?" + r.URL.RawQuery
	}
	c.requests = append(c.requests, req)
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/_bulk"):
		sc := bufio.NewScanner(bytes.NewReader(body))
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var line map[string]any
			_ = json.Unmarshal(sc.Bytes(), &line)
			for _, verb := range []string{"index", "create", "delete"} {
				if _, ok := line[verb]; ok {
					c.bulk = append(c.bulk, line)
				}
			}
		}
		_, _ = w.Write([]byte(`{"took":1,"errors":false,"items":[]}`))
	case strings.HasSuffix(r.URL.Path, "/_search"):
		hits := make([]string, 0, len(c.existing))
		for _, id := range c.existing {
			hits = append(hits, `{"_id":"`+id+`","_index":"products_v1"}`)
		}
		_, _ = w.Write([]byte(`{"took":1,"hits":{"total":{"value":0,"relation":"eq"},"hits":[` + strings.Join(hits, ",") + `]}}`))
	case strings.HasPrefix(r.URL.Path, "/_alias/"):
		name := strings.TrimPrefix(r.URL.Path, "/_alias/")
		indices := c.aliases[name]
		if len(indices) == 0 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"alias [` + name + `] missing","status":404}`))
			return
		}
		entries := make([]string, 0, len(indices))
		for _, idx := range indices {
			entries = append(entries, `"`+idx+`":{"aliases":{"`+name+`":{}}}`)
		}
		_, _ = w.Write([]byte(`{` + strings.Join(entries, ",") + `}`))
	case r.URL.Path == "/_aliases":
		c.requests[len(c.requests)-1] += " " + string(body)
		_, _ = w.Write([]byte(`{"acknowledged":true}`))
	case strings.HasSuffix(r.URL.Path, "/_delete_by_query"):
		c.requests[len(c.requests)-1] += " " + string(body)
		_, _ = w.Write([]byte(`{"took":1,"deleted":0,"failures":[]}`))
	default:
		_, _ = w.Write([]byte(`{"result":"created","_id":"x","_index":"products_v1"}`))
	}
}

func (c *recordingCluster) indexesWritten() map[string]int {
	out := map[string]int{}
	for _, line := range c.bulk {
		for _, meta := range line {
			if m, ok := meta.(map[string]any); ok {
				out[m["_index"].(string)]++
			}
		}
	}
	return out
}

func (c *recordingCluster) count(verb string) int {
	n := 0
	for _, line := range c.bulk {
		if _, ok := line[verb]; ok {
			n++
		}
	}
	return n
}

func manyChannelFixture(t *testing.T, channels, products int) (*ProductIndexer, *recordingCluster, *fakeChannels, *indexCatalog) {
	t.Helper()
	cluster := &recordingCluster{}
	srv := httptest.NewServer(http.HandlerFunc(cluster.serve))
	t.Cleanup(srv.Close)
	client, err := NewClient([]string{srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	chs := &fakeChannels{}
	for range channels {
		chs.all = append(chs.all, channel.Channel{ID: uuid.New(), DefaultLocale: "en", Locales: []locale.Locale{{Code: "en", IsActive: true}, {Code: "de", IsActive: true}}})
	}
	cat := &indexCatalog{products: map[uuid.UUID]catalog.Product{}}
	for range products {
		p := catalog.Product{ID: uuid.New(), SKU: "SKU-" + uuid.NewString()[:6]}
		cat.products[p.ID] = p
		for _, c := range chs.all {
			cat.listings = append(cat.listings, catalog.ProductChannelListing{ProductID: p.ID, ChannelID: c.ID, IsPublished: true})
		}
	}
	return NewProductIndexer(client, cat, chs, nil, nil), cluster, chs, cat
}

func TestAProductOnManyChannelsIndexesInOneBulkWithOneChannelRead(t *testing.T) {
	idx, cluster, chs, cat := manyChannelFixture(t, 20, 1)
	var pid uuid.UUID
	for id := range cat.products {
		pid = id
	}
	cluster.existing = []string{docID(pid, uuid.New(), "fr")}
	if err := idx.Upsert(context.Background(), pid); err != nil {
		t.Fatal(err)
	}
	bulks := 0
	for _, r := range cluster.requests {
		if strings.HasSuffix(r, "/_bulk") {
			bulks++
		}
		if strings.HasPrefix(r, "PUT") || strings.Contains(r, "/_doc/") {
			t.Fatalf("a doc went out on its own request %q, want everything in the _bulk", r)
		}
	}
	if bulks != 1 || cluster.count("index") != 40 || cluster.count("delete") != 1 {
		t.Fatalf("%d bulks with %d docs and %d deletes, want 1 bulk of 40 docs plus the stale one's delete", bulks, cluster.count("index"), cluster.count("delete"))
	}
	if chs.gets != 0 || chs.listings != 1 {
		t.Fatalf("read channels %d times one by one and listed them %d times, want a single list", chs.gets, chs.listings)
	}
}

func TestAChannelReindexWritesOnlyThatChannelsDocs(t *testing.T) {
	idx, cluster, chs, _ := manyChannelFixture(t, 5, 3)
	target := chs.all[2].ID
	n, err := idx.ReindexChannel(context.Background(), target)
	if err != nil || n != 3 {
		t.Fatalf("reindex = %d, %v, want the channel's 3 products", n, err)
	}
	for _, line := range cluster.bulk {
		meta, _ := line["index"].(map[string]any)
		if id, _ := meta["_id"].(string); !strings.Contains(id, target.String()) {
			t.Fatalf("a channel reindex wrote %q, which belongs to another channel", id)
		}
	}
	if cluster.count("index") != 6 {
		t.Fatalf("wrote %d docs, want the channel's 3 products in its 2 locales", cluster.count("index"))
	}
	pruned := false
	for _, r := range cluster.requests {
		if strings.Contains(r, "_delete_by_query") && strings.Contains(r, target.String()) && strings.Contains(r, "must_not") {
			pruned = true
		}
	}
	if !pruned {
		t.Fatal("the channel's docs for locales it dropped were never removed")
	}
}

func TestAnUnsetShardCountGrowsWithTheCatalogAndRoutesPastOne(t *testing.T) {
	small, _ := buildProductMapping(context.Background(), nil, 1000)
	if strings.Contains(small, `"_routing"`) {
		t.Fatal("a one-shard index was made to require routing")
	}
	big, _ := buildProductMapping(context.Background(), nil, 50_000_000)
	if !strings.Contains(big, `"number_of_shards":5`) || !strings.Contains(big, `"_routing":{"required":true}`) {
		t.Fatalf("a 50M-doc mapping = %s, want five shards routed by channel", big)
	}
}

func TestEveryWriteIsRoutedEvenFromAProcessThatBootedUnrouted(t *testing.T) {
	idx, cluster, chs, cat := manyChannelFixture(t, 3, 1)
	var pid uuid.UUID
	for id := range cat.products {
		pid = id
	}
	stale := docID(pid, uuid.New(), "fr")
	cluster.existing = []string{stale}
	if err := idx.Upsert(context.Background(), pid); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.ReindexChannel(context.Background(), chs.all[0].ID); err != nil {
		t.Fatal(err)
	}
	if len(cluster.bulk) < 3 {
		t.Fatalf("only %d bulk lines recorded", len(cluster.bulk))
	}
	for _, line := range cluster.bulk {
		for _, verb := range []string{"index", "delete"} {
			meta, ok := line[verb].(map[string]any)
			if !ok {
				continue
			}
			id, _ := meta["_id"].(string)
			if meta["routing"] != docRouting(id) || docRouting(id) == "" {
				t.Fatalf("%s %s carried routing %v, want its channel", verb, id, meta["routing"])
			}
		}
	}
}

func TestTheClientReadsRoutingOffALiveIndex(t *testing.T) {
	url := os.Getenv("OPENSEARCH_URL")
	if url == "" {
		t.Skip("set OPENSEARCH_URL to exercise a live cluster")
	}
	client, err := NewClient([]string{url})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	name := "test-routing-" + uuid.NewString()
	if err := client.CreateIndex(ctx, name, `{"settings":{"index":{"number_of_shards":2,"number_of_replicas":0}},"mappings":{"_routing":{"required":true},"properties":{"channel_id":{"type":"keyword"},"locale":{"type":"keyword"}}}}`); err != nil {
		t.Skipf("create index: %v", err)
	}
	t.Cleanup(func() { _ = client.DeleteIndex(context.Background(), name) })
	routed, err := client.RoutingRequired(ctx, name)
	if err != nil || !routed {
		t.Fatalf("routing required = %v, %v, want true", routed, err)
	}
	ch := uuid.NewString()
	docs := []BulkDoc{
		{ID: "p:" + ch + ":en", Doc: map[string]any{"channel_id": ch, "locale": "en"}, Routing: ch},
		{ID: "p:" + ch + ":fr", Doc: map[string]any{"channel_id": ch, "locale": "fr"}, Routing: ch},
	}
	if err := client.Bulk(ctx, name, docs); err != nil {
		t.Fatalf("routed bulk: %v", err)
	}
	_ = client.Refresh(ctx, name)
	if err := client.DeleteByQuery(ctx, name, map[string]any{"bool": map[string]any{
		"filter":   []any{map[string]any{"term": map[string]any{"channel_id": ch}}},
		"must_not": []any{map[string]any{"terms": map[string]any{"locale": []string{"en"}}}},
	}}); err != nil {
		t.Fatal(err)
	}
	_ = client.Refresh(ctx, name)
	ids, err := client.ListDocIDsByField(ctx, name, "channel_id", ch)
	if err != nil || len(ids) != 1 || ids[0] != "p:"+ch+":en" {
		t.Fatalf("after pruning the dropped locale the channel holds %v, %v, want only its en doc", ids, err)
	}
	if err := client.Bulk(ctx, name, []BulkDoc{{ID: "p:" + ch + ":en", Delete: true, Routing: ch}, {ID: "gone:" + ch + ":xx", Delete: true, Routing: ch}}); err != nil {
		t.Fatalf("routed delete, including one already gone: %v", err)
	}
}

func TestLiveWritesReachAnInFlightRebuild(t *testing.T) {
	idx, cluster, _, cat := manyChannelFixture(t, 2, 1)
	cluster.aliases = map[string][]string{NextAlias(ProductIndex): {"products_v1-1700000000"}}
	var pid uuid.UUID
	for id := range cat.products {
		pid = id
	}
	cluster.existing = []string{docID(pid, uuid.New(), "fr")}
	if err := idx.Upsert(context.Background(), pid); err != nil {
		t.Fatal(err)
	}
	if got := cluster.indexesWritten(); got[ProductIndex] != 5 || got["products_v1-1700000000"] != 5 {
		t.Fatalf("bulk lines per index = %v, want the 4 docs and the stale delete in both the live index and the rebuild", got)
	}
	if err := idx.Delete(context.Background(), pid); err != nil {
		t.Fatal(err)
	}
	last := cluster.requests[len(cluster.requests)-1]
	if !strings.Contains(last, "/products_v1,products_v1-1700000000/_delete_by_query") || !strings.Contains(last, "conflicts=proceed") {
		t.Fatalf("delete went out as %q, want both indexes past version conflicts", last)
	}
}

func TestARebuildNeverOverwritesALiveWrite(t *testing.T) {
	idx, cluster, _, _ := manyChannelFixture(t, 1, 2)
	if _, err := idx.reindexAllInto(context.Background(), "products_v1-1700000000"); err != nil {
		t.Fatal(err)
	}
	if cluster.count("create") != 4 || cluster.count("index") != 0 {
		t.Fatalf("rebuild wrote %d creates and %d overwrites, want creates only", cluster.count("create"), cluster.count("index"))
	}
	cluster.bulk = nil
	if _, err := idx.ReindexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cluster.count("index") != 4 || cluster.count("create") != 0 {
		t.Fatal("a reindex into the live index must overwrite what's there")
	}
}

func TestARebuildOpensItsNextAliasBeforeBuildingAndClosesItAfter(t *testing.T) {
	_, cluster, _, _ := manyChannelFixture(t, 1, 1)
	srv := httptest.NewServer(http.HandlerFunc(cluster.serve))
	t.Cleanup(srv.Close)
	client, err := NewClient([]string{srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	cluster.aliases = map[string][]string{ProductIndex: {"products_v1-000"}}
	var during []string
	_, err = rebuildAndSwap(context.Background(), client, ProductIndex, `{}`, func(context.Context, string) (int, error) {
		during = append([]string(nil), cluster.requests...)
		return 1, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	opened := slices.IndexFunc(during, func(r string) bool {
		return strings.Contains(r, `"add"`) && strings.Contains(r, NextAlias(ProductIndex))
	})
	closed := slices.IndexFunc(cluster.requests, func(r string) bool {
		return strings.Contains(r, `"remove"`) && strings.Contains(r, NextAlias(ProductIndex))
	})
	if opened < 0 || closed < 0 {
		t.Fatalf("next alias opened at %d, closed at %d - want both:\n%s", opened, closed, strings.Join(cluster.requests, "\n"))
	}
}

type countedCatalog struct {
	*indexCatalog
	listings map[uuid.UUID]int64
}

func (c countedCatalog) CountProductChannelListingsByChannel(context.Context) (map[uuid.UUID]int64, error) {
	return c.listings, nil
}

func TestShardSizingCountsListingsInsteadOfLoadingTheCatalogue(t *testing.T) {
	idx, _, chs, cat := manyChannelFixture(t, 2, 0)
	idx.catalog = countedCatalog{indexCatalog: cat, listings: map[uuid.UUID]int64{chs.all[0].ID: 1000, chs.all[1].ID: 10}}
	if got := idx.estimateDocs(context.Background()); got != 2020 {
		t.Fatalf("estimate = %d, want each channel's listings times its 2 locales", got)
	}
}
