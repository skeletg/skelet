package admin

import (
	"context"
	"strings"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/subscription"
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
	statusFilter := ""
	if status != nil {
		statusFilter = strings.ToLower(string(*status))
	}
	subs, err := r.SubscriptionSvc.List(ctx, statusFilter, int32Or(limit, 50), int32Or(offset, 0))
	if err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "subscriptions.view") // scope-filter mirrors the orders/customers lists
	if err != nil {
		return nil, err
	}
	visible := make([]subscription.Subscription, 0, len(subs))
	for _, sub := range subs {
		if perm.Bypass() || (sub.ChannelID != nil && perm.Allows(*sub.ChannelID)) {
			visible = append(visible, sub)
		}
	}
	out := make([]*model.CustomerSubscription, len(visible))
	for i, s := range visible {
		out[i] = toCustomerSubscriptionModel(s, nil)
	}
	return out, nil
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
