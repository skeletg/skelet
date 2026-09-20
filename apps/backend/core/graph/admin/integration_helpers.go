package admin

import (
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
)

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
