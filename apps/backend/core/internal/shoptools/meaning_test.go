package shoptools

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/embedding"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
)

type fakeMeaning struct {
	hits     []embedding.Hit
	passages []embedding.Passage
	err      error
	asked    int
	locales  []string
	texts    map[uuid.UUID]string
	similar  embedding.Similar
	seeds    []uuid.UUID
}

func (f *fakeMeaning) Similar(_ context.Context, _ uuid.UUID, _ []string, products []uuid.UUID, _ int) (embedding.Similar, error) {
	f.seeds = products
	return f.similar, f.err
}

func (f *fakeMeaning) Products(_ context.Context, _ uuid.UUID, locales []string, _ string, _ int) ([]embedding.Hit, error) {
	f.asked++
	f.locales = locales
	return f.hits, f.err
}

func (f *fakeMeaning) ProductTexts(_ context.Context, ids []uuid.UUID, _ []string) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	for _, id := range ids {
		if t, ok := f.texts[id]; ok {
			out[id] = t
		}
	}
	return out, nil
}

func (f *fakeMeaning) Passages(_ context.Context, _ uuid.UUID, locales []string, _ string, _ int) ([]embedding.Passage, error) {
	f.asked++
	f.locales = locales
	return f.passages, f.err
}

func meaningRig(gql *fakeGraphQL, m *fakeMeaning) ToolSet {
	return New(Deps{GraphQL: gql, Carts: &fakeCarts{carts: map[uuid.UUID]cart.Cart{}}, Meaning: m}).ToolSet(shop)
}

func hit(id, name string) map[string]any {
	return map[string]any{"id": id, "sku": strings.ToUpper(name), "name": name, "kind": "SIMPLE", "url": name, "price": 1000, "inStock": true}
}

func searchAnswering(words []map[string]any, byID map[string]map[string]any) *fakeGraphQL {
	return &fakeGraphQL{answers: map[string]func(map[string]any) (any, error){
		"AgentSearch": func(map[string]any) (any, error) {
			items := make([]any, len(words))
			for i, w := range words {
				items[i] = w
			}
			return map[string]any{"browseProducts": map[string]any{"totalCount": len(words), "items": items}}, nil
		},
		"AgentHits": func(vars map[string]any) (any, error) {
			var out []any
			for _, id := range vars["ids"].([]string) {
				if p, ok := byID[id]; ok {
					out = append(out, p)
				}
			}
			return map[string]any{"productsByIds": out}, nil
		},
	}}
}

func near(id string, distance float64) embedding.Hit {
	return embedding.Hit{ProductID: uuid.MustParse(id), Distance: distance}
}

func TestMeaningFindsWhatTheWordsMissed(t *testing.T) {
	p1, p2, far, hidden := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	gql := searchAnswering(nil, map[string]map[string]any{p1: hit(p1, "Down jacket"), p2: hit(p2, "Wool beanie"), far: hit(far, "Beach towel")})
	m := &fakeMeaning{hits: []embedding.Hit{near(p1, 0.30), near(hidden, 0.32), near(p2, 0.40), near(far, 0.90)}}
	out, isErr := run(t, meaningRig(gql, m), "search_products", map[string]any{"query": "something warm for winter hikes"})
	if isErr || !strings.HasPrefix(out, "No product uses those words, but these are close in what they mean:") {
		t.Fatalf("got %v %q", isErr, out)
	}
	if !strings.Contains(out, "Down jacket") || !strings.Contains(out, "Wool beanie") || strings.Contains(out, "Beach towel") {
		t.Errorf("want the near ones and not the far one:\n%s", out)
	}
	if asked := gql.calls[1].vars["ids"].([]string); !slices.Equal(asked, []string{p1, hidden, p2}) {
		t.Errorf("read %v - want the three within reach of the nearest, the storefront dropping the one it doesn't show", asked)
	}
}

func TestMeaningIsMixedIntoTheFirstPageOfWords(t *testing.T) {
	a, b, c, d := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	gql := searchAnswering([]map[string]any{hit(a, "Alpha"), hit(b, "Bravo"), hit(c, "Charlie")}, map[string]map[string]any{d: hit(d, "Delta")})
	m := &fakeMeaning{hits: []embedding.Hit{near(d, 0.2), near(b, 0.25)}}
	out, _ := run(t, meaningRig(gql, m), "search_products", map[string]any{"query": "something"})
	var order []string
	for line := range strings.SplitSeq(out, "\n") {
		if name, ok := strings.CutPrefix(line, "- "); ok {
			order = append(order, strings.Fields(name)[0])
		}
	}
	if !slices.Equal(order, []string{"Bravo", "Alpha", "Delta", "Charlie"}) {
		t.Errorf("order %v - want the one both found first, then each list's best", order)
	}
	if asked := gql.calls[1].vars["ids"].([]string); !slices.Equal(asked, []string{d}) {
		t.Errorf("read %v again - only the one the words didn't bring needs reading", asked)
	}
}

