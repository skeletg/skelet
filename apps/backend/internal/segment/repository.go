package segment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/internal/db/sqlc"
)

var ErrSegmentNotFound = errors.New("segment: not found")

type Repository interface {
	Create(ctx context.Context, s Segment) (Segment, error)
	Update(ctx context.Context, s Segment) (Segment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Get(ctx context.Context, id uuid.UUID) (Segment, error)
	GetBySlug(ctx context.Context, slug string) (Segment, error)
	List(ctx context.Context, limit, offset int32) ([]Segment, int32, error)
	SetLifecycleWindows(ctx context.Context, id uuid.UUID, newDays, activeDays, atRiskDays *int32) (Segment, error)
	RecordDailySnapshot(ctx context.Context, segmentID uuid.UUID) error
	SetRules(ctx context.Context, id uuid.UUID, rules json.RawMessage) (Segment, error)
	ListDynamic(ctx context.Context) ([]Segment, error)

	AddMember(ctx context.Context, customerID, segmentID uuid.UUID) error
	BulkAddMembers(ctx context.Context, segmentID uuid.UUID, customerIDs []uuid.UUID) error
	RemoveMember(ctx context.Context, customerID, segmentID uuid.UUID) error
	ClearMembership(ctx context.Context, segmentID uuid.UUID) error
	CountMembers(ctx context.Context, segmentID uuid.UUID) (int32, error)
	CountMembersByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int32, error)
	MemberIDs(ctx context.Context, segmentID uuid.UUID) ([]uuid.UUID, error)
	ListMembers(ctx context.Context, segmentID uuid.UUID, limit, offset int32, channelIDs []uuid.UUID) ([]Member, error)

	SlugsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error)
	SegmentsForCustomer(ctx context.Context, customerID uuid.UUID) ([]MemberSegment, error)

	// WithTx returns a Repository scoped to the given transaction
	WithTx(tx *sql.Tx) Repository
}

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) WithTx(t *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(t)}
}

func (r *postgresRepository) Create(ctx context.Context, in Segment) (Segment, error) {
	row, err := r.queries.CreateCustomerSegment(ctx, db.CreateCustomerSegmentParams{
		Slug:        in.Slug,
		Name:        in.Name,
		Description: in.Description,
		RefreshSql:  in.RefreshSQL,
		IsActive:    in.IsActive,
	})
	if err != nil {
		return Segment{}, fmt.Errorf("create segment: %w", err)
	}
	return toSegment(row), nil
}

func (r *postgresRepository) Update(ctx context.Context, in Segment) (Segment, error) {
	row, err := r.queries.UpdateCustomerSegment(ctx, db.UpdateCustomerSegmentParams{
		ID:          in.ID,
		Slug:        in.Slug,
		Name:        in.Name,
		Description: in.Description,
		RefreshSql:  in.RefreshSQL,
		IsActive:    in.IsActive,
	})
	if err != nil {
		return Segment{}, fmt.Errorf("update segment: %w", err)
	}
	return toSegment(row), nil
}

func (r *postgresRepository) SetLifecycleWindows(ctx context.Context, id uuid.UUID, newDays, activeDays, atRiskDays *int32) (Segment, error) {
	row, err := r.queries.SetSegmentLifecycleWindows(ctx, db.SetSegmentLifecycleWindowsParams{
		ID:         id,
		NewDays:    int32PtrToNull(newDays),
		ActiveDays: int32PtrToNull(activeDays),
		AtRiskDays: int32PtrToNull(atRiskDays),
	})
	if err != nil {
		return Segment{}, fmt.Errorf("set segment lifecycle windows: %w", err)
	}
	return toSegment(row), nil
}

func (r *postgresRepository) RecordDailySnapshot(ctx context.Context, segmentID uuid.UUID) error {
	return r.queries.RecordSegmentSnapshot(ctx, segmentID)
}

func (r *postgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.queries.DeleteCustomerSegment(ctx, id)
}

func (r *postgresRepository) Get(ctx context.Context, id uuid.UUID) (Segment, error) {
	row, err := r.queries.GetCustomerSegment(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Segment{}, ErrSegmentNotFound
		}
		return Segment{}, fmt.Errorf("get segment: %w", err)
	}
	return toSegment(row), nil
}

func (r *postgresRepository) GetBySlug(ctx context.Context, slug string) (Segment, error) {
	row, err := r.queries.GetCustomerSegmentBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Segment{}, ErrSegmentNotFound
		}
		return Segment{}, fmt.Errorf("get segment by slug: %w", err)
	}
	return toSegment(row), nil
}

func (r *postgresRepository) List(ctx context.Context, limit, offset int32) ([]Segment, int32, error) {
	rows, err := r.queries.ListCustomerSegments(ctx, db.ListCustomerSegmentsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list segments: %w", err)
	}
	count, err := r.queries.CountCustomerSegments(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count segments: %w", err)
	}
	out := make([]Segment, len(rows))
	for i, row := range rows {
		out[i] = toSegment(row)
	}
	return out, count, nil
}

