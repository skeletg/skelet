package locale

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	pgdb "github.com/skeletg/skelet/internal/db"
	db "github.com/skeletg/skelet/internal/db/sqlc"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) Create(ctx context.Context, input CreateLocaleInput) (Locale, error) {
	row, err := r.queries.CreateLocale(ctx, db.CreateLocaleParams{
		Lower:                 input.Code,
		Name:                  input.Name,
		NativeName:            input.NativeName,
		IsActive:              input.IsActive,
		IsRtl:                 input.IsRTL,
		SortOrder:             input.SortOrder,
		DateFormat:            input.DateFormat,
		TimeFormat:            input.TimeFormat,
		DecimalSeparator:      input.DecimalSeparator,
		ThousandsSeparator:    input.ThousandsSeparator,
		FirstDayOfWeek:        input.FirstDayOfWeek,
		Upper:                 nullableUpper(input.DefaultCountryCode),
		FlagEmoji:             input.FlagEmoji,
		Lower_2:               input.HreflangValue,
		Script:                input.Script,
		PluralRule:            input.PluralRule,
		EmailFromNameOverride: input.EmailFromNameOverride,
		Description:           input.Description,
		Metadata:              mapToJSON(input.Metadata),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Locale{}, ErrLocaleCodeTaken
		}
		return Locale{}, fmt.Errorf("create locale: %w", err)
	}
	return toLocale(row), nil
}

func (r *postgresRepository) GetByCode(ctx context.Context, code string) (Locale, error) {
	row, err := r.queries.GetLocaleByCode(ctx, code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Locale{}, ErrLocaleNotFound
		}
		return Locale{}, fmt.Errorf("get locale: %w", err)
	}
	return toLocale(row), nil
}

func (r *postgresRepository) List(ctx context.Context, activeOnly bool) ([]Locale, error) {
	if activeOnly {
		rows, err := r.queries.ListActiveLocales(ctx)
		if err != nil {
			return nil, fmt.Errorf("list active locales: %w", err)
		}
		out := make([]Locale, len(rows))
		for i, row := range rows {
			out[i] = toLocale(row)
		}
		return out, nil
	}
	rows, err := r.queries.ListLocales(ctx)
	if err != nil {
		return nil, fmt.Errorf("list locales: %w", err)
	}
	out := make([]Locale, len(rows))
	for i, row := range rows {
		out[i] = toLocale(row)
	}
	return out, nil
}

