package segment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// The public surface for customer segments - a thin wrapper over Repository today, existing so future schedule-driven refresh logic lands in one place without ripping up call sites
type Service struct {
	repo Repository
	tx   Transactor
	bus  Publisher
	// The signals database, for the compiled rules query (rules.go) - the same one the repository writes memberships to,
	// since everything a rule reads is a projection the suite already holds
	dbc *sql.DB
	// The spine, for the one thing those projections cannot answer: orders inside a window (orders.go)
	orders WindowedOrders
}

func (s *Service) SetEventBus(bus Publisher) { s.bus = bus }

// Transactor runs a membership swap in one transaction, on whatever pool the segments live on
type Transactor interface {
	InTransaction(ctx context.Context, fn func(*sql.Tx) error) error
}

// Event is the shape a bus takes, named here so this package needs no vocabulary from one
type Event interface{ Type() string }

// Publisher carries a segment refresh out to whatever is listening - optional, nil publishes nothing
type Publisher interface {
	Publish(ctx context.Context, e Event)
}

func NewService(repo Repository, txMgr Transactor, dbc *sql.DB, orders WindowedOrders) *Service {
	return &Service{repo: repo, tx: txMgr, dbc: dbc, orders: orders}
}

// Atomically swaps a segment's membership for the given customer set - clear and re-add run in one transaction, so concurrent readers like campaign materialisation and the flow engine never see the empty in-between, and a failed refresh can't leave the segment drained
func (s *Service) ReplaceMembership(ctx context.Context, segmentID uuid.UUID, customerIDs []uuid.UUID) error {
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		if err := r.ClearMembership(ctx, segmentID); err != nil {
			return err
		}
		if len(customerIDs) == 0 {
			return nil
		}
		return r.BulkAddMembers(ctx, segmentID, customerIDs)
	})
}

func (s *Service) Repo() Repository { return s.repo }

func (s *Service) MemberIDs(ctx context.Context, segmentID uuid.UUID) ([]uuid.UUID, error) {
	return s.repo.MemberIDs(ctx, segmentID)
}

// The admin-mutable fields - Slug must be unique, and it doubles as the engine's customer-group token for any rule using core.customer_in_segment
type CreateInput struct {
	Slug        string
	Name        string
	Description string
	RefreshSQL  string
	IsActive    bool
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Segment, error) {
	if in.Slug == "" || in.Name == "" {
		return Segment{}, fmt.Errorf("slug and name are required")
	}
	return s.repo.Create(ctx, Segment{
		Slug:        in.Slug,
		Name:        in.Name,
		Description: in.Description,
		RefreshSQL:  in.RefreshSQL,
		IsActive:    in.IsActive,
	})
}

type UpdateInput struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Description string
	RefreshSQL  string
	IsActive    bool
}

