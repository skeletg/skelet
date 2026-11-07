package search

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func TestASentenceReadsAsNaturalLanguageAndKeywordsDont(t *testing.T) {
	for q, want := range map[string]bool{
		"i need some oversized high quality shirt": true,
		"something warm for winter hikes":          true,
		"do you sell gift cards?":                  true,
		"linen shirt for summer":                   true,
		"red summer linen dress":                   true,
		"ich suche eine warme jacke":               true,
		"the north face":                           false,
		"under armour":                             false,
		"good american":                            false,
		"linen shirt":                              false,
		"jeans":                                    false,
		"nike air max 90":                          false,
		"bosch 0 986 452 041":                      false,
		"TRS-CHN-NAVY-W32":                         false,
		"":                                         false,
	} {
		if got := NaturalLanguage(q, "en"); got != want {
			t.Errorf("NaturalLanguage(%q) = %v, want %v", q, got, want)
		}
	}
}

type browseCall struct {
	query  string
	filter BrowseFilter
	sort   string
	limit  int
	offset int
}

// a narrowed listing comes back reversed, so the blend has to put its own order back, minus what the filters hide
type fakeEngine struct {
	words      []uuid.UUID
	wordsTotal int64
	hidden     map[uuid.UUID]bool
	price      map[uuid.UUID]int
	inCategory map[uuid.UUID]bool
	outOfStock map[uuid.UUID]bool
	colour     map[uuid.UUID]string
	pageMax    int
	calls      []browseCall
}

func (f *fakeEngine) keeps(id uuid.UUID, filter BrowseFilter) bool {
	for _, a := range filter.AttributeIn {
		if a.Slug == "colour" && !slices.Contains(a.Values, f.colour[id]) {
			return false
		}
	}
	return !f.hidden[id] && (filter.CategoryID == nil || f.inCategory[id]) && (!filter.InStockOnly || !f.outOfStock[id])
}

func (f *fakeEngine) BrowseProducts(_ context.Context, _ uuid.UUID, _, query string, filter BrowseFilter, sort string, limit, offset int, _ map[uuid.UUID]float64) (BrowseResult, error) {
	f.calls = append(f.calls, browseCall{query, filter, sort, limit, offset})
	var ids []uuid.UUID
	var facets []FacetGroup
	switch {
	case len(filter.ProductIDs) > 0:
		for _, id := range filter.ProductIDs {
			if f.keeps(id, filter) {
				ids = append(ids, id)
			}
		}
		slices.Reverse(ids)
		if sort == "price-asc" {
			slices.SortStableFunc(ids, func(a, b uuid.UUID) int { return f.price[a] - f.price[b] })
		}
		facets = []FacetGroup{{Slug: "colour", Buckets: []FacetBucket{{Slug: "navy", Count: int64(len(ids))}}}}
	case query != "":
		ids = slices.DeleteFunc(slices.Clone(f.words), func(id uuid.UUID) bool { return len(filter.AttributeIn) > 0 && !f.keeps(id, filter) })
	}
	if limit <= 0 {
		limit = len(ids)
	}
	if f.pageMax > 0 && limit > f.pageMax {
		limit = f.pageMax
	}
	from, total := min(offset, len(ids)), int64(len(ids))
	if query != "" && f.wordsTotal > 0 && len(filter.AttributeIn) == 0 {
		total = f.wordsTotal
	}
	return BrowseResult{IDs: ids[from:min(from+limit, len(ids))], Total: total, Facets: facets}, nil
}

func (f *fakeEngine) SearchProducts(context.Context, string, uuid.UUID, string, int, int, map[uuid.UUID]float64) ([]ProductHit, int64, error) {
	return nil, 0, nil
}
func (f *fakeEngine) ListSortOptions(context.Context) []SortOption { return defaultSortOptions }
func (f *fakeEngine) Name() string                                 { return "fake" }

type fakeMeaning struct {
	near    []Near
	err     error
	queries []string
}

func (m *fakeMeaning) Near(_ context.Context, _ uuid.UUID, _ []string, query string, limit int) ([]Near, error) {
	m.queries = append(m.queries, query)
	return m.near[:min(limit, len(m.near))], m.err
}

type settings map[string]string

func (s settings) Upsert(context.Context, config.Scope, string, string) (config.StoredValue, error) {
	return config.StoredValue{}, nil
}
func (s settings) Delete(context.Context, config.Scope, string) error { return nil }
func (s settings) GetCandidates(_ context.Context, _ config.Scope, path string) ([]config.StoredValue, error) {
	if v, ok := s[path]; ok {
		return []config.StoredValue{{Scope: config.DefaultScope, Path: path, Value: v}}, nil
	}
	return nil, nil
}
func (s settings) ListInScopes(context.Context, config.Scope) ([]config.StoredValue, error) {
	return nil, nil
}
func (s settings) ListAll(context.Context) ([]config.StoredValue, error) { return nil, nil }

