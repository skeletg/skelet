package medialibrary

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	pgdb "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) CreateAsset(ctx context.Context, asset Asset) (Asset, error) {
	row, err := r.queries.CreateMediaAsset(ctx, db.CreateMediaAssetParams{
		ID:              asset.ID,
		DirectoryPrefix: asset.DirectoryPrefix,
		Filename:        asset.Filename,
		ObjectKey:       asset.ObjectKey,
		MimeType:        asset.MimeType,
		SizeBytes:       asset.SizeBytes,
		Width:           nullableInt32(asset.Width),
		Height:          nullableInt32(asset.Height),
		Status:          string(asset.Status),
		Title:           asset.Title,
		AltText:         asset.AltText,
		Caption:         asset.Caption,
		Notes:           asset.Notes,
		CreatedBy:       uuidToNull(asset.CreatedBy),
		UpdatedBy:       uuidToNull(asset.UpdatedBy),
	})
	if err != nil {
		if pgdb.IsUniqueViolation(err) {
			return Asset{}, ErrPathTaken
		}
		return Asset{}, fmt.Errorf("create media asset: %w", err)
	}
	return toAsset(row), nil
}

func (r *postgresRepository) GetAssetByID(ctx context.Context, id uuid.UUID) (Asset, error) {
	row, err := r.queries.GetMediaAssetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Asset{}, ErrAssetNotFound
		}
		return Asset{}, fmt.Errorf("get media asset: %w", err)
	}
	return toAsset(row), nil
}

func (r *postgresRepository) GetAssetsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Asset, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]Asset{}, nil
	}
	rows, err := r.queries.GetMediaAssetsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get media assets by ids: %w", err)
	}
	out := make(map[uuid.UUID]Asset, len(rows))
	for _, row := range rows {
		a := toAsset(row)
		out[a.ID] = a
	}
	return out, nil
}

func (r *postgresRepository) ListAssetsByDirectory(ctx context.Context, path string) ([]Asset, error) {
	rows, err := r.queries.ListMediaAssetsByDirectory(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list media assets by directory: %w", err)
	}
	return toAssets(rows), nil
}

func (r *postgresRepository) ListAssetsByPathRecursive(ctx context.Context, path string) ([]Asset, error) {
	rows, err := r.queries.ListMediaAssetsByPathRecursive(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list media assets by path prefix: %w", err)
	}
	return toAssets(rows), nil
}

func (r *postgresRepository) ListOrphanProductOptionAssets(ctx context.Context, cutoff time.Time, maxRows int32) ([]Asset, error) {
	rows, err := r.queries.ListOrphanProductOptionAssets(ctx, db.ListOrphanProductOptionAssetsParams{Cutoff: cutoff, MaxRows: maxRows})
	if err != nil {
		return nil, fmt.Errorf("list orphan product option assets: %w", err)
	}
	return toAssets(rows), nil
}

func (r *postgresRepository) SearchAssets(ctx context.Context, input SearchInput) (SearchResult, error) {
	var path string
	if input.Path != nil {
		path = *input.Path
	}
	var mimePrefix string
	if input.MimeGroup != nil {
		mimePrefix = string(*input.MimeGroup)
	}
	var query string
	if input.Query != nil {
		query = *input.Query
	}
	rows, err := r.queries.SearchMediaAssets(ctx, db.SearchMediaAssetsParams{
		Column1: path,
		Column2: mimePrefix,
		Column3: query,
		Limit:   input.Limit,
		Offset:  input.Offset,
	})
	if err != nil {
		return SearchResult{}, fmt.Errorf("search media assets: %w", err)
	}
	total, err := r.queries.CountSearchMediaAssets(ctx, db.CountSearchMediaAssetsParams{
		Column1: path,
		Column2: mimePrefix,
		Column3: query,
	})
	if err != nil {
		return SearchResult{}, fmt.Errorf("count media assets: %w", err)
	}
	return SearchResult{Items: toAssets(rows), TotalCount: int32(total)}, nil
}

func (r *postgresRepository) ConfirmAssetUpload(ctx context.Context, id uuid.UUID, width, height *int32, updatedBy *uuid.UUID) (Asset, error) {
	row, err := r.queries.ConfirmMediaAssetUpload(ctx, db.ConfirmMediaAssetUploadParams{
		ID:        id,
		Width:     nullableInt32(width),
		Height:    nullableInt32(height),
		UpdatedBy: uuidToNull(updatedBy),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Asset{}, ErrAssetNotFound
		}
		return Asset{}, fmt.Errorf("confirm media asset upload: %w", err)
	}
	return toAsset(row), nil
}

