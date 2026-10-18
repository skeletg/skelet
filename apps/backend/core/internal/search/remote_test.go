package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

func searchSource(t *testing.T, url string) RemoteSearchSource {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindSearch, Slug: "algolia-ish", Name: "Search", TargetURL: url,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewRemoteSearchSource(svc, remoteprovider.NewInvoker(svc, nil))
}

func TestRemoteSearch(t *testing.T) {
	pid := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"hits": [{"productId": "%s", "sku": "SKU-1", "name": "Widget", "url": "/p/widget"}], "total": 1}`, pid)
	}))
	defer srv.Close()

	hits, total, ok := searchSource(t, srv.URL).Search(context.Background(), "widget", uuid.New(), "en", 20, 0)
	if !ok || total != 1 || len(hits) != 1 {
		t.Fatalf("search: ok=%v total=%d hits=%d", ok, total, len(hits))
	}
	if hits[0].ProductID != pid || hits[0].SKU != "SKU-1" || hits[0].Name != "Widget" || hits[0].URL != "/p/widget" {
		t.Fatalf("hit mapped wrong: %+v", hits[0])
	}
}

func TestRemoteSearchFailOpen(t *testing.T) {
	if _, _, ok := searchSource(t, "http://127.0.0.1:9/down").Search(context.Background(), "widget", uuid.New(), "en", 20, 0); ok {
		t.Fatal("down provider should fall back to OpenSearch (ok=false)")
	}
}

func TestRemoteBrowse(t *testing.T) {
	pid := uuid.New()
	var gotFilter remoteBrowseFilter
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Payload remoteBrowseRequest `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotFilter = body.Payload.Filter
		fmt.Fprintf(w, `{"productIds": ["%s"], "total": 1, "facets": [{"field": "attr_color.keyword", "slug": "color", "buckets": [{"value": "Red", "slug": "red", "count": 3, "isIndexable": true}]}]}`, pid)
	}))
	defer srv.Close()

	cat := uuid.New()
	res, ok := searchSource(t, srv.URL).Browse(context.Background(), uuid.New(), "en", "",
		BrowseFilter{CategoryID: &cat, AttributeIn: []AttributeIn{{Slug: "color", Values: []string{"red"}}}}, "featured", 24, 0)
	if !ok || res.Total != 1 || len(res.IDs) != 1 || res.IDs[0] != pid {
		t.Fatalf("browse result wrong: ok=%v %+v", ok, res)
	}
	if len(res.Facets) != 1 || res.Facets[0].Slug != "color" || len(res.Facets[0].Buckets) != 1 {
		t.Fatalf("facets not mapped: %+v", res.Facets)
	}
	if res.Facets[0].Buckets[0].Value != "Red" || res.Facets[0].Buckets[0].Count != 3 || !res.Facets[0].Buckets[0].IsIndexable {
		t.Fatalf("bucket wrong: %+v", res.Facets[0].Buckets[0])
	}
	// the filter round-tripped to the provider
	if gotFilter.CategoryID != cat.String() || len(gotFilter.AttributeIn) != 1 || gotFilter.AttributeIn[0].Slug != "color" {
		t.Fatalf("filter not sent: %+v", gotFilter)
	}
}

func TestRemoteBrowseFailOpen(t *testing.T) {
	if _, ok := searchSource(t, "http://127.0.0.1:9/down").Browse(context.Background(), uuid.New(), "en", "", BrowseFilter{}, "featured", 24, 0); ok {
		t.Fatal("down provider should fall back to OpenSearch (ok=false)")
	}
}

func TestRemoteSearchPrefersTheChannelsOwnProvider(t *testing.T) {
	answer := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"hits": [{"productId": "%s", "name": "%s"}], "total": 1}`, uuid.New(), name)
		}))
	}
	global, own := answer("global"), answer("own")
	defer global.Close()
	defer own.Close()

	ch := uuid.New()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	for _, in := range []remoteprovider.CreateInput{
		{Kind: remoteprovider.KindSearch, Slug: "a-global", Name: "Global", TargetURL: global.URL},
		{Kind: remoteprovider.KindSearch, Slug: "z-store", Name: "Store", TargetURL: own.URL, ChannelID: &ch},
	} {
		if _, err := svc.Create(context.Background(), in); err != nil {
			t.Fatalf("create provider: %v", err)
		}
	}
	hits, _, ok := NewRemoteSearchSource(svc, remoteprovider.NewInvoker(svc, nil)).Search(context.Background(), "x", ch, "en", 20, 0)
	if !ok || len(hits) != 1 || hits[0].Name != "own" {
		t.Fatalf("search answered by %+v, want the channel's own provider", hits)
	}
}
