package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

func (r *postgresRepository) CreateGroupedMember(ctx context.Context, input AddGroupedMemberInput) (GroupedMember, error) {
	row, err := r.queries.CreateGroupedMember(ctx, db.CreateGroupedMemberParams{
		ParentID:        input.ParentID,
		MemberID:        input.MemberID,
		DefaultQuantity: input.DefaultQuantity,
		SortOrder:       input.SortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return GroupedMember{}, ErrGroupedMemberDuplicate
		}
		return GroupedMember{}, fmt.Errorf("create grouped member: %w", err)
	}
	return toGroupedMember(row), nil
}

func (r *postgresRepository) GetGroupedMemberByID(ctx context.Context, id uuid.UUID) (GroupedMember, error) {
	row, err := r.queries.GetGroupedMemberByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GroupedMember{}, ErrGroupedMemberNotFound
		}
		return GroupedMember{}, fmt.Errorf("get grouped member: %w", err)
	}
	return toGroupedMember(row), nil
}

func (r *postgresRepository) ListGroupedMembersByParentID(ctx context.Context, parentID uuid.UUID) ([]GroupedMember, error) {
	rows, err := r.queries.ListGroupedMembersByParentID(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("list grouped members: %w", err)
	}
	out := make([]GroupedMember, len(rows))
	for i, row := range rows {
		out[i] = toGroupedMember(row)
	}
	return out, nil
}

func (r *postgresRepository) ListGroupedMembersByParentIDs(ctx context.Context, parentIDs []uuid.UUID) (map[uuid.UUID][]GroupedMember, error) {
	if len(parentIDs) == 0 {
		return map[uuid.UUID][]GroupedMember{}, nil
	}
	rows, err := r.queries.ListGroupedMembersByParentIDs(ctx, parentIDs)
	if err != nil {
		return nil, fmt.Errorf("list grouped members by ids: %w", err)
	}
	out := make(map[uuid.UUID][]GroupedMember, len(parentIDs))
	for _, row := range rows {
		out[row.ParentID] = append(out[row.ParentID], toGroupedMember(row))
	}
	return out, nil
}

func (r *postgresRepository) ListGroupedMemberLinksByMemberIDs(ctx context.Context, memberIDs []uuid.UUID) (map[uuid.UUID][]GroupedMemberLink, error) {
	out := make(map[uuid.UUID][]GroupedMemberLink, len(memberIDs))
	if len(memberIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListGroupedMemberLinksByMemberIDs(ctx, memberIDs)
	if err != nil {
		return nil, fmt.Errorf("list grouped member links: %w", err)
	}
	for _, row := range rows {
		out[row.MemberID] = append(out[row.MemberID], GroupedMemberLink{
			ParentID:        row.ParentID,
			DefaultQuantity: row.DefaultQuantity,
			SortOrder:       row.SortOrder,
		})
	}
	return out, nil
}

func (r *postgresRepository) UpdateGroupedMember(ctx context.Context, id uuid.UUID, defaultQuantity float64, sortOrder int32) (GroupedMember, error) {
	row, err := r.queries.UpdateGroupedMember(ctx, db.UpdateGroupedMemberParams{
		ID:              id,
		DefaultQuantity: defaultQuantity,
		SortOrder:       sortOrder,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GroupedMember{}, ErrGroupedMemberNotFound
		}
		return GroupedMember{}, fmt.Errorf("update grouped member: %w", err)
	}
	return toGroupedMember(row), nil
}

func (r *postgresRepository) DeleteGroupedMember(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteGroupedMember(ctx, id); err != nil {
		return fmt.Errorf("delete grouped member: %w", err)
	}
	return nil
}

func toGroupedMember(row db.CatalogProductGroupedMember) GroupedMember {
	return GroupedMember{
		ID:              row.ID,
		ParentID:        row.ParentID,
		MemberID:        row.MemberID,
		DefaultQuantity: row.DefaultQuantity,
		SortOrder:       row.SortOrder,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}
