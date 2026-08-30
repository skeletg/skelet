package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sqlc-dev/pqtype"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/internal/db/sqlc"
)

// ==================== Product options ====================

func (r *postgresRepository) CreateProductOption(ctx context.Context, input AddProductOptionInput) (ProductOption, error) {
	var out ProductOption
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		row, err := q.CreateProductOption(ctx, db.CreateProductOptionParams{
			ProductID:      input.ProductID,
			InputType:      db.CatalogProductOptionInputType(input.InputType),
			IsRequired:     input.IsRequired,
			SortOrder:      input.SortOrder,
			PriceValue:     nullInt32FromPtr(input.PriceValue),
			PriceType:      nullPriceType(input.PriceType),
			MaxChars:       nullInt32FromPtr(input.MaxChars),
			MaxFileBytes:   nullInt64FromPtr(input.MaxFileBytes),
			FileExtensions: nullStringFromStr(input.FileExtensions),
			VisibleIf:      nullRawFromGroup(input.VisibleIf),
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
		out = toProductOption(row)
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
	return toProductOption(row), nil
}

func (r *postgresRepository) ListProductOptionsByProductID(ctx context.Context, productID uuid.UUID) ([]ProductOption, error) {
	rows, err := r.queries.ListProductOptionsByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list product options: %w", err)
	}
	out := make([]ProductOption, len(rows))
	for i, row := range rows {
		out[i] = toProductOption(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateProductOption(ctx context.Context, id uuid.UUID, params UpdateProductOptionFields) (ProductOption, error) {
	row, err := r.queries.UpdateProductOption(ctx, db.UpdateProductOptionParams{
		ID:             id,
		InputType:      db.CatalogProductOptionInputType(params.InputType),
		IsRequired:     params.IsRequired,
		SortOrder:      params.SortOrder,
		PriceValue:     nullInt32FromPtr(params.PriceValue),
		PriceType:      nullPriceType(params.PriceType),
		MaxChars:       nullInt32FromPtr(params.MaxChars),
		MaxFileBytes:   nullInt64FromPtr(params.MaxFileBytes),
		FileExtensions: nullStringFromStr(params.FileExtensions),
		VisibleIf:      nullRawFromGroup(params.VisibleIf),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductOption{}, ErrProductOptionNotFound
		}
		return ProductOption{}, fmt.Errorf("update product option: %w", err)
	}
	return toProductOption(row), nil
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
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		qty := input.Quantity
		if qty < 1 {
			qty = 1
		}
		row, err := q.CreateProductOptionValue(ctx, db.CreateProductOptionValueParams{
			OptionID:        input.OptionID,
			SortOrder:       input.SortOrder,
			PriceValue:      input.PriceValue,
			PriceType:       db.CatalogProductOptionPriceType(input.PriceType),
			IsDefault:       input.IsDefault,
			SourceProductID: nullUUIDFromPtr(input.SourceProductID),
			Quantity:        qty,
			ForcedLineQty:   nullFloatFromPtr(input.ForcedLineQty),
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
	if quantity < 1 {
		quantity = 1
	}
	row, err := r.queries.UpdateProductOptionValue(ctx, db.UpdateProductOptionValueParams{
		ID:              id,
		SortOrder:       sortOrder,
		PriceValue:      priceValue,
		PriceType:       db.CatalogProductOptionPriceType(priceType),
		IsDefault:       isDefault,
		SourceProductID: nullUUIDFromPtr(sourceProductID),
		Quantity:        quantity,
		ForcedLineQty:   nullFloatFromPtr(forcedLineQty),
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

func toProductOption(row db.CatalogProductOption) ProductOption {
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
		if err := json.Unmarshal(row.VisibleIf.RawMessage, &g); err == nil {
			out.VisibleIf = &g
		}
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
	return out
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

// nullFloatFromPtr maps nil -> SQL NULL, else the pointed-at value
func nullFloatFromPtr(f *float64) sql.NullFloat64 {
	if f == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *f, Valid: true}
}

// --- null-helpers (local to options for now) ---

// nullRawFromGroup marshals a visibility condition to JSONB (NULL when nil)
func nullRawFromGroup(g *OptionConditionGroup) pqtype.NullRawMessage {
	if g == nil {
		return pqtype.NullRawMessage{}
	}
	raw, err := json.Marshal(g)
	if err != nil {
		return pqtype.NullRawMessage{}
	}
	return pqtype.NullRawMessage{RawMessage: raw, Valid: true}
}

func nullInt32FromPtr(p *int32) sql.NullInt32 {
	if p == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Valid: true, Int32: *p}
}

func nullInt64FromPtr(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Valid: true, Int64: *p}
}

func nullStringFromStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{Valid: true, String: s}
}

func nullUUIDFromPtr(p *uuid.UUID) uuid.NullUUID {
	if p == nil || *p == (uuid.UUID{}) {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{Valid: true, UUID: *p}
}

func nullPriceType(t *ProductOptionPriceType) db.NullCatalogProductOptionPriceType {
	if t == nil {
		return db.NullCatalogProductOptionPriceType{}
	}
	return db.NullCatalogProductOptionPriceType{Valid: true, CatalogProductOptionPriceType: db.CatalogProductOptionPriceType(*t)}
}
