package front

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	model1 "github.com/skeletg/skelet/apps/backend/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/internal/search"
)

// anyTrue is the bundle/grouped gate - one purchasable member is enough to let the wrapper be bought
func TestAnyTrue(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	for name, tc := range map[string]struct {
		in   map[uuid.UUID]bool
		want bool
	}{
		"one sellable":      {map[uuid.UUID]bool{a: true, b: false}, true},
		"all sellable":      {map[uuid.UUID]bool{a: true, b: true}, true},
		"none sellable":     {map[uuid.UUID]bool{a: false, b: false}, false},
		"empty":             {map[uuid.UUID]bool{}, false},
		"nil":               {nil, false},
		"single sellable":   {map[uuid.UUID]bool{a: true}, true},
		"single unsellable": {map[uuid.UUID]bool{a: false}, false},
	} {
		if got := anyTrue(tc.in); got != tc.want {
			t.Errorf("%s: anyTrue = %v, want %v", name, got, tc.want)
		}
	}
}

// allTrue is the kit gate - one out-of-stock component blocks the whole kit, and a component missing
// from the map entirely must read as unsellable rather than defaulting to purchasable
func TestAllTrue(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()

	if !allTrue(map[uuid.UUID]bool{a: true, b: true}, []uuid.UUID{a, b}) {
		t.Error("every component sellable = false, want true")
	}
	if allTrue(map[uuid.UUID]bool{a: true, b: false}, []uuid.UUID{a, b}) {
		t.Error("one unsellable component did not block the kit")
	}
	// c never made it into the salable map, e.g. its lookup failed
	if allTrue(map[uuid.UUID]bool{a: true, b: true}, []uuid.UUID{a, b, c}) {
		t.Error("a component absent from the map read as sellable, want the kit blocked")
	}
	if allTrue(nil, []uuid.UUID{a}) {
		t.Error("a nil map read as sellable")
	}
	// vacuously true - a kit with no components has nothing to block it, and the caller gates on that
	if !allTrue(map[uuid.UUID]bool{}, nil) {
		t.Error("allTrue over no ids = false, want the vacuous true")
	}
}

// The promotion engine matches on these three, so a dropped category or attribute silently stops a
// rule from applying to a product it should discount
func TestProductCandidateFromModel(t *testing.T) {
	catA, catB := uuid.New(), uuid.New()
	p := &model.Product{
		Sku:  "TIRE-205",
		Kind: model.ProductKindSimple,
		Categories: []*model.Category{
			{ID: catA.String()},
			nil,                // a nil entry must not panic
			{ID: "not-a-uuid"}, // an unparseable id is skipped, not fatal
			{ID: catB.String()},
		},
		AttributeValues: []*model.ProductAttributeValue{
			{Attribute: &model.Attribute{Slug: "season"}, SelectedValues: []*model.AttributeValue{{Slug: "winter"}, {Slug: "all-season"}}},
			{Attribute: &model.Attribute{Slug: "width"}, ScalarValue: 205},
			nil,
			{Attribute: nil, ScalarValue: "orphan"}, // no attribute to key on
			{Attribute: &model.Attribute{Slug: ""}, ScalarValue: 1}, // blank slug is unkeyable
		},
	}

	got, err := productCandidateFromModel(p)
	if err != nil {
		t.Fatalf("productCandidateFromModel: %v", err)
	}
	if got.ProductSKU != "TIRE-205" || got.Kind != "simple" {
		t.Errorf("candidate = {%q %q}, want {TIRE-205 simple} - the engine compares lowercase kinds case-sensitively", got.ProductSKU, got.Kind)
	}
	if len(got.CategoryIDs) != 2 {
		t.Errorf("categories = %v, want only the two parseable ids", got.CategoryIDs)
	}
	// multi-select values join comma-separated, which is what the condition evaluator splits on
	if got.AttributeKVs["season"] != "winter,all-season" {
		t.Errorf("season = %q, want %q", got.AttributeKVs["season"], "winter,all-season")
	}
	if got.AttributeKVs["width"] != "205" {
		t.Errorf("width = %q, want the scalar stringified", got.AttributeKVs["width"])
	}
	if len(got.AttributeKVs) != 2 {
		t.Errorf("attributes = %v, want only the two keyable ones", got.AttributeKVs)
	}
}

