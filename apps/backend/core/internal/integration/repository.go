package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *txrunner.Manager
}

// Create and Update write the row and its grant set in one transaction - the rest are single statements
func NewRepository(queries *db.Queries, txMgr *txrunner.Manager) Repository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

// A missing row is ErrNotFound, anything else stays the database's error so an outage never reads as "not found"
func notFound(err error, op string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}

func (r *postgresRepository) Create(ctx context.Context, input CreateIntegrationInput, tokenHash string) (Integration, error) {
	var created Integration
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		row, err := q.CreateIntegration(ctx, db.CreateIntegrationParams{
			Name:        input.Name,
			Description: input.Description,
			TokenHash:   tokenHash,
			ExpiresAt:   input.ExpiresAt,
			ChannelID:   sqlnull.UUID(input.ChannelID),
		})
		if err != nil {
			return fmt.Errorf("create integration: %w", err)
		}
		created = toIntegration(row)
		if len(input.Permissions) > 0 {
			if err := setPermissionsTx(ctx, q, created.ID, input.Permissions); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Integration{}, err
	}
	created.Permissions, err = r.GetPermissions(ctx, created.ID)
	if err != nil {
		return Integration{}, err
	}
	return created, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Integration, error) {
	row, err := r.queries.GetIntegrationByID(ctx, id)
	if err != nil {
		return Integration{}, notFound(err, "get integration")
	}
	i := toIntegration(row)
	i.Permissions, err = r.GetPermissions(ctx, id)
	if err != nil {
		return Integration{}, err
	}
	return i, nil
}

func (r *postgresRepository) GetByHash(ctx context.Context, tokenHash string) (Integration, error) {
	row, err := r.queries.GetIntegrationByHash(ctx, tokenHash)
	if err != nil {
		return Integration{}, notFound(err, "get integration by token")
	}
	i := toIntegration(row.Integration)
	i.Permissions = make([]Permission, len(row.PermissionIds))
	for n, id := range row.PermissionIds {
		i.Permissions[n] = Permission{ID: id, Slug: row.PermissionSlugs[n], Name: row.PermissionNames[n]}
	}
	return i, nil
}

func (r *postgresRepository) List(ctx context.Context) ([]Integration, error) {
	rows, err := r.queries.ListIntegrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}
	perms, err := r.queries.ListAllIntegrationPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list integration permissions: %w", err)
	}
	byIntegration := make(map[uuid.UUID][]Permission, len(rows))
	for _, p := range perms {
		byIntegration[p.IntegrationID] = append(byIntegration[p.IntegrationID], Permission{ID: p.ID, Slug: p.Slug, Name: p.Name})
	}
	out := make([]Integration, len(rows))
	for i, row := range rows {
		out[i] = toIntegration(row)
		out[i].Permissions = byIntegration[row.ID]
	}
	return out, nil
}

func (r *postgresRepository) Update(ctx context.Context, input UpdateIntegrationInput) (Integration, error) {
	var updated Integration
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		current, err := q.GetIntegrationByIDForUpdate(ctx, input.ID)
		if err != nil {
			return notFound(err, "get integration")
		}

		name := current.Name
		if input.Name != nil {
			name = *input.Name
		}
		description := current.Description
		if input.Description != nil {
			description = *input.Description
		}

		// clearExpiresAt wins over a provided expiresAt value
		var expiresAt sql.NullTime
		switch {
		case input.ClearExpiresAt:
			expiresAt = sql.NullTime{Valid: false}
		case input.ExpiresAt != nil:
			expiresAt = sql.NullTime{Time: *input.ExpiresAt, Valid: true}
		default:
			expiresAt = current.ExpiresAt
		}

		channelID := current.ChannelID // db row - already uuid.NullUUID
		if input.SetChannel {
			channelID = sqlnull.UUID(input.ChannelID)
		}

		row, err := q.UpdateIntegration(ctx, db.UpdateIntegrationParams{
			ID:          input.ID,
			Name:        name,
			Description: description,
			ExpiresAt:   expiresAt,
			ChannelID:   channelID,
		})
		if err != nil {
			return fmt.Errorf("update integration: %w", err)
		}
		updated = toIntegration(row)

		if input.PermissionSlugs != nil {
			if err := setPermissionsTx(ctx, q, input.ID, input.PermissionSlugs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Integration{}, err
	}
	updated.Permissions, err = r.GetPermissions(ctx, input.ID)
	if err != nil {
		return Integration{}, err
	}
	return updated, nil
}

func (r *postgresRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) (Integration, error) {
	row, err := r.queries.SetIntegrationActive(ctx, db.SetIntegrationActiveParams{
		ID:       id,
		IsActive: active,
	})
	if err != nil {
		return Integration{}, notFound(err, "set integration active")
	}
	i := toIntegration(row)
	i.Permissions, err = r.GetPermissions(ctx, id)
	if err != nil {
		return Integration{}, err
	}
	return i, nil
}

func (r *postgresRepository) RegenerateTokenHash(ctx context.Context, id uuid.UUID, newHash string) (Integration, error) {
	row, err := r.queries.UpdateIntegrationTokenHash(ctx, db.UpdateIntegrationTokenHashParams{
		ID:        id,
		TokenHash: newHash,
	})
	if err != nil {
		return Integration{}, notFound(err, "regenerate token")
	}
	i := toIntegration(row)
	i.Permissions, err = r.GetPermissions(ctx, id)
	if err != nil {
		return Integration{}, err
	}
	return i, nil
}

func (r *postgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := r.queries.DeleteIntegration(ctx, id)
	if err != nil {
		return fmt.Errorf("delete integration: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *postgresRepository) GetPermissions(ctx context.Context, integrationID uuid.UUID) ([]Permission, error) {
	rows, err := r.queries.GetIntegrationPermissions(ctx, integrationID)
	if err != nil {
		return nil, fmt.Errorf("get integration permissions: %w", err)
	}
	perms := make([]Permission, len(rows))
	for i, row := range rows {
		perms[i] = Permission{ID: row.ID, Slug: row.Slug, Name: row.Name}
	}
	return perms, nil
}

// setPermissionsTx is the tx-local delete-then-insert used by Create and Update - It takes an already-bound *db.Queries
// so the caller controls the transaction boundary
func setPermissionsTx(ctx context.Context, q *db.Queries, integrationID uuid.UUID, slugs []string) error {
	if err := q.DeleteIntegrationPermissions(ctx, integrationID); err != nil {
		return fmt.Errorf("clear integration permissions: %w", err)
	}
	for _, slug := range slugs {
		perm, err := q.GetPermissionBySlug(ctx, slug)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrUnknownPermission, slug)
		}
		if err != nil {
			return fmt.Errorf("get permission %s: %w", slug, err)
		}
		if err := q.AddIntegrationPermission(ctx, db.AddIntegrationPermissionParams{
			IntegrationID: integrationID,
			PermissionID:  perm.ID,
		}); err != nil {
			return fmt.Errorf("add integration permission: %w", err)
		}
	}
	return nil
}

func toIntegration(row db.Integration) Integration {
	i := Integration{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		IsActive:    row.IsActive,
		CreatedAt:   row.CreatedAt,
		ExpiresAt:   row.ExpiresAt,
	}
	if row.ChannelID.Valid {
		id := row.ChannelID.UUID
		i.ChannelID = &id
	}
	return i
}
