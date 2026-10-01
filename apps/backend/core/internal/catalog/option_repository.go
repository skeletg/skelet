package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sqlc-dev/pqtype"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

// ==================== Product options ====================

func (r *postgresRepository) CreateProductOption(ctx context.Context, input AddProductOptionInput) (ProductOption, error) {
	var out ProductOption
	err := r.atomic(ctx, func(q *db.Queries) error {
		visibleIf, err := nullRawFromGroup(input.VisibleIf)
		if err != nil {
			return err
		}
		row, err := q.CreateProductOption(ctx, db.CreateProductOptionParams{
			ProductID:      input.ProductID,
			InputType:      db.CatalogProductOptionInputType(input.InputType),
			IsRequired:     input.IsRequired,
			SortOrder:      input.SortOrder,
			PriceValue:     sqlnull.Int32(input.PriceValue),
			PriceType:      nullPriceType(input.PriceType),
			MaxChars:       sqlnull.Int32(input.MaxChars),
			MaxFileBytes:   sqlnull.Int64(input.MaxFileBytes),
			FileExtensions: sqlnull.StringValueNonEmpty(input.FileExtensions),
			VisibleIf:      visibleIf,
		})
		if err != nil {
			return fmt.Errorf("create product option: %w", err)
		}
		content, err := q.UpsertProductOptionContent(ctx, db.UpsertProductOptionContentParams{
			OptionID: row.ID,
			Locale:   input.Locale,
			Title:    input.Title,
			HelpText: input.HelpText,
		})
		if err != nil {
			return fmt.Errorf("upsert product option content: %w", err)
		}
		if out, err = toProductOption(row); err != nil {
			return err
		}
		out.Title = content.Title
		out.HelpText = content.HelpText
		out.AllContent = []ProductOptionContent{{Locale: content.Locale, Title: content.Title, HelpText: content.HelpText}}
		return nil
	})
	if err != nil {
		return ProductOption{}, err
	}
	return out, nil
}

func (r *postgresRepository) GetProductOptionByID(ctx context.Context, id uuid.UUID) (ProductOption, error) {
	row, err := r.queries.GetProductOptionByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductOption{}, ErrProductOptionNotFound
		}
		return ProductOption{}, fmt.Errorf("get product option: %w", err)
	}
	return toProductOption(row)
}

func (r *postgresRepository) GetProductOptionByIDForUpdate(ctx context.Context, id uuid.UUID) (ProductOption, error) {
	row, err := r.queries.GetProductOptionByIDForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductOption{}, ErrProductOptionNotFound
		}
		return ProductOption{}, fmt.Errorf("lock product option: %w", err)
	}
	return toProductOption(row)
}

func (r *postgresRepository) ListProductOptionsByProductID(ctx context.Context, productID uuid.UUID) ([]ProductOption, error) {
	rows, err := r.queries.ListProductOptionsByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list product options: %w", err)
	}
	out := make([]ProductOption, len(rows))
	for i, row := range rows {
		if out[i], err = toProductOption(row); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *postgresRepository) ListProductOptionsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductOption, error) {
	if len(productIDs) == 0 {
		return map[uuid.UUID][]ProductOption{}, nil
	}
	rows, err := r.queries.ListProductOptionsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list product options by product ids: %w", err)
	}
	out := make(map[uuid.UUID][]ProductOption, len(productIDs))
	for _, row := range rows {
		o, err := toProductOption(row)
		if err != nil {
			return nil, err
		}
		out[row.ProductID] = append(out[row.ProductID], o)
	}
	return out, nil
}

func (r *postgresRepository) UpdateProductOption(ctx context.Context, id uuid.UUID, params UpdateProductOptionFields) (ProductOption, error) {
	visibleIf, err := nullRawFromGroup(params.VisibleIf)
	if err != nil {
		return ProductOption{}, err
	}
	row, err := r.queries.UpdateProductOption(ctx, db.UpdateProductOptionParams{
		ID:             id,
		InputType:      db.CatalogProductOptionInputType(params.InputType),
		IsRequired:     params.IsRequired,
		SortOrder:      params.SortOrder,
		PriceValue:     sqlnull.Int32(params.PriceValue),
		PriceType:      nullPriceType(params.PriceType),
		MaxChars:       sqlnull.Int32(params.MaxChars),
		MaxFileBytes:   sqlnull.Int64(params.MaxFileBytes),
		FileExtensions: sqlnull.StringValueNonEmpty(params.FileExtensions),
		VisibleIf:      visibleIf,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductOption{}, ErrProductOptionNotFound
		}
		return ProductOption{}, fmt.Errorf("update product option: %w", err)
	}
	return toProductOption(row)
}

