package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateCurrency(ctx context.Context, input model.CreateCurrencyInput) (*model.Currency, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "currencies.manage"); err != nil {
		return nil, err
	}
	c, err := r.CurrencySvc.Create(ctx, currency.CreateCurrencyInput{
		Code:           input.Code,
		Name:           input.Name,
		Symbol:         stringOr(input.Symbol, ""),
		Decimals:       int16(int32Or(input.Decimals, 2)),
		SymbolPosition: currency.SymbolPosition(stringOr(input.SymbolPosition, "")),
		IsActive:       boolOrDefault(input.IsActive, true),
		SortOrder:      int32Or(input.SortOrder, 0),
		Metadata:       metadataInputToStringMap(input.Metadata),
	})
	if err != nil {
		return nil, err
	}
	return toCurrencyModel(c), nil
}

func (r *mutationResolver) UpdateCurrency(ctx context.Context, input model.UpdateCurrencyInput) (*model.Currency, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "currencies.manage"); err != nil {
		return nil, err
	}
	domainInput := currency.UpdateCurrencyInput{
		Code:     input.Code,
		Name:     input.Name,
		Symbol:   input.Symbol,
		IsActive: input.IsActive,
	}
	if input.Decimals != nil {
		v := int16(*input.Decimals)
		domainInput.Decimals = &v
	}
	if input.SymbolPosition != nil {
		v := currency.SymbolPosition(*input.SymbolPosition)
		domainInput.SymbolPosition = &v
	}
	if input.SortOrder != nil {
		v := int32(*input.SortOrder)
		domainInput.SortOrder = &v
	}
	if input.Metadata != nil {
		m := metadataInputToStringMap(input.Metadata)
		domainInput.Metadata = &m
	}
	c, err := r.CurrencySvc.Update(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	if input.ExchangeRate != nil {
		if err := r.CurrencySvc.SetRate(ctx, c.Code, *input.ExchangeRate); err != nil {
			return nil, err
		}
		if c, err = r.CurrencySvc.GetByCode(ctx, c.Code); err != nil {
			return nil, err
		}
	}
	return toCurrencyModel(c), nil
}

func (r *mutationResolver) DeleteCurrency(ctx context.Context, code string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "currencies.manage"); err != nil {
		return false, err
	}
	if err := r.CurrencySvc.Delete(ctx, code); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) Currencies(ctx context.Context, activeOnly *bool) ([]*model.Currency, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "currencies.view"); err != nil {
		return nil, err
	}
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
	if err := auth.RequirePermission(ctx, r.AdminSvc, "currencies.view"); err != nil {
		return nil, err
	}
	c, err := r.CurrencySvc.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return toCurrencyModel(c), nil
}
