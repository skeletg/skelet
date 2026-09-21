package search

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

func TestBucketKeyToString(t *testing.T) { // OpenSearch hands terms-agg keys back as string, float64 or bool depending on the field mapping
	for name, tc := range map[string]struct {
		in   any
		want string
	}{
		"string":         {"Red", "Red"},
		"integral float": {float64(91), "91"},
		"real float":     {91.5, "91.5"},
		"bool true":      {true, "true"},
		"bool false":     {false, "false"},
		"nil":            {nil, "<nil>"},
	} {
		if got := bucketKeyToString(tc.in); got != tc.want {
			t.Errorf("%s: bucketKeyToString = %q, want %q", name, got, tc.want)
		}
	}
}

func TestBucketKeyToBool(t *testing.T) { // the isIndexable sub-agg comes back as whatever JSON type the doc stored - anything unrecognized must read false, keeping the facet OUT of the sitemap rather than in
	for name, tc := range map[string]struct {
		in   any
		want bool
	}{
		"bool":          {true, true},
		"string true":   {"true", true},
		"string one":    {"1", true},
		"string false":  {"false", false},
		"string yes":    {"yes", false}, // not a recognized spelling - fail closed
		"float nonzero": {float64(1), true},
		"float zero":    {float64(0), false},
		"nil":           {nil, false},
	} {
		if got := bucketKeyToBool(tc.in); got != tc.want {
			t.Errorf("%s: bucketKeyToBool = %v, want %v", name, got, tc.want)
		}
	}
}

func swatchAttr(names ...string) catalog.Attribute {
	a := catalog.Attribute{ID: uuid.New(), Slug: "color", InputType: catalog.InputTypeSwatch}
	for _, n := range names {
		a.Values = append(a.Values, catalog.AttributeValue{
			ID: uuid.New(), Slug: slugify(n), Name: n, IsIndexable: n == "Red", // only Red is operator-blessed for SEO
		})
	}
	return a
}

func TestSelectableBucketMetaResolvesTheOperatorsSlug(t *testing.T) { // the slug in the facet URL must be the operator's, not a re-derived one, or every rename breaks the storefront's filter links
	a := swatchAttr("Red", "Navy Blue")

	slug, indexable := selectableBucketMeta(a, "Red")
	if slug != "red" || !indexable {
		t.Errorf("Red -> (%q, %v), want (red, true)", slug, indexable)
	}
	slug, indexable = selectableBucketMeta(a, "Navy Blue")
	if slug != "navy-blue" || indexable {
		t.Errorf("Navy Blue -> (%q, %v), want (navy-blue, false)", slug, indexable)
	}
	// a value renamed but not yet reindexed falls back to a derived slug and stays out of the sitemap
	slug, indexable = selectableBucketMeta(a, "Deep Ocean")
	if slug != "deep-ocean" || indexable {
		t.Errorf("an unknown display value -> (%q, %v), want the derived slug and not indexable", slug, indexable)
	}
}

func TestResolveAttributeFilterValuesMapsSlugsToDisplayNames(t *testing.T) { // the doc stores display names, so a storefront filter arriving as a slug must translate or match nothing
	a := swatchAttr("Red", "Navy Blue")

	got := resolveAttributeFilterValues(a, []string{"navy-blue", "Red", "unknown-slug"})
	if len(got) != 3 || got[0] != "Navy Blue" || got[1] != "Red" || got[2] != "unknown-slug" {
		t.Errorf("resolved = %v, want [Navy Blue Red unknown-slug] - slugs translated, display names and unknowns passed through", got)
	}

	// scalars have no slug records, values pass through untouched
	scalar := catalog.Attribute{Slug: "material", InputType: catalog.InputTypePlainText}
	got = resolveAttributeFilterValues(scalar, []string{"handmade"})
	if len(got) != 1 || got[0] != "handmade" {
		t.Errorf("scalar values = %v, want them untouched", got)
	}
}

