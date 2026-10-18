package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func integrationGrants(slugs []string, channelID *uuid.UUID) []auth.Grant {
	out := make([]auth.Grant, len(slugs))
	for i, s := range slugs {
		out[i] = auth.Grant{Slug: s, ChannelID: channelID}
	}
	return out
}

func existingIntegrationGrants(i integration.Integration) []auth.Grant {
	slugs := make([]string, len(i.Permissions))
	for j, p := range i.Permissions {
		slugs[j] = p.Slug
	}
	return integrationGrants(slugs, i.ChannelID)
}

// A token is only the caller's to touch when everything it can do, they can too
func (r *Resolver) requireIntegrationWithinOwn(ctx context.Context, id uuid.UUID) (integration.Integration, error) {
	i, err := r.IntegrationSvc.Get(ctx, id)
	if err != nil {
		return integration.Integration{}, err
	}
	return i, r.requireGrantable(ctx, existingIntegrationGrants(i))
}

func toIntegrationModel(i integration.Integration) *model.Integration {
	perms := make([]*model.Permission, len(i.Permissions))
	for j, p := range i.Permissions {
		perms[j] = &model.Permission{ID: p.ID.String(), Name: p.Name, Slug: p.Slug, Description: permissionDescription(p.Slug)}
	}
	m := &model.Integration{
		ID:          i.ID.String(),
		Name:        i.Name,
		Description: i.Description,
		IsActive:    i.IsActive,
		CreatedAt:   i.CreatedAt,
		Permissions: perms,
	}
	if i.ExpiresAt.Valid {
		m.ExpiresAt = &i.ExpiresAt.Time
	}
	if i.ChannelID != nil {
		id := i.ChannelID.String()
		m.ChannelID = &id
	}
	return m
}