func TestProductCandidateFromModelHandlesAnEmptyProduct(t *testing.T) {
	got, err := productCandidateFromModel(&model.Product{Sku: "X", Kind: model.ProductKindSimple})
	if err != nil {
		t.Fatalf("productCandidateFromModel: %v", err)
	}
	if got.CategoryIDs == nil || len(got.CategoryIDs) != 0 {
		t.Errorf("categories = %v, want an empty slice", got.CategoryIDs)
	}
	if got.AttributeKVs == nil || len(got.AttributeKVs) != 0 {
		t.Errorf("attributes = %v, want an empty map", got.AttributeKVs)
	}
}

// The ids feed one batched category load, so a duplicate would fetch twice and a missed primary
// would leave a product without its breadcrumb
func TestCollectCategoryIDs(t *testing.T) {
	p1, p2 := uuid.New(), uuid.New()
	shared, only1, primaryOnly := uuid.New(), uuid.New(), uuid.New()

	got := collectCategoryIDs(
		map[uuid.UUID][]catalog.Category{
			p1: {{ID: shared}, {ID: only1}, {ID: shared}}, // duplicate within one product
			p2: {{ID: shared}},                            // duplicate across products
		},
		map[uuid.UUID]*catalog.Category{
			p1: {ID: shared},      // already collected
			p2: {ID: primaryOnly}, // only reachable through the primary map
		},
	)

	seen := map[uuid.UUID]int{}
	for _, id := range got {
		seen[id]++
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("id %s appears %d times, want exactly once", id, n)
		}
	}
	for _, want := range []uuid.UUID{shared, only1, primaryOnly} {
		if seen[want] != 1 {
			t.Errorf("id %s is missing from %v", want, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("collected %d ids, want 3", len(got))
	}
}

// A nil primary is the common case for a product with no primary category set
func TestCollectCategoryIDsSkipsNilPrimaries(t *testing.T) {
	p := uuid.New()
	got := collectCategoryIDs(nil, map[uuid.UUID]*catalog.Category{p: nil})
	if got == nil || len(got) != 0 {
		t.Errorf("collectCategoryIDs = %v, want an empty non-nil slice", got)
	}
	if got := collectCategoryIDs(nil, nil); got == nil || len(got) != 0 {
		t.Errorf("collectCategoryIDs(nil, nil) = %v, want an empty non-nil slice", got)
	}
}

// Search pins are the merchandiser's override, so pinned hits lead in their configured order and
// everything else keeps the relevance order the engine returned
func TestSortHitsByPin(t *testing.T) {
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	hits := []search.ProductHit{{ProductID: a}, {ProductID: b}, {ProductID: c}, {ProductID: d}}
	rank := map[string]int{d.String(): 0, b.String(): 1}

	sortHitsByPin(hits, rank)

	if hits[0].ProductID != d || hits[1].ProductID != b {
		t.Errorf("head = %v, %v, want the pins in rank order (d, b)", hits[0].ProductID, hits[1].ProductID)
	}
	// the unpinned tail keeps its original relative order
	if hits[2].ProductID != a || hits[3].ProductID != c {
		t.Errorf("tail = %v, %v, want the relevance order (a, c) preserved", hits[2].ProductID, hits[3].ProductID)
	}
}

func TestSortHitsByPinWithNoPinsIsANoOp(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	for name, rank := range map[string]map[string]int{"nil": nil, "empty": {}} {
		hits := []search.ProductHit{{ProductID: a}, {ProductID: b}, {ProductID: c}}
		sortHitsByPin(hits, rank)
		if hits[0].ProductID != a || hits[1].ProductID != b || hits[2].ProductID != c {
			t.Errorf("%s rank reordered the hits to %v", name, hits)
		}
	}
}

// A pin pointing at a product this query didn't return must not disturb the ordering
func TestSortHitsByPinIgnoresPinsNotInTheResults(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	hits := []search.ProductHit{{ProductID: a}, {ProductID: b}}
	sortHitsByPin(hits, map[string]int{uuid.NewString(): 0})
	if hits[0].ProductID != a || hits[1].ProductID != b {
		t.Errorf("hits = %v, want the order untouched", hits)
	}
}

func TestSortHitsByPinHandlesEmptyInput(t *testing.T) {
	sortHitsByPin(nil, map[string]int{uuid.NewString(): 0})      // must not panic
	sortHitsByPin([]search.ProductHit{}, map[string]int{"x": 0}) //
}

// The facet H1 knob comes out of config as a free string, and anything unrecognized has to land on
// BEFORE the way the dashboard's own fallback does
func TestFacetH1PositionToGQL(t *testing.T) {
	if got := facetH1PositionToGQL(string(catalog.H1PositionAfter)); got != model1.BrowseFacetH1PositionAfter {
		t.Errorf("after = %q, want AFTER", got)
	}
	for _, in := range []string{"", "before", "BEFORE", "sideways", "After"} {
		if got := facetH1PositionToGQL(in); got != model1.BrowseFacetH1PositionBefore {
			t.Errorf("facetH1PositionToGQL(%q) = %q, want BEFORE", in, got)
		}
	}
}

// GraphQL Int is 32-bit while the domain carries int64, so nil has to survive as GraphQL null
func TestInt32PtrFrom64(t *testing.T) {
	if got := int32PtrFrom64(nil); got != nil {
		t.Errorf("int32PtrFrom64(nil) = %v, want nil", got)
	}
	for _, in := range []int64{0, 1, -1, 2147483647} {
		v := in
		got := int32PtrFrom64(&v)
		if got == nil || int64(*got) != in {
			t.Errorf("int32PtrFrom64(%d) = %v, want %d", in, got, in)
		}
	}
}

// The JSON scalar has to emit plain maps with the catalog struct tags' snake_case keys, or the
// storefront's visible_if evaluator reads fields that aren't there
func TestVisibleIfToAny(t *testing.T) {
	if got := visibleIfToAny(nil); got != nil {
		t.Errorf("visibleIfToAny(nil) = %v, want nil for a GraphQL null", got)
	}

	g := &catalog.OptionConditionGroup{
		Logic: "or",
		Rules: []catalog.OptionRule{{OptionID: uuid.New(), Op: "anyOf", ValueIDs: []uuid.UUID{uuid.New()}}},
		Groups: []catalog.OptionConditionGroup{
			{Rules: []catalog.OptionRule{{OptionID: uuid.New(), Op: "filled"}}},
		},
	}
	got := visibleIfToAny(g)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("visibleIfToAny = %#v, want a plain map", got)
	}
	// re-marshal and compare against the struct's own JSON decoded the same way, so key order doesn't matter
	var want any
	direct, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal struct: %v", err)
	}
	if err := json.Unmarshal(direct, &want); err != nil {
		t.Fatalf("unmarshal struct json: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("visibleIfToAny produced %#v, want the struct's own shape %#v", got, want)
	}
	if _, hasLogic := m["logic"]; !hasLogic {
		t.Errorf("map = %v, want the struct tags rather than Go field names", m)
	}
	rules, ok := m["rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("rules = %#v, want one rule", m["rules"])
	}
	// the storefront evaluator reads option_id, not OptionID
	if _, ok := rules[0].(map[string]any)["option_id"]; !ok {
		t.Errorf("rule = %v, want the snake_case option_id key the evaluator reads", rules[0])
	}
	if _, ok := m["groups"].([]any); !ok {
		t.Errorf("groups = %#v, want the nested tree carried through", m["groups"])
	}
}