func (r *postgresRepository) UpdateAssetMetadata(ctx context.Context, input UpdateAssetInput, updatedBy *uuid.UUID) (Asset, error) {
	current, err := r.GetAssetByID(ctx, input.ID)
	if err != nil {
		return Asset{}, err
	}
	if input.Title != nil {
		current.Title = *input.Title
	}
	if input.AltText != nil {
		current.AltText = *input.AltText
	}
	if input.Caption != nil {
		current.Caption = *input.Caption
	}
	if input.Notes != nil {
		current.Notes = *input.Notes
	}
	row, err := r.queries.UpdateMediaAssetMetadata(ctx, db.UpdateMediaAssetMetadataParams{
		ID:        input.ID,
		Title:     current.Title,
		AltText:   current.AltText,
		Caption:   current.Caption,
		Notes:     current.Notes,
		UpdatedBy: uuidToNull(updatedBy),
	})
	if err != nil {
		return Asset{}, fmt.Errorf("update media asset metadata: %w", err)
	}
	return toAsset(row), nil
}

func (r *postgresRepository) UpdateAssetLocation(ctx context.Context, id uuid.UUID, path, filename, objectKey string, updatedBy *uuid.UUID) (Asset, error) {
	row, err := r.queries.UpdateMediaAssetLocation(ctx, db.UpdateMediaAssetLocationParams{
		ID:              id,
		DirectoryPrefix: path,
		Filename:        filename,
		ObjectKey:       objectKey,
		UpdatedBy:       uuidToNull(updatedBy),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Asset{}, ErrAssetNotFound
		}
		return Asset{}, fmt.Errorf("update media asset location: %w", err)
	}
	return toAsset(row), nil
}

func (r *postgresRepository) DeleteAsset(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteMediaAsset(ctx, id); err != nil {
		return fmt.Errorf("delete media asset: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteAssetsByDirectoryPrefix(ctx context.Context, path string) error {
	if err := r.queries.DeleteMediaAssetsByDirectoryPrefix(ctx, path); err != nil {
		return fmt.Errorf("delete media assets by directory prefix: %w", err)
	}
	return nil
}

func toAssets(rows []db.MediaAsset) []Asset {
	out := make([]Asset, len(rows))
	for i, row := range rows {
		out[i] = toAsset(row)
	}
	return out
}

func toAsset(row db.MediaAsset) Asset {
	return Asset{
		ID:              row.ID,
		DirectoryPrefix: row.DirectoryPrefix,
		Filename:        row.Filename,
		ObjectKey:       row.ObjectKey,
		MimeType:        row.MimeType,
		SizeBytes:       row.SizeBytes,
		Width:           int32Ptr(row.Width),
		Height:          int32Ptr(row.Height),
		FocalX:          nullFloat64Ptr(row.FocalX),
		FocalY:          nullFloat64Ptr(row.FocalY),
		Status:          AssetStatus(row.Status),
		Title:           row.Title,
		AltText:         row.AltText,
		Caption:         row.Caption,
		Notes:           row.Notes,
		CreatedBy:       nullUUIDPtr(row.CreatedBy),
		UpdatedBy:       nullUUIDPtr(row.UpdatedBy),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func nullFloat64Ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

func nullableInt32(v *int32) sql.NullInt32 {
	if v == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *v, Valid: true}
}

func int32Ptr(v sql.NullInt32) *int32 {
	if !v.Valid {
		return nil
	}
	n := v.Int32
	return &n
}

func uuidToNull(v *uuid.UUID) uuid.NullUUID {
	if v == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *v, Valid: true}
}

func nullUUIDPtr(v uuid.NullUUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := v.UUID
	return &id
}

// SetFocalPoint stores (or clears, with nils) the smart-crop focal
// point for an asset
func (r *postgresRepository) SetFocalPoint(ctx context.Context, id uuid.UUID, x, y *float64) (Asset, error) {
	fx, fy := sql.NullFloat64{}, sql.NullFloat64{}
	if x != nil {
		fx = sql.NullFloat64{Float64: *x, Valid: true}
	}
	if y != nil {
		fy = sql.NullFloat64{Float64: *y, Valid: true}
	}
	row, err := r.queries.SetMediaFocalPoint(ctx, db.SetMediaFocalPointParams{ID: id, FocalX: fx, FocalY: fy})
	if err != nil {
		return Asset{}, fmt.Errorf("set focal point: %w", err)
	}
	return toAsset(row), nil
}

// GetByObjectKey resolves an active asset from its storage key - the
// transform endpoint uses it to pick up the focal point
func (r *postgresRepository) GetByObjectKey(ctx context.Context, key string) (Asset, bool, error) {
	row, err := r.queries.GetMediaAssetByObjectKey(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Asset{}, false, nil
		}
		return Asset{}, false, err
	}
	return toAsset(row), true, nil
}
