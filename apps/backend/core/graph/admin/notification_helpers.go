package admin

// Lives here rather than notifications.resolvers.go - gqlgen moves helper methods into a /* */ block when it regenerates a resolver file

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/notification"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *Resolver) notifScope(ctx context.Context) (notification.Scope, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "notifications.view"); err != nil {
		return notification.Scope{}, err
	}
	// The inbox is a person's - read state is kept per admin, so a token has nothing to read it as
	claims, err := auth.RequireAdmin(ctx)
	if err != nil {
		return notification.Scope{}, err
	}
	pc, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "notifications.view")
	if err != nil {
		return notification.Scope{}, err
	}
	sc := notification.Scope{AdminID: claims.AdminID, Bypass: claims.IsSuperuser}
	if !sc.Bypass { // each category shows on the channels where both it and the inbox are held
		sc.PermChannels = map[string][]uuid.UUID{}
		for _, slug := range notification.CategoryPermissions {
			held, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, slug)
			if err != nil {
				return notification.Scope{}, err
			}
			if !held.Bypass() && len(held.IDs()) == 0 {
				continue
			}
			sc.Perms = append(sc.Perms, slug)
			switch {
			case held.Bypass() && pc.Bypass():
				sc.GlobalPerms = append(sc.GlobalPerms, slug)
			case held.Bypass():
				sc.PermChannels[slug] = pc.IDs()
			default:
				for _, id := range held.IDs() {
					if pc.Allows(id) {
						sc.PermChannels[slug] = append(sc.PermChannels[slug], id)
					}
				}
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