// Bundle input types are lowercase in the domain and SCREAMING in the schema, so an un-uppercased one
// would fail gqlgen enum validation at response time
func TestToBundleOptionModelsUppercasesTheInputType(t *testing.T) {
	opt := catalog.BundleOption{
		ID:        uuid.New(),
		InputType: catalog.BundleInputRadio,
		Title:     "Wheel",
		Selections: []catalog.BundleSelection{
			{ID: uuid.New(), ProductID: uuid.New(), ProductSKU: "W-1", ProductName: "Alloy", ProductDefaultPrice: 5000, ProductCurrency: "USD", Quantity: 1},
		},
		AllContent: []catalog.BundleOptionContent{{Locale: "de", Title: "Rad"}},
	}

	got := toBundleOptionModels([]catalog.BundleOption{opt})
	if len(got) != 1 {
		t.Fatalf("%d options, want 1", len(got))
	}
	if string(got[0].InputType) != "RADIO" {
		t.Errorf("InputType = %q, want RADIO", got[0].InputType)
	}
	if len(got[0].Content) != 1 || got[0].Content[0].Locale != "de" {
		t.Errorf("content = %+v, want the localized title carried through", got[0].Content)
	}
	if len(got[0].Selections) != 1 {
		t.Fatalf("%d selections, want 1", len(got[0].Selections))
	}
	sel := got[0].Selections[0]
	if sel.Product == nil {
		t.Fatal("the selection carries no product")
	}
	if sel.Product.Name == nil || *sel.Product.Name != "Alloy" {
		t.Errorf("product name = %v, want Alloy", sel.Product.Name)
	}
	// price and defaultPrice both come off the snapshot, so the PDP running total matches
	if sel.Product.Price != 5000 || sel.Product.DefaultPrice != 5000 {
		t.Errorf("prices = %d / %d, want both 5000", sel.Product.Price, sel.Product.DefaultPrice)
	}
}

