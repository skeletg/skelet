package admin

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

const maxPreviewQuery = 500

func previewProduct(p shoptools.PreviewProduct) *model.SearchPreviewProduct {
	return &model.SearchPreviewProduct{ID: p.ID, Name: p.Name, Sku: p.SKU, URL: p.URL, Price: int(p.Price), CurrencyCode: p.Currency, InStock: p.InStock}
}

func searchPreview(p shoptools.Preview) *model.SearchPreview {
	out := &model.SearchPreview{Locales: append([]string{}, p.Locales...), WordsTotal: int32(p.WordsTotal), ProductsText: p.Products, PagesText: p.Pages, MeaningFailed: p.MeaningFailed,
		ByWords: []*model.SearchPreviewProduct{}, ByMeaning: []*model.SearchPreviewNear{}, Shown: []*model.SearchPreviewShown{},
		PagesByWords: []*model.SearchPreviewPage{}, Passages: []*model.SearchPreviewPassage{}}
	for _, w := range p.ByWords {
		out.ByWords = append(out.ByWords, previewProduct(w))
	}
	for _, n := range p.ByMeaning {
		out.ByMeaning = append(out.ByMeaning, &model.SearchPreviewNear{Product: previewProduct(n.Product), Distance: n.Distance, Counted: n.Counted})
	}
	for _, s := range p.Shown {
		out.Shown = append(out.Shown, &model.SearchPreviewShown{Product: previewProduct(s.Product), ByWords: s.ByWords, ByMeaning: s.ByMeaning})
	}
	for _, pg := range p.PagesByWords {
		out.PagesByWords = append(out.PagesByWords, &model.SearchPreviewPage{Title: pg.Title, URL: pg.URL})
	}
	for _, ps := range p.Passages {
		out.Passages = append(out.Passages, &model.SearchPreviewPassage{Title: ps.Title, URL: ps.URL, Text: ps.Text, Distance: ps.Distance, Counted: ps.Counted})
	}
	return out
}

func storefrontReading(r search.Reading) model.StorefrontSearchReading {
	switch r {
	case search.ReadsWords:
		return model.StorefrontSearchReadingWords
	case search.ReadsSentence:
		return model.StorefrontSearchReadingSentence
	case search.ReadsBlended:
		return model.StorefrontSearchReadingBlended
	}
	return model.StorefrontSearchReadingKeywords
}

func storefrontSearchRequest(ch channel.Channel, locale *string, query string) search.BrowseRequest {
	lang := ch.DefaultLocale
	if locale != nil && strings.TrimSpace(*locale) != "" {
		lang = strings.TrimSpace(*locale)
	}
	return search.BrowseRequest{ChannelID: ch.ID, Locale: lang, Locales: slices.Compact([]string{lang, ch.DefaultLocale}), Query: query, Words: query}
}

// trying runs what follows with the page's unsaved choices, so both previews answer as the store would once they're saved
func trying(ctx context.Context, mode *model.SearchByMeaningMode, strictness *model.SearchByMeaningStrictness) context.Context {
	lower := func(v *string) string {
		if v == nil {
			return ""
		}
		return strings.ToLower(*v)
	}
	return search.Trying(ctx, lower((*string)(mode)), lower((*string)(strictness)))
}

func storefrontSearchPreview(res search.BrowseResult, ex search.Explanation, products map[uuid.UUID]shoptools.PreviewProduct) *model.StorefrontSearchPreview {
	out := &model.StorefrontSearchPreview{Reads: storefrontReading(ex.Reads), AnsweredBy: model.StorefrontSearchAnswer(strings.ToUpper(string(ex.Answered))),
		Margin: ex.Margin, Total: int32(res.Total), WordsTotal: int32(ex.WordsTotal), MeaningFailed: ex.Failed,
		Listed: []*model.SearchPreviewShown{}, ByWords: []*model.SearchPreviewProduct{}, ByMeaning: []*model.SearchPreviewNear{}}
	counted := map[uuid.UUID]bool{}
	for _, n := range ex.Near {
		counted[n.ID] = n.Counted
		if p, ok := products[n.ID]; ok {
			out.ByMeaning = append(out.ByMeaning, &model.SearchPreviewNear{Product: previewProduct(p), Distance: n.Distance, Counted: n.Counted})
		}
	}
	for _, id := range ex.Words {
		if p, ok := products[id]; ok {
			out.ByWords = append(out.ByWords, previewProduct(p))
		}
	}
	for _, id := range res.IDs {
		if p, ok := products[id]; ok {
			out.Listed = append(out.Listed, &model.SearchPreviewShown{Product: previewProduct(p), ByWords: slices.Contains(ex.Words, id), ByMeaning: counted[id]})
		}
	}
	return out
}

var providerStatus = regexp.MustCompile(`^.*?\b\d{3} from [^\s:]+`)

// The provider's own words can carry account details, so an admin over every store sees them and a store's own admins
// the status and host
func (r *Resolver) providerError(ctx context.Context, msg string) string {
	return providerError(ctx, r.AdminSvc, msg)
}

func providerError(ctx context.Context, perms auth.PermissionChecker, msg string) string {
	if msg == "" || auth.RequirePermission(ctx, perms, "config.view") == nil {
		return msg
	}
	if m := providerStatus.FindString(msg); m != "" {
		return m
	}
	return "the AI provider returned an error - an admin over every store can see it"
}

func overviewPreview(status model.SearchOverviewStatus, ch channel.Channel, o shoptools.Overview) *model.SearchOverviewPreview {
	out := &model.SearchOverviewPreview{Status: status, Text: o.Text, Products: make([]*model.SearchOverviewPreviewProduct, len(o.Products))}
	for i, p := range o.Products {
		out.Products[i] = &model.SearchOverviewPreviewProduct{ID: p.ID.String(), Name: p.Name, URL: p.Link(ch)}
	}
	return out
}
