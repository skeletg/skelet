// Package configstore is core's settings store - the config_values table behind config.Repository
package configstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type postgresRepository struct {
	queries *db.Queries
}

// New returns a postgres-backed config.Repository - every write is a single statement, so it needs no transaction manager
func New(queries *db.Queries) config.Repository {
	return &postgresRepository{queries: queries}
}

// Locale codes are lowercase in the locales table the column references, so "EN" and "en" land on the same row
func locale(scope config.Scope) sql.NullString {
	if scope.Locale == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.ToLower(*scope.Locale), Valid: true}
}

func (r *postgresRepository) Upsert(ctx context.Context, scope config.Scope, path, value string) (config.StoredValue, error) {
	row, err := r.queries.UpsertConfigValue(ctx, db.UpsertConfigValueParams{
		Scope:     scope.Kind,
		ChannelID: sqlnull.UUID(scope.ChannelID),
		Locale:    locale(scope),
		Path:      path,
		Value:     value,
	})
	if err != nil {
		return config.StoredValue{}, fmt.Errorf("upsert config value: %w", err)
	}
	return toStoredValue(row), nil
}

func (r *postgresRepository) Delete(ctx context.Context, scope config.Scope, path string) error {
	if err := r.queries.DeleteConfigValue(ctx, db.DeleteConfigValueParams{
		Scope:     scope.Kind,
		ChannelID: sqlnull.UUID(scope.ChannelID),
		Locale:    locale(scope),
		Path:      path,
	}); err != nil {
		return fmt.Errorf("delete config value: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetCandidates(ctx context.Context, scope config.Scope, path string) ([]config.StoredValue, error) {
	rows, err := r.queries.GetConfigValueWithFallback(ctx, db.GetConfigValueWithFallbackParams{
		Path:      path,
		Scope:     scope.Kind,
		ChannelID: sqlnull.UUID(scope.ChannelID),
		Locale:    locale(scope),
	})
	if err != nil {
		return nil, fmt.Errorf("get config value: %w", err)
	}
	return toStoredValues(rows), nil
}

func (r *postgresRepository) ListInScopes(ctx context.Context, scope config.Scope) ([]config.StoredValue, error) {
	rows, err := r.queries.ListConfigValuesInScopes(ctx, db.ListConfigValuesInScopesParams{
		Scope:     scope.Kind,
		ChannelID: sqlnull.UUID(scope.ChannelID),
		Locale:    locale(scope),
	})
	if err != nil {
		return nil, fmt.Errorf("list config values: %w", err)
	}
	return toStoredValues(rows), nil
}

func (r *postgresRepository) ListAll(ctx context.Context) ([]config.StoredValue, error) {
	rows, err := r.queries.ListAllConfigValues(ctx)
	if err != nil {
		return nil, fmt.Errorf("list config values: %w", err)
	}
	return toStoredValues(rows), nil
}

func toStoredValues(rows []db.ConfigValue) []config.StoredValue {
	out := make([]config.StoredValue, len(rows))
	for i, row := range rows {
		out[i] = toStoredValue(row)
	}
	return out
}

func toStoredValue(row db.ConfigValue) config.StoredValue {
	return config.StoredValue{
		Scope: config.Scope{
			Kind:      row.Scope,
			ChannelID: sqlnull.UUIDPtr(row.ChannelID),
			Locale:    sqlnull.StringPtr(row.Locale),
		},
		Path:      row.Path,
		Value:     row.Value,
		UpdatedAt: row.UpdatedAt,
	}
}
