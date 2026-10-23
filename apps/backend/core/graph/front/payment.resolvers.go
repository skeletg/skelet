package front

import (
	"context"
	"slices"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
)

func (r *queryResolver) PaymentMethods(ctx context.Context, countryCode string, cartID *string) ([]*model.PaymentMethodOption, error) {
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
	if cartID != nil && *cartID != "" {
		c, err := r.checkoutCart(ctx, cartID)
		if err != nil {
			return nil, err
		}
		if !r.checkoutSettings(ctx, c.ChannelID).capturesPONumber(c.Checkout) {
			methods = slices.DeleteFunc(methods, func(m payment.PaymentMethod) bool { return m.ProviderSlug == purchaseOrderSlug })
		}
	}
	out := make([]*model.PaymentMethodOption, len(methods))
	for i, m := range methods {
		reg, regErr := payment.GetProvider(m.ProviderSlug)
		out[i] = &model.PaymentMethodOption{
			ProviderSlug: m.ProviderSlug,
			DisplayName:  m.DisplayName,
			SortOrder:    m.SortOrder,
			Instructions: r.paymentInstructions(ctx, m.ProviderSlug, requestConfigScope(ctx)),
			IsOffline:    regErr == nil && reg.IsOffline,
		}
	}
	return out, nil
}
