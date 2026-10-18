package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

func (r *postgresRepository) CreateProductMedia(ctx context.Context, id, productID uuid.UUID, s3Key, mimeType string, sizeBytes int64, altText string, sortOrder int32) (ProductMedia, error) {
	row, err := r.queries.CreateProductMedia(ctx, db.CreateProductMediaParams{
		ID:        id,
		ProductID: productID,
		S3Key:     s3Key,
		MimeType:  mimeType,
		SizeBytes: sizeBytes,
		AltText:   altText,
		SortOrder: sortOrder,
		Status:    string(MediaStatusPending),
	})
	if err != nil {
		return ProductMedia{}, fmt.Errorf("create product media: %w", err)
	}
	return toProductMedia(row), nil
}

func (r *postgresRepository) GetProductMediaByID(ctx context.Context, id uuid.UUID) (ProductMedia, error) {
	row, err := r.queries.GetProductMediaByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductMedia{}, ErrMediaNotFound
		}
		return ProductMedia{}, fmt.Errorf("get product media: %w", err)
	}
	return toProductMedia(row), nil
}

func (r *postgresRepository) ListProductMediaByProduct(ctx context.Context, productID uuid.UUID) ([]ProductMedia, error) {
	rows, err := r.queries.ListProductMediaByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list product media: %w", err)
	}
	out := make([]ProductMedia, len(rows))
	for i, row := range rows {
		out[i] = toProductMedia(row)
	}
	return out, nil
}

func (r *postgresRepository) ListProductMediaByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductMedia, error) {
	out := make(map[uuid.UUID][]ProductMedia, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListProductMediaByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("batch list product media: %w", err)
	}
	for _, row := range rows {
		m := toProductMedia(row)
		out[m.ProductID] = append(out[m.ProductID], m)
	}
	return out, nil
}

func (r *postgresRepository) ListReadyProductMediaByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductMedia, error) {
	out := make(map[uuid.UUID][]ProductMedia, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListReadyProductMediaByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("batch list ready product media: %w", err)
	}
	for _, row := range rows {
		m := toProductMedia(row)
		out[m.ProductID] = append(out[m.ProductID], m)
	}
	return out, nil
}

func (r *postgresRepository) UpdateProductMediaMetadata(ctx context.Context, input UpdateProductMediaInput) (ProductMedia, error) {
	current, err := r.queries.GetProductMediaByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductMedia{}, ErrMediaNotFound
		}
		return ProductMedia{}, fmt.Errorf("load media: %w", err)
	}
	altText := current.AltText
	if input.AltText != nil {
		altText = *input.AltText
	}
	sortOrder := current.SortOrder
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}
	row, err := r.queries.UpdateProductMediaMetadata(ctx, db.UpdateProductMediaMetadataParams{
		ID:        input.ID,
		AltText:   altText,
		SortOrder: sortOrder,
	})
	if err != nil {
		return ProductMedia{}, fmt.Errorf("update media metadata: %w", err)
	}
	return toProductMedia(row), nil
}

func (r *postgresRepository) MarkProductMediaReady(ctx context.Context, id uuid.UUID, width, height int32) (ProductMedia, error) {
	row, err := r.queries.MarkProductMediaReady(ctx, db.MarkProductMediaReadyParams{
		ID:     id,
		Width:  width,
		Height: height,
	})
	if err != nil {
		return ProductMedia{}, fmt.Errorf("mark media ready: %w", err)
	}
	return toProductMedia(row), nil
}

func (r *postgresRepository) SetProductMediaSortOrder(ctx context.Context, id uuid.UUID, order int32) error {
	if err := r.queries.SetProductMediaSortOrder(ctx, db.SetProductMediaSortOrderParams{
		ID:        id,
		SortOrder: order,
	}); err != nil {
		return fmt.Errorf("set media sort order: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteProductMedia(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteProductMedia(ctx, id); err != nil {
		return fmt.Errorf("delete product media: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteProductMediaByIDs(ctx context.Context, productID uuid.UUID, ids []uuid.UUID) ([]PendingMediaRow, error) {
	rows, err := r.queries.DeleteProductMediaByIDs(ctx, db.DeleteProductMediaByIDsParams{
		ProductID: productID,
		Ids:       ids,
	})
	if err != nil {
		return nil, fmt.Errorf("bulk delete product media: %w", err)
	}
	out := make([]PendingMediaRow, len(rows))
	for i, row := range rows {
		out[i] = PendingMediaRow{ID: row.ID, S3Key: row.S3Key}
	}
	return out, nil
}

func (r *postgresRepository) DeletePendingMediaOlderThan(ctx context.Context, before time.Time) ([]PendingMediaRow, error) {
	rows, err := r.queries.DeletePendingMediaOlderThan(ctx, before)
	if err != nil {
		return nil, fmt.Errorf("delete pending media: %w", err)
	}
	out := make([]PendingMediaRow, len(rows))
	for i, row := range rows {
		out[i] = PendingMediaRow{ID: row.ID, S3Key: row.S3Key}
	}
	return out, nil
}

func (r *postgresRepository) SetMediaRole(ctx context.Context, productID, mediaID uuid.UUID, role MediaRole) error {
	if err := r.queries.SetProductMediaRole(ctx, db.SetProductMediaRoleParams{
		ProductID: productID,
		Role:      string(role),
		MediaID:   mediaID,
	}); err != nil {
		return fmt.Errorf("set media role: %w", err)
	}
	return nil
}

func (r *postgresRepository) ClearMediaRole(ctx context.Context, productID uuid.UUID, role MediaRole) error {
	if err := r.queries.ClearProductMediaRole(ctx, db.ClearProductMediaRoleParams{
		ProductID: productID,
		Role:      string(role),
	}); err != nil {
		return fmt.Errorf("clear media role: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetMediaRoles(ctx context.Context, productID uuid.UUID) (map[MediaRole]uuid.UUID, error) {
	rows, err := r.queries.GetProductMediaRoles(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get media roles: %w", err)
	}
	out := make(map[MediaRole]uuid.UUID, len(rows))
	for _, row := range rows {
		out[MediaRole(row.Role)] = row.MediaID
	}
	return out, nil
}

func (r *postgresRepository) GetMediaRolesByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]map[MediaRole]uuid.UUID, error) {
	out := make(map[uuid.UUID]map[MediaRole]uuid.UUID, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.GetProductMediaRolesByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("batch get media roles: %w", err)
	}
	for _, row := range rows {
		if out[row.ProductID] == nil {
			out[row.ProductID] = make(map[MediaRole]uuid.UUID)
		}
		out[row.ProductID][MediaRole(row.Role)] = row.MediaID
	}
	return out, nil
}

func toProductMedia(row db.ProductMedium) ProductMedia {
	return ProductMedia{
		ID:        row.ID,
		ProductID: row.ProductID,
		S3Key:     row.S3Key,
		MimeType:  row.MimeType,
		SizeBytes: row.SizeBytes,
		Width:     row.Width,
		Height:    row.Height,
		AltText:   row.AltText,
		SortOrder: row.SortOrder,
		Status:    MediaStatus(row.Status),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
