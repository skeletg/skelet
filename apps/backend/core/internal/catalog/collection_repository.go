package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/sqlc-dev/pqtype"
)

// ==================== Collections ====================

func (r *postgresRepository) CreateCollection(ctx context.Context, input CreateCollectionInput) (Collection, error) {
	row, err := r.queries.CreateCollection(ctx, db.CreateCollectionParams{
		Slug:            input.Slug,
		IsPublished:     input.IsPublished,
		SeoNoindex:      input.SEONoindex,
		Metadata:        metadataToJSON(input.Metadata),
		PrivateMetadata: metadataToJSON(input.PrivateMetadata),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Collection{}, ErrSlugTaken
		}
		return Collection{}, fmt.Errorf("create collection: %w", err)
	}
	return toCollection(row), nil
}

func (r *postgresRepository) GetCollectionByID(ctx context.Context, id uuid.UUID) (Collection, error) {
	row, err := r.queries.GetCollectionByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Collection{}, ErrCollectionNotFound
		}
		return Collection{}, fmt.Errorf("get collection: %w", err)
	}
	c := toCollection(row)

	content, err := r.queries.ListCollectionContent(ctx, id)
	if err != nil {
		return Collection{}, fmt.Errorf("list collection content: %w", err)
	}
	c.AllContent = make([]CollectionContent, len(content))
	for i, row := range content {
		c.AllContent[i] = toCollectionContent(row)
	}

	return c, nil
}

