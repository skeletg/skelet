package download

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) WithTx(tx *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(tx)}
}

func (r *postgresRepository) CreateGrant(ctx context.Context, in GrantInput) (bool, error) {
	n, err := r.queries.CreateDownloadGrant(ctx, db.CreateDownloadGrantParams{
		Token:        in.Token,
		OrderID:      sqlnull.UUID(in.OrderID),
		CustomerID:   sqlnull.UUID(in.CustomerID),
		Email:        in.Email,
		ProductID:    uuid.NullUUID{UUID: in.ProductID, Valid: true},
		DownloadID:   uuid.NullUUID{UUID: in.DownloadID, Valid: true},
		Title:        in.Title,
		Url:          in.URL,
		MaxDownloads: sqlnull.Int32(in.MaxDownloads),
		ExpiresAt:    sqlnull.Time(in.ExpiresAt),
	})
	if err != nil {
		return false, fmt.Errorf("create download grant: %w", err)
	}
	return n == 1, nil
}

func (r *postgresRepository) RevokeForOrder(ctx context.Context, orderID uuid.UUID) (int64, error) {
	n, err := r.queries.RevokeOrderDownloadGrants(ctx, uuid.NullUUID{UUID: orderID, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("revoke download grants: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) GetByToken(ctx context.Context, token string) (Grant, error) {
	row, err := r.queries.GetDownloadGrantByToken(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Grant{}, ErrNotFound
		}
		return Grant{}, fmt.Errorf("get download grant: %w", err)
	}
	return toGrant(row), nil
}

func (r *postgresRepository) GetByTokenForUpdate(ctx context.Context, token string) (Grant, error) {
	row, err := r.queries.GetDownloadGrantByTokenForUpdate(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Grant{}, ErrNotFound
		}
		return Grant{}, fmt.Errorf("get download grant for update: %w", err)
	}
	return toGrant(row), nil
}

func (r *postgresRepository) IncrementUse(ctx context.Context, id uuid.UUID) (Grant, error) {
	row, err := r.queries.IncrementDownloadGrantUse(ctx, id)
	if err != nil {
		return Grant{}, fmt.Errorf("increment download grant use: %w", err)
	}
	return toGrant(row), nil
}

func (r *postgresRepository) ListByOrder(ctx context.Context, orderID uuid.UUID) ([]Grant, error) {
	rows, err := r.queries.ListDownloadGrantsByOrder(ctx, uuid.NullUUID{UUID: orderID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list download grants by order: %w", err)
	}
	return toGrants(rows), nil
}

func (r *postgresRepository) ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]Grant, error) {
	rows, err := r.queries.ListDownloadGrantsByCustomer(ctx, uuid.NullUUID{UUID: customerID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list download grants by customer: %w", err)
	}
	return toGrants(rows), nil
}

func (r *postgresRepository) ShareAsset(ctx context.Context, token string, in ShareInput) (Grant, error) {
	row, err := r.queries.ShareAssetWithCustomer(ctx, db.ShareAssetWithCustomerParams{
		Token:        token,
		CustomerID:   uuid.NullUUID{UUID: in.CustomerID, Valid: true},
		Email:        in.Email,
		AssetID:      uuid.NullUUID{UUID: in.AssetID, Valid: true},
		Title:        in.Title,
		MaxDownloads: sqlnull.Int32(in.MaxDownloads),
		ExpiresAt:    sqlnull.Time(in.ExpiresAt),
	})
	if err != nil {
		return Grant{}, fmt.Errorf("share asset: %w", err)
	}
	return toGrant(row), nil
}

func (r *postgresRepository) ListByAsset(ctx context.Context, assetID uuid.UUID) ([]Grant, error) {
	rows, err := r.queries.ListDownloadGrantsByAsset(ctx, uuid.NullUUID{UUID: assetID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list shares of asset: %w", err)
	}
	return toGrants(rows), nil
}

func (r *postgresRepository) DeleteSharesWith(ctx context.Context, customerID uuid.UUID) error {
	if err := r.queries.DeleteCustomerAssetShares(ctx, uuid.NullUUID{UUID: customerID, Valid: true}); err != nil {
		return fmt.Errorf("delete a customer's shares: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteShare(ctx context.Context, id uuid.UUID) (bool, error) {
	n, err := r.queries.DeleteAssetDownloadGrant(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete share: %w", err)
	}
	return n == 1, nil
}

func toGrants(rows []db.DownloadGrant) []Grant {
	out := make([]Grant, len(rows))
	for i, row := range rows {
		out[i] = toGrant(row)
	}
	return out
}

func toGrant(row db.DownloadGrant) Grant {
	g := Grant{
		ID:            row.ID,
		Token:         row.Token,
		Email:         row.Email,
		ProductID:     row.ProductID.UUID,
		DownloadID:    row.DownloadID.UUID,
		Title:         row.Title,
		URL:           row.Url,
		DownloadsUsed: row.DownloadsUsed,
		CreatedAt:     row.CreatedAt,
	}
	if row.OrderID.Valid {
		id := row.OrderID.UUID
		g.OrderID = &id
	}
	if row.AssetID.Valid {
		id := row.AssetID.UUID
		g.AssetID = &id
	}
	if row.CustomerID.Valid {
		id := row.CustomerID.UUID
		g.CustomerID = &id
	}
	if row.MaxDownloads.Valid {
		v := row.MaxDownloads.Int32
		g.MaxDownloads = &v
	}
	if row.ExpiresAt.Valid {
		t := row.ExpiresAt.Time
		g.ExpiresAt = &t
	}
	return g
}
