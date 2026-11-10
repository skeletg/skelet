package front

import (
	"context"
	"errors"
	"log/slog"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *queryResolver) SearchOverview(ctx context.Context, query string) (*model.SearchOverview, error) {
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return nil, storeRequired()
	}
	if r.ShopAssistant == nil || r.ProductBrowse == nil || !r.ShopAssistant.OverviewOffered(ctx, *sv) {
		return nil, nil
	}
	if err := r.checkCatalogRate(ctx); err != nil {
		return nil, err
	}
	locale := r.activeLocale(ctx)
	ids, ok := r.meaningListing(ctx, sv.ID, locale, query)
	if !ok {
		return nil, nil
	}
	who := shoptools.Shopper{Customer: channel.OptionalCustomerID(ctx)}
	if req, ok := auth.RequestFromContext(ctx); ok {
		who.Address = auth.ClientIP(req)
	}
	o, err := r.ShopAssistant.Overview(ctx, *sv, who, locale, query, ids, false)
	if err != nil {
		if !errors.Is(err, shoptools.ErrAssistantUnavailable) && !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "front: search overview failed", "channel", sv.ID, "err", err)
		}
		return nil, nil
	}
	return searchOverviewOut(o), nil
}
