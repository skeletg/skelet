package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/internal/db/sqlc"
)

func (r *postgresRepository) CreateKitComponent(ctx context.Context, input AddKitComponentInput) (KitComponent, error) {
	row, err := r.queries.CreateKitComponent(ctx, db.CreateKitComponentParams{
		ProductID:          input.ProductID,
		ComponentProductID: input.ComponentProductID,
		Quantity:           input.Quantity,
		SortOrder:          input.SortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return KitComponent{}, fmt.Errorf("component already in this kit")
		}
		return KitComponent{}, fmt.Errorf("create kit component: %w", err)
	}
	return toKitComponent(row), nil
}

func (r *postgresRepository) GetKitComponentByID(ctx context.Context, id uuid.UUID) (KitComponent, error) {
	row, err := r.queries.GetKitComponentByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return KitComponent{}, ErrKitComponentNotFound
		}
		return KitComponent{}, fmt.Errorf("get kit component: %w", err)
	}
	return toKitComponent(row), nil
}

func (r *postgresRepository) ListKitComponentsByProductID(ctx context.Context, productID uuid.UUID) ([]KitComponent, error) {
	rows, err := r.queries.ListKitComponentsByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list kit components: %w", err)
	}
	out := make([]KitComponent, len(rows))
	for i, row := range rows {
		out[i] = toKitComponent(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateKitComponent(ctx context.Context, id uuid.UUID, quantity float64, sortOrder int32) (KitComponent, error) {
	row, err := r.queries.UpdateKitComponent(ctx, db.UpdateKitComponentParams{
		ID:        id,
		Quantity:  quantity,
		SortOrder: sortOrder,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return KitComponent{}, ErrKitComponentNotFound
		}
		return KitComponent{}, fmt.Errorf("update kit component: %w", err)
	}
	return toKitComponent(row), nil
}

func (r *postgresRepository) DeleteKitComponent(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteKitComponent(ctx, id); err != nil {
		return fmt.Errorf("delete kit component: %w", err)
	}
	return nil
}

func toKitComponent(row db.CatalogProductKitComponent) KitComponent {
	return KitComponent{
		ID:                 row.ID,
		ProductID:          row.ProductID,
		ComponentProductID: row.ComponentProductID,
		Quantity:           row.Quantity,
		SortOrder:          row.SortOrder,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}