// A selection whose product has no name must leave the field nil rather than sending an empty string
func TestToBundleOptionModelsLeavesAnAbsentNameNil(t *testing.T) {
	got := toBundleOptionModels([]catalog.BundleOption{{
		ID:         uuid.New(),
		InputType:  catalog.BundleInputCheckbox,
		Selections: []catalog.BundleSelection{{ID: uuid.New(), ProductID: uuid.New()}},
	}})
	if got[0].Selections[0].Product.Name != nil {
		t.Errorf("name = %v, want nil for a product with no name", got[0].Selections[0].Product.Name)
	}
}

// The lists are non-null in the schema, so an option with nothing in it must serialize as [] not null
func TestToBundleOptionModelsNeverReturnsNilLists(t *testing.T) {
	got := toBundleOptionModels([]catalog.BundleOption{{ID: uuid.New(), InputType: catalog.BundleInputRadio}})
	if got[0].Content == nil {
		t.Error("Content = nil, want an empty slice")
	}
	if got[0].Selections == nil {
		t.Error("Selections = nil, want an empty slice")
	}
	if all := toBundleOptionModels(nil); all == nil || len(all) != 0 {
		t.Errorf("toBundleOptionModels(nil) = %v, want an empty non-nil slice", all)
	}
}

// The badge on a PLP card comes from MatchesProduct, and core.product_kind params are validated to be
// lowercase - an un-lowered GraphQL enum compared case-sensitively matched nothing at all
func TestProductCandidateKindMatchesAKindCondition(t *testing.T) {
	for _, tc := range []struct {
		kind      model.ProductKind
		ruleKinds string
		want      bool
	}{
		{model.ProductKindBundle, `["bundle"]`, true},
		{model.ProductKindSimple, `["simple"]`, true},
		{model.ProductKindConfigurable, `["configurable","bundle"]`, true},
		{model.ProductKindSimple, `["bundle"]`, false},
		{model.ProductKindGrouped, `["simple"]`, false},
	} {
		rule := promotion.Rule{Conditions: []promotion.Condition{{
			Type:   "core.product_kind",
			Params: json.RawMessage(`{"kinds":` + tc.ruleKinds + `}`),
		}}}
		cand, err := productCandidateFromModel(&model.Product{Sku: "X", Kind: tc.kind})
		if err != nil {
			t.Fatalf("productCandidateFromModel: %v", err)
		}
		if got := promotion.MatchesProduct(rule, cand); got != tc.want {
			t.Errorf("a %s product against kinds %s = %v, want %v (candidate kind %q)", tc.kind, tc.ruleKinds, got, tc.want, cand.Kind)
		}
	}
}

