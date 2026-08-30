package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/sqlc-dev/pqtype"
)

// ==================== Category CRUD ====================

func (r *postgresRepository) CreateCategory(ctx context.Context, input CreateCategoryInput) (Category, error) {
	var parentID uuid.NullUUID
	if input.ParentID != nil {
		parentID = uuid.NullUUID{UUID: *input.ParentID, Valid: true}
	}
	row, err := r.queries.CreateCategory(ctx, db.CreateCategoryParams{
		ParentID:        parentID,
		Slug:            input.Slug,
		UrlPath:         input.Slug, // computed by service before calling - default to slug
		Position:        input.Position,
		IsPublished:     input.IsPublished,
		SeoNoindex:      input.SEONoindex,
		Metadata:        metadataToJSON(input.Metadata),
		PrivateMetadata: metadataToJSON(input.PrivateMetadata),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Category{}, ErrCategorySlugTaken
		}
		return Category{}, fmt.Errorf("create category: %w", err)
	}
	return toCategory(row), nil
}

func (r *postgresRepository) GetCategoryByID(ctx context.Context, id uuid.UUID) (Category, error) {
	row, err := r.queries.GetCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("get category: %w", err)
	}
	c := toCategory(row)

	content, err := r.queries.ListCategoryContent(ctx, id)
	if err != nil {
		return Category{}, fmt.Errorf("list category content: %w", err)
	}
	c.AllContent = make([]CategoryContent, len(content))
	for i, cr := range content {
		c.AllContent[i] = toCategoryContent(cr)
	}

	products, err := r.queries.ListProductsByCategory(ctx, id)
	if err != nil {
		return Category{}, fmt.Errorf("list category products: %w", err)
	}
	c.Products = make([]Product, len(products))
	for i, row := range products {
		c.Products[i] = toProduct(row)
	}

	return c, nil
}

func (r *postgresRepository) GetCategoryBySlug(ctx context.Context, slug string) (Category, error) {
	row, err := r.queries.GetCategoryBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("get category by slug: %w", err)
	}
	return toCategory(row), nil
}

func (r *postgresRepository) ListAllCategories(ctx context.Context) ([]Category, error) {
	rows, err := r.queries.ListAllCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all categories: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		out[i] = toCategory(row)
	}
	return out, nil
}

func (r *postgresRepository) ListRootCategories(ctx context.Context) ([]Category, error) {
	rows, err := r.queries.ListRootCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list root categories: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		out[i] = toCategory(row)
	}
	return out, nil
}

func (r *postgresRepository) ListCategoryChildren(ctx context.Context, parentID uuid.UUID) ([]Category, error) {
	rows, err := r.queries.ListCategoryChildren(ctx, uuid.NullUUID{UUID: parentID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list category children: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		out[i] = toCategory(row)
	}
	return out, nil
}

// GetCategoryAncestors traverses parent_id links in Go to avoid a recursive CTE - Category trees are shallow (<10 levels) so the N round-trips are fast - Returns ancestors ordered root→immediate parent
func (r *postgresRepository) GetCategoryAncestors(ctx context.Context, id uuid.UUID) ([]Category, error) {
	var ancestors []Category
	current, err := r.queries.GetCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCategoryNotFound
		}
		return nil, fmt.Errorf("get category: %w", err)
	}
	parentID := current.ParentID
	for parentID.Valid {
		parent, err := r.queries.GetCategoryByID(ctx, parentID.UUID)
		if err != nil {
			return nil, fmt.Errorf("get ancestor: %w", err)
		}
		ancestors = append(ancestors, toCategory(parent))
		parentID = parent.ParentID
	}
	// Reverse to root→parent order
	for i, j := 0, len(ancestors)-1; i < j; i, j = i+1, j-1 {
		ancestors[i], ancestors[j] = ancestors[j], ancestors[i]
	}
	return ancestors, nil
}

