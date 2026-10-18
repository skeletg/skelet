package segment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// The public surface for customer segments - a thin wrapper over Repository today, existing so future schedule-driven refresh logic lands in one place without ripping up call sites
type Service struct {
	repo Repository
	tx   Transactor
	// The signals database, for the compiled rules query (rules.go) - the same one the repository writes memberships to,
	// since everything a rule reads is a projection the suite already holds
	dbc *sql.DB
	// The spine, for the one thing those projections cannot answer: orders inside a window (orders.go)
	orders WindowedOrders
}

// Transactor runs a membership swap in one transaction, on whatever pool the segments live on
type Transactor interface {
	InTransaction(ctx context.Context, fn func(*sql.Tx) error) error
}

func NewService(repo Repository, txMgr Transactor, dbc *sql.DB, orders WindowedOrders) *Service {
	return &Service{repo: repo, tx: txMgr, dbc: dbc, orders: orders}
}

// Leavers and joiners in one transaction, so readers never see a half-swapped set
// A diff, so members who stay keep their row and their added_at
func (s *Service) ReplaceMembership(ctx context.Context, segmentID uuid.UUID, customerIDs []uuid.UUID) error {
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		if err := r.RemoveMembersNotIn(ctx, segmentID, customerIDs); err != nil {
			return err
		}
		if len(customerIDs) == 0 {
			return nil
		}
		return r.BulkAddMembers(ctx, segmentID, customerIDs)
	})
}

func (s *Service) MemberIDs(ctx context.Context, segmentID uuid.UUID) ([]uuid.UUID, error) {
	return s.repo.MemberIDs(ctx, segmentID)
}

// The admin-mutable fields - Slug must be unique, and it doubles as the engine's customer-group token for any rule using core.customer_in_segment
type CreateInput struct {
	Slug        string
	Name        string
	Description string
	Definition  string
	IsActive    bool
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

func validateNames(slug, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: a name is required", ErrInvalid)
	}
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("%w: slug %q must be lowercase letters, digits and dashes, up to 64", ErrInvalid, slug)
	}
	return nil
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Segment, error) {
	if err := validateNames(in.Slug, in.Name); err != nil {
		return Segment{}, err
	}
	return s.repo.Create(ctx, Segment{
		Slug:        in.Slug,
		Name:        in.Name,
		Description: in.Description,
		Definition:  in.Definition,
		IsActive:    in.IsActive,
	})
}

type UpdateInput struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Description string
	Definition  string
	IsActive    bool
}