func (r *postgresRepository) ListCollections(ctx context.Context) ([]Collection, error) {
	rows, err := r.queries.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	out := make([]Collection, len(rows))
	for i, row := range rows {
		out[i] = toCollection(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateCollection(ctx context.Context, input UpdateCollectionInput) (Collection, error) {
	current, err := r.queries.GetCollectionByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Collection{}, ErrCollectionNotFound
		}
		return Collection{}, fmt.Errorf("load collection: %w", err)
	}

	slug := current.Slug
	if input.Slug != nil {
		slug = *input.Slug
	}
	isPublished := current.IsPublished
	if input.IsPublished != nil {
		isPublished = *input.IsPublished
	}
	seoNoindex := current.SeoNoindex
	if input.SEONoindex != nil {
		seoNoindex = *input.SEONoindex
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = metadataToJSON(*input.Metadata)
	}
	privateMetadata := current.PrivateMetadata
	if input.PrivateMetadata != nil {
		privateMetadata = metadataToJSON(*input.PrivateMetadata)
	}

	row, err := r.queries.UpdateCollection(ctx, db.UpdateCollectionParams{
		ID:              input.ID,
		Slug:            slug,
		IsPublished:     isPublished,
		SeoNoindex:      seoNoindex,
		Metadata:        metadata,
		PrivateMetadata: privateMetadata,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Collection{}, ErrSlugTaken
		}
		return Collection{}, fmt.Errorf("update collection: %w", err)
	}
	return toCollection(row), nil
}

func (r *postgresRepository) DeleteCollection(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCollection(ctx, id); err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	return nil
}

// ==================== Collection Content ====================

func (r *postgresRepository) UpsertCollectionContent(ctx context.Context, collectionID uuid.UUID, input CollectionContentInput) (CollectionContent, error) {
	row, err := r.queries.UpsertCollectionContent(ctx, db.UpsertCollectionContentParams{
		CollectionID:    collectionID,
		Locale:          input.Locale,
		Name:            input.Name,
		Description:     pqtype.NullRawMessage{RawMessage: input.Description, Valid: len(input.Description) > 0},
		MetaTitle:       sqlnull.StringValueNonEmpty(input.MetaTitle),
		MetaDescription: sqlnull.StringValueNonEmpty(input.MetaDescription),
		MetaKeywords:    sqlnull.StringValueNonEmpty(input.MetaKeywords),
	})
	if err != nil {
		return CollectionContent{}, fmt.Errorf("upsert collection content: %w", err)
	}
	return toCollectionContent(row), nil
}

func (r *postgresRepository) GetCollectionContent(ctx context.Context, collectionID uuid.UUID, locale string) (CollectionContent, error) {
	row, err := r.queries.GetCollectionContent(ctx, db.GetCollectionContentParams{
		CollectionID: collectionID,
		Locale:       locale,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CollectionContent{}, ErrCollectionContentNotFound
		}
		return CollectionContent{}, fmt.Errorf("get collection content: %w", err)
	}
	return toCollectionContent(row), nil
}

func (r *postgresRepository) ListCollectionContent(ctx context.Context, collectionID uuid.UUID) ([]CollectionContent, error) {
	rows, err := r.queries.ListCollectionContent(ctx, collectionID)
	if err != nil {
		return nil, fmt.Errorf("list collection content: %w", err)
	}
	out := make([]CollectionContent, len(rows))
	for i, row := range rows {
		out[i] = toCollectionContent(row)
	}
	return out, nil
}

// Every locale's row per collection, keyed by collection - a collection with no rows is simply absent
func (r *postgresRepository) ListCollectionContentForCollections(ctx context.Context, collectionIDs []uuid.UUID) (map[uuid.UUID][]CollectionContent, error) {
	out := make(map[uuid.UUID][]CollectionContent, len(collectionIDs))
	if len(collectionIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListCollectionContentForCollections(ctx, collectionIDs)
	if err != nil {
		return nil, fmt.Errorf("list collection content for collections: %w", err)
	}
	for _, row := range rows {
		out[row.CollectionID] = append(out[row.CollectionID], toCollectionContent(row))
	}
	return out, nil
}

func (r *postgresRepository) ListCollectionContentByScope(ctx context.Context, collectionIDs []uuid.UUID, locale string) (map[uuid.UUID]CollectionContent, error) {
	if len(collectionIDs) == 0 {
		return map[uuid.UUID]CollectionContent{}, nil
	}
	rows, err := r.queries.ListCollectionContentByScope(ctx, db.ListCollectionContentByScopeParams{
		CollectionIds: collectionIDs,
		Locale:        locale,
	})
	if err != nil {
		return nil, fmt.Errorf("list collection content by scope: %w", err)
	}
	out := make(map[uuid.UUID]CollectionContent, len(rows))
	for _, row := range rows {
		out[row.CollectionID] = toCollectionContent(row)
	}
	return out, nil
}

func (r *postgresRepository) DeleteCollectionContent(ctx context.Context, collectionID uuid.UUID, locale string) error {
	_, err := r.queries.DeleteCollectionContent(ctx, db.DeleteCollectionContentParams{
		CollectionID: collectionID,
		Locale:       locale,
	})
	if err != nil {
		return fmt.Errorf("delete collection content: %w", err)
	}
	return nil
}

// ==================== Collection Products ====================

func (r *postgresRepository) AddProductToCollection(ctx context.Context, collectionID, productID uuid.UUID, sortOrder int32) error {
	err := r.queries.AddProductToCollection(ctx, db.AddProductToCollectionParams{
		CollectionID: collectionID,
		ProductID:    productID,
		SortOrder:    sortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return ErrCollectionProductAlreadyAdded
		}
		return fmt.Errorf("add product to collection: %w", err)
	}
	return nil
}

func (r *postgresRepository) RemoveProductFromCollection(ctx context.Context, collectionID, productID uuid.UUID) error {
	n, err := r.queries.RemoveProductFromCollection(ctx, db.RemoveProductFromCollectionParams{
		CollectionID: collectionID,
		ProductID:    productID,
	})
	if err != nil {
		return fmt.Errorf("remove product from collection: %w", err)
	}
	if n == 0 {
		return ErrCollectionProductNotFound
	}
	return nil
}

// limit 0 means every member
func (r *postgresRepository) ListProductsByCollection(ctx context.Context, collectionID uuid.UUID, limit int32) ([]Product, error) {
	rows, err := r.queries.ListProductsByCollection(ctx, db.ListProductsByCollectionParams{CollectionID: collectionID, RowLimit: limit})
	if err != nil {
		return nil, fmt.Errorf("list products by collection: %w", err)
	}
	out := make([]Product, len(rows))
	for i, row := range rows {
		out[i] = toProduct(row)
	}
	return out, nil
}

func (r *postgresRepository) ListCollectionsByProduct(ctx context.Context, productID uuid.UUID) ([]Collection, error) {
	rows, err := r.queries.ListCollectionsByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list collections by product: %w", err)
	}
	out := make([]Collection, len(rows))
	for i, row := range rows {
		out[i] = toCollection(row)
	}
	return out, nil
}

func (r *postgresRepository) ListCollectionsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]Collection, error) {
	if len(productIDs) == 0 {
		return map[uuid.UUID][]Collection{}, nil
	}
	rows, err := r.queries.ListCollectionsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list collections by product ids: %w", err)
	}
	out := make(map[uuid.UUID][]Collection)
	for _, row := range rows {
		c := toCollectionFromProductIDRow(row)
		out[row.ProductID] = append(out[row.ProductID], c)
	}
	return out, nil
}

