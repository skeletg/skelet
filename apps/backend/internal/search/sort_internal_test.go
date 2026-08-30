package search

import (
	"context"
	"testing"
)

// Browse ordering is the shape of every listing page, and two of the rules here were written after
// they went wrong: relevance sorted by indexed_at, so editing any product's description pushed it
// to position 1 of every listing, and _score rejects the missing/unmapped sub-keys OpenSearch
// accepts everywhere else

func TestDefaultSortOptionsSortRelevanceByScore(t *testing.T) {
	opts := loadSortOptions(context.Background(), nil)
	if len(opts) == 0 {
		t.Fatal("no default sort options - resolveSortOption indexes opts[0] and would panic")
	}
	first := opts[0]
	if first.Slug != "relevance" || first.Field != sortFieldScore {
		t.Errorf("first option = %+v, want relevance sorted on %s", first, sortFieldScore)
	}
	// the canonical browse url drops the /s/ segment for the first option, so indexing it would
	// put the same page under two urls
	if first.IsIndexable {
		t.Error("the default first option is indexable, which duplicates the canonical listing url")
	}
}

func TestDefaultSortOptionsUseProductAgeForNewest(t *testing.T) {
	for _, o := range loadSortOptions(context.Background(), nil) {
		if o.Slug != "newest" {
			continue
		}
		// indexed_at is when the doc was last written, so a stock tweak on a 3-year-old sku would
		// make it the newest product on the site
		if o.Field != sortFieldCreatedAt {
			t.Errorf("newest sorts on %q, want %q", o.Field, sortFieldCreatedAt)
		}
		return
	}
	t.Error("no `newest` option among the defaults")
}

func TestLoadSortOptionsFallsBackWithoutConfig(t *testing.T) {
	if got := loadSortOptions(context.Background(), nil); len(got) != len(defaultSortOptions) {
		t.Errorf("got %d options with no config service, want the %d defaults", len(got), len(defaultSortOptions))
	}
}

func TestResolveSortOptionMatchesBySlug(t *testing.T) {
	opts := loadSortOptions(context.Background(), nil)
	if got := resolveSortOption("price-desc", opts); got.Slug != "price-desc" {
		t.Errorf("resolved %q, want price-desc", got.Slug)
	}
}

func TestResolveSortOptionToleratesAHandEditedURL(t *testing.T) {
	opts := loadSortOptions(context.Background(), nil)
	cases := []struct{ name, slug string }{
		{"unknown slug", "made-up"},
		{"empty", ""},
		{"whitespace only", "   "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// a pasted url must render the default listing, never 500 or an empty page
			if got := resolveSortOption(c.slug, opts); got.Slug != opts[0].Slug {
				t.Errorf("resolved %q, want the first option %q", got.Slug, opts[0].Slug)
			}
		})
	}
	if got := resolveSortOption("  price-asc  ", opts); got.Slug != "price-asc" {
		t.Errorf("padded slug resolved to %q, want price-asc", got.Slug)
	}
}

func firstKey(t *testing.T, m map[string]any) string {
	t.Helper()
	for k := range m {
		return k
	}
	t.Fatal("empty sort clause entry")
	return ""
}

func TestBuildOSSortClauseAlwaysTieBreaksOnProductID(t *testing.T) {
	clause := buildOSSortClause(SortOption{Field: sortFieldPrice, Direction: sortDirAsc})
	if len(clause) != 2 {
		t.Fatalf("clause has %d entries, want the sort key plus a tie-breaker", len(clause))
	}
	// without it, items with equal keys shuffle between pages and deep pagination repeats or skips
	tie, ok := clause[1].(map[string]any)
	if !ok || tie["product_id"] != "asc" {
		t.Errorf("second entry = %v, want a product_id asc tie-breaker", clause[1])
	}
}

func TestBuildOSSortClauseCarriesTheMissingAndUnmappedGuards(t *testing.T) {
	clause := buildOSSortClause(SortOption{Field: "attr_colour.keyword", Direction: sortDirDesc})
	primary := clause[0].(map[string]any)
	body := primary[firstKey(t, primary)].(map[string]any)
	if body["order"] != sortDirDesc || body["missing"] != "_last" {
		t.Errorf("primary sort = %v, want desc with missing:_last", body)
	}
	// a filterable attribute nobody has reindexed for is absent from the mapping, and OpenSearch
	// 400s on an unmapped sort field without this
	if body["unmapped_type"] != "keyword" {
		t.Errorf("unmapped_type = %v, want keyword", body["unmapped_type"])
	}
}

func TestBuildOSSortClauseUsesTheBareFormForScore(t *testing.T) {
	primary := buildOSSortClause(SortOption{Field: sortFieldScore, Direction: sortDirDesc})[0].(map[string]any)
	body := primary[sortFieldScore].(map[string]any)
	// "[_score] unknown field [missing]" - _score takes order and nothing else
	if _, ok := body["missing"]; ok {
		t.Error("_score clause carries `missing`, which OpenSearch rejects")
	}
	if _, ok := body["unmapped_type"]; ok {
		t.Error("_score clause carries `unmapped_type`, which OpenSearch rejects")
	}
	if body["order"] != sortDirDesc {
		t.Errorf("order = %v, want desc", body["order"])
	}
}

func TestBuildOSSortClauseFallsBackOnAGarbageDirection(t *testing.T) {
	for _, dir := range []string{"", "sideways", "ASC"} {
		primary := buildOSSortClause(SortOption{Field: sortFieldPrice, Direction: dir})[0].(map[string]any)
		body := primary[sortFieldPrice].(map[string]any)
		// an operator-authored config row reaches this unvalidated, and OpenSearch rejects anything
		// that is not asc or desc
		if body["order"] != sortDirDesc {
			t.Errorf("direction %q produced order %v, want a desc fallback", dir, body["order"])
		}
	}
}
