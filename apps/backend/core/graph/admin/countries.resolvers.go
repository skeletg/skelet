package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Regions is the resolver for the regions field. Lazy - only the
// country detail page asks for it, so the countries list pays nothing.
func (r *countryResolver) Regions(ctx context.Context, obj *model.Country) ([]*model.Region, error) {
	regions, err := r.CountrySvc.Regions(ctx, obj.Code)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Region, len(regions))
	for i, rg := range regions {
		out[i] = &model.Region{
			Code:      rg.Code,
			Name:      rg.Name,
			IsActive:  rg.IsActive,
			SortOrder: rg.SortOrder,
		}
	}
	return out, nil
}

func (r *mutationResolver) CreateCountry(ctx context.Context, input model.CreateCountryInput) (*model.Country, error) {
	if err := r.requireGlobal(ctx, "countries.manage"); err != nil {
		return nil, err
	}
	c, err := r.CountrySvc.Create(ctx, country.CreateCountryInput{
		Code:                input.Code,
		Name:                input.Name,
		NativeName:          stringOr(input.NativeName, ""),
		EmojiFlag:           stringOr(input.EmojiFlag, ""),
		PhoneCode:           stringOr(input.PhoneCode, ""),
		DefaultCurrencyCode: stringOr(input.DefaultCurrencyCode, ""),
		IsActive:            boolOrDefault(input.IsActive, true),
		SortOrder:           int32Or(input.SortOrder, 0),
		Metadata:            metadataInputToStringMap(input.Metadata),
		BusinessIDFields:    businessIDFieldInputs(input.BusinessIDFields),
	})
	if err != nil {
		return nil, err
	}
	return r.toCountryModel(ctx, c), nil
}

func (r *mutationResolver) UpdateCountry(ctx context.Context, input model.UpdateCountryInput) (*model.Country, error) {
	if err := r.requireGlobal(ctx, "countries.manage"); err != nil {
		return nil, err
	}
	domainInput := country.UpdateCountryInput{
		Code:                input.Code,
		Name:                input.Name,
		NativeName:          input.NativeName,
		EmojiFlag:           input.EmojiFlag,
		PhoneCode:           input.PhoneCode,
		DefaultCurrencyCode: input.DefaultCurrencyCode,
		IsActive:            input.IsActive,
	}
	if input.SortOrder != nil {
		v := int32(*input.SortOrder)
		domainInput.SortOrder = &v
	}
	if input.Metadata != nil {
		m := metadataInputToStringMap(input.Metadata)
		domainInput.Metadata = &m
	}
	if input.BusinessIDFields != nil {
		f := businessIDFieldInputs(input.BusinessIDFields)
		domainInput.BusinessIDFields = &f
	}
	c, err := r.CountrySvc.Update(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return r.toCountryModel(ctx, c), nil
}

func (r *mutationResolver) DeleteCountry(ctx context.Context, code string) (bool, error) {
	if err := r.requireGlobal(ctx, "countries.manage"); err != nil {
		return false, err
	}
	if err := r.CountrySvc.Delete(ctx, code); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) Countries(ctx context.Context, activeOnly *bool) ([]*model.Country, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "countries.view"); err != nil {
		return nil, err
	}
	rows, err := r.CountrySvc.List(ctx, boolOrFalse(activeOnly))
	if err != nil {
		return nil, err
	}
	out := make([]*model.Country, len(rows))
	for i, c := range rows {
		out[i] = r.toCountryModel(ctx, c)
	}
	return out, nil
}

func (r *queryResolver) Country(ctx context.Context, code string) (*model.Country, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "countries.view"); err != nil {
		return nil, err
	}
	c, err := r.CountrySvc.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return r.toCountryModel(ctx, c), nil
}

func (r *Resolver) Country() CountryResolver { return &countryResolver{r} }

type countryResolver struct{ *Resolver }