func (s *Service) Update(ctx context.Context, in UpdateInput) (Segment, error) {
	return s.repo.Update(ctx, Segment{
		ID:          in.ID,
		Slug:        in.Slug,
		Name:        in.Name,
		Description: in.Description,
		RefreshSQL:  in.RefreshSQL,
		IsActive:    in.IsActive,
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

// Sets, or with nil values clears, a segment's optional lifecycle-cadence override - pass a complete ascending triple to set or all nil to clear, with the resolver validating - Trait-compute reads the columns per member
func (s *Service) SetLifecycleWindows(ctx context.Context, id uuid.UUID, newDays, activeDays, atRiskDays *int32) (Segment, error) {
	return s.repo.SetLifecycleWindows(ctx, id, newDays, activeDays, atRiskDays)
}

// Upserts today's member-count snapshot - call after re-materialising membership so the growth chart accrues history
func (s *Service) RecordDailySnapshot(ctx context.Context, segmentID uuid.UUID) error {
	return s.repo.RecordDailySnapshot(ctx, segmentID)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Segment, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, limit, offset int32) ([]Segment, int32, error) {
	if limit <= 0 {
		limit = 25
	}
	return s.repo.List(ctx, limit, offset)
}

func (s *Service) AssignCustomers(ctx context.Context, segmentID uuid.UUID, customerIDs []uuid.UUID) error {
	return s.repo.BulkAddMembers(ctx, segmentID, customerIDs)
}

func (s *Service) RemoveCustomer(ctx context.Context, customerID, segmentID uuid.UUID) error {
	return s.repo.RemoveMember(ctx, customerID, segmentID)
}

func (s *Service) ClearMembership(ctx context.Context, segmentID uuid.UUID) error {
	return s.repo.ClearMembership(ctx, segmentID)
}

// channelIDs scopes the rows to what the caller may see - nil means every channel, so callers that can't prove global scope must pass an explicit list
func (s *Service) ListMembers(ctx context.Context, segmentID uuid.UUID, limit, offset int32, channelIDs []uuid.UUID) ([]Member, error) {
	return s.repo.ListMembers(ctx, segmentID, limit, offset, channelIDs)
}

func (s *Service) MemberCount(ctx context.Context, segmentID uuid.UUID) (int32, error) {
	return s.repo.CountMembers(ctx, segmentID)
}

// Member counts for many segments in one query keyed by segment id, with empty segments absent and treated as 0 - backs the segment-list resolver without an N+1
func (s *Service) MemberCounts(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int32, error) {
	return s.repo.CountMembersByIDs(ctx, ids)
}

// The id, name and slug of every active segment a customer belongs to, for the Synapse profile audiences panel
func (s *Service) SegmentsForCustomer(ctx context.Context, customerID uuid.UUID) ([]MemberSegment, error) {
	return s.repo.SegmentsForCustomer(ctx, customerID)
}

// Validates and stores a dynamic segment's condition tree - empty raw clears the rules, reverting to a static segment
func (s *Service) SetRules(ctx context.Context, id uuid.UUID, raw json.RawMessage) (Segment, error) {
	if len(raw) > 0 {
		if _, err := ParseRules(raw); err != nil {
			return Segment{}, err
		}
	}
	return s.repo.SetRules(ctx, id, raw)
}

// Recomputes one dynamic segment's membership from its rules and atomically swaps it in, returning the member count
func (s *Service) RefreshDynamic(ctx context.Context, id uuid.UUID) (int, error) {
	if s.dbc == nil {
		return 0, fmt.Errorf("dynamic segments need the signals database")
	}
	seg, err := s.repo.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	rules, err := ParseRules(seg.Rules)
	if err != nil {
		return 0, err
	}
	if rules == nil {
		return 0, fmt.Errorf("segment %q has no rules - it is a static segment", seg.Slug)
	}
	resolved, err := s.resolveWindowedConditions(ctx, rules)
	if err != nil {
		return 0, err
	}
	query, args := rules.Compile(resolved)
	rows, err := s.dbc.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("run segment rules: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var cid uuid.UUID
		if err := rows.Scan(&cid); err != nil {
			return 0, err
		}
		ids = append(ids, cid)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := s.ReplaceMembership(ctx, id, ids); err != nil {
		return 0, err
	}
	_ = s.repo.RecordDailySnapshot(ctx, id)
	if s.bus != nil {
		s.bus.Publish(ctx, &Refreshed{SegmentID: id, Slug: seg.Slug, Matched: len(ids)})
	}
	return len(ids), nil
}

// Sweeps every active rules-bearing segment - wired into the synapse-segment-refresh job alongside trait segments
func (s *Service) RefreshAllDynamic(ctx context.Context) (segments, members int, err error) {
	dyn, err := s.repo.ListDynamic(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, seg := range dyn {
		n, rerr := s.RefreshDynamic(ctx, seg.ID)
		if rerr != nil {
			// One bad segment must not starve the rest of the sweep
			err = errors.Join(err, fmt.Errorf("segment %s: %w", seg.Slug, rerr))
			continue
		}
		segments++
		members += n
	}
	return segments, members, err
}