func (r *postgresRepository) DeleteProductOption(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteProductOption(ctx, id); err != nil {
		return fmt.Errorf("delete product option: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpsertProductOptionContent(ctx context.Context, input UpsertProductOptionContentInput) (ProductOptionContent, error) {
	row, err := r.queries.UpsertProductOptionContent(ctx, db.UpsertProductOptionContentParams{
		OptionID: input.OptionID,
		Locale:   input.Locale,
		Title:    input.Title,
		HelpText: input.HelpText,
	})
	if err != nil {
		return ProductOptionContent{}, fmt.Errorf("upsert product option content: %w", err)
	}
	return ProductOptionContent{Locale: row.Locale, Title: row.Title, HelpText: row.HelpText}, nil
}

func (r *postgresRepository) ListProductOptionContent(ctx context.Context, optionID uuid.UUID) ([]ProductOptionContent, error) {
	rows, err := r.queries.ListProductOptionContent(ctx, optionID)
	if err != nil {
		return nil, fmt.Errorf("list product option content: %w", err)
	}
	out := make([]ProductOptionContent, len(rows))
	for i, row := range rows {
		out[i] = ProductOptionContent{Locale: row.Locale, Title: row.Title, HelpText: row.HelpText}
	}
	return out, nil
}

func (r *postgresRepository) ListProductOptionContentByOptionIDs(ctx context.Context, optionIDs []uuid.UUID) (map[uuid.UUID][]ProductOptionContent, error) {
	if len(optionIDs) == 0 {
		return map[uuid.UUID][]ProductOptionContent{}, nil
	}
	rows, err := r.queries.ListProductOptionContentByOptionIDs(ctx, optionIDs)
	if err != nil {
		return nil, fmt.Errorf("list product option content by ids: %w", err)
	}
	out := make(map[uuid.UUID][]ProductOptionContent, len(optionIDs))
	for _, row := range rows {
		out[row.OptionID] = append(out[row.OptionID], ProductOptionContent{Locale: row.Locale, Title: row.Title, HelpText: row.HelpText})
	}
	return out, nil
}

func (r *postgresRepository) DeleteProductOptionContent(ctx context.Context, optionID uuid.UUID, locale string) error {
	if err := r.queries.DeleteProductOptionContent(ctx, db.DeleteProductOptionContentParams{OptionID: optionID, Locale: locale}); err != nil {
		return fmt.Errorf("delete product option content: %w", err)
	}
	return nil
}

// ==================== Product option values ====================

func (r *postgresRepository) CreateProductOptionValue(ctx context.Context, input AddProductOptionValueInput) (ProductOptionValue, error) {
	var out ProductOptionValue
	err := r.atomic(ctx, func(q *db.Queries) error {
		row, err := q.CreateProductOptionValue(ctx, db.CreateProductOptionValueParams{
			OptionID:        input.OptionID,
			SortOrder:       input.SortOrder,
			PriceValue:      input.PriceValue,
			PriceType:       db.CatalogProductOptionPriceType(input.PriceType),
			IsDefault:       input.IsDefault,
			SourceProductID: sqlnull.UUIDNonZero(input.SourceProductID),
			Quantity:        input.Quantity,
			ForcedLineQty:   sqlnull.Float64(input.ForcedLineQty),
		})
		if err != nil {
			return fmt.Errorf("create product option value: %w", err)
		}
		content, err := q.UpsertProductOptionValueContent(ctx, db.UpsertProductOptionValueContentParams{
			ValueID: row.ID,
			Locale:  input.Locale,
			Title:   input.Title,
		})
		if err != nil {
			return fmt.Errorf("upsert product option value content: %w", err)
		}
		out = toProductOptionValue(row)
		out.Title = content.Title
		out.AllContent = []ProductOptionValueContent{{Locale: content.Locale, Title: content.Title}}
		return nil
	})
	if err != nil {
		return ProductOptionValue{}, err
	}
	return out, nil
}

func (r *postgresRepository) GetProductOptionValueByID(ctx context.Context, id uuid.UUID) (ProductOptionValue, error) {
	row, err := r.queries.GetProductOptionValueByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductOptionValue{}, ErrProductOptionValueNotFound
		}
		return ProductOptionValue{}, fmt.Errorf("get product option value: %w", err)
	}
	return toProductOptionValue(row), nil
}

