package front

import (
	"context"
	"strings"

	"github.com/skeletg/skelet/apps/backend/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
)

func (r *queryResolver) PaymentMethods(ctx context.Context, countryCode string) ([]*model.PaymentMethodOption, error) {
	if r.PaymentSvc == nil {
		return nil, nil
	}
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return nil, storeRequired()
	}
	methods, err := r.PaymentSvc.ListEligiblePaymentMethods(ctx, sv.ID, strings.ToUpper(strings.TrimSpace(countryCode)))
	if err != nil {
		return nil, err
	}
	out := make([]*model.PaymentMethodOption, len(methods))
	for i, m := range methods {
		out[i] = &model.PaymentMethodOption{
			ProviderSlug: m.ProviderSlug,
			DisplayName:  m.DisplayName,
			SortOrder:    m.SortOrder,
			Instructions: r.paymentInstructions(ctx, m.ProviderSlug, requestConfigScope(ctx)),
		}
	}
	return out, nil
}
