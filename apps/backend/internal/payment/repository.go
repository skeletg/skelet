package payment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) WithTx(t *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(t)}
}

func (r *postgresRepository) UpsertPaymentMethod(ctx context.Context, channelID uuid.UUID, providerSlug, displayName string) (PaymentMethod, error) {
	row, err := r.queries.UpsertPaymentMethod(ctx, db.UpsertPaymentMethodParams{
		ChannelID:    channelID,
		ProviderSlug: providerSlug,
		DisplayName:  displayName,
	})
	if err != nil {
		return PaymentMethod{}, fmt.Errorf("upsert payment method: %w", err)
	}
	return toPaymentMethod(row), nil
}

func (r *postgresRepository) GetPaymentMethod(ctx context.Context, channelID uuid.UUID, providerSlug string) (PaymentMethod, error) {
	row, err := r.queries.GetPaymentMethod(ctx, db.GetPaymentMethodParams{
		ChannelID:    channelID,
		ProviderSlug: providerSlug,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PaymentMethod{}, ErrProviderNotInstalled
		}
		return PaymentMethod{}, fmt.Errorf("get payment method: %w", err)
	}
	return toPaymentMethod(row), nil
}

func (r *postgresRepository) ListPaymentMethodsByChannel(ctx context.Context, channelID uuid.UUID) ([]PaymentMethod, error) {
	rows, err := r.queries.ListPaymentMethodsByChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list payment methods by store: %w", err)
	}
	out := make([]PaymentMethod, len(rows))
	for i, row := range rows {
		out[i] = toPaymentMethod(row)
	}
	return out, nil
}

func (r *postgresRepository) ListEnabledPaymentMethodsByChannel(ctx context.Context, channelID uuid.UUID) ([]PaymentMethod, error) {
	rows, err := r.queries.ListEnabledPaymentMethodsByChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list enabled payment methods: %w", err)
	}
	out := make([]PaymentMethod, len(rows))
	for i, row := range rows {
		out[i] = toPaymentMethod(row)
	}
	return out, nil
}

func (r *postgresRepository) ListPaymentMethodsByProvider(ctx context.Context, providerSlug string) ([]PaymentMethod, error) {
	rows, err := r.queries.ListPaymentMethodsByProvider(ctx, providerSlug)
	if err != nil {
		return nil, fmt.Errorf("list payment methods by provider: %w", err)
	}
	out := make([]PaymentMethod, len(rows))
	for i, row := range rows {
		out[i] = toPaymentMethod(row)
	}
	return out, nil
}

func (r *postgresRepository) SetPaymentMethodEnabled(ctx context.Context, channelID uuid.UUID, providerSlug string, enabled bool) (PaymentMethod, error) {
	row, err := r.queries.SetPaymentMethodEnabled(ctx, db.SetPaymentMethodEnabledParams{
		ChannelID:    channelID,
		ProviderSlug: providerSlug,
		IsEnabled:    enabled,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PaymentMethod{}, ErrProviderNotInstalled
		}
		return PaymentMethod{}, fmt.Errorf("set payment method enabled: %w", err)
	}
	return toPaymentMethod(row), nil
}

func (r *postgresRepository) SetPaymentMethodSortOrder(ctx context.Context, channelID uuid.UUID, providerSlug string, sortOrder int32) (PaymentMethod, error) {
	row, err := r.queries.SetPaymentMethodSortOrder(ctx, db.SetPaymentMethodSortOrderParams{
		ChannelID:    channelID,
		ProviderSlug: providerSlug,
		SortOrder:    sortOrder,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PaymentMethod{}, ErrProviderNotInstalled
		}
		return PaymentMethod{}, fmt.Errorf("set payment method sort order: %w", err)
	}
	return toPaymentMethod(row), nil
}

func (r *postgresRepository) SetPaymentMethodCountryMode(ctx context.Context, channelID uuid.UUID, providerSlug, mode string) (PaymentMethod, error) {
	row, err := r.queries.SetPaymentMethodCountryMode(ctx, db.SetPaymentMethodCountryModeParams{
		ChannelID:    channelID,
		ProviderSlug: providerSlug,
		CountryMode:  normalizePaymentMode(mode),
	})
	if err != nil {
		return PaymentMethod{}, fmt.Errorf("set payment method country mode: %w", err)
	}
	return toPaymentMethod(row), nil
}

func (r *postgresRepository) AddCountryToPaymentMethod(ctx context.Context, methodID uuid.UUID, country string) error {
	if err := r.queries.AddCountryToPaymentMethod(ctx, db.AddCountryToPaymentMethodParams{
		PaymentMethodID: methodID,
		CountryCode:     country,
	}); err != nil {
		return fmt.Errorf("add country to payment method: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListCountriesForPaymentMethod(ctx context.Context, methodID uuid.UUID) ([]string, error) {
	rows, err := r.queries.ListCountriesForPaymentMethod(ctx, methodID)
	if err != nil {
		return nil, fmt.Errorf("list countries for payment method: %w", err)
	}
	return rows, nil
}

func (r *postgresRepository) ClearPaymentMethodCountries(ctx context.Context, methodID uuid.UUID) error {
	if err := r.queries.ClearPaymentMethodCountries(ctx, methodID); err != nil {
		return fmt.Errorf("clear payment method countries: %w", err)
	}
	return nil
}

// normalizePaymentMode coerces empty/unknown to "all" for the CHECK
func normalizePaymentMode(mode string) string {
	if mode == "specific" {
		return "specific"
	}
	return "all"
}

func toPaymentMethod(row db.PaymentMethod) PaymentMethod {
	return PaymentMethod{
		ID:           row.ID,
		ChannelID:    row.ChannelID,
		ProviderSlug: row.ProviderSlug,
		IsEnabled:    row.IsEnabled,
		DisplayName:  row.DisplayName,
		SortOrder:    row.SortOrder,
		CountryMode:  row.CountryMode,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}