func (r *postgresRepository) ListProductOptionValuesByOptionID(ctx context.Context, optionID uuid.UUID) ([]ProductOptionValue, error) {
	rows, err := r.queries.ListProductOptionValuesByOptionID(ctx, optionID)
	if err != nil {
		return nil, fmt.Errorf("list product option values: %w", err)
	}
	out := make([]ProductOptionValue, len(rows))
	for i, row := range rows {
		out[i] = toProductOptionValue(row)
	}
	return out, nil
}

func (r *postgresRepository) ListProductOptionValuesByOptionIDs(ctx context.Context, optionIDs []uuid.UUID) (map[uuid.UUID][]ProductOptionValue, error) {
	if len(optionIDs) == 0 {
		return map[uuid.UUID][]ProductOptionValue{}, nil
	}
	rows, err := r.queries.ListProductOptionValuesByOptionIDs(ctx, optionIDs)
	if err != nil {
		return nil, fmt.Errorf("list product option values by ids: %w", err)
	}
	out := make(map[uuid.UUID][]ProductOptionValue, len(optionIDs))
	for _, row := range rows {
		out[row.OptionID] = append(out[row.OptionID], toProductOptionValue(row))
	}
	return out, nil
}

func (r *postgresRepository) CountProductOptionValueDefaultsForOption(ctx context.Context, optionID uuid.UUID) (int64, error) {
	n, err := r.queries.CountProductOptionValueDefaultsForOption(ctx, optionID)
	if err != nil {
		return 0, fmt.Errorf("count default option values: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) UpdateProductOptionValue(ctx context.Context, id uuid.UUID, sortOrder, priceValue int32, priceType ProductOptionPriceType, isDefault bool, sourceProductID *uuid.UUID, quantity float64, forcedLineQty *float64) (ProductOptionValue, error) {
	row, err := r.queries.UpdateProductOptionValue(ctx, db.UpdateProductOptionValueParams{
		ID:              id,
		SortOrder:       sortOrder,
		PriceValue:      priceValue,
		PriceType:       db.CatalogProductOptionPriceType(priceType),
		IsDefault:       isDefault,
		SourceProductID: sqlnull.UUIDNonZero(sourceProductID),
		Quantity:        quantity,
		ForcedLineQty:   sqlnull.Float64(forcedLineQty),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductOptionValue{}, ErrProductOptionValueNotFound
		}
		return ProductOptionValue{}, fmt.Errorf("update product option value: %w", err)
	}
	return toProductOptionValue(row), nil
}

func (r *postgresRepository) DeleteProductOptionValue(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteProductOptionValue(ctx, id); err != nil {
		return fmt.Errorf("delete product option value: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpsertProductOptionValueContent(ctx context.Context, input UpsertProductOptionValueContentInput) (ProductOptionValueContent, error) {
	row, err := r.queries.UpsertProductOptionValueContent(ctx, db.UpsertProductOptionValueContentParams{
		ValueID: input.ValueID,
		Locale:  input.Locale,
		Title:   input.Title,
	})
	if err != nil {
		return ProductOptionValueContent{}, fmt.Errorf("upsert product option value content: %w", err)
	}
	return ProductOptionValueContent{Locale: row.Locale, Title: row.Title}, nil
}

func (r *postgresRepository) ListProductOptionValueContent(ctx context.Context, valueID uuid.UUID) ([]ProductOptionValueContent, error) {
	rows, err := r.queries.ListProductOptionValueContent(ctx, valueID)
	if err != nil {
		return nil, fmt.Errorf("list product option value content: %w", err)
	}
	out := make([]ProductOptionValueContent, len(rows))
	for i, row := range rows {
		out[i] = ProductOptionValueContent{Locale: row.Locale, Title: row.Title}
	}
	return out, nil
}

func (r *postgresRepository) ListProductOptionValueContentByValueIDs(ctx context.Context, valueIDs []uuid.UUID) (map[uuid.UUID][]ProductOptionValueContent, error) {
	if len(valueIDs) == 0 {
		return map[uuid.UUID][]ProductOptionValueContent{}, nil
	}
	rows, err := r.queries.ListProductOptionValueContentByValueIDs(ctx, valueIDs)
	if err != nil {
		return nil, fmt.Errorf("list product option value content by ids: %w", err)
	}
	out := make(map[uuid.UUID][]ProductOptionValueContent, len(valueIDs))
	for _, row := range rows {
		out[row.ValueID] = append(out[row.ValueID], ProductOptionValueContent{Locale: row.Locale, Title: row.Title})
	}
	return out, nil
}

func (r *postgresRepository) DeleteProductOptionValueContent(ctx context.Context, valueID uuid.UUID, locale string) error {
	if err := r.queries.DeleteProductOptionValueContent(ctx, db.DeleteProductOptionValueContentParams{ValueID: valueID, Locale: locale}); err != nil {
		return fmt.Errorf("delete product option value content: %w", err)
	}
	return nil
}

// --- row mappers ---

// A stored visibility rule that won't decode is an error, never "always visible"
func toProductOption(row db.CatalogProductOption) (ProductOption, error) {
	out := ProductOption{
		ID:         row.ID,
		ProductID:  row.ProductID,
		InputType:  ProductOptionInputType(row.InputType),
		IsRequired: row.IsRequired,
		SortOrder:  row.SortOrder,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
	if row.VisibleIf.Valid && len(row.VisibleIf.RawMessage) > 0 {
		var g OptionConditionGroup
		if err := json.Unmarshal(row.VisibleIf.RawMessage, &g); err != nil {
			return ProductOption{}, fmt.Errorf("decode visible_if of option %s: %w", row.ID, err)
		}
		out.VisibleIf = &g
	}
	if row.PriceValue.Valid {
		v := row.PriceValue.Int32
		out.PriceValue = &v
	}
	if row.PriceType.Valid {
		t := ProductOptionPriceType(row.PriceType.CatalogProductOptionPriceType)
		out.PriceType = &t
	}
	if row.MaxChars.Valid {
		v := row.MaxChars.Int32
		out.MaxChars = &v
	}
	if row.MaxFileBytes.Valid {
		v := row.MaxFileBytes.Int64
		out.MaxFileBytes = &v
	}
	if row.FileExtensions.Valid {
		out.FileExtensions = row.FileExtensions.String
	}
	return out, nil
}

func toProductOptionValue(row db.CatalogProductOptionValue) ProductOptionValue {
	out := ProductOptionValue{
		ID:         row.ID,
		OptionID:   row.OptionID,
		SortOrder:  row.SortOrder,
		PriceValue: row.PriceValue,
		PriceType:  ProductOptionPriceType(row.PriceType),
		IsDefault:  row.IsDefault,
		Quantity:   row.Quantity,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
	if row.SourceProductID.Valid {
		id := row.SourceProductID.UUID
		out.SourceProductID = &id
	}
	if row.ForcedLineQty.Valid {
		q := row.ForcedLineQty.Float64
		out.ForcedLineQty = &q
	}
	return out
}

// UpdateProductOptionFields is a flat, fully-resolved struct passed by the service to the repository's UpdateProductOption - The service has already applied "no change = current value" defaulting, so every field is the final value to write
type UpdateProductOptionFields struct {
	InputType      ProductOptionInputType
	IsRequired     bool
	SortOrder      int32
	PriceValue     *int32
	PriceType      *ProductOptionPriceType
	MaxChars       *int32
	MaxFileBytes   *int64
	FileExtensions string
	VisibleIf      *OptionConditionGroup
}

// --- null-helpers (local to options for now) ---

// nullRawFromGroup marshals a visibility condition to JSONB (NULL when nil)
func nullRawFromGroup(g *OptionConditionGroup) (pqtype.NullRawMessage, error) {
	if g == nil {
		return pqtype.NullRawMessage{}, nil
	}
	raw, err := json.Marshal(g)
	if err != nil {
		return pqtype.NullRawMessage{}, fmt.Errorf("encode visible_if: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: raw, Valid: true}, nil
}

func nullPriceType(t *ProductOptionPriceType) db.NullCatalogProductOptionPriceType {
	if t == nil {
		return db.NullCatalogProductOptionPriceType{}
	}
	return db.NullCatalogProductOptionPriceType{Valid: true, CatalogProductOptionPriceType: db.CatalogProductOptionPriceType(*t)}
}
