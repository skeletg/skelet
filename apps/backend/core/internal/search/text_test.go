package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	opensearch "github.com/opensearch-project/opensearch-go/v5"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

var _ listingCounter = (*catalog.Service)(nil)

func TestStripJSONIndexesOnlyTheWordsAShopperReads(t *testing.T) {
	lexical := `{"root":{"type":"root","direction":"ltr","format":"","indent":0,"version":1,"children":[
		{"type":"paragraph","direction":"ltr","format":"","children":[
			{"type":"text","format":0,"style":"","mode":"normal","text":"Soft "},
			{"type":"text","format":1,"text":"cotton"},
			{"type":"text","text":" tee."}]},
		{"type":"image","src":"https://cdn.example/tee.png","altText":"tee"},
		{"type":"list","listType":"bullet","children":[
			{"type":"listitem","value":1,"children":[{"type":"text","text":"Machine"},{"type":"linebreak"},{"type":"text","text":"washable"}]},
			{"type":"listitem","value":2,"children":[{"type":"link","url":"https://example.com/care","children":[{"type":"text","text":"care guide"}]}]}]}]}}`
	if got, want := stripJSON(json.RawMessage(lexical)), "Soft cotton tee. Machine washable care guide"; got != want {
		t.Errorf("lexical = %q, want %q", got, want)
	}
	blocks := `{"version":1,"blocks":[{"type":"hero","props":{"title":"Summer sale","href":"/sale"}}]}`
	if got := stripJSON(json.RawMessage(blocks)); got != "Summer sale" {
		t.Errorf("block document = %q, want its human text", got)
	}
	if got := stripJSON(json.RawMessage(`"plain words"`)); got != "plain words" {
		t.Errorf("plain string = %q", got)
	}
}

func TestAStructuredAttributeIndexesItsWordsNotGoSyntax(t *testing.T) {
	attr := catalog.Attribute{Slug: "tagline", InputType: catalog.InputTypePlainText}
	p := catalog.Product{Attributes: map[string]any{"tagline": map[string]any{"en": "Red", "de": "Rot"}}}
	if got := extractAttributeValue(p, attr); got != "Rot Red" {
		t.Errorf("map value = %q, want its strings in key order", got)
	}
	p.Attributes["tagline"] = map[string]any{"root": map[string]any{"children": []any{map[string]any{"type": "paragraph", "children": []any{map[string]any{"type": "text", "text": "Hand made"}}}}}}
	if got := extractAttributeValue(p, attr); got != "Hand made" {
		t.Errorf("rich-text value = %q, want its text", got)
	}
}

type countingLister struct{ calls int }

func (c *countingLister) ListAttributes(context.Context) ([]catalog.Attribute, error) {
	c.calls++
	return []catalog.Attribute{{Slug: "color"}}, nil
}

func TestAttributeCacheServesRepeatsUntilInvalidated(t *testing.T) {
	src := &countingLister{}
	c := NewAttributeCache(src, time.Hour)
	for range 2 {
		got, _ := c.List(context.Background())
		got[0].Slug = "mutated"
	}
	again, _ := c.List(context.Background())
	if src.calls != 1 || again[0].Slug != "color" {
		t.Fatalf("calls=%d slug=%q, want one load and a cache the caller can't mutate", src.calls, again[0].Slug)
	}
	c.Invalidate()
	_, _ = c.List(context.Background())
	if src.calls != 2 {
		t.Fatalf("calls=%d after Invalidate, want a reload", src.calls)
	}
	expired := NewAttributeCache(src, 0)
	_, _ = expired.List(context.Background())
	_, _ = expired.List(context.Background())
	if src.calls != 4 {
		t.Fatalf("calls=%d, want an expired entry reloaded", src.calls)
	}
}

func TestIsAlreadyExistsReadsTheErrorType(t *testing.T) {
	if !isAlreadyExists(&opensearch.StructError{Err: opensearch.Err{Type: "resource_already_exists_exception"}, Status: 400}) {
		t.Error("the typed already-exists error was not recognised")
	}
	if isAlreadyExists(errors.New("index blocked: resource_already_exists_exception mentioned in a reason")) {
		t.Error("an untyped error matched on its text")
	}
	if isAlreadyExists(nil) {
		t.Error("nil matched")
	}
}

func TestSearchRawKeepsTheAggregationsAndHits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"took":1,"hits":{"total":{"value":2,"relation":"eq"},"hits":[{"_id":"a","_index":"p","_source":{"sku":"A"}}]},
			"aggregations":{"color":{"buckets":[{"key":"Red","doc_count":2}]}}}`))
	}))
	defer srv.Close()
	client, err := NewClient([]string{srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.SearchRawRouted(context.Background(), "p", "", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Hits.Total.Value != 2 || len(resp.Hits.Hits) != 1 || string(resp.Hits.Hits[0].Source) != `{"sku":"A"}` {
		t.Fatalf("hits = %+v", resp.Hits)
	}
	if agg := string(resp.Aggregations["color"]); !strings.Contains(agg, `"key":"Red"`) {
		t.Fatalf("color aggregation = %s", agg)
	}
}

func TestBuildSearchQuery(t *testing.T) {
	q := func(attrs []Attribute) string {
		b, _ := json.Marshal(buildSearchQuery("tee", attrs))
		return string(b)
	}
	if got := q([]Attribute{{Name: "name", Searchable: false}}); got != `{"match_all":{}}` {
		t.Errorf("nothing searchable = %s, want match_all", got)
	}
	same := q([]Attribute{{Name: "name", Searchable: true, Weight: 3, MatchType: matchMultiMatch, Fuzziness: "AUTO"}, {Name: "sku", Searchable: true, Weight: 2, MatchType: matchMultiMatch, Fuzziness: "AUTO"}})
	if !strings.Contains(same, `"multi_match"`) || !strings.Contains(same, `"name^3"`) || !strings.Contains(same, `"sku^2"`) || !strings.Contains(same, `"fuzziness":"AUTO"`) {
		t.Errorf("homogeneous = %s, want one weighted fuzzy multi_match", same)
	}
	mixed := q([]Attribute{{Name: "name", Searchable: true, Weight: 3, MatchType: matchBoolPrefix}, {Name: "sku", Searchable: true, Weight: 2, MatchType: matchExact}})
	if !strings.Contains(mixed, `"should"`) || !strings.Contains(mixed, `"match_bool_prefix"`) || !strings.Contains(mixed, `"case_insensitive":true`) {
		t.Errorf("mixed = %s, want a should of per-field clauses", mixed)
	}
}

func TestABulkIgnoresCreateConflictsButNotRealFailures(t *testing.T) {
	items := `{"create":{"_id":"a","_index":"p","status":409,"error":{"type":"version_conflict_engine_exception","reason":"exists"}}}`
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/_bulk" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"missing","status":404}`))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	client, err := NewClient([]string{srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	docs := []BulkDoc{{ID: "a", Doc: map[string]any{}, IfAbsent: true}}
	body = `{"took":1,"errors":true,"items":[` + items + `]}`
	if err := client.Bulk(context.Background(), "p", docs); err != nil {
		t.Fatalf("a create conflict failed the bulk: %v", err)
	}
	body = `{"took":1,"errors":true,"items":[` + items + `,{"index":{"_id":"b","_index":"p","status":400,"error":{"type":"mapper_parsing_exception","reason":"bad"}}}]}`
	if err := client.Bulk(context.Background(), "p", docs); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("bulk = %v, want the mapping failure reported", err)
	}
}
