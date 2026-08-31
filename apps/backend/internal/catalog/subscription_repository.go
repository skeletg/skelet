package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

func (r *postgresRepository) GetSubscriptionTerms(ctx context.Context, productID uuid.UUID) (SubscriptionTerms, error) {
	row, err := r.queries.GetSubscriptionTerms(ctx, productID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SubscriptionTerms{}, ErrSubscriptionTermsNotFound
		}
		return SubscriptionTerms{}, fmt.Errorf("get subscription terms: %w", err)
	}
	return toSubscriptionTerms(row), nil
}

func (r *postgresRepository) UpsertSubscriptionTerms(ctx context.Context, input SetSubscriptionTermsInput) (SubscriptionTerms, error) {
	row, err := r.queries.UpsertSubscriptionTerms(ctx, db.UpsertSubscriptionTermsParams{
		ProductID:     input.ProductID,
		IntervalUnit:  input.IntervalUnit,
		IntervalCount: input.IntervalCount,
		TrialDays:     input.TrialDays,
	})
	if err != nil {
		return SubscriptionTerms{}, fmt.Errorf("upsert subscription terms: %w", err)
	}
	return toSubscriptionTerms(row), nil
}

func (r *postgresRepository) DeleteSubscriptionTerms(ctx context.Context, productID uuid.UUID) error {
	if err := r.queries.DeleteSubscriptionTerms(ctx, productID); err != nil {
		return fmt.Errorf("delete subscription terms: %w", err)
	}
	return nil
}

func toSubscriptionTerms(row db.CatalogSubscriptionTerm) SubscriptionTerms {
	return SubscriptionTerms{
		ProductID:     row.ProductID,
		IntervalUnit:  row.IntervalUnit,
		IntervalCount: row.IntervalCount,
		TrialDays:     row.TrialDays,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