func (r *postgresRepository) ReorderCollectionProduct(ctx context.Context, collectionID, productID uuid.UUID, sortOrder int32) error {
	n, err := r.queries.ReorderCollectionProduct(ctx, db.ReorderCollectionProductParams{
		CollectionID: collectionID,
		ProductID:    productID,
		SortOrder:    sortOrder,
	})
	if err != nil {
		return fmt.Errorf("reorder collection product: %w", err)
	}
	if n == 0 {
		return ErrCollectionProductNotFound
	}
	return nil
}

// ==================== Collection Cover ====================

func (r *postgresRepository) SetCollectionCover(ctx context.Context, collectionID, coverID uuid.UUID, s3Key, mimeType string, sizeBytes int64, altText string) (Collection, error) {
	row, err := r.queries.SetCollectionCover(ctx, db.SetCollectionCoverParams{
		ID:             collectionID,
		CoverID:        uuid.NullUUID{UUID: coverID, Valid: true},
		CoverS3Key:     sql.NullString{String: s3Key, Valid: true},
		CoverMimeType:  sql.NullString{String: mimeType, Valid: true},
		CoverSizeBytes: sql.NullInt64{Int64: sizeBytes, Valid: true},
		CoverAlt:       altText,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Collection{}, ErrCollectionNotFound
		}
		return Collection{}, fmt.Errorf("set collection cover: %w", err)
	}
	return toCollection(row), nil
}

// Pinned to the cover the caller checked - a replaced or already-confirmed cover matches no row
func (r *postgresRepository) ConfirmCollectionCover(ctx context.Context, collectionID, coverID uuid.UUID, width, height int32) (Collection, error) {
	row, err := r.queries.ConfirmCollectionCover(ctx, db.ConfirmCollectionCoverParams{
		ID:          collectionID,
		CoverID:     uuid.NullUUID{UUID: coverID, Valid: true},
		CoverWidth:  sql.NullInt32{Int32: width, Valid: true},
		CoverHeight: sql.NullInt32{Int32: height, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Collection{}, ErrCollectionCoverNotPending
		}
		return Collection{}, fmt.Errorf("confirm collection cover: %w", err)
	}
	return toCollection(row), nil
}

func (r *postgresRepository) ClearCollectionCover(ctx context.Context, collectionID uuid.UUID) (Collection, error) {
	row, err := r.queries.ClearCollectionCover(ctx, collectionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Collection{}, ErrCollectionNotFound
		}
		return Collection{}, fmt.Errorf("clear collection cover: %w", err)
	}
	return toCollection(row), nil
}

// ==================== mappers ====================

