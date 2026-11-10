package shoptools

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"
)

// Preview is what the assistant's two searches find for a question on one store, step by step, for the dashboard to show
// how it gets there - the words' matches, the nearest by meaning and which of them count, what the model is then shown,
// and the tools' answers word for word
type Preview struct {
	Locales []string

	WordsTotal int
	ByWords    []PreviewProduct
	ByMeaning  []PreviewNear
	Shown      []PreviewShown
	Products   string // search_products' answer

	PagesByWords []PreviewPage
	Passages     []PreviewPassage
	Pages        string // search_store_pages' answer

	MeaningFailed string // why search by meaning didn't answer - the tools then go on with the words alone, as here
}

type PreviewProduct struct {
	ID, Name, SKU, URL string
	Price              int64
	Currency           string
	InStock            bool
}

// PreviewNear is a product by meaning - Counted when it's close enough to the nearest one to join the words' results
type PreviewNear struct {
	Product  PreviewProduct
	Distance float64
	Counted  bool
}

type PreviewShown struct {
	Product            PreviewProduct
	ByWords, ByMeaning bool
}

type PreviewPage struct{ Title, URL string }

type PreviewPassage struct {
	Title, URL, Text string
	Distance         float64
	Counted          bool
}

// Preview runs the searches as the assistant would for a shopper on the store in that language - the storefront's own
// rules for what shows, the same blend, the same cut-off - and keeps every step
func (s *Service) Preview(ctx context.Context, ch channel.Channel, lang, query string) (Preview, error) {
	if lang = strings.TrimSpace(lang); lang != "" {
		ctx = locale.WithLocale(ctx, lang)
	}
	ctx = channel.WithChannel(ctx, &ch)
	c := call{svc: s, ch: ch, a: args{Query: strings.TrimSpace(query)}}
	p := Preview{Locales: searchLocales(ctx, ch)}

	var words struct {
		BrowseProducts struct {
			TotalCount int          `json:"totalCount"`
			Items      []productHit `json:"items"`
		} `json:"browseProducts"`
	}
	if err := c.gql().Do(ctx, searchDoc, map[string]any{"query": c.a.Query, "filter": map[string]any{}, "limit": searchPageSize, "offset": 0}, &words); err != nil {
		return p, err
	}
	p.WordsTotal = words.BrowseProducts.TotalCount
	byID := map[string]PreviewProduct{}
	wordIDs := make([]string, len(words.BrowseProducts.Items))
	for i, h := range words.BrowseProducts.Items {
		byID[h.ID], wordIDs[i] = previewProduct(c, h), h.ID
		p.ByWords = append(p.ByWords, byID[h.ID])
	}

	var counted []string
	share, on := c.reach(ctx)
	if m := s.deps.Meaning; m != nil && on {
		hits, err := m.Products(ctx, ch.ID, p.Locales, c.a.Query, searchPageSize)
		if err != nil {
			p.MeaningFailed = err.Error()
		}
		var missing []string
		for _, h := range hits {
			if _, ok := byID[h.ProductID.String()]; !ok {
				missing = append(missing, h.ProductID.String())
			}
		}
		if len(missing) > 0 {
			var out struct {
				ProductsByIds []productHit `json:"productsByIds"`
			}
			if err := c.gql().Do(ctx, hitsDoc, map[string]any{"ids": missing}, &out); err != nil {
				return p, err
			}
			for _, h := range out.ProductsByIds {
				byID[h.ID] = previewProduct(c, h)
			}
		}
		for _, h := range hits {
			id := h.ProductID.String()
			product, shows := byID[id]
			if !shows { // the storefront doesn't show it to this shopper, so the assistant never would either
				continue
			}
			near := PreviewNear{Product: product, Distance: h.Distance, Counted: search.Within(hits[0].Distance, h.Distance, share)}
			if near.Counted {
				counted = append(counted, id)
			}
			p.ByMeaning = append(p.ByMeaning, near)
		}
	}
	fromWords := map[string]bool{}
	for _, id := range wordIDs {
		fromWords[id] = true
	}
	for _, id := range fuse(wordIDs, counted, searchPageSize) {
		p.Shown = append(p.Shown, PreviewShown{Product: byID[id], ByWords: fromWords[id], ByMeaning: slices.Contains(counted, id)})
	}

	raw, _ := json.Marshal(map[string]any{"query": c.a.Query})
	p.Products = s.callShared(ctx, ch, "search_products", raw).Content

	pages, err := storePages(ctx, c)
	if err != nil {
		return p, err
	}
	if terms := strings.Fields(strings.ToLower(c.a.Query)); len(terms) > 0 {
		type scored struct {
			page  PreviewPage
			score int
		}
		var hits []scored
		for _, pg := range pages {
			if score, ok := pageScore(pg, terms); ok {
				hits = append(hits, scored{PreviewPage{Title: pg.title, URL: pg.url}, score})
			}
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
		for _, h := range hits {
			p.PagesByWords = append(p.PagesByWords, h.page)
		}
	}
	if m := s.deps.Meaning; m != nil && on && p.MeaningFailed == "" {
		found, err := m.Passages(ctx, ch.ID, p.Locales, c.a.Query, pagesShown*passagesPerPage)
		if err != nil {
			p.MeaningFailed = err.Error()
		}
		for _, f := range found {
			p.Passages = append(p.Passages, PreviewPassage{Title: f.Title, URL: link(ch, f.Path), Text: f.Text, Distance: f.Distance,
				Counted: search.Within(found[0].Distance, f.Distance, share)})
		}
	}
	p.Pages = s.callShared(ctx, ch, "search_store_pages", raw).Content
	return p, nil
}

func previewProduct(c call, h productHit) PreviewProduct {
	return PreviewProduct{ID: h.ID, Name: h.Name, SKU: h.SKU, URL: link(c.ch, h.URL), Price: h.Price, Currency: currency(h.DefaultCurrencyCode, c), InStock: h.InStock}
}

// PreviewProducts reads products the way the storefront shows them on a store, in that language - one it doesn't show is left out
func (s *Service) PreviewProducts(ctx context.Context, ch channel.Channel, lang string, ids []uuid.UUID) (map[uuid.UUID]PreviewProduct, error) {
	if lang = strings.TrimSpace(lang); lang != "" {
		ctx = locale.WithLocale(ctx, lang)
	}
	ctx = channel.WithChannel(ctx, &ch)
	c := call{svc: s, ch: ch}
	var keys []string
	for _, id := range ids {
		if k := id.String(); !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	out := make(map[uuid.UUID]PreviewProduct, len(keys))
	for chunk := range slices.Chunk(keys, catalog.ProductsPerLookup) {
		var page struct {
			ProductsByIds []productHit `json:"productsByIds"`
		}
		if err := c.gql().Do(ctx, hitsDoc, map[string]any{"ids": chunk}, &page); err != nil {
			return nil, err
		}
		for _, h := range page.ProductsByIds {
			if id, err := uuid.Parse(h.ID); err == nil {
				out[id] = previewProduct(c, h)
			}
		}
	}
	return out, nil
}
