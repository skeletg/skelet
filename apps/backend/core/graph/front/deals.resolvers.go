package front

import (
	"context"

	model1 "github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

func (r *queryResolver) StorePromotions(ctx context.Context) ([]*model.StorePromotion, error) {
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return nil, storeRequired()
	}
	customer := channel.OptionalCustomerID(ctx)
	promos, err := r.PromotionSvc.PublicPromotions(ctx, promotion.Audience{ChannelID: sv.ID, ChannelSlug: sv.Slug, CustomerID: customer,
		CustomerGroupSlug: r.resolveCustomerGroupSlug(ctx, customer), Locale: r.activeLocale(ctx)})
	if err != nil {
		return nil, err
	}
	out := make([]*model.StorePromotion, len(promos))
	for i, p := range promos {
		out[i] = &model.StorePromotion{Label: p.Label, Terms: p.Terms, EndsAt: p.EndsAt}
	}
	return out, nil
}

func (r *queryResolver) SaleProducts(ctx context.Context, limit *int32) ([]*model1.Product, error) {
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return nil, storeRequired()
	}
	n := 0
	if limit != nil {
		n = int(*limit)
	}
	ids, err := r.CatalogSvc.SaleProductIDs(ctx, sv.ID, n)
	if err != nil {
		return nil, err
	}
	return r.renderProductsByIDs(ctx, ids), nil
}
