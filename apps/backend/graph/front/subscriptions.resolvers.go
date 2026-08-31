package front

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

func (r *mutationResolver) CancelSubscription(ctx context.Context, id string) (*model.CustomerSubscription, error) {
	claims, err := auth.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, invalidID("subscription")
	}
	customerID := claims.CustomerID
	sub, err := r.SubscriptionSvc.Cancel(ctx, sid, &customerID)
	if err != nil {
		return nil, err
	}
	return toFrontSubscriptionModel(sub), nil
}

// Picks which saved method the renewals charge - BOTH the subscription and the method have to belong to the signed-in customer
func (r *mutationResolver) SetMySubscriptionPaymentMethod(ctx context.Context, subscriptionID string, paymentMethodID *string) (bool, error) {
	customerID := auth.OptionalCustomerID(ctx)
	if customerID == nil {
		return false, apperr.New("UNAUTHENTICATED", apperr.ClassAuth, "sign in to manage subscriptions")
	}
	sid, err := uuid.Parse(subscriptionID)
	if err != nil {
		return false, invalidID("subscription")
	}
	var pmID *uuid.UUID
	if paymentMethodID != nil && *paymentMethodID != "" {
		mid, err := uuid.Parse(*paymentMethodID)
		if err != nil {
			return false, invalidID("payment method")
		}
		if _, err := r.PaymentVaultSvc.Get(ctx, *customerID, mid); err != nil { // ownership check on the method row itself
			return false, err
		}
		pmID = &mid
	}
	if _, err := r.SubscriptionSvc.SetPaymentMethodForCustomer(ctx, *customerID, sid, pmID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) MySubscriptions(ctx context.Context) ([]*model.CustomerSubscription, error) {
	claims, err := auth.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := r.SubscriptionSvc.ListForCustomer(ctx, claims.CustomerID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CustomerSubscription, len(subs))
	for i, s := range subs {
		out[i] = toFrontSubscriptionModel(s)
	}
	return out, nil
}