// Category conditions compare parsed uuids, so the ids the candidate carries have to reach the matcher
func TestProductCandidateCategoriesMatchACategoryCondition(t *testing.T) {
	inRule, notInRule := uuid.New(), uuid.New()
	rule := promotion.Rule{Conditions: []promotion.Condition{{
		Type:   "core.product_in_category",
		Params: json.RawMessage(`{"category_ids":["` + inRule.String() + `"]}`),
	}}}

	hit, err := productCandidateFromModel(&model.Product{Sku: "A", Kind: model.ProductKindSimple,
		Categories: []*model.Category{{ID: inRule.String()}}})
	if err != nil {
		t.Fatalf("productCandidateFromModel: %v", err)
	}
	if !promotion.MatchesProduct(rule, hit) {
		t.Error("a product in the rule's category did not match")
	}

	miss, _ := productCandidateFromModel(&model.Product{Sku: "B", Kind: model.ProductKindSimple,
		Categories: []*model.Category{{ID: notInRule.String()}}})
	if promotion.MatchesProduct(rule, miss) {
		t.Error("a product outside the rule's category matched")
	}
}

// Multi-select attribute values join with commas, and the matcher splits on that - a mismatch here
// means an attribute-targeted promo silently stops advertising
func TestProductCandidateAttributesMatchAnAttributeCondition(t *testing.T) {
	rule := promotion.Rule{Conditions: []promotion.Condition{{
		Type:   "core.product_has_attribute",
		Params: json.RawMessage(`{"attribute":"season","values":["winter"]}`),
	}}}

	multi, err := productCandidateFromModel(&model.Product{Sku: "A", Kind: model.ProductKindSimple,
		AttributeValues: []*model.ProductAttributeValue{{
			Attribute:      &model.Attribute{Slug: "season"},
			SelectedValues: []*model.AttributeValue{{Slug: "all-season"}, {Slug: "winter"}},
		}}})
	if err != nil {
		t.Fatalf("productCandidateFromModel: %v", err)
	}
	if !promotion.MatchesProduct(rule, multi) {
		t.Errorf("a product carrying winter among %q did not match", multi.AttributeKVs["season"])
	}

	other, _ := productCandidateFromModel(&model.Product{Sku: "B", Kind: model.ProductKindSimple,
		AttributeValues: []*model.ProductAttributeValue{{
			Attribute:      &model.Attribute{Slug: "season"},
			SelectedValues: []*model.AttributeValue{{Slug: "summer"}},
		}}})
	if promotion.MatchesProduct(rule, other) {
		t.Error("a summer-only product matched a winter rule")
	}
}

// Every kind the schema can hand over has to survive as the lowercase form the condition validator accepts
func TestProductCandidateLowersEveryProductKind(t *testing.T) {
	accepted := map[string]bool{"simple": true, "configurable": true, "bundle": true, "grouped": true}
	for _, kind := range model.AllProductKind {
		got, err := productCandidateFromModel(&model.Product{Sku: "X", Kind: kind})
		if err != nil {
			t.Fatalf("productCandidateFromModel(%q): %v", kind, err)
		}
		want := strings.ToLower(string(kind))
		if got.Kind != want {
			t.Errorf("kind %q became %q, want %q", kind, got.Kind, want)
		}
		if accepted[want] && !accepted[got.Kind] {
			t.Errorf("kind %q became %q, which core.product_kind would never match", kind, got.Kind)
		}
	}
}

