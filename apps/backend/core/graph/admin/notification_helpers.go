package admin

// Lives here rather than notifications.resolvers.go - gqlgen moves helper methods into a /* */ block when it regenerates a resolver file

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/notification"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *Resolver) notifScope(ctx context.Context) (notification.Scope, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "notifications.view"); err != nil {
		return notification.Scope{}, err
	}
	claims, _ := auth.AdminFromContext(ctx)
	pc, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "notifications.view")
	if err != nil {
		return notification.Scope{}, err
	}
	sc := notification.Scope{
		AdminID:  claims.AdminID,
		Bypass:   claims.IsSuperuser || pc.Bypass(),
		Channels: pc.IDs(),
	}
	if !sc.Bypass { // non-bypass admins only see categories whose gating permission they hold
		for _, slug := range notification.CategoryPermissions {
			if ok, _ := r.AdminSvc.HasPermission(ctx, claims.AdminID, slug); ok {
				sc.Perms = append(sc.Perms, slug)
			}
		}
	}
	// Their own mutes, on top of the permission scope - carried on the scope so the list, the badge
	// count, mark-all and the subscription fan-out all apply the same predicate
	if r.PrefsSvc != nil {
		muted, err := r.PrefsSvc.Strings(ctx, claims.AdminID, notification.MutedCategoriesPref)
		if err != nil {
			return notification.Scope{}, err
		}
		sc.Muted = muted
	}
	return sc, nil
}

func notifToModel(n notification.Notification) *model.AdminNotification {
	var ch *string
	if n.ChannelID != nil {
		s := n.ChannelID.String()
		ch = &s
	}
	return &model.AdminNotification{
		ID:         n.ID.String(),
		Category:   n.Category,
		Severity:   n.Severity,
		Title:      n.Title,
		Body:       n.Body,
		EntityType: n.EntityType,
		EntityID:   n.EntityID,
		ActionURL:  n.ActionURL,
		ChannelID:  ch,
		CreatedAt:  n.CreatedAt,
		Read:       n.Read,
	}
}
