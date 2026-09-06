package currency

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	pgdb "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) Create(ctx context.Context, input CreateCurrencyInput) (Currency, error) {
	row, err := r.queries.CreateCurrency(ctx, db.CreateCurrencyParams{
		Upper:          input.Code,
		Name:           input.Name,
		Symbol:         input.Symbol,
		Decimals:       input.Decimals,
		SymbolPosition: string(input.SymbolPosition),
		IsActive:       input.IsActive,
		SortOrder:      input.SortOrder,
		Metadata:       mapToJSON(input.Metadata),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Currency{}, ErrCurrencyCodeTaken
		}
		return Currency{}, fmt.Errorf("create currency: %w", err)
	}
	return toCurrency(row), nil
}

func (r *postgresRepository) GetByCode(ctx context.Context, code string) (Currency, error) {
	row, err := r.queries.GetCurrencyByCode(ctx, code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Currency{}, ErrCurrencyNotFound
		}
		return Currency{}, fmt.Errorf("get currency: %w", err)
	}
	return toCurrency(row), nil
}

func (r *postgresRepository) List(ctx context.Context, activeOnly bool) ([]Currency, error) {
	if activeOnly {
		rows, err := r.queries.ListActiveCurrencies(ctx)
		if err != nil {
			return nil, fmt.Errorf("list active currencies: %w", err)
		}
		out := make([]Currency, len(rows))
		for i, row := range rows {
			out[i] = toCurrency(row)
		}
		return out, nil
	}
	rows, err := r.queries.ListCurrencies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list currencies: %w", err)
	}
	out := make([]Currency, len(rows))
	for i, row := range rows {
		out[i] = toCurrency(row)
	}
	return out, nil
}

func (r *postgresRepository) Update(ctx context.Context, input UpdateCurrencyInput) (Currency, error) {
	current, err := r.queries.GetCurrencyByCode(ctx, input.Code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Currency{}, ErrCurrencyNotFound
		}
		return Currency{}, fmt.Errorf("load currency: %w", err)
	}
	params := db.UpdateCurrencyParams{
		Upper:          input.Code,
		Name:           current.Name,
		Symbol:         current.Symbol,
		Decimals:       current.Decimals,
		SymbolPosition: current.SymbolPosition,
		IsActive:       current.IsActive,
		SortOrder:      current.SortOrder,
		Metadata:       current.Metadata,
	}
	if input.Name != nil {
		params.Name = *input.Name
	}
	if input.Symbol != nil {
		params.Symbol = *input.Symbol
	}
	if input.Decimals != nil {
		params.Decimals = *input.Decimals
	}
	if input.SymbolPosition != nil {
		params.SymbolPosition = string(*input.SymbolPosition)
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
	row, err := r.queries.UpdateCurrency(ctx, params)
	if err != nil {
		return Currency{}, fmt.Errorf("update currency: %w", err)
	}
	return toCurrency(row), nil
}

func (r *postgresRepository) Delete(ctx context.Context, code string) error {
	if err := r.queries.DeleteCurrency(ctx, code); err != nil {
		return fmt.Errorf("delete currency: %w", err)
	}
	return nil
}

func (r *postgresRepository) CountReferences(ctx context.Context, code string) (int64, error) {
	n, err := r.queries.CountCurrencyReferences(ctx, code)
	if err != nil {
		return 0, fmt.Errorf("count currency references: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) SetRate(ctx context.Context, code string, rate float64) error {
	if err := r.queries.SetCurrencyRate(ctx, db.SetCurrencyRateParams{
		Code:         code,
		ExchangeRate: sql.NullFloat64{Float64: rate, Valid: true},
	}); err != nil {
		return fmt.Errorf("set currency rate: %w", err)
	}
	return nil
}

func toCurrency(row db.Currency) Currency {
	c := Currency{
		Code:           row.Code,
		Name:           row.Name,
		Symbol:         row.Symbol,
		Decimals:       row.Decimals,
		SymbolPosition: SymbolPosition(row.SymbolPosition),
		IsActive:       row.IsActive,
		SortOrder:      row.SortOrder,
		Metadata:       jsonToMap(row.Metadata),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if row.ExchangeRate.Valid {
		v := row.ExchangeRate.Float64
		c.ExchangeRate = &v
	}
	if row.RateUpdatedAt.Valid {
		v := row.RateUpdatedAt.Time
		c.RateUpdatedAt = &v
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

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching
