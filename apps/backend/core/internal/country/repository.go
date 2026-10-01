package country

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) Create(ctx context.Context, input CreateCountryInput) (Country, error) {
	row, err := r.queries.CreateCountry(ctx, db.CreateCountryParams{
		Upper:      input.Code,
		Name:       input.Name,
		NativeName: input.NativeName,
		EmojiFlag:  input.EmojiFlag,
		PhoneCode:  input.PhoneCode,
		Upper_2:    input.DefaultCurrencyCode, // NULLIF('', '') in SQL
		IsActive:   input.IsActive,
		SortOrder:  input.SortOrder,
		Metadata:   mapToJSON(input.Metadata),
	})
	if err != nil {
		if pgdb.IsUniqueViolation(err) {
			return Country{}, ErrCountryCodeTaken
		}
		return Country{}, fmt.Errorf("create country: %w", err)
	}
	return toCountry(row), nil
}

func (r *postgresRepository) GetByCode(ctx context.Context, code string) (Country, error) {
	row, err := r.queries.GetCountryByCode(ctx, code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Country{}, ErrCountryNotFound
		}
		return Country{}, fmt.Errorf("get country: %w", err)
	}
	return toCountry(row), nil
}

func (r *postgresRepository) List(ctx context.Context, activeOnly bool) ([]Country, error) {
	if activeOnly {
		rows, err := r.queries.ListActiveCountries(ctx)
		if err != nil {
			return nil, fmt.Errorf("list active countries: %w", err)
		}
		out := make([]Country, len(rows))
		for i, row := range rows {
			out[i] = toCountry(row)
		}
		return out, nil
	}
	rows, err := r.queries.ListCountries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list countries: %w", err)
	}
	out := make([]Country, len(rows))
	for i, row := range rows {
		out[i] = toCountry(row)
	}
	return out, nil
}

func (r *postgresRepository) Update(ctx context.Context, input UpdateCountryInput) (Country, error) {
	current, err := r.queries.GetCountryByCode(ctx, input.Code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Country{}, ErrCountryNotFound
		}
		return Country{}, fmt.Errorf("load country: %w", err)
	}
	currentDefault := ""
	if current.DefaultCurrencyCode.Valid {
		currentDefault = current.DefaultCurrencyCode.String
	}
	params := db.UpdateCountryParams{
		Upper:      input.Code,
		Name:       current.Name,
		NativeName: current.NativeName,
		EmojiFlag:  current.EmojiFlag,
		PhoneCode:  current.PhoneCode,
		Upper_2:    currentDefault,
		IsActive:   current.IsActive,
		SortOrder:  current.SortOrder,
		Metadata:   current.Metadata,
	}
	if input.Name != nil {
		params.Name = *input.Name
	}
	if input.NativeName != nil {
		params.NativeName = *input.NativeName
	}
	if input.EmojiFlag != nil {
		params.EmojiFlag = *input.EmojiFlag
	}
	if input.PhoneCode != nil {
		params.PhoneCode = *input.PhoneCode
	}
	if input.DefaultCurrencyCode != nil {
		params.Upper_2 = *input.DefaultCurrencyCode
	}
	if input.IsActive != nil {
		params.IsActive = *input.IsActive
	}
	if input.SortOrder != nil {
		params.SortOrder = *input.SortOrder
	}
	if input.Metadata != nil {
		params.Metadata = mapToJSON(*input.Metadata)
	}
	row, err := r.queries.UpdateCountry(ctx, params)
	if err != nil {
		return Country{}, fmt.Errorf("update country: %w", err)
	}
	return toCountry(row), nil
}

func (r *postgresRepository) Delete(ctx context.Context, code string) error {
	if err := r.queries.DeleteCountry(ctx, code); err != nil {
		if pgdb.IsForeignKeyViolation(err) {
			return ErrCountryInUse // a reference landed after the count
		}
		return fmt.Errorf("delete country: %w", err)
	}
	return nil
}

func (r *postgresRepository) CountReferences(ctx context.Context, code string) (int64, error) {
	n, err := r.queries.CountCountryReferences(ctx, code)
	if err != nil {
		return 0, fmt.Errorf("count country references: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) ListRegions(ctx context.Context, countryCode string) ([]Region, error) {
	rows, err := r.queries.ListRegionsByCountry(ctx, countryCode)
	if err != nil {
		return nil, fmt.Errorf("list regions: %w", err)
	}
	out := make([]Region, len(rows))
	for i, row := range rows {
		out[i] = Region{
			CountryCode: row.CountryCode,
			Code:        row.Code,
			Name:        row.Name,
			IsActive:    row.IsActive,
			SortOrder:   row.SortOrder,
		}
	}
	return out, nil
}

func toCountry(row db.Country) Country {
	c := Country{
		Code:        row.Code,
		Name:        row.Name,
		NativeName:  row.NativeName,
		EmojiFlag:   row.EmojiFlag,
		PhoneCode:   callingCodeFor(row.Code, row.PhoneCode), // derived on read too, so a row stored before the rule reads right
		Iso3Code:    row.Iso3Code,
		NumericCode: row.NumericCode,
		IsActive:    row.IsActive,
		SortOrder:   row.SortOrder,
		Metadata:    jsonToMap(row.Metadata),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if row.DefaultCurrencyCode.Valid {
		c.DefaultCurrencyCode = row.DefaultCurrencyCode.String
	}
	return c
}

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