func ids(n int) []uuid.UUID {
	out := make([]uuid.UUID, n)
	for i := range out {
		out[i] = uuid.New()
	}
	return out
}

func near(at float64, step float64, products ...uuid.UUID) []Near {
	out := make([]Near, len(products))
	for i, id := range products {
		out[i] = Near{ID: id, Distance: at + step*float64(i)}
	}
	return out
}

func TestASentenceListsWhatItMeansAndTheWordsOnlyHelpOrderIt(t *testing.T) {
	p := ids(6)
	overshirt, chambray, oxford, hoodie, far, jeans := p[0], p[1], p[2], p[3], p[4], p[5]
	engine := &fakeEngine{words: []uuid.UUID{jeans, chambray, oxford}}
	meaning := &fakeMeaning{near: append(near(0.38, 0.02, overshirt, hoodie, chambray, oxford), Near{ID: far, Distance: 0.9})}
	h := NewHybrid(engine, meaning, nil)

	res, err := h.Browse(context.Background(), BrowseRequest{Query: "i need some oversized high quality shirt", Words: "i need some oversized high quality shirt", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(res.IDs, jeans) {
		t.Error("the jeans the words matched on \"high\" came back - in a sentence, meaning decides what's in")
	}
	if slices.Contains(res.IDs, far) {
		t.Error("a product far beyond the nearest counted")
	}
	if len(res.IDs) != 4 || res.Total != 4 || res.IDs[0] != overshirt {
		t.Fatalf("got %v (total %d) - want the four close products, the overshirt meaning ranks first still first", res.IDs, res.Total)
	}
	if slices.Index(res.IDs, chambray) > slices.Index(res.IDs, hoodie) {
		t.Error("the words didn't lift the shirt they also match above the hoodie they don't")
	}
	if len(res.Facets) == 0 || res.Facets[0].Buckets[0].Count != 4 {
		t.Errorf("facets %+v - want them counted over the narrowed listing", res.Facets)
	}
}

func TestAKeywordSearchStaysOnItsWordsUntilTheyFindNothing(t *testing.T) {
	p := ids(3)
	engine := &fakeEngine{words: p[:1]}
	meaning := &fakeMeaning{near: near(0.3, 0.01, p[1], p[2])}
	h := NewHybrid(engine, meaning, nil)

	res, err := h.Browse(context.Background(), BrowseRequest{Query: "jeans", Words: "jeans"})
	if err != nil || len(res.IDs) != 1 || res.IDs[0] != p[0] || len(meaning.queries) != 0 {
		t.Fatalf("a keyword with matches: %v, %v, meaning asked %v - want the words alone", res.IDs, err, meaning.queries)
	}

	engine.words = nil
	meaning.near = append(near(0.3, 0.02, p[1], p[2]), Near{ID: p[0], Distance: 0.4})
	res, err = h.Browse(context.Background(), BrowseRequest{Query: "sneakers", Words: "sneakers"})
	if err != nil || !slices.Equal(res.IDs, p[1:]) {
		t.Fatalf("a keyword nothing uses: %v, %v - want the closest by meaning, and only the closest", res.IDs, err)
	}
}

func TestTheSettingAndTheAssistantDecideWhetherMeaningJoinsIn(t *testing.T) {
	p := ids(3)
	sentence := BrowseRequest{Query: "something warm for winter hikes", Words: "something warm for winter hikes"}
	for name, tc := range map[string]struct {
		mode string
		ctx  context.Context
		want []uuid.UUID
	}{
		"off":                   {meaningOff, context.Background(), p[:1]},
		"the assistant's words": {meaningNatural, WordsOnly(context.Background()), p[:1]},
		"every search":          {meaningAlways, context.Background(), []uuid.UUID{p[0], p[1], p[2]}},
	} {
		engine := &fakeEngine{words: p[:1]}
		h := NewHybrid(engine, &fakeMeaning{near: near(0.3, 0.01, p[1], p[2])}, config.NewService(settings{pathMeaningMode: tc.mode}))
		r := sentence
		if tc.mode == meaningAlways {
			r = BrowseRequest{Query: "parka", Words: "parka"}
		}
		res, err := h.Browse(tc.ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		got := slices.Clone(res.IDs)
		slices.SortFunc(got, func(a, b uuid.UUID) int { return slices.Index(p, a) - slices.Index(p, b) })
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", name, res.IDs, tc.want)
		}
	}
}

func TestAFailedMeaningSearchFallsBackToTheWords(t *testing.T) {
	p := ids(2)
	h := NewHybrid(&fakeEngine{words: p}, &fakeMeaning{err: errors.New("provider down")}, nil)
	res, err := h.Browse(context.Background(), BrowseRequest{Query: "something for the beach", Words: "something for the beach"})
	if err != nil || !slices.Equal(res.IDs, p) {
		t.Fatalf("%v, %v - want the words' own listing", res.IDs, err)
	}
}

func TestFiltersSortsAndPagesWorkOnTheBlend(t *testing.T) {
	p := ids(8)
	engine := &fakeEngine{hidden: map[uuid.UUID]bool{p[1]: true}, price: map[uuid.UUID]int{}, pageMax: 3}
	for i, id := range p {
		engine.price[id] = 100 - i
	}
	h := NewHybrid(engine, &fakeMeaning{near: near(0.3, 0.01, p...)}, nil)
	r := BrowseRequest{Query: "a dress for a summer wedding", Words: "a dress for a summer wedding", Limit: 3, Offset: 3}

	res, err := h.Browse(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 7 || !slices.Equal(res.IDs, []uuid.UUID{p[4], p[5], p[6]}) {
		t.Errorf("second page = %v of %d - want meaning's 4th-6th once the filtered one is out", res.IDs, res.Total)
	}

	r.Sort, r.Offset = "price-asc", 0
	res, err = h.Browse(context.Background(), r)
	if err != nil || !slices.Equal(res.IDs, []uuid.UUID{p[7], p[6], p[5]}) {
		t.Errorf("by price = %v, %v - want the cheapest first, the engine's sort", res.IDs, err)
	}
	last := engine.calls[len(engine.calls)-1]
	if last.query != "" || last.sort != "price-asc" || len(last.filter.ProductIDs) != 8 {
		t.Errorf("the sorted listing ran as %+v - want the engine narrowed to the blend, sorted, no words", last)
	}
}

func TestStrictnessMovesTheCutAndExplainShowsTheWorking(t *testing.T) {
	p := ids(5)
	engine := &fakeEngine{words: []uuid.UUID{p[4], p[0]}}
	h := NewHybrid(engine, &fakeMeaning{near: near(0.30, 0.04, p[:4]...)}, config.NewService(settings{pathMeaningStrictness: "strict"}))
	r := BrowseRequest{Query: "a jacket for the rain", Words: "a jacket for the rain"}

	res, ex, err := h.Explain(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 || ex.Reads != ReadsSentence || ex.Answered != AnsweredMeaning || math.Abs(ex.Margin-0.06) > 1e-9 {
		t.Fatalf("strict: %d listed, %+v - want the two within a fifth of the closest's own distance", res.Total, ex)
	}
	if len(ex.Near) != 4 || ex.Near[2].Counted || !ex.Near[1].Counted || !slices.Equal(ex.Words, []uuid.UUID{p[4], p[0]}) {
		t.Errorf("the working = %+v - want every near one with whether it counted, and the words' own list", ex)
	}

	broad := Trying(context.Background(), "", "broad")
	if res, _, _ = h.Explain(broad, r); res.Total != 4 {
		t.Errorf("a broad try listed %d - want the tried setting over the saved one", res.Total)
	}
	if share, on := MeaningReach(broad, config.NewService(settings{pathMeaningStrictness: "strict"}), uuid.Nil); !on || share != reachOf("broad").sentence {
		t.Errorf("the assistant's reach under a broad try = %v, %v - want the tried one too", share, on)
	}
	off := Trying(context.Background(), meaningOff, "broad")
	if res, ex, _ = h.Explain(off, r); ex.Answered != AnsweredWords || len(ex.Near) != 0 || !slices.Equal(res.IDs, []uuid.UUID{p[4], p[0]}) {
		t.Errorf("with meaning off: %v, %+v - want the words alone and meaning never asked", res.IDs, ex)
	}
	if _, on := MeaningReach(off, nil, uuid.Nil); on {
		t.Error("the assistant still reached by meaning under a try that turned it off")
	}
}

func TestARescuedSearchKeepsItsPagesAndAScopedSentenceDrawsFromItsScope(t *testing.T) {
	p := ids(6)
	h := NewHybrid(&fakeEngine{}, &fakeMeaning{near: near(0.30, 0.005, p...)}, nil)
	second, err := h.Browse(context.Background(), BrowseRequest{Query: "sneakers", Words: "sneakers", Limit: 3, Offset: 3})
	if err != nil || second.Total != 6 || !slices.Equal(second.IDs, p[3:]) || !second.Rescued || !second.Meaning {
		t.Errorf("page 2 of a rescued search = %v of %d, rescued %v, %v - want the rest of what meaning found, flagged", second.IDs, second.Total, second.Rescued, err)
	}

	boots := uuid.New()
	inCat := &fakeEngine{inCategory: map[uuid.UUID]bool{p[4]: true, p[5]: true, boots: true}, words: []uuid.UUID{boots}}
	far := append(near(0.20, 0.01, p[:4]...), Near{ID: p[4], Distance: 0.50}, Near{ID: p[5], Distance: 0.52})
	scoped := NewHybrid(inCat, &fakeMeaning{near: far}, nil)
	cat := uuid.New()
	r := BrowseRequest{Query: "something warm for winter hikes", Words: "something warm for winter hikes", Filter: BrowseFilter{CategoryID: &cat}}
	res, err := scoped.Browse(context.Background(), r)
	if err != nil || !slices.Equal(res.IDs, []uuid.UUID{p[4], p[5]}) || res.Rescued || !res.Meaning {
		t.Errorf("a sentence in a category = %v, rescued %v, %v - want its own nearest, measured from the best inside it, and no rescue flag", res.IDs, res.Rescued, err)
	}

	inCat.inCategory = map[uuid.UUID]bool{boots: true}
	if res, err = scoped.Browse(context.Background(), r); err != nil || !slices.Equal(res.IDs, []uuid.UUID{boots}) {
		t.Errorf("nothing near is in the category: %v, %v - want the words' own listing, not an empty page", res.IDs, err)
	}
}

func TestEverySearchLeavesPlentyAloneAndTheBlendKeepsTheStoresRanking(t *testing.T) {
	p := ids(4)
	plenty := &fakeEngine{words: p[:1], wordsTotal: 500}
	h := NewHybrid(plenty, &fakeMeaning{near: near(0.3, 0.01, p[1:]...)}, config.NewService(settings{pathMeaningMode: meaningAlways}))
	if res, err := h.Browse(context.Background(), BrowseRequest{Query: "shirt", Words: "shirt"}); err != nil || res.Total != 500 {
		t.Errorf("every search, words find 500 = %d, %v - want the words' listing whole, not capped at the blend", res.Total, err)
	}
	few := &fakeEngine{words: p[:1], wordsTotal: 1}
	blend := NewHybrid(few, &fakeMeaning{near: near(0.3, 0.01, p[1:]...)}, config.NewService(settings{pathMeaningMode: meaningAlways}))
	if res, err := blend.Browse(context.Background(), BrowseRequest{Query: "shirt", Words: "shirt"}); err != nil || res.Total < 2 || res.Meaning {
		t.Errorf("every search blending a keyword = %d listed, meaning-led %v, %v - want a blend, not a listing by meaning", res.Total, res.Meaning, err)
	}

	stock := &fakeEngine{outOfStock: map[uuid.UUID]bool{p[0]: true}}
	last := NewHybrid(stock, &fakeMeaning{near: near(0.3, 0.01, p...)}, config.NewService(settings{pathOutOfStockDisplay: oosLast}))
	res, err := last.Browse(context.Background(), BrowseRequest{Query: "a coat for the rain", Words: "a coat for the rain", Boosts: map[uuid.UUID]float64{p[3]: 30}})
	if err != nil || res.IDs[0] != p[3] || res.IDs[len(res.IDs)-1] != p[0] {
		t.Errorf("order %v, %v - want the boosted one first and the one out of stock last", res.IDs, err)
	}
}

func TestEverySearchBlendsWithinTheShoppersFilters(t *testing.T) {
	shirts := ids(60)
	colour := map[uuid.UUID]string{}
	var navy []uuid.UUID
	for i, id := range shirts {
		colour[id] = "red"
		if i%6 == 5 { // the navy ones sit all through the words' order, past the blend's first 48 too
			colour[id], navy = "navy", append(navy, id)
		}
	}
	engine := &fakeEngine{words: shirts, colour: colour}
	h := NewHybrid(engine, &fakeMeaning{near: near(0.3, 0.01, navy[0], shirts[0])}, config.NewService(settings{pathMeaningMode: meaningAlways}))
	res, err := h.Browse(context.Background(), BrowseRequest{Query: "shirt", Words: "shirt", Limit: 100,
		Filter: BrowseFilter{AttributeIn: []AttributeIn{{Slug: "colour", Values: []string{"navy"}}}}})
	if err != nil || res.Total != int64(len(navy)) {
		t.Errorf("every search with colour=navy = %d listed, %v - want all %d navy shirts the words found, none lost past the blend's first page", res.Total, err, len(navy))
	}
}
