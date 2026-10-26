package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

func nullID(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil} }

func toOptionTemplate(row db.CatalogOptionTemplate) OptionTemplate {
	return OptionTemplate{
		ID:           row.ID,
		Name:         row.Name,
		Description:  row.Description,
		CurrencyCode: row.CurrencyCode,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func templateWriteError(err error, what string) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrOptionTemplateNotFound
	case pgdb.IsUniqueViolation(err):
		return ErrOptionTemplateNameTaken
	}
	return fmt.Errorf("%s option template: %w", what, err)
}

func (r *postgresRepository) CreateOptionTemplate(ctx context.Context, in OptionTemplateInput) (OptionTemplate, error) {
	row, err := r.queries.CreateOptionTemplate(ctx, db.CreateOptionTemplateParams{Name: in.Name, Description: in.Description, CurrencyCode: in.CurrencyCode})
	if err != nil {
		return OptionTemplate{}, templateWriteError(err, "create")
	}
	return toOptionTemplate(row), nil
}

func (r *postgresRepository) GetOptionTemplate(ctx context.Context, id uuid.UUID) (OptionTemplate, error) {
	row, err := r.queries.GetOptionTemplate(ctx, id)
	if err != nil {
		return OptionTemplate{}, templateWriteError(err, "get")
	}
	return toOptionTemplate(row), nil
}

func (r *postgresRepository) UpdateOptionTemplate(ctx context.Context, id uuid.UUID, in OptionTemplateInput) (OptionTemplate, error) {
	row, err := r.queries.UpdateOptionTemplate(ctx, db.UpdateOptionTemplateParams{ID: id, Name: in.Name, Description: in.Description, CurrencyCode: in.CurrencyCode})
	if err != nil {
		return OptionTemplate{}, templateWriteError(err, "update")
	}
	return toOptionTemplate(row), nil
}

func (r *postgresRepository) DeleteOptionTemplate(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteOptionTemplate(ctx, id); err != nil {
		if pgdb.IsForeignKeyViolation(err) {
			return fmt.Errorf("%w: products or product types still use it", ErrOptionTemplateInUse)
		}
		return fmt.Errorf("delete option template: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListOptionTemplates(ctx context.Context) ([]OptionTemplate, error) {
	rows, err := r.queries.ListOptionTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list option templates: %w", err)
	}
	out := make([]OptionTemplate, len(rows))
	for i, row := range rows {
		out[i] = OptionTemplate{
			ID: row.ID, Name: row.Name, Description: row.Description, CurrencyCode: row.CurrencyCode,
			OptionCount: int(row.OptionCount), RuleCount: int(row.RuleCount), ProductCount: int(row.ProductCount),
			ProductTypeCount: int(row.ProductTypeCount), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
	}
	return out, nil
}

func (r *postgresRepository) ListOptionTemplatesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]OptionTemplate, error) {
	out := make(map[uuid.UUID]OptionTemplate, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListOptionTemplatesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list option templates by ids: %w", err)
	}
	for _, row := range rows {
		out[row.ID] = OptionTemplate{
			ID: row.ID, Name: row.Name, Description: row.Description, CurrencyCode: row.CurrencyCode,
			OptionCount: int(row.OptionCount), RuleCount: int(row.RuleCount), ProductCount: int(row.ProductCount),
			ProductTypeCount: int(row.ProductTypeCount), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
	}
	return out, nil
}

func (r *postgresRepository) ListOptionTemplateAssignments(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	out := map[uuid.UUID][]uuid.UUID{}
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListOptionTemplateAssignmentsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list option template assignments: %w", err)
	}
	for _, row := range rows {
		out[row.ProductID] = append(out[row.ProductID], row.TemplateID)
	}
	return out, nil
}

func (r *postgresRepository) ReplaceOptionTemplateAssignments(ctx context.Context, productID uuid.UUID, templateIDs []uuid.UUID) error {
	return r.atomic(ctx, func(q *db.Queries) error {
		if err := q.DeleteOptionTemplateAssignmentsForProduct(ctx, productID); err != nil {
			return fmt.Errorf("clear option template assignments: %w", err)
		}
		for i, id := range templateIDs {
			if err := q.InsertOptionTemplateAssignment(ctx, db.InsertOptionTemplateAssignmentParams{ProductID: productID, TemplateID: id, Position: int32(i)}); err != nil {
				return fmt.Errorf("assign option template: %w", err)
			}
		}
		return nil
	})
}

func (r *postgresRepository) AppendOptionTemplateAssignment(ctx context.Context, productID, templateID uuid.UUID) error {
	if err := r.queries.AppendOptionTemplateAssignment(ctx, db.AppendOptionTemplateAssignmentParams{ProductID: productID, TemplateID: templateID}); err != nil {
		return fmt.Errorf("assign option template: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteOptionTemplateAssignment(ctx context.Context, productID, templateID uuid.UUID) error {
	if err := r.queries.DeleteOptionTemplateAssignment(ctx, db.DeleteOptionTemplateAssignmentParams{ProductID: productID, TemplateID: templateID}); err != nil {
		return fmt.Errorf("unassign option template: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListProductIDsUsingOptionTemplate(ctx context.Context, templateID uuid.UUID, limit, offset int32) ([]uuid.UUID, error) {
	ids, err := r.queries.ListProductIDsUsingOptionTemplate(ctx, db.ListProductIDsUsingOptionTemplateParams{TemplateID: templateID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, fmt.Errorf("list products using option template: %w", err)
	}
	return ids, nil
}

func (r *postgresRepository) ListAllProductIDsUsingOptionTemplate(ctx context.Context, templateID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := r.queries.ListAllProductIDsUsingOptionTemplate(ctx, db.ListAllProductIDsUsingOptionTemplateParams{TemplateID: templateID, Kinds: optionCarryingKindNames()})
	if err != nil {
		return nil, fmt.Errorf("list products using option template: %w", err)
	}
	return ids, nil
}

func (r *postgresRepository) CountProductsUsingOptionTemplate(ctx context.Context, templateID uuid.UUID) (int64, error) {
	n, err := r.queries.CountProductsUsingOptionTemplate(ctx, templateID)
	if err != nil {
		return 0, fmt.Errorf("count products using option template: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) ListEffectiveOptionTemplates(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]OptionTemplateUse, error) {
	out := map[uuid.UUID][]OptionTemplateUse{}
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListEffectiveOptionTemplatesByProductIDs(ctx, db.ListEffectiveOptionTemplatesByProductIDsParams{ProductIds: productIDs, Kinds: optionCarryingKindNames()})
	if err != nil {
		return nil, fmt.Errorf("list effective option templates: %w", err)
	}
	for _, row := range rows {
		if !slices.ContainsFunc(out[row.ProductID], func(u OptionTemplateUse) bool { return u.TemplateID == row.TemplateID }) {
			out[row.ProductID] = append(out[row.ProductID], OptionTemplateUse{TemplateID: row.TemplateID, FromProductType: row.FromType})
		}
	}
	return out, nil
}

func (r *postgresRepository) ListOptionTemplateIDsByProductTypeIDs(ctx context.Context, productTypeIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	out := map[uuid.UUID][]uuid.UUID{}
	if len(productTypeIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListOptionTemplateIDsByProductTypeIDs(ctx, productTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("list product type option templates: %w", err)
	}
	for _, row := range rows {
		out[row.ProductTypeID] = append(out[row.ProductTypeID], row.TemplateID)
	}
	return out, nil
}

func (r *postgresRepository) ReplaceProductTypeOptionTemplates(ctx context.Context, productTypeID uuid.UUID, templateIDs []uuid.UUID) error {
	return r.atomic(ctx, func(q *db.Queries) error {
		if err := q.DeleteOptionTemplateAssignmentsForProductType(ctx, productTypeID); err != nil {
			return fmt.Errorf("clear product type option templates: %w", err)
		}
		for i, id := range templateIDs {
			if err := q.InsertOptionTemplateProductTypeAssignment(ctx, db.InsertOptionTemplateProductTypeAssignmentParams{ProductTypeID: productTypeID, TemplateID: id, Position: int32(i)}); err != nil {
				// Either side can go between the service's checks and this insert - say which one did
				if pgdb.IsForeignKeyViolation(err) {
					if pgdb.ConstraintName(err) == "catalog_option_template_product_types_template_id_fkey" {
						return ErrOptionTemplateNotFound
					}
					return ErrProductTypeNotFound
				}
				return fmt.Errorf("assign option template to product type: %w", err)
			}
		}
		return nil
	})
}

func (r *postgresRepository) ListProductTypeIDsUsingOptionTemplate(ctx context.Context, templateID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := r.queries.ListProductTypeIDsUsingOptionTemplate(ctx, templateID)
	if err != nil {
		return nil, fmt.Errorf("list product types using option template: %w", err)
	}
	return ids, nil
}

func (r *postgresRepository) CountProductTypesUsingOptionTemplate(ctx context.Context, templateID uuid.UUID) (int64, error) {
	n, err := r.queries.CountProductTypesUsingOptionTemplate(ctx, templateID)
	if err != nil {
		return 0, fmt.Errorf("count product types using option template: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) ListOptionCarryingProductIDsByType(ctx context.Context, productTypeID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := r.queries.ListOptionCarryingProductIDsByType(ctx, db.ListOptionCarryingProductIDsByTypeParams{ProductTypeID: productTypeID, Kinds: optionCarryingKindNames()})
	if err != nil {
		return nil, fmt.Errorf("list products of type: %w", err)
	}
	return ids, nil
}

func (r *postgresRepository) GetProductConfiguratorLayout(ctx context.Context, productID uuid.UUID) (ConfiguratorLayout, error) {
	layout, err := r.queries.GetProductConfiguratorLayout(ctx, productID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrProductNotFound
		}
		return "", fmt.Errorf("get configurator layout: %w", err)
	}
	return ConfiguratorLayout(layout), nil
}

func (r *postgresRepository) SetProductConfiguratorLayout(ctx context.Context, productID uuid.UUID, layout ConfiguratorLayout) error {
	n, err := r.queries.SetProductConfiguratorLayout(ctx, db.SetProductConfiguratorLayoutParams{ID: productID, ConfiguratorLayout: string(layout)})
	if err != nil {
		return fmt.Errorf("set configurator layout: %w", err)
	}
	if n == 0 {
		return ErrProductNotFound
	}
	return nil
}

func (r *postgresRepository) ListProductOptionsByTemplateIDs(ctx context.Context, templateIDs []uuid.UUID) (map[uuid.UUID][]ProductOption, error) {
	out := map[uuid.UUID][]ProductOption{}
	if len(templateIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListProductOptionsByTemplateIDs(ctx, templateIDs)
	if err != nil {
		return nil, fmt.Errorf("list template options: %w", err)
	}
	for _, row := range rows {
		o, err := toProductOption(row)
		if err != nil {
			return nil, err
		}
		out[row.TemplateID.UUID] = append(out[row.TemplateID.UUID], o)
	}
	return out, nil
}
