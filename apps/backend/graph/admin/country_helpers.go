package admin

import (
	"context"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/country"
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