func (r *postgresRepository) Update(ctx context.Context, input UpdateLocaleInput) (Locale, error) {
	current, err := r.queries.GetLocaleByCode(ctx, input.Code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Locale{}, ErrLocaleNotFound
		}
		return Locale{}, fmt.Errorf("load locale: %w", err)
	}
	params := db.UpdateLocaleParams{
		Lower:                 input.Code,
		Name:                  current.Name,
		NativeName:            current.NativeName,
		IsActive:              current.IsActive,
		IsRtl:                 current.IsRtl,
		SortOrder:             current.SortOrder,
		DateFormat:            current.DateFormat,
		TimeFormat:            current.TimeFormat,
		DecimalSeparator:      current.DecimalSeparator,
		ThousandsSeparator:    current.ThousandsSeparator,
		FirstDayOfWeek:        current.FirstDayOfWeek,
		Upper:                 nullStringValue(current.DefaultCountryCode),
		FlagEmoji:             current.FlagEmoji,
		Lower_2:               current.HreflangValue,
		Script:                current.Script,
		PluralRule:            current.PluralRule,
		EmailFromNameOverride: current.EmailFromNameOverride,
		Description:           current.Description,
		Metadata:              current.Metadata,
	}
	if input.Name != nil {
		params.Name = *input.Name
	}
	if input.NativeName != nil {
		params.NativeName = *input.NativeName
	}
	if input.IsActive != nil {
		params.IsActive = *input.IsActive
	}
	if input.IsRTL != nil {
		params.IsRtl = *input.IsRTL
	}
	if input.SortOrder != nil {
		params.SortOrder = *input.SortOrder
	}
	if input.DateFormat != nil {
		params.DateFormat = *input.DateFormat
	}
	if input.TimeFormat != nil {
		params.TimeFormat = *input.TimeFormat
	}
	if input.DecimalSeparator != nil {
		params.DecimalSeparator = *input.DecimalSeparator
	}
	if input.ThousandsSeparator != nil {
		params.ThousandsSeparator = *input.ThousandsSeparator
	}
	if input.FirstDayOfWeek != nil {
		params.FirstDayOfWeek = *input.FirstDayOfWeek
	}
	if input.DefaultCountryCode != nil {
		params.Upper = nullableUpper(*input.DefaultCountryCode)
	}
	if input.FlagEmoji != nil {
		params.FlagEmoji = *input.FlagEmoji
	}
	if input.HreflangValue != nil {
		params.Lower_2 = *input.HreflangValue
	}
	if input.Script != nil {
		params.Script = *input.Script
	}
	if input.PluralRule != nil {
		params.PluralRule = *input.PluralRule
	}
	if input.EmailFromNameOverride != nil {
		params.EmailFromNameOverride = *input.EmailFromNameOverride
	}
	if input.Description != nil {
		params.Description = *input.Description
	}
	if input.Metadata != nil {
		params.Metadata = mapToJSON(*input.Metadata)
	}
	row, err := r.queries.UpdateLocale(ctx, params)
	if err != nil {
		return Locale{}, fmt.Errorf("update locale: %w", err)
	}
	return toLocale(row), nil
}

func (r *postgresRepository) Delete(ctx context.Context, code string) error {
	if err := r.queries.DeleteLocale(ctx, code); err != nil {
		return fmt.Errorf("delete locale: %w", err)
	}
	return nil
}

func (r *postgresRepository) CountReferences(ctx context.Context, code string) (int64, error) {
	n, err := r.queries.CountLocaleReferences(ctx, code)
	if err != nil {
		return 0, fmt.Errorf("count locale references: %w", err)
	}
	return n, nil
}

func toLocale(row db.Locale) Locale {
	return Locale{
		Code:                  row.Code,
		Name:                  row.Name,
		NativeName:            row.NativeName,
		IsActive:              row.IsActive,
		IsRTL:                 row.IsRtl,
		SortOrder:             row.SortOrder,
		DateFormat:            row.DateFormat,
		TimeFormat:            row.TimeFormat,
		DecimalSeparator:      row.DecimalSeparator,
		ThousandsSeparator:    row.ThousandsSeparator,
		FirstDayOfWeek:        row.FirstDayOfWeek,
		DefaultCountryCode:    nullStringValue(row.DefaultCountryCode),
		FlagEmoji:             row.FlagEmoji,
		HreflangValue:         row.HreflangValue,
		Script:                row.Script,
		PluralRule:            row.PluralRule,
		EmailFromNameOverride: row.EmailFromNameOverride,
		Description:           row.Description,
		Metadata:              jsonToMap(row.Metadata),
		CreatedAt:             row.CreatedAt,
		UpdatedAt:             row.UpdatedAt,
	}
}

// mapToJSON / jsonToMap mirror the helpers in channel - string→string
// maps round-tripped through the JSONB metadata column
func mapToJSON(m map[string]string) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage(`{}`)
	}
	out, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return out
}

func jsonToMap(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return map[string]string{}
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]string{}
	}
	return out
}

// nullableUpper converts an empty string to nil so the SQL's UPPER($N) gets NULL (not ”), which lets the FK on
// default_country_code accept "no country set." Non-empty values pass through
func nullableUpper(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullStringValue extracts the underlying string from sql.NullString or
// returns "" when the column was NULL
func nullStringValue(n sql.NullString) string {
	if n.Valid {
		return n.String
	}
	return ""
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching
