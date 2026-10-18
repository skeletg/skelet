package admin

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// The public URL decides which Host routes to a channel, so changing it takes the global grant - nil when absent or unchanged, so an unchanged value is never written
func (r *Resolver) publicURLChange(ctx context.Context, id uuid.UUID, in *string) (*string, error) {
	if in == nil {
		return nil, nil
	}
	current, err := r.ChannelSvc.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(*in) == current.PublicURL {
		return nil, nil
	}
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, channel.PermManage); err != nil {
		return nil, err
	}
	return in, nil
}

func toChannelModel(sv channel.Channel) *model.Channel {
	locales := make([]*model.Locale, len(sv.Locales))
	for i, l := range sv.Locales {
		locales[i] = toLocaleModel(l)
	}
	return &model.Channel{
		ID:            sv.ID.String(),
		Slug:          sv.Slug,
		Name:          sv.Name,
		DefaultLocale: sv.DefaultLocale,
		Locales:       locales,
		IsActive:      sv.IsActive,
		IsDefault:     sv.IsDefault,
		CurrencyCode:  sv.CurrencyCode,
		PublicURL:     sv.PublicURL,
		Metadata:      toStringMapItems(sv.Metadata),
		CreatedAt:     sv.CreatedAt,
	}
}
