package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
)

// The currency lookup is best-effort - the FK is ON DELETE SET NULL, so a missing or disabled one is a null relation rather than an error
func (r *Resolver) toCountryModel(ctx context.Context, c country.Country) *model.Country {
	m := &model.Country{
		Code:                c.Code,
		Name:                c.Name,
		NativeName:          c.NativeName,
		EmojiFlag:           c.EmojiFlag,
		PhoneCode:           c.PhoneCode,
		Iso3Code:            c.Iso3Code,
		NumericCode:         c.NumericCode,
		DefaultCurrencyCode: c.DefaultCurrencyCode,
		IsActive:            c.IsActive,
		SortOrder:           int32(c.SortOrder),
		Metadata:            toStringMapItems(c.Metadata),
		BusinessIDFields:    toBusinessIDFieldModels(c.BusinessIDFields),
		CreatedAt:           c.CreatedAt,
		UpdatedAt:           c.UpdatedAt,
	}
	if c.DefaultCurrencyCode != "" {
		if cur, err := r.CurrencySvc.GetByCode(ctx, c.DefaultCurrencyCode); err == nil { // a transient failure leaves DefaultCurrency unset rather than failing the render
			m.DefaultCurrency = toCurrencyModel(cur)
		}
	}
	return m
}

func toBusinessIDFieldModels(fields []country.BusinessIDField) []*model.BusinessIDField {
	out := make([]*model.BusinessIDField, len(fields))
	for i, f := range fields {
		out[i] = &model.BusinessIDField{Key: f.Key, Label: f.Label, Required: f.Required, Pattern: f.Pattern, Hint: f.Hint}
	}
	return out
}

func businessIDFieldInputs(in []*model.BusinessIDFieldInput) []country.BusinessIDField {
	out := make([]country.BusinessIDField, 0, len(in))
	for _, f := range in {
		if f == nil {
			continue
		}
		out = append(out, country.BusinessIDField{
			Key: stringOr(f.Key, ""), Label: f.Label, Required: f.Required,
			Pattern: stringOr(f.Pattern, ""), Hint: stringOr(f.Hint, ""),
		})
	}
	return out
}
