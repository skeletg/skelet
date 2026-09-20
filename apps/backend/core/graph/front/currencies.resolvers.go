package front

import (
	"context"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
)

func (r *queryResolver) Currencies(ctx context.Context, activeOnly *bool) ([]*model.Currency, error) {
	rows, err := r.CurrencySvc.List(ctx, boolOrFalse(activeOnly))
	if err != nil {
		return nil, err
	}
	out := make([]*model.Currency, len(rows))
	for i, c := range rows {
		out[i] = toCurrencyModel(c)
	}
	return out, nil
}

func (r *queryResolver) Currency(ctx context.Context, code string) (*model.Currency, error) {
	c, err := r.CurrencySvc.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return toCurrencyModel(c), nil
}

func (r *queryResolver) PresentmentCurrencies(ctx context.Context) ([]*model1.PresentmentCurrency, error) {
	sv, ok := channel.FromContext(ctx)
	if !ok || r.CurrencySvc == nil {
		return []*model1.PresentmentCurrency{}, nil
	}
	base, err := r.CurrencySvc.GetByCode(ctx, sv.CurrencyCode)
	if err != nil {
		return []*model1.PresentmentCurrency{}, nil
	}
	all, err := r.CurrencySvc.List(ctx, true)
	if err != nil {
		return nil, err
	}
	const freshness = 7 * 24 * time.Hour
	out := make([]*model1.PresentmentCurrency, 0, len(all))
	for _, c := range all {
		if c.Code == base.Code || c.RateUpdatedAt == nil || time.Since(*c.RateUpdatedAt) > freshness {
			continue
		}
		// Shared converter at 6-decimal precision, presentedMinor = round(baseMinor * factor) - pairs where either side has no usable rate are skipped
		converted, ok := currency.Convert(1_000_000, base, c)
		if !ok || converted <= 0 {
			continue
		}
		out = append(out, &model1.PresentmentCurrency{
			Code:           c.Code,
			Name:           c.Name,
			Symbol:         c.Symbol,
			Decimals:       int32(c.Decimals),
			SymbolPosition: string(c.SymbolPosition),
			FactorFromBase: float64(converted) / 1_000_000,
			RateUpdatedAt:  *c.RateUpdatedAt,
		})
	}
	return out, nil
}
