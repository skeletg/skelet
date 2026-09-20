package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateShippingZone(ctx context.Context, input model.CreateShippingZoneInput) (*model.ShippingZone, error) {
	channelID, err := uuid.Parse(input.ChannelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	// Zones are per-channel, so the gate is the channel being written to
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.manage", channelID, nil); err != nil {
		return nil, err
	}
	zone, err := r.ShippingSvc.Repo().CreateZone(ctx, shipping.Zone{
		ChannelID: channelID,
		Code:      input.Code,
		Name:      ptr.Deref(input.Name),
		SortOrder: intDeref(input.SortOrder, 0),
		IsActive:  boolDeref(input.IsActive, true),
	})
	if err != nil {
		return nil, err
	}
	if len(input.Countries) > 0 {
		if err := r.ShippingSvc.ReplaceZoneCountries(ctx, zone.ID, input.Countries); err != nil {
			return nil, err
		}
	}
	return r.hydrateZoneModel(ctx, zone.ID)
}

func (r *mutationResolver) UpdateShippingZone(ctx context.Context, input model.UpdateShippingZoneInput) (*model.ShippingZone, error) {
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	existing, err := r.requireZoneScope(ctx, id, "settings.manage")
	if err != nil {
		return nil, err
	}
	if input.Code != nil {
		existing.Code = *input.Code
	}
	if input.Name != nil {
		existing.Name = *input.Name
	}
	if input.SortOrder != nil {
		existing.SortOrder = *input.SortOrder
	}
	if input.IsActive != nil {
		existing.IsActive = *input.IsActive
	}
	if _, err := r.ShippingSvc.Repo().UpdateZone(ctx, existing); err != nil {
		return nil, err
	}
	if input.Countries != nil {
		if err := r.ShippingSvc.ReplaceZoneCountries(ctx, id, input.Countries); err != nil {
			return nil, err
		}
	}
	return r.hydrateZoneModel(ctx, id)
}