func (r *postgresRepository) AddMember(ctx context.Context, customerID, segmentID uuid.UUID) error {
	return r.queries.AddCustomerToSegment(ctx, db.AddCustomerToSegmentParams{
		CustomerID: customerID,
		SegmentID:  segmentID,
	})
}

func (r *postgresRepository) BulkAddMembers(ctx context.Context, segmentID uuid.UUID, customerIDs []uuid.UUID) error {
	if len(customerIDs) == 0 {
		return nil
	}
	return r.queries.BulkAddCustomersToSegment(ctx, db.BulkAddCustomersToSegmentParams{
		SegmentID:   segmentID,
		CustomerIds: customerIDs,
	})
}

func (r *postgresRepository) RemoveMember(ctx context.Context, customerID, segmentID uuid.UUID) error {
	return r.queries.RemoveCustomerFromSegment(ctx, db.RemoveCustomerFromSegmentParams{
		CustomerID: customerID,
		SegmentID:  segmentID,
	})
}

func (r *postgresRepository) ClearMembership(ctx context.Context, segmentID uuid.UUID) error {
	return r.queries.ClearSegmentMembership(ctx, segmentID)
}

func (r *postgresRepository) ListMembers(ctx context.Context, segmentID uuid.UUID, limit, offset int32, channelIDs []uuid.UUID) ([]Member, error) {
	rows, err := r.queries.ListSegmentMembers(ctx, db.ListSegmentMembersParams{
		SegmentID:   segmentID,
		AllChannels: len(channelIDs) == 0,
		ChannelIds:  channelIDs,
		Lim:         limit,
		Off:         offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Member, len(rows))
	for i, m := range rows {
		out[i] = Member{ID: m.ID, Email: m.Email, FirstName: m.FirstName, LastName: m.LastName, ChannelID: m.ChannelID, JoinedAt: m.AddedAt}
	}
	return out, nil
}

func (r *postgresRepository) CountMembers(ctx context.Context, segmentID uuid.UUID) (int32, error) {
	return r.queries.CountSegmentMembers(ctx, segmentID)
}

func (r *postgresRepository) MemberIDs(ctx context.Context, segmentID uuid.UUID) ([]uuid.UUID, error) {
	return r.queries.SegmentMemberCustomerIDs(ctx, segmentID)
}

func (r *postgresRepository) CountMembersByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int32, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]int32{}, nil
	}
	rows, err := r.queries.CountSegmentMembersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]int32, len(rows))
	for _, row := range rows {
		out[row.SegmentID] = row.Count
	}
	return out, nil
}

func (r *postgresRepository) SlugsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error) {
	return r.queries.SegmentSlugsForCustomer(ctx, customerID)
}

func (r *postgresRepository) SegmentsForCustomer(ctx context.Context, customerID uuid.UUID) ([]MemberSegment, error) {
	rows, err := r.queries.SegmentsForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	out := make([]MemberSegment, len(rows))
	for i, row := range rows {
		out[i] = MemberSegment{ID: row.ID, Name: row.Name, Slug: row.Slug}
	}
	return out, nil
}

func (r *postgresRepository) SetRules(ctx context.Context, id uuid.UUID, rules json.RawMessage) (Segment, error) {
	row, err := r.queries.SetSegmentRules(ctx, db.SetSegmentRulesParams{
		ID:    id,
		Rules: pqtype.NullRawMessage{RawMessage: rules, Valid: len(rules) > 0},
	})
	if err != nil {
		return Segment{}, fmt.Errorf("set segment rules: %w", err)
	}
	return toSegment(row), nil
}

func (r *postgresRepository) ListDynamic(ctx context.Context) ([]Segment, error) {
	rows, err := r.queries.ListDynamicSegments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list dynamic segments: %w", err)
	}
	out := make([]Segment, len(rows))
	for i, row := range rows {
		out[i] = toSegment(row)
	}
	return out, nil
}

func toSegment(row db.CustomerSegment) Segment {
	return Segment{
		ID:                  row.ID,
		Slug:                row.Slug,
		Name:                row.Name,
		Description:         row.Description,
		RefreshSQL:          row.RefreshSql,
		Rules:               row.Rules.RawMessage,
		IsActive:            row.IsActive,
		LifecycleNewDays:    nullToInt32Ptr(row.LifecycleNewDays),
		LifecycleActiveDays: nullToInt32Ptr(row.LifecycleActiveDays),
		LifecycleAtRiskDays: nullToInt32Ptr(row.LifecycleAtRiskDays),
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
}

func nullToInt32Ptr(n sql.NullInt32) *int32 {
	if !n.Valid {
		return nil
	}
	v := n.Int32
	return &v
}

func int32PtrToNull(p *int32) sql.NullInt32 {
	if p == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *p, Valid: true}
}
