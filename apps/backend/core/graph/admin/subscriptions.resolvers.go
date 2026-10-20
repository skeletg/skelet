package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CancelSubscription(ctx context.Context, id string) (*model.CustomerSubscription, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "subscriptions.manage"); err != nil {
		return nil, err
	}
	sid, err := parseID(id, "subscription")
	if err != nil {
		return nil, err
	}
	existing, err := r.SubscriptionSvc.Get(ctx, sid) // channel gate - a scoped admin must not reach another channel's subscription by id
	if err != nil {
		return nil, err
	}
	if err := auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "subscriptions.manage", existing.ChannelID); err != nil {
		return nil, err
	}
	sub, err := r.SubscriptionSvc.Cancel(ctx, sid, nil)
	if err != nil {
		return nil, err
	}
	return toCustomerSubscriptionModel(sub, nil), nil
}

func (r *mutationResolver) PauseSubscription(ctx context.Context, id string) (*model.CustomerSubscription, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "subscriptions.manage"); err != nil {
		return nil, err
	}
	sid, err := parseID(id, "subscription")
	if err != nil {
		return nil, err
	}
	existing, err := r.SubscriptionSvc.Get(ctx, sid) // channel gate - a scoped admin must not reach another channel's subscription by id
	if err != nil {
		return nil, err
	}
	if err := auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "subscriptions.manage", existing.ChannelID); err != nil {
		return nil, err
	}
	sub, err := r.SubscriptionSvc.Pause(ctx, sid)
	if err != nil {
		return nil, err
	}
	return toCustomerSubscriptionModel(sub, nil), nil
}

func (r *mutationResolver) ResumeSubscription(ctx context.Context, id string) (*model.CustomerSubscription, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "subscriptions.manage"); err != nil {
		return nil, err
	}
	sid, err := parseID(id, "subscription")
	if err != nil {
		return nil, err
	}
	existing, err := r.SubscriptionSvc.Get(ctx, sid) // channel gate - a scoped admin must not reach another channel's subscription by id
	if err != nil {
		return nil, err
	}
	if err := auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "subscriptions.manage", existing.ChannelID); err != nil {
		return nil, err
	}
	sub, err := r.SubscriptionSvc.Resume(ctx, sid)
	if err != nil {
		return nil, err
	}
	return toCustomerSubscriptionModel(sub, nil), nil
}

func (r *queryResolver) Subscriptions(ctx context.Context, status *model.SubscriptionStatus, limit *int32, offset *int32) ([]*model.CustomerSubscription, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "subscriptions.view"); err != nil {
		return nil, err
	}
	items, _, err := r.subscriptionPage(ctx, status, limit, offset, false)
	return items, err
}

func (r *queryResolver) SubscriptionsPaged(ctx context.Context, status *model.SubscriptionStatus, limit *int32, offset *int32) (*model.SubscriptionPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "subscriptions.view"); err != nil {
		return nil, err
	}
	items, total, err := r.subscriptionPage(ctx, status, limit, offset, true)
	if err != nil {
		return nil, err
	}
	return &model.SubscriptionPage{TotalCount: total, Items: items}, nil
}

func (r *queryResolver) Subscription(ctx context.Context, id string) (*model.CustomerSubscription, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "subscriptions.view"); err != nil {
		return nil, err
	}
	sid, err := parseID(id, "subscription")
	if err != nil {
		return nil, err
	}
	sub, err := r.SubscriptionSvc.Get(ctx, sid)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "subscriptions.view", sub.ChannelID); err != nil {
		return nil, err
	}
	invoices, err := r.SubscriptionSvc.Invoices(ctx, sid)
	if err != nil {
		return nil, err
	}
	return toCustomerSubscriptionModel(sub, invoices), nil
}
