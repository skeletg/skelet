package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

func (r *postgresRepository) ListProductOptionRulesByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductOptionRule, error) {
	out := map[uuid.UUID][]ProductOptionRule{}
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListProductOptionRulesByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list product option rules: %w", err)
	}
	for _, row := range rows {
		rule, err := toProductOptionRule(row)
		if err != nil {
			return nil, err
		}
		out[row.ProductID.UUID] = append(out[row.ProductID.UUID], rule)
	}
	return out, nil
}

func (r *postgresRepository) ListProductOptionRulesByTemplateIDs(ctx context.Context, templateIDs []uuid.UUID) (map[uuid.UUID][]ProductOptionRule, error) {
	out := map[uuid.UUID][]ProductOptionRule{}
	if len(templateIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListProductOptionRulesByTemplateIDs(ctx, templateIDs)
	if err != nil {
		return nil, fmt.Errorf("list template option rules: %w", err)
	}
	for _, row := range rows {
		rule, err := toProductOptionRule(row)
		if err != nil {
			return nil, err
		}
		out[row.TemplateID.UUID] = append(out[row.TemplateID.UUID], rule)
	}
	return out, nil
}

func (r *postgresRepository) CreateProductOptionRule(ctx context.Context, owner OptionOwner, sortOrder int32, in ProductOptionRuleInput) (ProductOptionRule, error) {
	labels, condition, actions, err := encodeOptionRule(in)
	if err != nil {
		return ProductOptionRule{}, err
	}
	row, err := r.queries.CreateProductOptionRule(ctx, db.CreateProductOptionRuleParams{
		ProductID:  nullID(owner.ProductID),
		TemplateID: nullID(owner.TemplateID),
		SortOrder:  sortOrder,
		Labels:     labels,
		Condition:  condition,
		Actions:    actions,
	})
	if err != nil {
		return ProductOptionRule{}, fmt.Errorf("create product option rule: %w", err)
	}
	return toProductOptionRule(row)
}

func (r *postgresRepository) UpdateProductOptionRule(ctx context.Context, id uuid.UUID, owner OptionOwner, sortOrder int32, in ProductOptionRuleInput) (ProductOptionRule, error) {
	labels, condition, actions, err := encodeOptionRule(in)
	if err != nil {
		return ProductOptionRule{}, err
	}
	row, err := r.queries.UpdateProductOptionRule(ctx, db.UpdateProductOptionRuleParams{
		ID:         id,
		ProductID:  nullID(owner.ProductID),
		TemplateID: nullID(owner.TemplateID),
		SortOrder:  sortOrder,
		Labels:     labels,
		Condition:  condition,
		Actions:    actions,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductOptionRule{}, fmt.Errorf("%w: no such rule here", ErrOptionRuleInvalid)
		}
		return ProductOptionRule{}, fmt.Errorf("update product option rule: %w", err)
	}
	return toProductOptionRule(row)
}

func (r *postgresRepository) DeleteProductOptionRule(ctx context.Context, id uuid.UUID, owner OptionOwner) error {
	params := db.DeleteProductOptionRuleParams{ID: id, ProductID: nullID(owner.ProductID), TemplateID: nullID(owner.TemplateID)}
	if err := r.queries.DeleteProductOptionRule(ctx, params); err != nil {
		return fmt.Errorf("delete product option rule: %w", err)
	}
	return nil
}

func encodeOptionRule(in ProductOptionRuleInput) (labels, condition, actions json.RawMessage, err error) {
	if in.Labels == nil {
		in.Labels = map[string]string{}
	}
	if labels, err = json.Marshal(in.Labels); err != nil {
		return nil, nil, nil, fmt.Errorf("encode rule labels: %w", err)
	}
	if condition, err = json.Marshal(in.Condition); err != nil {
		return nil, nil, nil, fmt.Errorf("encode rule condition: %w", err)
	}
	if actions, err = json.Marshal(in.Actions); err != nil {
		return nil, nil, nil, fmt.Errorf("encode rule actions: %w", err)
	}
	return labels, condition, actions, nil
}

func toProductOptionRule(row db.CatalogProductOptionRule) (ProductOptionRule, error) {
	out := ProductOptionRule{
		ID:         row.ID,
		ProductID:  row.ProductID.UUID,
		TemplateID: row.TemplateID.UUID,
		SortOrder:  row.SortOrder,
		Labels:     map[string]string{},
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
	if err := json.Unmarshal(row.Labels, &out.Labels); err != nil {
		return ProductOptionRule{}, fmt.Errorf("decode rule %s labels: %w", row.ID, err)
	}
	if err := json.Unmarshal(row.Condition, &out.Condition); err != nil {
		return ProductOptionRule{}, fmt.Errorf("decode rule %s condition: %w", row.ID, err)
	}
	if err := json.Unmarshal(row.Actions, &out.Actions); err != nil {
		return ProductOptionRule{}, fmt.Errorf("decode rule %s actions: %w", row.ID, err)
	}
	return out, nil
}
