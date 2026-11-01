package consent

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

// NewRepository returns a postgres-backed Repository
func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) UpsertTx(ctx context.Context, sqlTx *sql.Tx, c Change) (StoredConsent, error) {
	return upsert(ctx, r.queries.WithTx(sqlTx), c)
}

func (r *postgresRepository) Upsert(ctx context.Context, c Change) (StoredConsent, error) {
	return upsert(ctx, r.queries, c)
}

func upsert(ctx context.Context, q *db.Queries, c Change) (StoredConsent, error) {
	row, err := q.UpsertCustomerConsent(ctx, db.UpsertCustomerConsentParams{
		CustomerID: c.CustomerID,
		Kind:       c.Kind,
		OptedIn:    c.OptedIn,
		Source:     c.Source,
		AdminID:    sqlnull.UUIDNonZero(c.AdminID),
	})
	if err != nil {
		return StoredConsent{}, fmt.Errorf("upsert consent: %w", err)
	}
	return StoredConsent{
		CustomerID: row.CustomerID,
		Kind:       row.Kind,
		OptedIn:    row.OptedIn,
		Source:     row.Source,
		UpdatedAt:  row.UpdatedAt,
	}, nil
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

func (r *postgresRepository) ListEvents(ctx context.Context, customerID uuid.UUID, limit int32) ([]Event, error) {
	rows, err := r.queries.ListCustomerConsentEvents(ctx, db.ListCustomerConsentEventsParams{CustomerID: customerID, Lim: limit})
	if err != nil {
		return nil, fmt.Errorf("list consent history: %w", err)
	}
	out := make([]Event, len(rows))
	for i, row := range rows {
		out[i] = Event{
			ID: row.ID, CustomerID: row.CustomerID, Kind: row.Kind, OptedIn: row.OptedIn,
			Source: row.Source, AdminID: sqlnull.UUIDPtr(row.AdminID), CreatedAt: row.CreatedAt,
		}
	}
	return out, nil
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

func (r *postgresRepository) RecordEmailOptIn(ctx context.Context, o EmailOptIn) error {
	if err := r.queries.RecordEmailOptIn(ctx, db.RecordEmailOptInParams{
		ChannelID: o.ChannelID, Email: o.Email, Kind: o.Kind, Source: o.Source, Reference: o.Reference,
	}); err != nil {
		return fmt.Errorf("record email opt-in: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListEmailOptIns(ctx context.Context, channelID uuid.UUID, email string) ([]EmailOptIn, error) {
	rows, err := r.queries.ListEmailOptIns(ctx, db.ListEmailOptInsParams{ChannelID: channelID, Email: email})
	if err != nil {
		return nil, fmt.Errorf("list email opt-ins: %w", err)
	}
	out := make([]EmailOptIn, len(rows))
	for i, row := range rows {
		out[i] = EmailOptIn{ChannelID: row.ChannelID, Email: row.Email, Kind: row.Kind, Source: row.Source, Reference: row.Reference, CreatedAt: row.CreatedAt}
	}
	return out, nil
}
