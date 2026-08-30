package consent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/internal/db/sqlc"
)

type postgresRepository struct {
	queries *db.Queries
}

// NewRepository returns a postgres-backed Repository
func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) Upsert(ctx context.Context, customerID uuid.UUID, kind string, optedIn bool, source string) (StoredConsent, error) {
	row, err := r.queries.UpsertCustomerConsent(ctx, db.UpsertCustomerConsentParams{
		CustomerID: customerID,
		Kind:       kind,
		OptedIn:    optedIn,
		Source:     source,
	})
	if err != nil {
		return StoredConsent{}, fmt.Errorf("upsert consent: %w", err)
	}
	return toStored(row), nil
}

func (r *postgresRepository) Get(ctx context.Context, customerID uuid.UUID, kind string) (*StoredConsent, error) {
	row, err := r.queries.GetCustomerConsent(ctx, db.GetCustomerConsentParams{
		CustomerID: customerID,
		Kind:       kind,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get consent: %w", err)
	}
	sc := toStored(row)
	return &sc, nil
}

func (r *postgresRepository) ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]StoredConsent, error) {
	rows, err := r.queries.ListCustomerConsents(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("list consents: %w", err)
	}
	out := make([]StoredConsent, len(rows))
	for i, row := range rows {
		out[i] = toStored(row)
	}
	return out, nil
}

func (r *postgresRepository) ListByCustomerIDs(ctx context.Context, customerIDs []uuid.UUID, kind string) ([]StoredConsent, error) {
	if len(customerIDs) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListCustomerConsentsByCustomerIDs(ctx, db.ListCustomerConsentsByCustomerIDsParams{
		Kind:        kind,
		CustomerIds: customerIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("list consents by ids: %w", err)
	}
	out := make([]StoredConsent, len(rows))
	for i, row := range rows {
		out[i] = toStored(row)
	}
	return out, nil
}

func (r *postgresRepository) DeleteByCustomer(ctx context.Context, customerID uuid.UUID) error {
	if err := r.queries.DeleteCustomerConsents(ctx, customerID); err != nil {
		return fmt.Errorf("delete consents: %w", err)
	}
	return nil
}

func toStored(row db.CustomerConsent) StoredConsent {
	return StoredConsent{
		CustomerID: row.CustomerID,
		Kind:       row.Kind,
		OptedIn:    row.OptedIn,
		Source:     row.Source,
		UpdatedAt:  row.UpdatedAt,
	}
}
