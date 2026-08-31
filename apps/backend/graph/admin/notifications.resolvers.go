package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

func (r *mutationResolver) MarkNotificationRead(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "notifications.view"); err != nil {
		return false, err
	}
	claims, _ := auth.AdminFromContext(ctx)
	nid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.NotificationSvc.MarkRead(ctx, claims.AdminID, nid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) MarkAllNotificationsRead(ctx context.Context) (bool, error) {
	sc, err := r.notifScope(ctx) // resolved ONCE at subscribe time - grants changing mid-connection only bite on reconnect
	if err != nil {
		return false, err
	}
	if err := r.NotificationSvc.MarkAllRead(ctx, sc); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) Notifications(ctx context.Context, unreadOnly *bool, limit *int32) ([]*model.AdminNotification, error) {
	sc, err := r.notifScope(ctx)
	if err != nil {
		return nil, err
	}
	uo := unreadOnly != nil && *unreadOnly
	var lim int32
	if limit != nil {
		lim = *limit
	}
	items, err := r.NotificationSvc.List(ctx, sc, uo, lim)
	if err != nil {
		return nil, err
	}
	out := make([]*model.AdminNotification, 0, len(items))
	for _, n := range items {
		out = append(out, notifToModel(n))
	}
	return out, nil
}

func (r *queryResolver) NotificationUnreadCount(ctx context.Context) (int32, error) {
	sc, err := r.notifScope(ctx)
	if err != nil {
		return 0, err
	}
	c, err := r.NotificationSvc.UnreadCount(ctx, sc)
	if err != nil {
		return 0, err
	}
	return int32(c), nil
}

func (r *subscriptionResolver) NotificationAdded(ctx context.Context) (<-chan *model.AdminNotification, error) {
	sc, err := r.notifScope(ctx)
	if err != nil {
		return nil, err
	}
	if r.NotificationHub == nil {
		return nil, fmt.Errorf("live notifications unavailable")
	}
	src, cancel := r.NotificationHub.Subscribe() // drop-on-slow, so this goroutine can never back-pressure producers - the dashboard's poll backfills any gap
	out := make(chan *model.AdminNotification, 8)
	go func() {
		defer cancel()
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case n, ok := <-src:
				if !ok {
					return
				}
				if !sc.CanSee(n) { // same rules the inbox SQL applies
					continue
				}
				select {
				case out <- notifToModel(n):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func (r *Resolver) Subscription() SubscriptionResolver { return &subscriptionResolver{r} }

type subscriptionResolver struct{ *Resolver }