func toCollection(row db.Collection) Collection {
	c := Collection{
		ID:              row.ID,
		Slug:            row.Slug,
		IsPublished:     row.IsPublished,
		SEONoindex:      row.SeoNoindex,
		Metadata:        metadataFromJSON(row.Metadata),
		PrivateMetadata: metadataFromJSON(row.PrivateMetadata),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
	if row.CoverID.Valid && row.CoverS3Key.Valid && row.CoverStatus.Valid {
		c.CoverImage = &CollectionCover{
			ID:       row.CoverID.UUID,
			S3Key:    row.CoverS3Key.String,
			AltText:  row.CoverAlt,
			MimeType: row.CoverMimeType.String,
			Status:   row.CoverStatus.String,
		}
		if row.CoverWidth.Valid {
			c.CoverImage.Width = row.CoverWidth.Int32
		}
		if row.CoverHeight.Valid {
			c.CoverImage.Height = row.CoverHeight.Int32
		}
	}
	return c
}

func toCollectionContent(row db.CollectionContent) CollectionContent {
	c := CollectionContent{
		Locale:    row.Locale,
		Name:      row.Name,
		UpdatedAt: row.UpdatedAt,
	}
	if row.Description.Valid {
		c.Description = row.Description.RawMessage
	}
	if row.MetaTitle.Valid {
		c.MetaTitle = &row.MetaTitle.String
	}
	if row.MetaDescription.Valid {
		c.MetaDescription = &row.MetaDescription.String
	}
	if row.MetaKeywords.Valid {
		c.MetaKeywords = &row.MetaKeywords.String
	}
	return c
}

func toCollectionFromProductIDRow(row db.ListCollectionsByProductIDsRow) Collection {
	return toCollection(db.Collection{
		ID:              row.ID,
		Slug:            row.Slug,
		IsPublished:     row.IsPublished,
		SeoNoindex:      row.SeoNoindex,
		CoverID:         row.CoverID,
		CoverS3Key:      row.CoverS3Key,
		CoverMimeType:   row.CoverMimeType,
		CoverSizeBytes:  row.CoverSizeBytes,
		CoverWidth:      row.CoverWidth,
		CoverHeight:     row.CoverHeight,
		CoverStatus:     row.CoverStatus,
		CoverAlt:        row.CoverAlt,
		Metadata:        row.Metadata,
		PrivateMetadata: row.PrivateMetadata,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	})
}

func (r *postgresRepository) ListCollectionFacetOverrides(ctx context.Context, collectionID uuid.UUID) ([]FacetOverride, error) {
	rows, err := r.queries.ListCollectionFacetOverrides(ctx, collectionID)
	if err != nil {
		return nil, fmt.Errorf("list collection facet overrides: %w", err)
	}
	out := make([]FacetOverride, len(rows))
	for i, row := range rows {
		out[i] = FacetOverride{
			AttributeID:   row.AttributeID,
			AttributeSlug: row.AttributeSlug,
			SortOrder:     row.SortOrder,
			Hidden:        row.IsHidden,
		}
	}
	return out, nil
}

// Same clear-and-rebuild shape as the category twin
func (r *postgresRepository) ReplaceCollectionFacetOverrides(ctx context.Context, collectionID uuid.UUID, overrides []FacetOverride) error {
	return r.atomic(ctx, func(q *db.Queries) error {
		if err := q.DeleteCollectionFacetOverrides(ctx, collectionID); err != nil {
			return fmt.Errorf("clear collection facet overrides: %w", err)
		}
		for _, o := range overrides {
			if err := q.InsertCollectionFacetOverride(ctx, db.InsertCollectionFacetOverrideParams{
				CollectionID: collectionID,
				AttributeID:  o.AttributeID,
				SortOrder:    o.SortOrder,
				IsHidden:     o.Hidden,
			}); err != nil {
				if pgdb.IsForeignKeyViolation(err) {
					return ErrAttributeNotFound
				}
				return fmt.Errorf("insert collection facet override: %w", err)
			}
		}
		return nil
	})
}
