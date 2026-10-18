package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
)

func (r *mutationResolver) shippingCarrierModel(ctx context.Context, channelID uuid.UUID, slug string) (*model.ShippingCarrierAdmin, error) { // re-reads after a mutation so the row reflects what was stored
	c, err := r.ShippingSvc.CarrierForChannel(ctx, channelID, slug)
	if err != nil {
		return nil, err
	}
	return toShippingCarrierAdminModel(c), nil
}

func toShippingCarrierAdminModel(c shipping.CarrierWithConfig) *model.ShippingCarrierAdmin { // a carrier with no config row reads as disabled, mode "all"
	mode := c.Config.CountryMode
	if mode == "" {
		mode = "all"
	}
	countries := c.Config.AllowedCountries
	if countries == nil {
		countries = []string{}
	}
	return &model.ShippingCarrierAdmin{
		CarrierSlug:        c.Slug,
		DisplayName:        c.DisplayName,
		IsEnabled:          c.Config.IsEnabled,
		SortOrder:          c.Config.SortOrder,
		CountryMode:        mode,
		AllowedCountries:   countries,
		SupportsTracking:   c.SupportsTrack,
		SupportsLabels:     c.SupportsLabel,
		DeliversOnWeekends: c.Config.DeliversOnWeekends,
	}
}
