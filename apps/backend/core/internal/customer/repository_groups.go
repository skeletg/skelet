package customer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

func (r *postgresRepository) CreateGroup(ctx context.Context, input CreateCustomerGroupInput) (CustomerGroup, error) {
	row, err := r.queries.CreateCustomerGroup(ctx, db.CreateCustomerGroupParams{
		Name:        input.Name,
		Slug:        input.Slug,
		Description: input.Description,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return CustomerGroup{}, ErrGroupSlugTaken
		}
		return CustomerGroup{}, fmt.Errorf("create customer group: %w", err)
	}
	return toCustomerGroup(row), nil
}

func (r *postgresRepository) UpdateGroup(ctx context.Context, id uuid.UUID, input UpdateCustomerGroupInput) (CustomerGroup, error) {
	current, err := r.GetGroupByID(ctx, id)
	if err != nil {
		return CustomerGroup{}, err
	}
	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}
	description := current.Description
	if input.Description != nil {
		description = *input.Description
	}
	row, err := r.queries.UpdateCustomerGroup(ctx, db.UpdateCustomerGroupParams{
		ID:          id,
		Name:        name,
		Description: description,
	})
	if err != nil {
		return CustomerGroup{}, groupRowError(err, "update customer group")
	}
	return toCustomerGroup(row), nil
}

// The member count gives the clean refusal; a customer moved in after it trips the FK, which reads the same
func (r *postgresRepository) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	group, err := r.GetGroupByID(ctx, id)
	if err != nil {
		return err
	}
	if group.IsDefault {
		return ErrCannotDeleteDefaultGroup
	}
	n, err := r.queries.CountCustomerGroupMembers(ctx, id)
	if err != nil {
		return fmt.Errorf("count group members: %w", err)
	}
	if n > 0 {
		return ErrGroupInUse
	}
	return deleteGroupRow(ctx, r, id)
}

func deleteGroupRow(ctx context.Context, r *postgresRepository, id uuid.UUID) error {
	if err := r.queries.DeleteCustomerGroup(ctx, id); err != nil {
		if pgdb.IsForeignKeyViolation(err) {
			return ErrGroupInUse
		}
		return fmt.Errorf("delete customer group: %w", err)
	}
	return nil
}

// Only a missing row is ErrGroupNotFound - anything else is the database failing and says so
func groupRowError(err error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrGroupNotFound
	}
	return fmt.Errorf("%s: %w", what, err)
}

func (r *postgresRepository) GetGroupByID(ctx context.Context, id uuid.UUID) (CustomerGroup, error) {
	row, err := r.queries.GetCustomerGroupByID(ctx, id)
	if err != nil {
		return CustomerGroup{}, groupRowError(err, "get customer group")
	}
	return toCustomerGroup(row), nil
}

func (r *postgresRepository) GetGroupBySlug(ctx context.Context, slug string) (CustomerGroup, error) {
	row, err := r.queries.GetCustomerGroupBySlug(ctx, slug)
	if err != nil {
		return CustomerGroup{}, groupRowError(err, "get customer group by slug")
	}
	return toCustomerGroup(row), nil
}

func (r *postgresRepository) GetDefaultGroup(ctx context.Context) (CustomerGroup, error) {
	row, err := r.queries.GetDefaultCustomerGroup(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CustomerGroup{}, ErrNoDefaultGroup
		}
		return CustomerGroup{}, fmt.Errorf("get default group: %w", err)
	}
	return toCustomerGroup(row), nil
}

func (r *postgresRepository) ListGroups(ctx context.Context) ([]CustomerGroup, error) {
	rows, err := r.queries.ListCustomerGroupsWithMemberCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customer groups: %w", err)
	}
	out := make([]CustomerGroup, len(rows))
	for i, row := range rows {
		out[i] = CustomerGroup{
			ID:          row.ID,
			Name:        row.Name,
			Slug:        row.Slug,
			Description: row.Description,
			IsDefault:   row.IsDefault,
			CreatedAt:   row.CreatedAt,
			MemberCount: row.MemberCount,
		}
	}
	return out, nil
}

// Clears the current default and marks the new one on a transaction-scoped repository - a concurrent swap that committed
// first leaves its default where this one's clear didn't see it, and the partial unique index refuses the second default
func (r *postgresRepository) SetDefaultGroup(ctx context.Context, id uuid.UUID) (CustomerGroup, error) {
	if err := r.queries.ClearDefaultCustomerGroup(ctx); err != nil {
		return CustomerGroup{}, fmt.Errorf("clear default: %w", err)
	}
	if err := r.queries.MarkCustomerGroupDefault(ctx, id); err != nil {
		if isUniqueViolation(err) {
			return CustomerGroup{}, ErrDefaultGroupConflict
		}
		return CustomerGroup{}, fmt.Errorf("mark default: %w", err)
	}
	return r.GetGroupByID(ctx, id)
}

func (r *postgresRepository) SetCustomerGroup(ctx context.Context, customerID, groupID uuid.UUID) error {
	n, err := r.queries.SetCustomerGroup(ctx, db.SetCustomerGroupParams{ID: customerID, CustomerGroupID: groupID})
	if pgdb.IsForeignKeyViolation(err) {
		return ErrGroupNotFound
	}
	return affectedCustomer(n, err, "set customer group")
}

func toCustomerGroup(row db.CustomerGroup) CustomerGroup {
	return CustomerGroup{
		ID:          row.ID,
		Name:        row.Name,
		Slug:        row.Slug,
		Description: row.Description,
		IsDefault:   row.IsDefault,
		CreatedAt:   row.CreatedAt,
	}
}
