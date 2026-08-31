package adminprefs

import (
	"context"
	"database/sql"
	"encoding/json"
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

func (r *postgresRepository) List(ctx context.Context, adminID uuid.UUID) ([]Preference, error) {
	rows, err := r.queries.ListAdminPreferences(ctx, adminID)
	if err != nil {
		return nil, fmt.Errorf("list admin preferences: %w", err)
	}
	out := make([]Preference, len(rows))
	for i, row := range rows {
		out[i] = Preference{Key: row.Key, Value: row.Value, UpdatedAt: row.UpdatedAt}
	}
	return out, nil
}

func (r *postgresRepository) Get(ctx context.Context, adminID uuid.UUID, key string) (json.RawMessage, error) {
	row, err := r.queries.GetAdminPreference(ctx, db.GetAdminPreferenceParams{AdminID: adminID, Key: key})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get admin preference: %w", err)
	}
	return row.Value, nil
}

func (r *postgresRepository) Set(ctx context.Context, adminID uuid.UUID, key string, value json.RawMessage) error {
	if err := r.queries.UpsertAdminPreference(ctx, db.UpsertAdminPreferenceParams{
		AdminID: adminID,
		Key:     key,
		Value:   value,
	}); err != nil {
		return fmt.Errorf("upsert admin preference: %w", err)
	}
	return nil
}

func (r *postgresRepository) Delete(ctx context.Context, adminID uuid.UUID, key string) error {
	if err := r.queries.DeleteAdminPreference(ctx, db.DeleteAdminPreferenceParams{
		AdminID: adminID,
		Key:     key,
	}); err != nil {
		return fmt.Errorf("delete admin preference: %w", err)
	}
	return nil
}
