package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

func (r *mutationResolver) SetPaymentMethodEnabled(ctx context.Context, channelID string, providerSlug string, enabled bool) (*model.PaymentMethodAdmin, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "payments.manage"); err != nil {
		return nil, err
	}
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, err
	}
	if _, err := r.PaymentSvc.SetEnabled(ctx, chID, providerSlug, enabled); err != nil {
		return nil, err
	}
	return r.paymentMethodModel(ctx, chID, providerSlug)
}

func (r *mutationResolver) SetPaymentMethodCountries(ctx context.Context, channelID string, providerSlug string, mode string, countries []string) (*model.PaymentMethodAdmin, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "payments.manage"); err != nil {
		return nil, err
	}
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, err
	}
	if err := r.PaymentSvc.SetPaymentMethodCountries(ctx, chID, providerSlug, mode, countries); err != nil {
		return nil, err
	}
	return r.paymentMethodModel(ctx, chID, providerSlug)
}

func (r *queryResolver) PaymentMethods(ctx context.Context, channelID string) ([]*model.PaymentMethodAdmin, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "payments.view"); err != nil {
		return nil, err
	}
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, err
	}
	providers, err := r.PaymentSvc.ListProvidersForChannel(ctx, chID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.PaymentMethodAdmin, len(providers))
	for i, p := range providers {
		out[i] = toPaymentMethodAdminModel(p)
	}
	return out, nil
}
