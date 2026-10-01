package front

import (
	"context"
	"sort"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
)

// Honors the channel's "allowed countries" setting, all active when unset, and pins "top destinations" to the front
func (r *queryResolver) Countries(ctx context.Context, activeOnly *bool) ([]*model.Country, error) {
	rows, err := r.CountrySvc.List(ctx, boolOrFalse(activeOnly))
	if err != nil {
		return nil, err
	}
	scope := countryScope(ctx)

	if allowed := r.CountrySvc.AllowedCountries(ctx, scope); allowed != nil {
		filtered := make([]country.Country, 0, len(rows))
		for _, c := range rows {
			if allowed[c.Code] {
				filtered = append(filtered, c)
			}
		}
		rows = filtered
	}

	if top := r.CountrySvc.TopDestinations(ctx, scope); len(top) > 0 {
		rank := make(map[string]int, len(top))
		for i, code := range top {
			rank[code] = i
		}
		sort.SliceStable(rows, func(i, j int) bool {
			ri, iOK := rank[rows[i].Code]
			rj, jOK := rank[rows[j].Code]
			if iOK && jOK {
				return ri < rj
			}
			return iOK && !jOK // pinned destinations sort before the rest
		})
	}

	out := make([]*model.Country, len(rows))
	for i, c := range rows {
		out[i] = r.toCountryModel(ctx, c)
	}
	return out, nil
}

func (r *queryResolver) Country(ctx context.Context, code string) (*model.Country, error) {
	c, err := r.CountrySvc.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return r.toCountryModel(ctx, c), nil
}

// ISO 3166-2 subdivisions - empty when the country has none, and the storefront falls back to a free-text field
func (r *queryResolver) Regions(ctx context.Context, countryCode string) ([]*model.Region, error) {
	regions, err := r.CountrySvc.Regions(ctx, countryCode)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Region, len(regions))
	for i, rg := range regions {
		out[i] = &model.Region{Code: rg.Code, Name: rg.Name, IsActive: rg.IsActive, SortOrder: rg.SortOrder}
	}
	return out, nil
}

func (r *queryResolver) CountrySettings(ctx context.Context) (*model1.CountrySettings, error) {
	scope := countryScope(ctx)
	return &model1.CountrySettings{
		DefaultCountryCode:         r.CountrySvc.DefaultCountry(ctx, scope),
		AllowOptionalState:         r.CountrySvc.AllowOptionalState(ctx, scope),
		RequiredRegionCountryCodes: sortedSet(r.CountrySvc.RequiredRegionCountries(ctx, scope)),
		TopDestinationCountryCodes: r.CountrySvc.TopDestinations(ctx, scope),
		EuCountryCodes:             sortedSet(r.CountrySvc.EUCountries(ctx, scope)),
	}, nil
}

func (r *queryResolver) PhoneSettings(ctx context.Context) (*model1.PhoneSettings, error) {
	settings, err := r.CountrySvc.PhoneSettings(ctx, countryScope(ctx))
	if err != nil {
		return nil, err
	}
	return toPhoneSettingsModel(settings), nil
}
