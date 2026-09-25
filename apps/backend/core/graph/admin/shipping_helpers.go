package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Lives here rather than shipping.resolvers.go - gqlgen codegen moves helpers into a "WARNING" comment block on regen

func (r *Resolver) hydrateZoneModel(ctx context.Context, zoneID uuid.UUID) (*model.ShippingZone, error) {
	zone, err := r.ShippingSvc.Repo().GetZone(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	countries, err := r.ShippingSvc.Repo().ListCountriesForZone(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	zone.Countries = countries
	methods, err := r.ShippingSvc.Repo().ListMethodsByZone(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	if methods, err = r.ShippingSvc.HydrateMethodCountries(ctx, methods); err != nil {
		return nil, err
	}
	return toShippingZoneModel(zone, methods), nil
}

func toShippingZoneModel(z shipping.Zone, methods []shipping.Method) *model.ShippingZone {
	mm := make([]*model.ShippingMethod, len(methods))
	for i := range methods {
		mm[i] = toShippingMethodModel(methods[i])
	}
	return &model.ShippingZone{
		ID:        z.ID.String(),
		ChannelID: z.ChannelID.String(),
		Code:      z.Code,
		Name:      z.Name,
		SortOrder: z.SortOrder,
		IsActive:  z.IsActive,
		Countries: z.Countries,
		Methods:   mm,
		CreatedAt: z.CreatedAt,
		UpdatedAt: z.UpdatedAt,
	}
}

func toShippingMethodModel(m shipping.Method) *model.ShippingMethod {
	return &model.ShippingMethod{
		ID:               m.ID.String(),
		ZoneID:           m.ZoneID.String(),
		Code:             m.Code,
		Name:             m.Name,
		CalcType:         shippingCalcTypeToModel(m.CalcType),
		FlatCents:        int32(m.FlatCents),
		FreeAboveCents:   int32(m.FreeAboveCents),
		SortOrder:        m.SortOrder,
		IsActive:         m.IsActive,
		CountryMode:      m.CountryMode,
		AllowedCountries: append([]string{}, m.AllowedCountries...),
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}

func shippingCalcTypeFromModel(t model.ShippingCalcType) shipping.CalcType {
	switch t {
	case model.ShippingCalcTypeFree:
		return shipping.CalcFree
	case model.ShippingCalcTypeFreeAbove:
		return shipping.CalcFreeAbove
	default:
		return shipping.CalcFlat
	}
}

func shippingCalcTypeToModel(t shipping.CalcType) model.ShippingCalcType {
	switch t {
	case shipping.CalcFree:
		return model.ShippingCalcTypeFree
	case shipping.CalcFreeAbove:
		return model.ShippingCalcTypeFreeAbove
	default:
		return model.ShippingCalcTypeFlat
	}
}

func toShippingBoxModel(b shipping.Box) *model.ShippingBox {
	return &model.ShippingBox{
		ID:             b.ID.String(),
		ChannelID:      b.ChannelID.String(),
		Code:           b.Code,
		Name:           b.Name,
		InnerLengthMm:  b.InnerLengthMm,
		InnerWidthMm:   b.InnerWidthMm,
		InnerHeightMm:  b.InnerHeightMm,
		MaxWeightGrams: b.MaxWeightGrams,
		BoxWeightGrams: b.BoxWeightGrams,
		IsActive:       b.IsActive,
		SortOrder:      b.SortOrder,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}
}

// Shipping rows are per-channel - a zone carries the channel, a method inherits its zone's, and boxes and carrier settings take it as an argument
// An unscoped settings.manage / settings.view here lets an admin scoped to one channel rewrite every other channel's shipping
func (r *Resolver) requireZoneScope(ctx context.Context, zoneID uuid.UUID, slug string) (shipping.Zone, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return shipping.Zone{}, err
	}
	zone, err := r.ShippingSvc.Repo().GetZone(ctx, zoneID)
	if err != nil {
		return shipping.Zone{}, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, slug, zone.ChannelID, nil); err != nil {
		return shipping.Zone{}, err
	}
	return zone, nil
}

func (r *Resolver) requireMethodScope(ctx context.Context, methodID uuid.UUID, slug string) (shipping.Method, error) { // a method's channel is its zone's
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return shipping.Method{}, err
	}
	method, err := r.ShippingSvc.Repo().GetMethod(ctx, methodID)
	if err != nil {
		return shipping.Method{}, err
	}
	if _, err := r.requireZoneScope(ctx, method.ZoneID, slug); err != nil {
		return shipping.Method{}, err
	}
	return method, nil
}