func TestMeaningStaysOutOfFilteredSortedOrLaterPages(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"no words":     {},
		"a filter":     {"query": "jacket", "filters": map[string][]string{"color": {"blue"}}},
		"a category":   {"query": "jacket", "category": uuid.NewString()},
		"a price":      {"query": "jacket", "price_max": 50},
		"a sort":       {"query": "jacket", "sort": "price_asc"},
		"a later page": {"query": "jacket", "offset": 10},
	} {
		m := &fakeMeaning{hits: []embedding.Hit{near(uuid.NewString(), 0.1)}}
		run(t, meaningRig(searchAnswering(nil, nil), m), "search_products", args)
		if m.asked != 0 {
			t.Errorf("%s asked for meaning - the model wanted exactly that list", name)
		}
	}
}

func TestAFailedMeaningSearchLeavesTheWords(t *testing.T) {
	a := uuid.NewString()
	m := &fakeMeaning{err: errors.New("provider down")}
	out, isErr := run(t, meaningRig(searchAnswering([]map[string]any{hit(a, "Alpha")}, nil), m), "search_products", map[string]any{"query": "alpha"})
	if isErr || !strings.HasPrefix(out, "1 products match") || !strings.Contains(out, "Alpha") {
		t.Errorf("got %v %q", isErr, out)
	}
}

func TestMeaningLooksInTheShoppersLanguageThenTheStores(t *testing.T) {
	m := &fakeMeaning{}
	store := shop
	store.DefaultLocale = "en"
	ts := New(Deps{GraphQL: searchAnswering(nil, nil), Meaning: m}).ToolSet(store)
	raw, _ := json.Marshal(map[string]any{"query": "jacke"})
	ts.Call(locale.WithLocale(context.Background(), "de"), "search_products", raw)
	if !slices.Equal(m.locales, []string{"de", "en"}) {
		t.Errorf("locales %v", m.locales)
	}
	ts.Call(locale.WithLocale(context.Background(), "en"), "search_products", raw)
	if !slices.Equal(m.locales, []string{"en"}) {
		t.Errorf("locales %v - the store's own language once", m.locales)
	}
}

func TestPageSearchHandsOverThePassageThatAnswers(t *testing.T) {
	gql := &fakeGraphQL{answers: map[string]func(map[string]any) (any, error){
		"AgentPages": func(map[string]any) (any, error) {
			return map[string]any{"cmsPages": []any{
				map[string]any{"urlPath": "returns", "content": map[string]any{"title": "Returns", "body": "Send it back within 30 days."}},
				map[string]any{"urlPath": "about", "content": map[string]any{"title": "About us", "body": "We ship to Norway since 2019."}},
			}}, nil
		},
	}}
	shipping := uuid.New()
	m := &fakeMeaning{passages: []embedding.Passage{
		{PageID: shipping, Title: "Shipping", Path: "shipping", Text: "Shipping - Outside the EU\nNorway takes a week.", Distance: 0.2},
		{PageID: shipping, Title: "Shipping", Path: "shipping", Text: "Shipping - Customs\nDuties are paid on delivery.", Distance: 0.26},
		{PageID: uuid.New(), Title: "Careers", Path: "careers", Text: "Careers\nJoin us.", Distance: 0.8},
	}}
	out, _ := run(t, meaningRig(gql, m), "search_store_pages", map[string]any{"query": "norway"})
	if !strings.Contains(out, "## Shipping - https://shop.example/shipping\nShipping - Outside the EU\nNorway takes a week.\nShipping - Customs") {
		t.Errorf("want the shipping page with the passages that answer:\n%q", out)
	}
	if !strings.Contains(out, "## About us - https://shop.example/about\nWe ship to Norway") || strings.Contains(out, "Careers") {
		t.Errorf("want the page the words found too, and not the far one:\n%s", out)
	}

	m = &fakeMeaning{passages: []embedding.Passage{{PageID: shipping, Title: "Shipping", Path: "shipping", Text: "Shipping - EU\nTwo days.", Distance: 0.2}}}
	if out, _ := run(t, meaningRig(gql, m), "search_store_pages", map[string]any{"query": "delivery abroad"}); !strings.HasPrefix(out, "## Shipping - https://shop.example/shipping\nShipping - EU\nTwo days.") {
		t.Errorf("a page no word matched should still come back by meaning:\n%s", out)
	}
}
