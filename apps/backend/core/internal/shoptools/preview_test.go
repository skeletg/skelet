package shoptools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/embedding"
)

func previewNames(ps []PreviewProduct) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}

func TestPreviewKeepsEveryStepOfBothSearches(t *testing.T) {
	a, b, d, hidden, far := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	gql := searchAnswering([]map[string]any{hit(a, "Alpha"), hit(b, "Bravo")}, map[string]map[string]any{d: hit(d, "Delta"), far: hit(far, "Foxtrot")})
	gql.answers["AgentPages"] = func(map[string]any) (any, error) {
		return map[string]any{"cmsPages": []any{
			map[string]any{"urlPath": "about", "content": map[string]any{"title": "About us", "body": "We ship to Norway since 2019."}},
			map[string]any{"urlPath": "returns", "content": map[string]any{"title": "Returns", "body": "Send it back within 30 days."}},
		}}, nil
	}
	m := &fakeMeaning{
		hits: []embedding.Hit{near(d, 0.2), near(b, 0.25), near(hidden, 0.3), near(far, 0.9)},
		passages: []embedding.Passage{
			{PageID: uuid.New(), Title: "Shipping", Path: "shipping", Text: "Shipping - Outside the EU\nNorway takes a week.", Distance: 0.2},
			{PageID: uuid.New(), Title: "Careers", Path: "careers", Text: "Careers\nJoin us.", Distance: 0.8},
		},
	}
	store := shop
	store.DefaultLocale = "en"
	p, err := New(Deps{GraphQL: gql, Meaning: m}).Preview(context.Background(), store, "de", "norway")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.Locales, ",") != "de,en" || p.WordsTotal != 2 || strings.Join(previewNames(p.ByWords), ",") != "Alpha,Bravo" {
		t.Errorf("locales %v, words %d %v", p.Locales, p.WordsTotal, previewNames(p.ByWords))
	}

	var meaning []string
	for _, n := range p.ByMeaning {
		meaning = append(meaning, n.Product.Name)
		if n.Counted != (n.Product.Name != "Foxtrot") {
			t.Errorf("%s at %.2f counted=%v - only the far one should be left out", n.Product.Name, n.Distance, n.Counted)
		}
	}
	if strings.Join(meaning, ",") != "Delta,Bravo,Foxtrot" {
		t.Errorf("by meaning %v - want the storefront's hidden one dropped, the far one kept but not counted", meaning)
	}

	var shown []string
	for _, s := range p.Shown {
		shown = append(shown, s.Product.Name)
	}
	if strings.Join(shown, ",") != "Bravo,Alpha,Delta" {
		t.Errorf("shown %v - want the fused order the tool answers with", shown)
	}
	if s := p.Shown[0]; !s.ByWords || !s.ByMeaning || p.Shown[2].ByWords || !p.Shown[2].ByMeaning {
		t.Errorf("where each came from: %+v", p.Shown)
	}
	if i, j := strings.Index(p.Products, "Bravo"), strings.Index(p.Products, "Delta"); i < 0 || j < i || strings.Contains(p.Products, "Foxtrot") {
		t.Errorf("search_products answer:\n%s", p.Products)
	}

	if len(p.PagesByWords) != 1 || p.PagesByWords[0].Title != "About us" || p.PagesByWords[0].URL != "https://shop.example/about" {
		t.Errorf("pages by words %+v", p.PagesByWords)
	}
	if len(p.Passages) != 2 || !p.Passages[0].Counted || p.Passages[1].Counted {
		t.Errorf("passages %+v", p.Passages)
	}
	if !strings.Contains(p.Pages, "## Shipping") || strings.Contains(p.Pages, "Careers") {
		t.Errorf("search_store_pages answer:\n%s", p.Pages)
	}
	if p.MeaningFailed != "" {
		t.Errorf("meaning failed %q", p.MeaningFailed)
	}
}

func TestPreviewShowsTheWordsWhenMeaningFails(t *testing.T) {
	a := uuid.NewString()
	gql := searchAnswering([]map[string]any{hit(a, "Alpha")}, nil)
	gql.answers["AgentPages"] = func(map[string]any) (any, error) { return map[string]any{"cmsPages": []any{}}, nil }
	p, err := New(Deps{GraphQL: gql, Meaning: &fakeMeaning{err: errors.New("provider down")}}).Preview(context.Background(), shop, "", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if p.MeaningFailed != "provider down" || len(p.Shown) != 1 || p.Shown[0].Product.Name != "Alpha" || p.Shown[0].ByMeaning {
		t.Errorf("failed %q, shown %+v - want the words alone and the reason", p.MeaningFailed, p.Shown)
	}
}