func TestSearchSlugify(t *testing.T) { // mirrors catalog.slugifyForSKU - a drift between them would give the fallback slug a different shape than the stored one
	for in, want := range map[string]string{
		"Navy Blue":     "navy-blue",
		"  Rough  Cut ": "rough-cut",
		"205/55 R16":    "205-55-r16",
		"ALL CAPS":      "all-caps",
		"--edge--":      "edge",
		"":              "",
		"???":           "",
		"éclair":        "clair", // ascii-only on purpose, same as the SKU slugifier
	} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBrowseSortClausePinsStockFirstOnlyInLastMode(t *testing.T) { // oos=last means out-of-stock sinks to the bottom of every sort - in the other modes the base sort must come back untouched
	base := []any{map[string]any{"price_cents": "asc"}}

	if got := browseSortClause(oosShow, base); len(got) != 1 {
		t.Errorf("show mode = %v, want the base clause untouched", got)
	}
	if got := browseSortClause(oosHide, base); len(got) != 1 {
		t.Errorf("hide mode = %v, want the base clause untouched", got)
	}
	got := browseSortClause(oosLast, base)
	if len(got) != 2 {
		t.Fatalf("last mode = %v, want the stock clause prepended", got)
	}
	first, ok := got[0].(map[string]any)
	if !ok || first["is_in_stock"] == nil {
		t.Errorf("first clause = %v, want is_in_stock desc before everything", got[0])
	}
	if got[1].(map[string]any)["price_cents"] != "asc" {
		t.Errorf("second clause = %v, want the caller's sort preserved after it", got[1])
	}
}

func TestExtractAttributeValue(t *testing.T) { // this feeds the searchable text - a dropped value makes the product unfindable by that word
	sizeID := uuid.New()
	p := catalog.Product{
		Attributes: catalog.ProductScalarAttributes{
			"material": "leather",
			"weight":   float64(2.5),
			"count":    float64(4),
			"fragile":  true,
			"empty":    nil,
		},
		AttributeSelections: []catalog.ProductAttributeSelection{
			{AttributeID: sizeID, Value: catalog.AttributeValue{Name: "Small"}},
			{AttributeID: sizeID, Value: catalog.AttributeValue{Name: "Large"}},
			{AttributeID: uuid.New(), Value: catalog.AttributeValue{Name: "Other Attr Value"}},
		},
	}

	scalar := func(slug string) catalog.Attribute {
		return catalog.Attribute{Slug: slug, InputType: catalog.InputTypePlainText}
	}
	for name, tc := range map[string]struct {
		attr catalog.Attribute
		want string
	}{
		"string scalar":  {scalar("material"), "leather"},
		"float scalar":   {scalar("weight"), "2.5"},
		"integral float": {scalar("count"), "4"},
		"bool scalar":    {scalar("fragile"), "true"},
		"missing":        {scalar("nope"), ""},
		"nil value":      {scalar("empty"), ""},
		"selectable":     {catalog.Attribute{ID: sizeID, Slug: "size", InputType: catalog.InputTypeMultiselect}, "Small Large"},
	} {
		if got := extractAttributeValue(p, tc.attr); got != tc.want {
			t.Errorf("%s: extractAttributeValue = %q, want %q", name, got, tc.want)
		}
	}
}

func TestAttributeFieldNamesStayDistinct(t *testing.T) { // three parallel fields per attribute - display, slug, indexable flag - and a shared prefix would make the dynamic template swallow the wrong one
	slug := "color"
	names := map[string]string{
		"display": AttributeFieldName(slug),
		"slug":    AttributeSlugFieldName(slug),
		"index":   AttributeIndexableFieldName(slug),
	}
	seen := map[string]string{}
	for role, n := range names {
		if prev, dup := seen[n]; dup {
			t.Errorf("%s and %s share the field name %q", role, prev, n)
		}
		seen[n] = role
	}
	if names["display"] != "attr_color" {
		t.Errorf("display field = %q, want attr_color - the dynamic template matches on this prefix", names["display"])
	}
}

func TestResolveSortOptionFallsBackToTheFirst(t *testing.T) { // the first option is the storefront's default ordering, so an unknown or stale slug in a bookmarked URL must land there
	opts := []SortOption{
		{Slug: "relevance", Field: sortFieldScore},
		{Slug: "price-asc", Field: sortFieldPrice, Direction: sortDirAsc},
	}
	if got := resolveSortOption("price-asc", opts); got.Field != sortFieldPrice {
		t.Errorf("price-asc resolved to %+v", got)
	}
	if got := resolveSortOption(" price-asc ", opts); got.Field != sortFieldPrice {
		t.Errorf("a padded slug resolved to %+v, want it trimmed first", got)
	}
	for _, unknown := range []string{"", "bogus", "PRICE-ASC"} {
		if got := resolveSortOption(unknown, opts); got.Slug != "relevance" {
			t.Errorf("%q resolved to %+v, want the first option", unknown, got)
		}
	}
}

func TestBuildOSSortClause(t *testing.T) {
	got := buildOSSortClause(SortOption{Field: sortFieldPrice, Direction: sortDirAsc})
	if len(got) != 2 {
		t.Fatalf("clause = %v, want primary plus the product_id tie-breaker", got)
	}
	primary := got[0].(map[string]any)[sortFieldPrice].(map[string]any)
	if primary["order"] != "asc" || primary["unmapped_type"] != "keyword" {
		t.Errorf("primary = %v, want asc with unmapped_type keyword so an unindexed attr sort doesn't 400", primary)
	}
	if got[1].(map[string]any)["product_id"] != "asc" {
		t.Errorf("tie-breaker = %v, want product_id asc so deep pagination doesn't shuffle", got[1])
	}

	// a bad direction falls to desc rather than reaching OpenSearch as a 400
	got = buildOSSortClause(SortOption{Field: sortFieldPrice, Direction: "sideways"})
	if got[0].(map[string]any)[sortFieldPrice].(map[string]any)["order"] != "desc" {
		t.Errorf("clause = %v, want the direction defaulted to desc", got[0])
	}

	// _score rejects the missing/unmapped sub-keys outright - the bare short form is load-bearing
	got = buildOSSortClause(SortOption{Field: sortFieldScore, Direction: sortDirDesc})
	score := got[0].(map[string]any)[sortFieldScore].(map[string]any)
	if _, has := score["unmapped_type"]; has {
		t.Errorf("_score clause = %v, OpenSearch 400s on unmapped_type here", score)
	}
	if score["order"] != "desc" {
		t.Errorf("_score order = %v, want desc", score["order"])
	}
}
