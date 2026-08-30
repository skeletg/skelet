package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/payment"
)

// Re-reads after a mutation so the returned row carries the hydrated country allow-list, same as the list query builds it
func (r *mutationResolver) paymentMethodModel(ctx context.Context, channelID uuid.UUID, slug string) (*model.PaymentMethodAdmin, error) {
	providers, err := r.PaymentSvc.ListProvidersForChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	for _, p := range providers {
		if p.Slug == slug {
			return toPaymentMethodAdminModel(p), nil
		}
	}
	return nil, fmt.Errorf("payment provider %q not found for channel", slug)
}

func toPaymentMethodAdminModel(p payment.ProviderWithMethod) *model.PaymentMethodAdmin {
	displayName := p.Method.DisplayName
	if displayName == "" {
		displayName = p.DisplayName // method row not synced yet, so fall back to the registry's name
	}
	mode := p.Method.CountryMode
	if mode == "" {
		mode = "all" // the UI always wants a concrete mode
	}
	return &model.PaymentMethodAdmin{
		IsOffline:        p.IsOffline,
		ProviderSlug:     p.Slug,
		DisplayName:      displayName,
		IsEnabled:        p.Method.IsEnabled,
		SortOrder:        p.Method.SortOrder,
		CountryMode:      mode,
		AllowedCountries: append([]string{}, p.Method.AllowedCountries...),
	}
}
