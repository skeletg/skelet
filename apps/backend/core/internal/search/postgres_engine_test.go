package search

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

type fakeCatalog struct {
	attrs []catalog.Attribute
	// records which target attribute got which "other" selection, so the
	// multi-select exclusion can be asserted
	seen map[uuid.UUID]map[uuid.UUID][]uuid.UUID
	// what BrowseFallback was asked to filter on
	gotValues []uuid.UUID
	gotCount  int32
	gotSort   string
	gotLocale string
	// the locale each facet count was asked for - it must match the browse call's
	// or the counts describe a different result set than the grid
	facetLocales []string
}

func (f *fakeCatalog) ListAttributes(context.Context) ([]catalog.Attribute, error) {
	return f.attrs, nil
}
func (f *fakeCatalog) BrowseFallback(_ context.Context, flt catalog.BrowseFallbackFilter) ([]uuid.UUID, int64, error) {
	f.gotValues, f.gotCount = flt.AttrValueIDs, flt.AttrCount
	f.gotSort, f.gotLocale = flt.Sort, flt.Locale
	return []uuid.UUID{}, 0, nil
}
func (f *fakeCatalog) SearchFallback(context.Context, uuid.UUID, string, string, int32, int32) ([]catalog.SearchHit, int64, error) {
	return nil, 0, nil
}
func (f *fakeCatalog) FacetCounts(_ context.Context, target uuid.UUID, flt catalog.FacetCountFilter) ([]catalog.FacetBucketCount, error) {
	if f.seen == nil {
		f.seen = map[uuid.UUID]map[uuid.UUID][]uuid.UUID{}
	}
	f.seen[target] = flt.OtherSelected
	f.facetLocales = append(f.facetLocales, flt.Locale)
	return []catalog.FacetBucketCount{{ValueID: uuid.New(), Slug: "s", Name: "S", Count: 1}}, nil
}

func selectable(name, slug string, valueSlugs ...string) catalog.Attribute {
	a := catalog.Attribute{
		ID: uuid.New(), Slug: slug, Name: name,
		Filterable: true, IsPublic: true, InputType: catalog.InputTypeDropdown,
	}
	for _, vs := range valueSlugs {
		a.Values = append(a.Values, catalog.AttributeValue{ID: uuid.New(), Slug: vs, Name: vs})
	}
	return a
}

func TestPostgresEngineFacets(t *testing.T) {
	colour := selectable("Colour", "colour", "red", "blue")
	size := selectable("Size", "size", "s", "m")
	// Not selectable - scalar attributes have no value rows to count, so they must
	// not be offered as facets on this tier
	scalar := catalog.Attribute{ID: uuid.New(), Slug: "material", Name: "Material", Filterable: true, IsPublic: true, InputType: catalog.InputTypePlainText}
	// Filterable but private - must be neither offered as a facet nor accepted as a filter
	internal := selectable("Margin tier", "margin-tier", "a", "b")
	internal.IsPublic = false

	fc := &fakeCatalog{attrs: []catalog.Attribute{colour, size, scalar, internal}}
	e := NewPostgresEngine(fc, nil)

	res, err := e.BrowseProducts(context.Background(), uuid.New(), "en", "",
		BrowseFilter{AttributeIn: []AttributeIn{
			{Slug: "colour", Values: []string{"red"}},
			{Slug: "nonexistent", Values: []string{"x"}},
			{Slug: "margin-tier", Values: []string{"a"}},
		}}, "", 24, 0, nil)
	if err != nil {
		t.Fatalf("browse: %v", err)
	}

	if len(res.Facets) != 2 {
		t.Fatalf("facet groups = %d, want 2 (scalar and private attributes must not be offered)", len(res.Facets))
	}

	// The colour facet must be counted WITHOUT colour's own selection, or ticking
	// "red" would collapse the colour panel to just red
	if sel := fc.seen[colour.ID]; len(sel) != 0 {
		t.Fatalf("colour was counted with its own selection applied: %+v", sel)
	}
	// The size facet must still see colour's selection
	if sel := fc.seen[size.ID]; len(sel) != 1 {
		t.Fatalf("size facet lost the colour constraint: %+v", sel)
	}

	// Unknown and private slugs are dropped, so only colour constrains the result set
	if fc.gotCount != 1 || len(fc.gotValues) != 1 {
		t.Fatalf("browse filter = %d values across %d attrs, want 1/1 (unknown and private slugs dropped)", len(fc.gotValues), fc.gotCount)
	}
}

// The engine's OWN sort options are the ones the storefront is offered when this tier serves, so their slugs are what arrives here -
// passing them through untranslated matched no branch in the fallback SQL and silently served created_at DESC for every sort
func TestPostgresEngineSortSlugsReachTheQuery(t *testing.T) {
	fc := &fakeCatalog{}
	e := NewPostgresEngine(fc, nil)

	offered := e.ListSortOptions(context.Background())
	want := map[string]string{"newest": "", "price-asc": "price_asc", "price-desc": "price_desc"}
	if len(offered) != len(want) {
		t.Fatalf("offered %d sort options, want %d", len(offered), len(want))
	}
	for _, opt := range offered {
		expect, known := want[opt.Slug]
		if !known {
			t.Fatalf("engine offers slug %q with no expected mapping", opt.Slug)
		}
		if _, err := e.BrowseProducts(context.Background(), uuid.New(), "en", "", BrowseFilter{}, opt.Slug, 24, 0, nil); err != nil {
			t.Fatalf("browse %s: %v", opt.Slug, err)
		}
		if fc.gotSort != expect {
			t.Fatalf("sort %q reached the query as %q, want %q", opt.Slug, fc.gotSort, expect)
		}
	}

	// An OpenSearch-configured list can carry any slug, and a mid-request failover hands it straight to this tier -
	// anything unrecognized falls to the SQL's own created_at DESC tail rather than erroring
	for _, slug := range []string{"", "relevance", "price_asc", "operator-invented"} {
		if _, err := e.BrowseProducts(context.Background(), uuid.New(), "en", "", BrowseFilter{}, slug, 24, 0, nil); err != nil {
			t.Fatalf("browse %q: %v", slug, err)
		}
		if slug == "price_asc" && fc.gotSort != "price_asc" {
			t.Fatalf("underscore spelling of the SQL's own vocabulary was dropped: %q", fc.gotSort)
		}
	}
}

// The grid and the facet counts must be scoped to the same locale, or the counts
// describe a set the shopper isn't looking at
func TestPostgresEngineThreadsLocaleToGridAndFacets(t *testing.T) {
	colour := selectable("Colour", "colour", "red", "blue")
	fc := &fakeCatalog{attrs: []catalog.Attribute{colour}}
	e := NewPostgresEngine(fc, nil)

	if _, err := e.BrowseProducts(context.Background(), uuid.New(), "de", "hemd", BrowseFilter{}, "", 24, 0, nil); err != nil {
		t.Fatalf("browse: %v", err)
	}
	if fc.gotLocale != "de" {
		t.Fatalf("browse locale = %q, want de - without it a name query can only match SKU", fc.gotLocale)
	}
	if len(fc.facetLocales) == 0 {
		t.Fatal("no facet was counted")
	}
	for _, l := range fc.facetLocales {
		if l != "de" {
			t.Fatalf("facet counted at locale %q while the grid used de", l)
		}
	}
}
