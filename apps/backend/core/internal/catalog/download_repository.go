package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

func (r *postgresRepository) CreateProductDownload(ctx context.Context, input AddProductDownloadInput) (ProductDownload, error) {
	row, err := r.queries.CreateProductDownload(ctx, db.CreateProductDownloadParams{
		ProductID:    input.ProductID,
		Title:        input.Title,
		Url:          input.URL,
		MaxDownloads: sqlnull.Int32(input.MaxDownloads),
		SortOrder:    input.SortOrder,
	})
	if err != nil {
		return ProductDownload{}, fmt.Errorf("create product download: %w", err)
	}
	return toProductDownload(row), nil
}

func (r *postgresRepository) GetProductDownloadByID(ctx context.Context, id uuid.UUID) (ProductDownload, error) {
	row, err := r.queries.GetProductDownloadByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductDownload{}, ErrProductDownloadNotFound
		}
		return ProductDownload{}, fmt.Errorf("get product download: %w", err)
	}
	return toProductDownload(row), nil
}

func (r *postgresRepository) ListProductDownloadsByProductID(ctx context.Context, productID uuid.UUID) ([]ProductDownload, error) {
	rows, err := r.queries.ListProductDownloadsByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list product downloads: %w", err)
	}
	out := make([]ProductDownload, len(rows))
	for i, row := range rows {
		out[i] = toProductDownload(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateProductDownload(ctx context.Context, id uuid.UUID, title, url string, maxDownloads *int32, sortOrder int32) (ProductDownload, error) {
	row, err := r.queries.UpdateProductDownload(ctx, db.UpdateProductDownloadParams{
		ID:           id,
		Title:        title,
		Url:          url,
		MaxDownloads: sqlnull.Int32(maxDownloads),
		SortOrder:    sortOrder,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductDownload{}, ErrProductDownloadNotFound
		}
		return ProductDownload{}, fmt.Errorf("update product download: %w", err)
	}
	return toProductDownload(row), nil
}

func (r *postgresRepository) DeleteProductDownload(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteProductDownload(ctx, id); err != nil {
		return fmt.Errorf("delete product download: %w", err)
	}
	return nil
}

func toProductDownload(row db.CatalogProductDownload) ProductDownload {
	d := ProductDownload{
		ID:        row.ID,
		ProductID: row.ProductID,
		Title:     row.Title,
		URL:       row.Url,
		SortOrder: row.SortOrder,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if row.MaxDownloads.Valid {
		v := row.MaxDownloads.Int32
		d.MaxDownloads = &v
	}
	return d
}
