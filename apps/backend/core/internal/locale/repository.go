package locale

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sqlc-dev/pqtype"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) Create(ctx context.Context, input CreateLocaleInput) (Locale, error) {
	firstDay := int16(1)
	if input.FirstDayOfWeek != nil {
		firstDay = *input.FirstDayOfWeek
	}
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
		FirstDayOfWeek:        firstDay,
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
	list := r.queries.ListLocales
	if activeOnly {
		list = r.queries.ListActiveLocales
	}
	rows, err := list(ctx)
	if err != nil {
		return nil, fmt.Errorf("list locales: %w", err)
	}
	out := make([]Locale, len(rows))
	for i, row := range rows {
		out[i] = toLocale(row)
	}
	return out, nil
}

func (r *postgresRepository) Update(ctx context.Context, input UpdateLocaleInput) (Locale, bool, error) {
	params := db.UpdateLocaleParams{
		Code:                  input.Code,
		Name:                  sqlnull.String(input.Name),
		NativeName:            sqlnull.String(input.NativeName),
		IsActive:              sqlnull.Bool(input.IsActive),
		IsRtl:                 sqlnull.Bool(input.IsRTL),
		DateFormat:            sqlnull.String(input.DateFormat),
		TimeFormat:            sqlnull.String(input.TimeFormat),
		DecimalSeparator:      sqlnull.String(input.DecimalSeparator),
		ThousandsSeparator:    sqlnull.String(input.ThousandsSeparator),
		SetCountry:            input.DefaultCountryCode != nil,
		DefaultCountryCode:    sqlnull.String(input.DefaultCountryCode),
		FlagEmoji:             sqlnull.String(input.FlagEmoji),
		HreflangValue:         sqlnull.String(input.HreflangValue),
		Script:                sqlnull.String(input.Script),
		PluralRule:            sqlnull.String(input.PluralRule),
		EmailFromNameOverride: sqlnull.String(input.EmailFromNameOverride),
		Description:           sqlnull.String(input.Description),
	}
	if input.SortOrder != nil {
		params.SortOrder = sql.NullInt32{Int32: *input.SortOrder, Valid: true}
	}
	if input.FirstDayOfWeek != nil {
		params.FirstDayOfWeek = sql.NullInt16{Int16: *input.FirstDayOfWeek, Valid: true}
	}
	if input.Metadata != nil {
		params.Metadata = pqtype.NullRawMessage{RawMessage: mapToJSON(*input.Metadata), Valid: true}
	}
	row, err := r.queries.UpdateLocale(ctx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Locale{}, false, ErrLocaleNotFound
		}
		return Locale{}, false, fmt.Errorf("update locale: %w", err)
	}
	return toLocale(row.Locale), row.WasActive, nil
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
		DefaultCountryCode:    sqlnull.StringValue(row.DefaultCountryCode),
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

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching
