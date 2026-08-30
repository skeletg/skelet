package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/internal/db/sqlc"
)

// ==================== Bundle options ====================

func (r *postgresRepository) CreateBundleOption(ctx context.Context, input AddBundleOptionInput) (BundleOption, error) {
	var out BundleOption
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		row, err := q.CreateBundleOption(ctx, db.CreateBundleOptionParams{
			ProductID:     input.ProductID,
			InputType:     db.CatalogBundleInputType(input.InputType),
			IsRequired:    input.IsRequired,
			SortOrder:     input.SortOrder,
			MinSelections: input.MinSelections,
			MaxSelections: nullInt32(input.MaxSelections),
		})
		if err != nil {
			return fmt.Errorf("create bundle option: %w", err)
		}
		content, err := q.UpsertBundleOptionContent(ctx, db.UpsertBundleOptionContentParams{
			OptionID: row.ID,
			Locale:   input.Locale,
			Title:    input.Title,
		})
		if err != nil {
			return fmt.Errorf("upsert bundle option content: %w", err)
		}
		out = toBundleOption(row)
		out.Title = content.Title
		out.AllContent = []BundleOptionContent{{Locale: content.Locale, Title: content.Title}}
		return nil
	})
	if err != nil {
		return BundleOption{}, err
	}
	return out, nil
}

func (r *postgresRepository) GetBundleOptionByID(ctx context.Context, id uuid.UUID) (BundleOption, error) {
	row, err := r.queries.GetBundleOptionByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BundleOption{}, ErrBundleOptionNotFound
		}
		return BundleOption{}, fmt.Errorf("get bundle option: %w", err)
	}
	return toBundleOption(row), nil
}

func (r *postgresRepository) ListBundleOptionsByProductID(ctx context.Context, productID uuid.UUID) ([]BundleOption, error) {
	rows, err := r.queries.ListBundleOptionsByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list bundle options: %w", err)
	}
	out := make([]BundleOption, len(rows))
	for i, row := range rows {
		out[i] = toBundleOption(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateBundleOption(ctx context.Context, id uuid.UUID, inputType BundleInputType, isRequired bool, sortOrder, minSelections int32, maxSelections *int32) (BundleOption, error) {
	row, err := r.queries.UpdateBundleOption(ctx, db.UpdateBundleOptionParams{
		ID:            id,
		InputType:     db.CatalogBundleInputType(inputType),
		IsRequired:    isRequired,
		SortOrder:     sortOrder,
		MinSelections: minSelections,
		MaxSelections: nullInt32(maxSelections),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BundleOption{}, ErrBundleOptionNotFound
		}
		return BundleOption{}, fmt.Errorf("update bundle option: %w", err)
	}
	return toBundleOption(row), nil
}

func (r *postgresRepository) DeleteBundleOption(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteBundleOption(ctx, id); err != nil {
		return fmt.Errorf("delete bundle option: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpsertBundleOptionContent(ctx context.Context, input UpsertBundleOptionContentInput) (BundleOptionContent, error) {
	row, err := r.queries.UpsertBundleOptionContent(ctx, db.UpsertBundleOptionContentParams{
		OptionID: input.OptionID,
		Locale:   input.Locale,
		Title:    input.Title,
	})
	if err != nil {
		return BundleOptionContent{}, fmt.Errorf("upsert bundle option content: %w", err)
	}
	return BundleOptionContent{Locale: row.Locale, Title: row.Title}, nil
}

func (r *postgresRepository) ListBundleOptionContent(ctx context.Context, optionID uuid.UUID) ([]BundleOptionContent, error) {
	rows, err := r.queries.ListBundleOptionContent(ctx, optionID)
	if err != nil {
		return nil, fmt.Errorf("list bundle option content: %w", err)
	}
	out := make([]BundleOptionContent, len(rows))
	for i, row := range rows {
		out[i] = BundleOptionContent{Locale: row.Locale, Title: row.Title}
	}
	return out, nil
}

func (r *postgresRepository) ListBundleOptionContentByOptionIDs(ctx context.Context, optionIDs []uuid.UUID) (map[uuid.UUID][]BundleOptionContent, error) {
	if len(optionIDs) == 0 {
		return map[uuid.UUID][]BundleOptionContent{}, nil
	}
	rows, err := r.queries.ListBundleOptionContentByOptionIDs(ctx, optionIDs)
	if err != nil {
		return nil, fmt.Errorf("list bundle option content by ids: %w", err)
	}
	out := make(map[uuid.UUID][]BundleOptionContent, len(optionIDs))
	for _, row := range rows {
		out[row.OptionID] = append(out[row.OptionID], BundleOptionContent{Locale: row.Locale, Title: row.Title})
	}
	return out, nil
}

func (r *postgresRepository) DeleteBundleOptionContent(ctx context.Context, optionID uuid.UUID, locale string) error {
	if err := r.queries.DeleteBundleOptionContent(ctx, db.DeleteBundleOptionContentParams{OptionID: optionID, Locale: locale}); err != nil {
		return fmt.Errorf("delete bundle option content: %w", err)
	}
	return nil
}

// ==================== Bundle selections ====================

func (r *postgresRepository) CreateBundleSelection(ctx context.Context, input AddBundleSelectionInput) (BundleSelection, error) {
	row, err := r.queries.CreateBundleSelection(ctx, db.CreateBundleSelectionParams{
		OptionID:  input.OptionID,
		ProductID: input.ProductID,
		Quantity:  input.Quantity,
		IsDefault: input.IsDefault,
		SortOrder: input.SortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return BundleSelection{}, fmt.Errorf("selection already exists for this option/product")
		}
		return BundleSelection{}, fmt.Errorf("create bundle selection: %w", err)
	}
	return toBundleSelection(row), nil
}

func (r *postgresRepository) GetBundleSelectionByID(ctx context.Context, id uuid.UUID) (BundleSelection, error) {
	row, err := r.queries.GetBundleSelectionByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BundleSelection{}, ErrBundleSelectionNotFound
		}
		return BundleSelection{}, fmt.Errorf("get bundle selection: %w", err)
	}
	return toBundleSelection(row), nil
}

func (r *postgresRepository) ListBundleSelectionsByOptionID(ctx context.Context, optionID uuid.UUID) ([]BundleSelection, error) {
	rows, err := r.queries.ListBundleSelectionsByOptionID(ctx, optionID)
	if err != nil {
		return nil, fmt.Errorf("list bundle selections: %w", err)
	}
	out := make([]BundleSelection, len(rows))
	for i, row := range rows {
		out[i] = toBundleSelection(row)
	}
	return out, nil
}

func (r *postgresRepository) ListBundleSelectionsByOptionIDs(ctx context.Context, optionIDs []uuid.UUID) (map[uuid.UUID][]BundleSelection, error) {
	if len(optionIDs) == 0 {
		return map[uuid.UUID][]BundleSelection{}, nil
	}
	rows, err := r.queries.ListBundleSelectionsByOptionIDs(ctx, optionIDs)
	if err != nil {
		return nil, fmt.Errorf("list bundle selections by ids: %w", err)
	}
	out := make(map[uuid.UUID][]BundleSelection, len(optionIDs))
	for _, row := range rows {
		out[row.OptionID] = append(out[row.OptionID], toBundleSelection(row))
	}
	return out, nil
}

func (r *postgresRepository) ListBundleSelectionLinksByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]BundleSelectionLink, error) {
	out := make(map[uuid.UUID][]BundleSelectionLink, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListBundleSelectionLinksByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list bundle selection links: %w", err)
	}
	for _, row := range rows {
		out[row.ProductID] = append(out[row.ProductID], BundleSelectionLink{
			BundleID:  row.BundleID,
			OptionID:  row.OptionID,
			Quantity:  row.Quantity,
			IsDefault: row.IsDefault,
			SortOrder: row.SortOrder,
		})
	}
	return out, nil
}

func (r *postgresRepository) CountBundleSelectionDefaultsForOption(ctx context.Context, optionID uuid.UUID) (int64, error) {
	n, err := r.queries.CountBundleSelectionDefaultsForOption(ctx, optionID)
	if err != nil {
		return 0, fmt.Errorf("count default selections: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) UpdateBundleSelection(ctx context.Context, id uuid.UUID, quantity float64, isDefault bool, sortOrder int32) (BundleSelection, error) {
	row, err := r.queries.UpdateBundleSelection(ctx, db.UpdateBundleSelectionParams{
		ID:        id,
		Quantity:  quantity,
		IsDefault: isDefault,
		SortOrder: sortOrder,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BundleSelection{}, ErrBundleSelectionNotFound
		}
		return BundleSelection{}, fmt.Errorf("update bundle selection: %w", err)
	}
	return toBundleSelection(row), nil
}

func (r *postgresRepository) DeleteBundleSelection(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteBundleSelection(ctx, id); err != nil {
		return fmt.Errorf("delete bundle selection: %w", err)
	}
	return nil
}

// --- row mappers ---

func toBundleOption(row db.CatalogProductBundleOption) BundleOption {
	o := BundleOption{
		ID:            row.ID,
		ProductID:     row.ProductID,
		InputType:     BundleInputType(row.InputType),
		IsRequired:    row.IsRequired,
		SortOrder:     row.SortOrder,
		MinSelections: row.MinSelections,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
	if row.MaxSelections.Valid {
		v := row.MaxSelections.Int32
		o.MaxSelections = &v
	}
	return o
}

// nullInt32 converts an optional int32 to the sql.NullInt32 sqlc expects for a nullable INT column
func nullInt32(p *int32) sql.NullInt32 {
	if p == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *p, Valid: true}
}

func toBundleSelection(row db.CatalogProductBundleSelection) BundleSelection {
	return BundleSelection{
		ID:        row.ID,
		OptionID:  row.OptionID,
		ProductID: row.ProductID,
		Quantity:  row.Quantity,
		IsDefault: row.IsDefault,
		SortOrder: row.SortOrder,
	}
}
