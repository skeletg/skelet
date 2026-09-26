// Package configstore is core's settings store - the config_values table behind config.Repository
package configstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *txrunner.Manager
}

// New returns a postgres-backed config.Repository - the txrunner.Manager is for parity, config writes are single statements
func New(queries *db.Queries, txMgr *txrunner.Manager) config.Repository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

func (r *postgresRepository) Upsert(ctx context.Context, scope config.Scope, path, value string) (config.StoredValue, error) {
	row, err := r.queries.UpsertConfigValue(ctx, db.UpsertConfigValueParams{
		Scope:     scope.Kind,
		ChannelID: sqlnull.UUID(scope.ChannelID),
		Locale:    sqlnull.String(scope.Locale),
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
		Locale:    sqlnull.String(scope.Locale),
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
		Locale:    sqlnull.String(scope.Locale),
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get config value: %w", err)
	}
	out := make([]config.StoredValue, 0, len(rows))
	for i := range rows {
		out = append(out, toStoredValue(rows[i]))
	}
	return out, nil
}

func (r *postgresRepository) ListInScopes(ctx context.Context, scope config.Scope) ([]config.StoredValue, error) {
	rows, err := r.queries.ListConfigValuesInScopes(ctx, db.ListConfigValuesInScopesParams{
		Scope:     scope.Kind,
		ChannelID: sqlnull.UUID(scope.ChannelID),
		Locale:    sqlnull.String(scope.Locale),
	})
	if err != nil {
		return nil, fmt.Errorf("list config values: %w", err)
	}
	out := make([]config.StoredValue, len(rows))
	for i, row := range rows {
		out[i] = toStoredValue(row)
	}
	return out, nil
}

func (r *postgresRepository) ListAll(ctx context.Context) ([]config.StoredValue, error) {
	rows, err := r.queries.ListAllConfigValues(ctx)
	if err != nil {
		return nil, fmt.Errorf("list config values: %w", err)
	}
	out := make([]config.StoredValue, len(rows))
	for i, row := range rows {
		out[i] = toStoredValue(row)
	}
	return out, nil
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