func (r *mutationResolver) DeleteShippingZone(ctx context.Context, id string) (bool, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if _, err := r.requireZoneScope(ctx, uid, "settings.manage"); err != nil {
		return false, err
	}
	if err := r.ShippingSvc.Repo().DeleteZone(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreateShippingMethod(ctx context.Context, input model.CreateShippingMethodInput) (*model.ShippingMethod, error) {
	zoneID, err := uuid.Parse(input.ZoneID)
	if err != nil {
		return nil, errInvalidID("zoneId", err)
	}
	// A method has no channel of its own - it inherits the zone's
	if _, err := r.requireZoneScope(ctx, zoneID, "settings.manage"); err != nil {
		return nil, err
	}
	method, err := r.ShippingSvc.Repo().CreateMethod(ctx, shipping.Method{
		ZoneID:         zoneID,
		Code:           input.Code,
		Name:           ptr.Deref(input.Name),
		CalcType:       shippingCalcTypeFromModel(input.CalcType),
		FlatCents:      int64(intDeref(input.FlatCents, 0)),
		FreeAboveCents: int64(intDeref(input.FreeAboveCents, 0)),
		SortOrder:      intDeref(input.SortOrder, 0),
		IsActive:       boolDeref(input.IsActive, true),
		CountryMode:    ptr.Deref(input.CountryMode),
	})
	if err != nil {
		return nil, err
	}
	if input.AllowedCountries != nil {
		if err := r.ShippingSvc.ReplaceMethodCountries(ctx, method.ID, input.AllowedCountries); err != nil {
			return nil, err
		}
		method.AllowedCountries = input.AllowedCountries
	}
	return toShippingMethodModel(method), nil
}

func (r *mutationResolver) UpdateShippingMethod(ctx context.Context, input model.UpdateShippingMethodInput) (*model.ShippingMethod, error) {
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	existing, err := r.requireMethodScope(ctx, id, "settings.manage")
	if err != nil {
		return nil, err
	}
	if input.Code != nil {
		existing.Code = *input.Code
	}
	if input.Name != nil {
		existing.Name = *input.Name
	}
	if input.CalcType != nil {
		existing.CalcType = shippingCalcTypeFromModel(*input.CalcType)
	}
	if input.FlatCents != nil {
		existing.FlatCents = int64(*input.FlatCents)
	}
	if input.FreeAboveCents != nil {
		existing.FreeAboveCents = int64(*input.FreeAboveCents)
	}
	if input.SortOrder != nil {
		existing.SortOrder = *input.SortOrder
	}
	if input.IsActive != nil {
		existing.IsActive = *input.IsActive
	}
	if input.CountryMode != nil {
		existing.CountryMode = *input.CountryMode
	}
	updated, err := r.ShippingSvc.Repo().UpdateMethod(ctx, existing)
	if err != nil {
		return nil, err
	}
	if input.AllowedCountries != nil {
		if err := r.ShippingSvc.ReplaceMethodCountries(ctx, updated.ID, input.AllowedCountries); err != nil {
			return nil, err
		}
		updated.AllowedCountries = input.AllowedCountries
	}
	return toShippingMethodModel(updated), nil
}

func (r *mutationResolver) DeleteShippingMethod(ctx context.Context, id string) (bool, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if _, err := r.requireMethodScope(ctx, uid, "settings.manage"); err != nil {
		return false, err
	}
	if err := r.ShippingSvc.Repo().DeleteMethod(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UpsertShippingBox(ctx context.Context, input model.ShippingBoxInput) (*model.ShippingBox, error) {
	channelID, err := uuid.Parse(input.ChannelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.manage", channelID, nil); err != nil {
		return nil, err
	}
	box, err := r.ShippingSvc.UpsertBox(ctx, shipping.Box{
		ChannelID:      channelID,
		Code:           input.Code,
		Name:           ptr.Deref(input.Name),
		InnerLengthMm:  input.InnerLengthMm,
		InnerWidthMm:   input.InnerWidthMm,
		InnerHeightMm:  input.InnerHeightMm,
		MaxWeightGrams: intDeref(input.MaxWeightGrams, 0),
		BoxWeightGrams: intDeref(input.BoxWeightGrams, 0),
		IsActive:       boolDeref(input.IsActive, true),
		SortOrder:      intDeref(input.SortOrder, 0),
	})
	if err != nil {
		return nil, err
	}
	return toShippingBoxModel(box), nil
}

func (r *mutationResolver) DeleteShippingBox(ctx context.Context, id string) (bool, error) {
	boxID, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	box, err := r.ShippingSvc.GetBox(ctx, boxID)
	if err != nil {
		return false, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.manage", box.ChannelID, nil); err != nil {
		return false, err
	}
	if err := r.ShippingSvc.DeleteBox(ctx, boxID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetShippingCarrierEnabled(ctx context.Context, channelID string, carrierSlug string, enabled bool) (*model.ShippingCarrierAdmin, error) {
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, err
	}
	// Carrier settings are stored per channel - gate on the one being changed
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.manage", chID, nil); err != nil {
		return nil, err
	}
	if _, err := r.ShippingSvc.SetCarrierEnabled(ctx, chID, carrierSlug, enabled); err != nil {
		return nil, err
	}
	return r.shippingCarrierModel(ctx, chID, carrierSlug)
}

func (r *mutationResolver) SetShippingCarrierCountries(ctx context.Context, channelID string, carrierSlug string, mode string, countries []string) (*model.ShippingCarrierAdmin, error) {
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, err
	}
	// Carrier settings are stored per channel - gate on the one being changed
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.manage", chID, nil); err != nil {
		return nil, err
	}
	if err := r.ShippingSvc.SetCarrierCountries(ctx, chID, carrierSlug, mode, countries); err != nil {
		return nil, err
	}
	return r.shippingCarrierModel(ctx, chID, carrierSlug)
}

func (r *queryResolver) ShippingZones(ctx context.Context, channelID string) ([]*model.ShippingZone, error) {
	sid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.view", sid, nil); err != nil {
		return nil, err
	}
	zones, err := r.ShippingSvc.Repo().ListZonesByChannel(ctx, sid)
	if err != nil {
		return nil, err
	}
	zones, err = r.ShippingSvc.HydrateZoneCountries(ctx, zones)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ShippingZone, len(zones))
	for i := range zones {
		methods, err := r.ShippingSvc.Repo().ListMethodsByZone(ctx, zones[i].ID)
		if err != nil {
			return nil, err
		}
		out[i] = toShippingZoneModel(zones[i], methods)
	}
	return out, nil
}

func (r *queryResolver) ShippingZone(ctx context.Context, id string) (*model.ShippingZone, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	// Load-then-gate: a scoped admin asking by id for another channel's zone gets FORBIDDEN, not a silent empty
	if _, err := r.requireZoneScope(ctx, uid, "settings.view"); err != nil {
		return nil, err
	}
	return r.hydrateZoneModel(ctx, uid)
}

func (r *queryResolver) ShippingCarriers(ctx context.Context, channelID string) ([]*model.ShippingCarrierAdmin, error) {
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.view", chID, nil); err != nil {
		return nil, err
	}
	carriers, err := r.ShippingSvc.ListCarriersForChannel(ctx, chID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ShippingCarrierAdmin, len(carriers))
	for i, c := range carriers {
		out[i] = toShippingCarrierAdminModel(c)
	}
	return out, nil
}

func (r *queryResolver) ShippingBoxes(ctx context.Context, channelID string) ([]*model.ShippingBox, error) {
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "settings.manage", chID, nil); err != nil {
		return nil, err
	}
	boxes, err := r.ShippingSvc.ListBoxes(ctx, chID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ShippingBox, len(boxes))
	for i, b := range boxes {
		out[i] = toShippingBoxModel(b)
	}
	return out, nil
}
