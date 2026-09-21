package admin

import (
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
)

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
