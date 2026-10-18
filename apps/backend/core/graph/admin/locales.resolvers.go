package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateLocale(ctx context.Context, input model.CreateLocaleInput) (*model.Locale, error) {
	if err := r.requireGlobal(ctx, "locales.manage"); err != nil {
		return nil, err
	}
	domainInput := locale.CreateLocaleInput{
		Code:                  input.Code,
		Name:                  input.Name,
		NativeName:            stringOr(input.NativeName, ""),
		IsActive:              boolOrDefault(input.IsActive, true),
		IsRTL:                 boolOrFalse(input.IsRtl),
		SortOrder:             int32Or(input.SortOrder, 0),
		DateFormat:            stringOr(input.DateFormat, ""),
		TimeFormat:            stringOr(input.TimeFormat, ""),
		DecimalSeparator:      stringOr(input.DecimalSeparator, ""),
		ThousandsSeparator:    stringOr(input.ThousandsSeparator, ""),
		FirstDayOfWeek:        int16Ptr(input.FirstDayOfWeek),
		DefaultCountryCode:    stringOr(input.DefaultCountryCode, ""),
		FlagEmoji:             stringOr(input.FlagEmoji, ""),
		HreflangValue:         stringOr(input.HreflangValue, ""),
		Script:                stringOr(input.Script, ""),
		PluralRule:            stringOr(input.PluralRule, ""),
		EmailFromNameOverride: stringOr(input.EmailFromNameOverride, ""),
		Description:           stringOr(input.Description, ""),
		Metadata:              metadataInputToStringMap(input.Metadata),
	}
	l, err := r.LocaleSvc.Create(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toLocaleModel(l), nil
}

func (r *mutationResolver) UpdateLocale(ctx context.Context, input model.UpdateLocaleInput) (*model.Locale, error) {
	if err := r.requireGlobal(ctx, "locales.manage"); err != nil {
		return nil, err
	}
	domainInput := locale.UpdateLocaleInput{
		Code:                  input.Code,
		Name:                  input.Name,
		NativeName:            input.NativeName,
		IsActive:              input.IsActive,
		IsRTL:                 input.IsRtl,
		DateFormat:            input.DateFormat,
		TimeFormat:            input.TimeFormat,
		DecimalSeparator:      input.DecimalSeparator,
		ThousandsSeparator:    input.ThousandsSeparator,
		DefaultCountryCode:    input.DefaultCountryCode,
		FlagEmoji:             input.FlagEmoji,
		HreflangValue:         input.HreflangValue,
		Script:                input.Script,
		PluralRule:            input.PluralRule,
		EmailFromNameOverride: input.EmailFromNameOverride,
		Description:           input.Description,
	}
	if input.SortOrder != nil {
		v := int32(*input.SortOrder)
		domainInput.SortOrder = &v
	}
	if input.FirstDayOfWeek != nil {
		v := int16(*input.FirstDayOfWeek)
		domainInput.FirstDayOfWeek = &v
	}
	if input.Metadata != nil {
		m := metadataInputToStringMap(input.Metadata)
		domainInput.Metadata = &m
	}
	l, err := r.LocaleSvc.Update(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toLocaleModel(l), nil
}

func (r *mutationResolver) DeleteLocale(ctx context.Context, code string) (bool, error) {
	if err := r.requireGlobal(ctx, "locales.manage"); err != nil {
		return false, err
	}
	if err := r.LocaleSvc.Delete(ctx, code); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) Locales(ctx context.Context, activeOnly *bool) ([]*model.Locale, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "locales.view"); err != nil {
		return nil, err
	}
	rows, err := r.LocaleSvc.List(ctx, boolOrFalse(activeOnly))
	if err != nil {
		return nil, err
	}
	out := make([]*model.Locale, len(rows))
	for i, l := range rows {
		out[i] = toLocaleModel(l)
	}
	return out, nil
}

func (r *queryResolver) Locale(ctx context.Context, code string) (*model.Locale, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "locales.view"); err != nil {
		return nil, err
	}
	l, err := r.LocaleSvc.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return toLocaleModel(l), nil
}
