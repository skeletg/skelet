package front

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
)

// meaningListing is the first products the search page lists for a query, when meaning answered it - the same
// synonyms and boosts the page applies, so the overview speaks of what the shopper sees
func (r *Resolver) meaningListing(ctx context.Context, channelID uuid.UUID, locale, query string) ([]uuid.UUID, bool) {
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > shoptools.MaxOverviewQuery {
		return nil, false
	}
	req := search.BrowseRequest{ChannelID: channelID, Locale: locale, Locales: r.meaningLocales(ctx, locale), Query: query, Words: query, Limit: shoptools.OverviewProducts}
	if r.SearchMerchSvc != nil {
		req.Boosts = r.SearchMerchSvc.BoostsFor(ctx, channelID, query)
		req.Words = r.SearchMerchSvc.ExpandQuery(ctx, channelID, query)
	}
	res, ex, err := r.ProductBrowse.Explain(ctx, req)
	if err != nil || ex.Answered != search.AnsweredMeaning || len(res.IDs) == 0 {
		return nil, false
	}
	return res.IDs, true
}

func searchOverviewOut(o shoptools.Overview) *model.SearchOverview {
	if o.Text == "" {
		return nil
	}
	out := &model.SearchOverview{Text: o.Text, Recap: shoptools.OverviewRecap(o), Products: make([]*model.SearchOverviewProduct, len(o.Products))}
	for i, p := range o.Products {
		out.Products[i] = &model.SearchOverviewProduct{ID: p.ID.String(), Name: p.Name, URL: p.URL}
	}
	return out
}

func unfiltered(f *model.BrowseProductsFilter) bool {
	return f == nil || (f.CategoryID == nil && f.CollectionID == nil && len(f.Attributes) == 0 && f.PriceMinCents == nil && f.PriceMaxCents == nil && len(f.Kinds) == 0)
}

func (r *Resolver) overviewOffered(ctx context.Context) bool {
	sv, ok := channel.FromContext(ctx)
	return ok && sv != nil && r.ShopAssistant != nil && r.ShopAssistant.OverviewOffered(ctx, *sv)
}