func (r *postgresRepository) UpdateCategory(ctx context.Context, input UpdateCategoryInput) (Category, error) {
	current, err := r.queries.GetCategoryByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("get category: %w", err)
	}
	slug := current.Slug
	if input.Slug != nil {
		slug = *input.Slug
	}
	urlPath := current.UrlPath
	// url_path is recomputed by the service before calling Update - accept if provided
	if input.Slug != nil {
		urlPath = current.UrlPath // service sets this via UpdateCategoryURLPath after
	}
	parentID := current.ParentID
	if input.ClearParent {
		parentID = uuid.NullUUID{}
	} else if input.ParentID != nil {
		parentID = uuid.NullUUID{UUID: *input.ParentID, Valid: true}
	}
	position := current.Position
	if input.Position != nil {
		position = *input.Position
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
	row, err := r.queries.UpdateCategory(ctx, db.UpdateCategoryParams{
		ID:              input.ID,
		Slug:            slug,
		UrlPath:         urlPath,
		ParentID:        parentID,
		Position:        position,
		IsPublished:     isPublished,
		SeoNoindex:      seoNoindex,
		Metadata:        metadata,
		PrivateMetadata: privateMetadata,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Category{}, ErrCategorySlugTaken
		}
		return Category{}, fmt.Errorf("update category: %w", err)
	}
	return toCategory(row), nil
}

func (r *postgresRepository) UpdateCategoryURLPath(ctx context.Context, id uuid.UUID, urlPath string) error {
	return r.queries.UpdateCategoryURLPath(ctx, db.UpdateCategoryURLPathParams{ID: id, UrlPath: urlPath})
}

func (r *postgresRepository) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCategory(ctx, id); err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	return nil
}

// ==================== Category Content ====================

func (r *postgresRepository) UpsertCategoryContent(ctx context.Context, categoryID uuid.UUID, input CategoryContentInput) (CategoryContent, error) {
	row, err := r.queries.UpsertCategoryContent(ctx, db.UpsertCategoryContentParams{
		CategoryID:       categoryID,
		Locale:           input.Locale,
		Name:             input.Name,
		ShortDescription: pqtype.NullRawMessage{RawMessage: input.ShortDescription, Valid: len(input.ShortDescription) > 0},
		Description:      pqtype.NullRawMessage{RawMessage: input.Description, Valid: len(input.Description) > 0},
		MetaTitle:        nullableString(input.MetaTitle),
		MetaDescription:  nullableString(input.MetaDescription),
		MetaKeywords:     nullableString(input.MetaKeywords),
	})
	if err != nil {
		return CategoryContent{}, fmt.Errorf("upsert category content: %w", err)
	}
	return toCategoryContent(row), nil
}

func (r *postgresRepository) GetCategoryContent(ctx context.Context, categoryID uuid.UUID, locale string) (CategoryContent, error) {
	row, err := r.queries.GetCategoryContent(ctx, db.GetCategoryContentParams{CategoryID: categoryID, Locale: locale})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CategoryContent{}, ErrCollectionContentNotFound
		}
		return CategoryContent{}, fmt.Errorf("get category content: %w", err)
	}
	return toCategoryContent(row), nil
}

func (r *postgresRepository) ListCategoryContent(ctx context.Context, categoryID uuid.UUID) ([]CategoryContent, error) {
	rows, err := r.queries.ListCategoryContent(ctx, categoryID)
	if err != nil {
		return nil, fmt.Errorf("list category content: %w", err)
	}
	out := make([]CategoryContent, len(rows))
	for i, row := range rows {
		out[i] = toCategoryContent(row)
	}
	return out, nil
}

func (r *postgresRepository) ListCategoryContentByScope(ctx context.Context, categoryIDs []uuid.UUID, locale string) (map[uuid.UUID]CategoryContent, error) {
	rows, err := r.queries.ListCategoryContentByScope(ctx, db.ListCategoryContentByScopeParams{
		CategoryIds: categoryIDs,
		Locale:      locale,
	})
	if err != nil {
		return nil, fmt.Errorf("list category content by scope: %w", err)
	}
	out := make(map[uuid.UUID]CategoryContent, len(rows))
	for _, row := range rows {
		out[row.CategoryID] = toCategoryContent(row)
	}
	return out, nil
}

