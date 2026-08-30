package front

import (
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/channel"
)

func toFrontChannelModel(sv channel.Channel) *model.Channel { // identity fields only - branding/contact/legal/timezone moved to the config registry, reachable from a `configValue` query rather than here
	// Hydrated by the channel middleware's GetByID, so this costs no extra query - the storefront
	// needs them to know which languages it may route and advertise
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
		CurrencyCode:  sv.CurrencyCode,
		PublicURL:     sv.PublicURL,
	}
}

func toChannelModel(sv channel.Channel) *model.Channel { // the admin shape, for ProductChannelListing.Channel and similar nested references - identity-only for the same reason
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
		CurrencyCode:  sv.CurrencyCode,
		PublicURL:     sv.PublicURL,
		Metadata:      toStringMapItems(sv.Metadata),
		CreatedAt:     sv.CreatedAt,
	}
}