func (s *Service) Update(ctx context.Context, in UpdateInput) (Segment, error) {
	existing, err := s.repo.Get(ctx, in.ID)
	if err != nil {
		return Segment{}, err
	}
	if in.Slug != existing.Slug {
		return Segment{}, ErrSlugImmutable
	}
	if err := validateNames(in.Slug, in.Name); err != nil {
		return Segment{}, err
	}
	return s.repo.Update(ctx, Segment{
		ID:          in.ID,
		Slug:        in.Slug,
		Name:        in.Name,
		Description: in.Description,
		Definition:  in.Definition,
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

func (s *Service) ListVisible(ctx context.Context, scope ChannelScope, limit, offset int32) ([]Segment, int32, error) {
	if limit <= 0 {
		limit = 25
	}
	return s.repo.ListVisible(ctx, scope, limit, offset)
}

func (s *Service) MemberCountsIn(ctx context.Context, ids []uuid.UUID, scope ChannelScope) (map[uuid.UUID]int32, error) {
	return s.repo.CountMembersIn(ctx, ids, scope)
}

func (s *Service) CustomerChannels(ctx context.Context, customerIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	return s.repo.CustomerChannels(ctx, customerIDs)
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
		rules, err := ParseRules(raw)
		if err != nil {
			return Segment{}, err
		}
		seg, err := s.repo.Get(ctx, id)
		if err != nil {
			return Segment{}, err
		}
		if owner, owned := seg.OwnerChannel(); owned {
			if err := rules.ValidateOwner(owner); err != nil {
				return Segment{}, fmt.Errorf("%w: %w", ErrInvalid, err)
			}
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
	if seg, err = s.fillSoleChannel(ctx, seg); err != nil {
		return 0, err
	}
	rules, err := ParseRules(seg.Rules)
	if err != nil {
		return 0, err
	}
	if rules == nil {
		return 0, fmt.Errorf("segment %q has no rules - it is a static segment", seg.Slug)
	}
	owner, owned := seg.OwnerChannel()
	if owned {
		if err := rules.ValidateOwner(owner); err != nil {
			return 0, err
		}
		rules.scopeTo(owner)
	}
	resolved, err := s.resolveWindowedConditions(ctx, rules)
	if err != nil {
		return 0, err
	}
	query, args := rules.CompileIn(resolved, owner)
	matched, err := s.swapInRuleMatches(ctx, id, query, args)
	if err != nil {
		return 0, err
	}
	if err := s.repo.RecordDailySnapshot(ctx, id); err != nil {
		log.Printf("[segment] daily snapshot %s: %v", seg.Slug, err) // it only feeds the growth chart
	}
	return matched, nil
}

// The rule query's matches land in a transaction-local table and the membership is diffed against it in SQL, so a refresh
// never carries a big segment's ids through Go and back - members who stay keep their row and their added_at
// The compiled query is the one dynamic statement Signals runs, so the statements around it are here beside it rather than in sqlc
func (s *Service) swapInRuleMatches(ctx context.Context, segmentID uuid.UUID, query string, args []any) (int, error) {
	var matched int
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		if _, err := t.ExecContext(ctx, `CREATE TEMP TABLE segment_matches (customer_id uuid PRIMARY KEY) ON COMMIT DROP`); err != nil {
			return fmt.Errorf("stage segment matches: %w", err)
		}
		if _, err := t.ExecContext(ctx, `INSERT INTO segment_matches `+query+` ON CONFLICT DO NOTHING`, args...); err != nil {
			return fmt.Errorf("run segment rules: %w", err)
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM customer_segment_memberships m WHERE m.segment_id = $1
			AND NOT EXISTS (SELECT 1 FROM segment_matches x WHERE x.customer_id = m.customer_id)`, segmentID); err != nil {
			return fmt.Errorf("drop segment leavers: %w", err)
		}
		if _, err := t.ExecContext(ctx, `INSERT INTO customer_segment_memberships (customer_id, segment_id)
			SELECT customer_id, $1 FROM segment_matches ON CONFLICT DO NOTHING`, segmentID); err != nil {
			return fmt.Errorf("add segment joiners: %w", err)
		}
		return t.QueryRowContext(ctx, `SELECT count(*) FROM segment_matches`).Scan(&matched)
	})
	return matched, err
}

// Rules saved before money needed a channel are unambiguous on a one-channel install, so they're completed and stored rather than left failing
func (s *Service) fillSoleChannel(ctx context.Context, seg Segment) (Segment, error) {
	if RulesProblem(seg.Rules) == "" {
		return seg, nil
	}
	channels, err := s.repo.RosterChannels(ctx)
	if err != nil || len(channels) != 1 {
		return seg, err
	}
	filled, changed := FillSoleChannel(seg.Rules, channels[0])
	if !changed {
		return seg, nil
	}
	ok, err := s.repo.FillRules(ctx, seg.ID, seg.Rules, filled)
	if err != nil {
		return seg, err
	}
	if !ok {
		return s.repo.Get(ctx, seg.ID)
	}
	seg.Rules = filled
	return seg, nil
}

// Sweeps every active rules-bearing segment - wired into the synapse-segment-refresh job alongside trait segments
func (s *Service) RefreshAllDynamic(ctx context.Context) (segments, members int, err error) {
	dyn, err := s.repo.ListDynamic(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, seg := range dyn {
		if ctx.Err() != nil {
			return segments, members, errors.Join(err, ctx.Err())
		}
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