func (r *postgresRepository) DeleteCategoryContent(ctx context.Context, categoryID uuid.UUID, locale string) error {
	_, err := r.queries.DeleteCategoryContent(ctx, db.DeleteCategoryContentParams{CategoryID: categoryID, Locale: locale})
	return err
}

// ==================== Cover image ====================

func (r *postgresRepository) SetCategoryCover(ctx context.Context, categoryID, coverID uuid.UUID, s3Key, mimeType string, sizeBytes int64, altText string) (Category, error) {
	row, err := r.queries.SetCategoryCover(ctx, db.SetCategoryCoverParams{
		ID:             categoryID,
		CoverID:        uuid.NullUUID{UUID: coverID, Valid: true},
		CoverS3Key:     sql.NullString{String: s3Key, Valid: true},
		CoverMimeType:  sql.NullString{String: mimeType, Valid: true},
		CoverSizeBytes: sql.NullInt64{Int64: sizeBytes, Valid: true},
		CoverAlt:       altText,
	})
	if err != nil {
		return Category{}, fmt.Errorf("set category cover: %w", err)
	}
	return toCategory(row), nil
}

func (r *postgresRepository) ConfirmCategoryCover(ctx context.Context, categoryID uuid.UUID, width, height int32) (Category, error) {
	row, err := r.queries.ConfirmCategoryCover(ctx, db.ConfirmCategoryCoverParams{ID: categoryID, CoverWidth: sql.NullInt32{Int32: width, Valid: true}, CoverHeight: sql.NullInt32{Int32: height, Valid: true}})
	if err != nil {
		return Category{}, fmt.Errorf("confirm category cover: %w", err)
	}
	return toCategory(row), nil
}

func (r *postgresRepository) ClearCategoryCover(ctx context.Context, categoryID uuid.UUID) (Category, error) {
	row, err := r.queries.ClearCategoryCover(ctx, categoryID)
	if err != nil {
		return Category{}, fmt.Errorf("clear category cover: %w", err)
	}
	return toCategory(row), nil
}

// ==================== Product-Category junction ====================

func (r *postgresRepository) AddProductToCategory(ctx context.Context, productID, categoryID uuid.UUID, position int32, isPrimary bool) error {
	return r.queries.AddProductToCategory(ctx, db.AddProductToCategoryParams{
		ProductID:  productID,
		CategoryID: categoryID,
		Position:   position,
		IsPrimary:  isPrimary,
	})
}

func (r *postgresRepository) RemoveProductFromCategory(ctx context.Context, productID, categoryID uuid.UUID) error {
	n, err := r.queries.RemoveProductFromCategory(ctx, db.RemoveProductFromCategoryParams{ProductID: productID, CategoryID: categoryID})
	if err != nil {
		return fmt.Errorf("remove product from category: %w", err)
	}
	if n == 0 {
		return ErrCategoryProductNotFound
	}
	return nil
}

func (r *postgresRepository) SetPrimaryProductCategory(ctx context.Context, productID, categoryID uuid.UUID) error {
	return r.queries.SetPrimaryProductCategory(ctx, db.SetPrimaryProductCategoryParams{
		ProductID:  productID,
		CategoryID: categoryID,
	})
}

func (r *postgresRepository) ListProductsByCategory(ctx context.Context, categoryID uuid.UUID) ([]Product, error) {
	rows, err := r.queries.ListProductsByCategory(ctx, categoryID)
	if err != nil {
		return nil, fmt.Errorf("list products by category: %w", err)
	}
	out := make([]Product, len(rows))
	for i, row := range rows {
		out[i] = toProduct(row)
	}
	return out, nil
}

func (r *postgresRepository) CountProductsInCategory(ctx context.Context, categoryID uuid.UUID) (int64, error) {
	n, err := r.queries.CountProductsInCategory(ctx, categoryID)
	if err != nil {
		return 0, fmt.Errorf("count products in category: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) ListCategoriesByProduct(ctx context.Context, productID uuid.UUID) ([]Category, error) {
	rows, err := r.queries.ListCategoriesByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list categories by product: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		out[i] = toCategoryFromProductRow(row)
	}
	return out, nil
}

func (r *postgresRepository) ListCategoriesByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]Category, error) {
	rows, err := r.queries.ListCategoriesByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list categories by product ids: %w", err)
	}
	out := make(map[uuid.UUID][]Category)
	for _, row := range rows {
		c := toCategoryFromProductIDsRow(row)
		out[row.PcProductID] = append(out[row.PcProductID], c)
	}
	return out, nil
}

