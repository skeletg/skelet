package signalsboot

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// The suite's settings store, in its own database - its processes read these on hot paths and can reach no other pool
// Locale never reaches here: these settings scope to the platform or to a channel, nothing deeper

type settingsRepository struct{ queries *signalsdb.Queries }

func NewSettingsRepository(queries *signalsdb.Queries) config.Repository {
	return &settingsRepository{queries: queries}
}

func (r *settingsRepository) Upsert(ctx context.Context, scope config.Scope, path, value string) (config.StoredValue, error) {
	row, err := r.queries.UpsertSignalsConfigValue(ctx, signalsdb.UpsertSignalsConfigValueParams{
		Scope: scope.Kind, ChannelID: nullableUUID(scope.ChannelID), Path: path, Value: value,
	})
	if err != nil {
		return config.StoredValue{}, err
	}
	return settingsStoredValue(row), nil
}

func (r *settingsRepository) Delete(ctx context.Context, scope config.Scope, path string) error {
	return r.queries.DeleteSignalsConfigValue(ctx, signalsdb.DeleteSignalsConfigValueParams{
		Scope: scope.Kind, ChannelID: nullableUUID(scope.ChannelID), Path: path,
	})
}

func (r *settingsRepository) GetCandidates(ctx context.Context, scope config.Scope, path string) ([]config.StoredValue, error) {
	rows, err := r.queries.SignalsConfigCandidates(ctx, signalsdb.SignalsConfigCandidatesParams{
		Path: path, Scope: scope.Kind, ChannelID: nullableUUID(scope.ChannelID),
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return settingsStoredValues(rows), nil
}

func (r *settingsRepository) ListInScopes(ctx context.Context, scope config.Scope) ([]config.StoredValue, error) {
	rows, err := r.queries.ListSignalsConfigValuesInScopes(ctx, signalsdb.ListSignalsConfigValuesInScopesParams{
		Scope: scope.Kind, ChannelID: nullableUUID(scope.ChannelID),
	})
	if err != nil {
		return nil, err
	}
	return settingsStoredValues(rows), nil
}

func (r *settingsRepository) ListAll(ctx context.Context) ([]config.StoredValue, error) {
	rows, err := r.queries.ListAllSignalsConfigValues(ctx)
	if err != nil {
		return nil, err
	}
	return settingsStoredValues(rows), nil
}

func settingsStoredValue(row signalsdb.ConfigValue) config.StoredValue {
	sv := config.StoredValue{
		Scope:     config.Scope{Kind: row.Scope},
		Path:      row.Path,
		Value:     row.Value,
		UpdatedAt: row.UpdatedAt,
	}
	if row.ChannelID.Valid {
		id := row.ChannelID.UUID
		sv.Scope.ChannelID = &id
	}
	return sv
}

func settingsStoredValues(rows []signalsdb.ConfigValue) []config.StoredValue {
	out := make([]config.StoredValue, 0, len(rows))
	for _, row := range rows {
		out = append(out, settingsStoredValue(row))
	}
	return out
}

func nullableUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}
