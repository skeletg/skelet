package admin

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *queryResolver) SearchPreview(ctx context.Context, channelID string, query string, locale *string, mode *model.SearchByMeaningMode, strictness *model.SearchByMeaningStrictness) (*model.SearchPreview, error) {
	cid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "catalog.view", cid, nil); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" || len([]rune(query)) > maxPreviewQuery {
		return nil, apperr.New("SEARCH_PREVIEW_QUERY", apperr.ClassValidation, "ask with a question of up to 500 characters")
	}
	if r.ShopTools == nil {
		return nil, apperr.New("SEARCH_PREVIEW_UNAVAILABLE", apperr.ClassUnavailable, "the assistant's tools aren't running on this server")
	}
	ch, err := r.ChannelSvc.GetByID(ctx, cid)
	if err != nil {
		return nil, err
	}
	lang := ""
	if locale != nil {
		lang = *locale
	}
	p, err := r.ShopTools.Preview(trying(ctx, mode, strictness), ch, lang, query)
	if err != nil {
		return nil, err
	}
	out := searchPreview(p)
	out.MeaningFailed = r.providerError(ctx, out.MeaningFailed)
	return out, nil
}

func (r *queryResolver) EmbeddingCoverage(ctx context.Context, channelID string) (*model.EmbeddingCoverage, error) {
	cid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "catalog.view", cid, nil); err != nil {
		return nil, err
	}
	if r.Embeddings == nil {
		return &model.EmbeddingCoverage{}, nil
	}
	st, err := r.Embeddings.Status(ctx, &cid)
	if err != nil {
		return nil, err
	}
	return &model.EmbeddingCoverage{Model: st.Model, Products: int32(st.Products), EmbeddedProducts: int32(st.EmbeddedProducts),
		Pages: int32(st.Pages), EmbeddedPages: int32(st.EmbeddedPages), Queued: int32(st.Queued), Failed: r.providerError(ctx, st.Failed)}, nil
}

func (r *queryResolver) StorefrontSearchPreview(ctx context.Context, channelID string, query string, locale *string, mode *model.SearchByMeaningMode, strictness *model.SearchByMeaningStrictness) (*model.StorefrontSearchPreview, error) {
	cid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "catalog.view", cid, nil); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" || len([]rune(query)) > maxPreviewQuery {
		return nil, apperr.New("SEARCH_PREVIEW_QUERY", apperr.ClassValidation, "search with up to 500 characters")
	}
	if r.StorefrontSearch == nil || r.ShopTools == nil {
		return nil, apperr.New("SEARCH_PREVIEW_UNAVAILABLE", apperr.ClassUnavailable, "the store's search isn't running on this server")
	}
	ch, err := r.ChannelSvc.GetByID(ctx, cid)
	if err != nil {
		return nil, err
	}
	req := storefrontSearchRequest(ch, locale, query)
	if r.SearchMerchSvc != nil {
		req.Boosts = r.SearchMerchSvc.BoostsFor(ctx, cid, query)
		req.Words = r.SearchMerchSvc.ExpandQuery(ctx, cid, query)
	}
	res, ex, err := r.StorefrontSearch.Explain(trying(ctx, mode, strictness), req)
	if err != nil {
		return nil, err
	}
	ids := slices.Concat(res.IDs, ex.Words)
	for _, n := range ex.Near {
		ids = append(ids, n.ID)
	}
	products, err := r.ShopTools.PreviewProducts(ctx, ch, req.Locale, ids)
	if err != nil {
		return nil, err
	}
	out := storefrontSearchPreview(res, ex, products)
	out.MeaningFailed = r.providerError(ctx, out.MeaningFailed)
	return out, nil
}

func (r *queryResolver) SearchOverviewPreview(ctx context.Context, channelID string, query string, locale *string, mode *model.SearchByMeaningMode, strictness *model.SearchByMeaningStrictness) (*model.SearchOverviewPreview, error) {
	cid, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	// it writes a paid overview from the store's daily allowance, so it takes whoever may switch overviews on
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.manage", cid, nil); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" || len([]rune(query)) > shoptools.MaxOverviewQuery {
		return nil, apperr.New("SEARCH_PREVIEW_QUERY", apperr.ClassValidation, "search with up to 200 characters")
	}
	if r.StorefrontSearch == nil || r.ShopTools == nil {
		return nil, apperr.New("SEARCH_PREVIEW_UNAVAILABLE", apperr.ClassUnavailable, "the store's search isn't running on this server")
	}
	ch, err := r.ChannelSvc.GetByID(ctx, cid)
	if err != nil {
		return nil, err
	}
	ctx = trying(ctx, mode, strictness)
	req := storefrontSearchRequest(ch, locale, query)
	req.Limit = shoptools.OverviewProducts
	if r.SearchMerchSvc != nil {
		req.Boosts = r.SearchMerchSvc.BoostsFor(ctx, cid, query)
		req.Words = r.SearchMerchSvc.ExpandQuery(ctx, cid, query)
	}
	res, ex, err := r.StorefrontSearch.Explain(ctx, req)
	if err != nil {
		return nil, err
	}
	if ex.Answered != search.AnsweredMeaning {
		return overviewPreview(model.SearchOverviewStatusKeywords, ch, shoptools.Overview{}), nil
	}
	o, err := r.ShopTools.Overview(ctx, ch, shoptools.Shopper{}, req.Locale, query, res.IDs, true)
	switch {
	case errors.Is(err, shoptools.ErrAssistantUnavailable):
		return overviewPreview(model.SearchOverviewStatusUnavailable, ch, o), nil
	case err != nil:
		return nil, apperr.New("SEARCH_OVERVIEW_FAILED", apperr.ClassUnavailable, r.providerError(ctx, err.Error()))
	case o.Text == "":
		return overviewPreview(model.SearchOverviewStatusNothing, ch, o), nil
	}
	return overviewPreview(model.SearchOverviewStatusWritten, ch, o), nil
}