func (r *postgresRepository) ListPrimaryCategoryByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]*Category, error) {
	rows, err := r.queries.ListPrimaryCategoryByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list primary category by product ids: %w", err)
	}
	out := make(map[uuid.UUID]*Category, len(rows))
	for _, row := range rows {
		c := toCategoryFromPrimaryRow(row)
		out[row.PcProductID] = &c
	}
	return out, nil
}

func (r *postgresRepository) ListProductIDsInPrimaryCategoryDescendants(ctx context.Context, rootID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := r.queries.ListProductIDsInPrimaryCategoryDescendants(ctx, rootID)
	if err != nil {
		return nil, fmt.Errorf("list product ids in category subtree: %w", err)
	}
	return ids, nil
}

// ==================== Mappers ====================

func toCategory(row db.CatalogCategory) Category {
	c := Category{
		ID:              row.ID,
		Slug:            row.Slug,
		URLPath:         row.UrlPath,
		Position:        row.Position,
		IsPublished:     row.IsPublished,
		SEONoindex:      row.SeoNoindex,
		AllContent:      []CategoryContent{},
		Metadata:        metadataFromJSON(row.Metadata),
		PrivateMetadata: metadataFromJSON(row.PrivateMetadata),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
	if row.ParentID.Valid {
		id := row.ParentID.UUID
		c.ParentID = &id
	}
	if row.CoverID.Valid && row.CoverS3Key.Valid && row.CoverStatus.Valid {
		c.CoverImage = &CategoryCover{
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

func toCategoryContent(row db.CatalogCategoryContent) CategoryContent {
	c := CategoryContent{
		Locale:    row.Locale,
		Name:      row.Name,
		UpdatedAt: row.UpdatedAt,
	}
	if row.ShortDescription.Valid {
		c.ShortDescription = row.ShortDescription.RawMessage
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

func toCategoryFromProductRow(row db.ListCategoriesByProductRow) Category {
	return toCategory(db.CatalogCategory{
		ID:              row.ID,
		ParentID:        row.ParentID,
		Slug:            row.Slug,
		UrlPath:         row.UrlPath,
		Position:        row.Position,
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

func toCategoryFromProductIDsRow(row db.ListCategoriesByProductIDsRow) Category {
	return toCategory(db.CatalogCategory{
		ID:              row.ID,
		ParentID:        row.ParentID,
		Slug:            row.Slug,
		UrlPath:         row.UrlPath,
		Position:        row.Position,
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

func toCategoryFromPrimaryRow(row db.ListPrimaryCategoryByProductIDsRow) Category {
	return toCategory(db.CatalogCategory{
		ID:              row.ID,
		ParentID:        row.ParentID,
		Slug:            row.Slug,
		UrlPath:         row.UrlPath,
		Position:        row.Position,
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

// ==================== Per-store category listings ====================

func (r *postgresRepository) UpsertCategoryChannelListing(ctx context.Context, in SetCategoryChannelListingInput) (CategoryChannelListing, error) {
	var override sql.NullInt32
	if in.SortOrderOverride != nil {
		override = sql.NullInt32{Int32: *in.SortOrderOverride, Valid: true}
	}
	row, err := r.queries.UpsertCatalogCategoryChannelListing(ctx, db.UpsertCatalogCategoryChannelListingParams{
		CategoryID:        in.CategoryID,
		ChannelID:         in.ChannelID,
		IsPublished:       in.IsPublished,
		SortOrderOverride: override,
	})
	if err != nil {
		return CategoryChannelListing{}, fmt.Errorf("upsert category store listing: %w", err)
	}
	return toCategoryChannelListing(row), nil
}

func (r *postgresRepository) DeleteCategoryChannelListing(ctx context.Context, categoryID, channelID uuid.UUID) error {
	_, err := r.queries.DeleteCatalogCategoryChannelListing(ctx, db.DeleteCatalogCategoryChannelListingParams{
		CategoryID: categoryID,
		ChannelID:  channelID,
	})
	if err != nil {
		return fmt.Errorf("delete category store listing: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListCategoryChannelListings(ctx context.Context, categoryID uuid.UUID) ([]CategoryChannelListing, error) {
	rows, err := r.queries.ListCatalogCategoryChannelListings(ctx, categoryID)
	if err != nil {
		return nil, fmt.Errorf("list category store listings: %w", err)
	}
	out := make([]CategoryChannelListing, len(rows))
	for i, row := range rows {
		out[i] = toCategoryChannelListing(row)
	}
	return out, nil
}

func (r *postgresRepository) ListCategoryChannelListingsByCategoryIDs(ctx context.Context, categoryIDs []uuid.UUID) (map[uuid.UUID][]CategoryChannelListing, error) {
	if len(categoryIDs) == 0 {
		return map[uuid.UUID][]CategoryChannelListing{}, nil
	}
	rows, err := r.queries.ListCatalogCategoryChannelListingsByCategoryIDs(ctx, categoryIDs)
	if err != nil {
		return nil, fmt.Errorf("list category store listings by ids: %w", err)
	}
	out := make(map[uuid.UUID][]CategoryChannelListing, len(categoryIDs))
	for _, row := range rows {
		out[row.CategoryID] = append(out[row.CategoryID], toCategoryChannelListing(row))
	}
	return out, nil
}

func (r *postgresRepository) GetCategoryChannelListing(ctx context.Context, categoryID, channelID uuid.UUID) (CategoryChannelListing, error) {
	row, err := r.queries.GetCatalogCategoryChannelListing(ctx, db.GetCatalogCategoryChannelListingParams{
		CategoryID: categoryID,
		ChannelID:  channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CategoryChannelListing{}, ErrCategoryChannelListingNotFound
		}
		return CategoryChannelListing{}, fmt.Errorf("get category store listing: %w", err)
	}
	return toCategoryChannelListing(row), nil
}

func (r *postgresRepository) ListPublishedRootCategoriesForChannel(ctx context.Context, channelID uuid.UUID) ([]Category, error) {
	rows, err := r.queries.ListPublishedRootCategoriesForChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list published root categories: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		c := toCategoryFromPublishedRoot(row)
		out[i] = c
	}
	return out, nil
}

func (r *postgresRepository) ListPublishedCategoriesForChannel(ctx context.Context, channelID uuid.UUID) ([]Category, error) {
	rows, err := r.queries.ListPublishedCategoriesForChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list published categories: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		c := toCategory(db.CatalogCategory{
			ID:              row.ID,
			ParentID:        row.ParentID,
			Slug:            row.Slug,
			UrlPath:         row.UrlPath,
			Position:        row.Position,
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
		if row.SortOrderOverride.Valid {
			c.Position = row.SortOrderOverride.Int32
		}
		out[i] = c
	}
	return out, nil
}

func (r *postgresRepository) ListPublishedCategoryChildrenForChannel(ctx context.Context, channelID, parentID uuid.UUID) ([]Category, error) {
	rows, err := r.queries.ListPublishedCategoryChildrenForChannel(ctx, db.ListPublishedCategoryChildrenForChannelParams{
		ChannelID: channelID,
		ParentID:  uuid.NullUUID{UUID: parentID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("list published category children: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		c := toCategoryFromPublishedChild(row)
		out[i] = c
	}
	return out, nil
}

func (r *postgresRepository) GetPublishedCategoryForChannel(ctx context.Context, channelID, categoryID uuid.UUID) (Category, error) {
	// Reuse the by-id fetch and gate on the store listing - The listing table lookup is a single keyed read, so the cost is fine
	c, err := r.GetCategoryByID(ctx, categoryID)
	if err != nil {
		return Category{}, err
	}
	listing, err := r.GetCategoryChannelListing(ctx, categoryID, channelID)
	if err != nil {
		if errors.Is(err, ErrCategoryChannelListingNotFound) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, err
	}
	if !listing.IsPublished {
		return Category{}, ErrCategoryNotFound
	}
	if !c.IsPublished {
		return Category{}, ErrCategoryNotFound
	}
	if listing.SortOrderOverride != nil {
		c.Position = *listing.SortOrderOverride
	}
	return c, nil
}

func (r *postgresRepository) FilterPublishedCategoryIDsForChannel(ctx context.Context, ids []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]struct{}{}, nil
	}
	rows, err := r.queries.ListPublishedCategoryIDsForChannelFiltered(ctx, db.ListPublishedCategoryIDsForChannelFilteredParams{
		CategoryIds: ids,
		ChannelID:   channelID,
	})
	if err != nil {
		return nil, fmt.Errorf("filter published category ids: %w", err)
	}
	out := make(map[uuid.UUID]struct{}, len(rows))
	for _, id := range rows {
		out[id] = struct{}{}
	}
	return out, nil
}

func (r *postgresRepository) FilterFullyVisibleCategoryIDsForChannel(ctx context.Context, ids []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]struct{}{}, nil
	}
	rows, err := r.queries.ListFullyVisibleCategoryIDsForChannelFiltered(ctx, db.ListFullyVisibleCategoryIDsForChannelFilteredParams{
		CategoryIds: ids,
		ChannelID:   channelID,
	})
	if err != nil {
		return nil, fmt.Errorf("filter fully visible category ids: %w", err)
	}
	out := make(map[uuid.UUID]struct{}, len(rows))
	for _, id := range rows {
		out[id] = struct{}{}
	}
	return out, nil
}

func toCategoryChannelListing(row db.CatalogCategoryChannelListing) CategoryChannelListing {
	out := CategoryChannelListing{
		ChannelID:   row.ChannelID,
		IsPublished: row.IsPublished,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if row.SortOrderOverride.Valid {
		v := row.SortOrderOverride.Int32
		out.SortOrderOverride = &v
	}
	return out
}

func toCategoryFromPublishedRoot(row db.ListPublishedRootCategoriesForChannelRow) Category {
	c := toCategory(db.CatalogCategory{
		ID:              row.ID,
		ParentID:        row.ParentID,
		Slug:            row.Slug,
		UrlPath:         row.UrlPath,
		Position:        row.Position,
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
	if row.SortOrderOverride.Valid {
		c.Position = row.SortOrderOverride.Int32
	}
	return c
}

func toCategoryFromPublishedChild(row db.ListPublishedCategoryChildrenForChannelRow) Category {
	c := toCategory(db.CatalogCategory{
		ID:              row.ID,
		ParentID:        row.ParentID,
		Slug:            row.Slug,
		UrlPath:         row.UrlPath,
		Position:        row.Position,
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
	if row.SortOrderOverride.Valid {
		c.Position = row.SortOrderOverride.Int32
	}
	return c
}

func (r *postgresRepository) ListCategoryFacetOverrides(ctx context.Context, categoryID uuid.UUID) ([]FacetOverride, error) {
	rows, err := r.queries.ListCategoryFacetOverrides(ctx, categoryID)
	if err != nil {
		return nil, fmt.Errorf("list category facet overrides: %w", err)
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

// Same clear-and-rebuild shape as ReplaceProductLinks - an empty slice resets the
// category to the attribute-order default
func (r *postgresRepository) ReplaceCategoryFacetOverrides(ctx context.Context, categoryID uuid.UUID, overrides []FacetOverride) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.DeleteCategoryFacetOverrides(ctx, categoryID); err != nil {
			return fmt.Errorf("clear category facet overrides: %w", err)
		}
		for _, o := range overrides {
			if err := q.InsertCategoryFacetOverride(ctx, db.InsertCategoryFacetOverrideParams{
				CategoryID:  categoryID,
				AttributeID: o.AttributeID,
				SortOrder:   o.SortOrder,
				IsHidden:    o.Hidden,
			}); err != nil {
				return fmt.Errorf("insert category facet override: %w", err)
			}
		}
		return nil
	})
}