// isPublic is enforced at the model edge, with one carve-out: a configurable axis stays visible because the variant picker reads it off every child
func TestKeepPublicAttributeValues(t *testing.T) {
	pub := &model.ProductAttributeValue{Attribute: &model.Attribute{ID: uuid.New().String(), Slug: "color", IsPublic: true}}
	priv := &model.ProductAttributeValue{Attribute: &model.Attribute{ID: uuid.New().String(), Slug: "margin-tier"}}
	axisID := uuid.New().String()
	axis := &model.ProductAttributeValue{Attribute: &model.Attribute{ID: axisID, Slug: "size"}} // private but an axis

	got := keepPublicAttributeValues([]*model.ProductAttributeValue{pub, priv, axis}, map[string]bool{axisID: true})
	if len(got) != 2 || got[0] != pub || got[1] != axis {
		t.Fatalf("kept %d values, want the public one plus the exempted axis", len(got))
	}
	if got := keepPublicAttributeValues([]*model.ProductAttributeValue{priv}, nil); len(got) != 0 {
		t.Fatalf("a private value survived with no exemption set")
	}
}

// The front schema doesn't expose productType attributes today, but the mapper fills them on the shared model struct - gate them so a future schema addition can't leak
func TestProductTypeModelDropsPrivateAttributes(t *testing.T) {
	pub := catalog.Attribute{ID: uuid.New(), Slug: "color", IsPublic: true}
	priv := catalog.Attribute{ID: uuid.New(), Slug: "cost-code"}
	pt := catalog.ProductType{
		ID:         uuid.New(),
		Attributes: []catalog.ProductTypeAttribute{{Attribute: pub}, {Attribute: priv}},
		Groups:     []catalog.AttributeGroup{{ID: uuid.New(), Attributes: []catalog.ProductTypeAttribute{{Attribute: priv}, {Attribute: pub}}}},
	}
	m := toProductTypeModel(pt)
	if len(m.Attributes) != 1 || m.Attributes[0].Attribute.Slug != "color" {
		t.Fatalf("attributes = %d, want only the public one", len(m.Attributes))
	}
	if len(m.Groups[0].Attributes) != 1 || m.Groups[0].Attributes[0].Attribute.Slug != "color" {
		t.Fatalf("the group projection leaked a private attribute")
	}
}

// Only an explicit setProductMediaRole call assigns MAIN, so products whose images arrived through import, a plugin or
// the API have media and no role - with no fallback that is a blank card, a blank variant row and a blank part stub
func TestToProductBaseModelFallsBackToTheFirstImage(t *testing.T) {
	first := &model.ProductMedia{ID: uuid.NewString(), URL: "first.jpg"}
	second := &model.ProductMedia{ID: uuid.NewString(), URL: "second.jpg"}
	mediaList := []*model.ProductMedia{first, second}

	out := toProductBaseModel(catalog.Product{}, nil, nil, nil, mediaList, nil, nil, nil, nil)
	if out.MainImage != first {
		t.Errorf("MainImage = %v, want the first image as the fallback", out.MainImage)
	}
	// the other two roles deliberately DON'T fall back - a hero standing in for a thumbnail is the
	// wrong size, and a missing og image should let the page's own metadata fallback decide
	if out.Thumbnail != nil || out.OgImage != nil {
		t.Errorf("thumbnail/og fell back: %v / %v", out.Thumbnail, out.OgImage)
	}

	// an explicit role still wins over document order
	roleID, err := uuid.Parse(second.ID)
	if err != nil {
		t.Fatalf("parse id: %v", err)
	}
	withRole := catalog.Product{MediaRoles: map[catalog.MediaRole]uuid.UUID{catalog.MediaRoleMain: roleID}}
	if got := toProductBaseModel(withRole, nil, nil, nil, mediaList, nil, nil, nil, nil); got.MainImage != second {
		t.Errorf("MainImage = %v, want the role-assigned image", got.MainImage)
	}

	// and no media at all stays nil rather than becoming a zero-value struct
	if got := toProductBaseModel(catalog.Product{}, nil, nil, nil, nil, nil, nil, nil, nil); got.MainImage != nil {
		t.Errorf("MainImage = %v, want nil with no media", got.MainImage)
	}
}
