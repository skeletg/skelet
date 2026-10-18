package admin

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) UpdatePaymentMethodPresentation(ctx context.Context, channelID string, providerSlug string, displayName string, sortOrder int32) (*model.PaymentMethodAdmin, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "payments.manage"); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channel", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "payments.manage", id, nil); err != nil {
		return nil, err
	}
	if _, err := r.PaymentSvc.UpdatePresentation(ctx, id, providerSlug, displayName, sortOrder); err != nil {
		return nil, err
	}
	return r.paymentMethodModel(ctx, id, providerSlug)
}

func (r *mutationResolver) SetPaymentMethodEnabled(ctx context.Context, channelID string, providerSlug string, enabled bool) (*model.PaymentMethodAdmin, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "payments.manage"); err != nil {
		return nil, err
	}
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "payments.manage", chID, nil); err != nil {
		return nil, err
	}
	if _, err := r.PaymentSvc.SetEnabled(ctx, chID, providerSlug, enabled); err != nil {
		if errors.Is(err, payment.ErrProviderDisabled) {
			return nil, apperr.New("PAYMENT_PROVIDER_DISABLED", apperr.ClassValidation, "The provider plugin is disabled. Enable it in Payment providers before offering this method.")
		}
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
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "payments.manage", chID, nil); err != nil {
		return nil, err
	}
	if err := r.PaymentSvc.SetPaymentMethodCountries(ctx, chID, providerSlug, mode, countries); err != nil {
		return nil, err
	}
	return r.paymentMethodModel(ctx, chID, providerSlug)
}

func (r *queryResolver) PaymentProviders(ctx context.Context, channelID *string) ([]*model.PaymentProviderAdmin, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "payments.view"); err != nil {
		return nil, err
	}
	id := uuid.Nil
	if channelID != nil {
		var err error
		id, err = uuid.Parse(*channelID)
		if err != nil {
			return nil, errInvalidID("channel", err)
		}
		if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "payments.view", id, nil); err != nil {
			return nil, err
		}
	}
	return paymentProviderModels(ctx, id), nil
}

func (r *queryResolver) PaymentMethods(ctx context.Context, channelID string) ([]*model.PaymentMethodAdmin, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "payments.view"); err != nil {
		return nil, err
	}
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "payments.view", chID, nil); err != nil {
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
