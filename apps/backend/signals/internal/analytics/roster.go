package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// The customer roster, kept in the signals database so trait compute produces a row per customer without joining across
// Core streams customers here, and one gone from the stream is what tells the recompute to drop their traits

// RosterVersion moves with every migration that adds a roster column - a full sync stamps it, and until one has, the roster reads as behind
const RosterVersion int32 = 4

const rosterTombstoneTTL = 24 * time.Hour

type RosterStatus struct {
	Version       int32
	FilledVersion int32
	FilledAt      *time.Time
	NeedsFullSync bool
}

// Roster is the suite's copy of core's customers - what core pushes, and the sweep that drops what it stopped pushing
type Roster struct{ qs *signalsdb.Queries }

func NewRoster(qs *signalsdb.Queries) *Roster { return &Roster{qs: qs} }

// RosterEntry is one customer as core holds them - identity for the cleanup sweep, the rest for
// addressing a marketing send
type RosterEntry struct {
	CustomerID       uuid.UUID
	ChannelID        uuid.UUID
	Email            string
	FirstName        string
	Phone            string
	IsActive         bool
	GroupID          uuid.NullUUID
	GroupSlug        string
	MarketingOptedIn bool
	SMSOptedIn       bool
	Consents         map[string]bool
	ReadAt           time.Time // when core read the row, on core's clock - zero when the caller did not say
	OrderCount       int32
	MonetaryCents    int64
	FirstOrderAt     *time.Time
	LastOrderAt      *time.Time
}

func consentsJSON(m map[string]bool) string {
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// RosterSync accumulates one push. Every entry is stamped with the run's start, so the sweep at the
// end can tell this run's rows from the last one's without a second pass
type RosterSync struct {
	r         *Roster
	startedAt time.Time
	synced    int
}

// BeginRosterSync opens a push - nothing is pruned until Commit, so a stream that dies partway leaves the roster as it was
func (r *Roster) BeginRosterSync() *RosterSync {
	return &RosterSync{r: r, startedAt: time.Now()}
}

// Page upserts one batch of the roster
func (s *RosterSync) Page(ctx context.Context, entries []RosterEntry) error {
	if s.r.qs == nil || len(entries) == 0 {
		return nil
	}
	p := signalsdb.UpsertCustomerRosterParams{SyncedAt: s.startedAt}
	for _, e := range entries {
		p.CustomerIds = append(p.CustomerIds, e.CustomerID)
		p.ChannelIds = append(p.ChannelIds, e.ChannelID)
		p.Emails = append(p.Emails, e.Email)
		p.FirstNames = append(p.FirstNames, e.FirstName)
		p.Phones = append(p.Phones, e.Phone)
		p.Actives = append(p.Actives, e.IsActive)
		p.GroupIds = append(p.GroupIds, e.GroupID.UUID) // a customer with no group lands as the nil uuid, which no group matches
		p.GroupSlugs = append(p.GroupSlugs, e.GroupSlug)
		p.OptedIn = append(p.OptedIn, e.MarketingOptedIn)
		p.SmsOptedIn = append(p.SmsOptedIn, e.SMSOptedIn)
		p.Consents = append(p.Consents, consentsJSON(e.Consents))
		p.ReadAts = append(p.ReadAts, e.ReadAt)
		p.OrderCounts = append(p.OrderCounts, e.OrderCount)
		p.MonetaryCents = append(p.MonetaryCents, e.MonetaryCents)
		p.FirstOrderAts = append(p.FirstOrderAts, timeOrZero(e.FirstOrderAt)) // zero lands as NULL
		p.LastOrderAts = append(p.LastOrderAts, timeOrZero(e.LastOrderAt))
	}
	if err := s.r.qs.UpsertCustomerRoster(ctx, p); err != nil {
		return fmt.Errorf("upsert roster: %w", err)
	}
	if err := s.r.qs.TouchCustomerRoster(ctx, signalsdb.TouchCustomerRosterParams{SyncedAt: s.startedAt, CustomerIds: p.CustomerIds}); err != nil {
		return fmt.Errorf("touch roster: %w", err)
	}
	s.synced += len(entries)
	return nil
}

// Commit sweeps everything this push did not carry, and the traits and memberships that went with them
// Only the caller that saw the whole stream may call it - a partial push here empties the roster
func (s *RosterSync) Commit(ctx context.Context) (synced, pruned int, err error) {
	if s.r.qs == nil {
		return s.synced, 0, nil
	}
	goneIDs, err := s.r.qs.PruneCustomerRoster(ctx, s.startedAt)
	gone := len(goneIDs)
	if err != nil {
		return s.synced, 0, fmt.Errorf("prune roster: %w", err)
	}
	if gone > 0 {
		// Their traits and memberships go with them, since an FK cascade cannot cross databases
		if _, err := s.r.qs.PruneOrphanedTraits(ctx); err != nil {
			return s.synced, gone, fmt.Errorf("prune orphaned traits: %w", err)
		}
		if _, err := s.r.qs.PruneOrphanedMemberships(ctx); err != nil {
			return s.synced, gone, fmt.Errorf("prune orphaned memberships: %w", err)
		}
		if err := s.r.pruneOrphanedMarketing(ctx, goneIDs); err != nil {
			return s.synced, gone, err
		}
	}
	if _, err := s.r.qs.DeleteOrphanedLiveFlowRuns(ctx, s.startedAt); err != nil {
		return s.synced, gone, fmt.Errorf("prune orphaned live runs: %w", err)
	}
	if _, err := s.r.qs.DetachOrphanedPendingRecipients(ctx); err != nil {
		return s.synced, gone, fmt.Errorf("detach orphaned pending recipients: %w", err)
	}
	if _, err := s.r.qs.PruneRosterTombstones(ctx, s.startedAt.Add(-rosterTombstoneTTL)); err != nil {
		return s.synced, gone, fmt.Errorf("prune roster tombstones: %w", err)
	}
	if err := s.r.qs.SetRosterFilled(ctx, RosterVersion); err != nil {
		return s.synced, gone, fmt.Errorf("stamp roster version: %w", err)
	}
	return s.synced, gone, nil
}

func (r *Roster) RosterStatus(ctx context.Context) (RosterStatus, error) {
	st := RosterStatus{Version: RosterVersion}
	if r.qs == nil {
		return st, nil
	}
	row, err := r.qs.GetRosterState(ctx)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return st, fmt.Errorf("roster state: %w", err)
	default:
		st.FilledVersion = row.FilledVersion
		if row.FilledAt.Valid {
			t := row.FilledAt.Time
			st.FilledAt = &t
		}
	}
	st.NeedsFullSync = st.FilledVersion < RosterVersion
	return st, nil
}

// UpsertRosterEntry lands one customer outside a sweep, stamping synced_at so the next one does not prune them
func (r *Roster) UpsertRosterEntry(ctx context.Context, e RosterEntry) error {
	if r.qs == nil {
		return nil
	}
	return r.qs.UpsertCustomerRosterEntry(ctx, signalsdb.UpsertCustomerRosterEntryParams{
		CustomerID: e.CustomerID, ChannelID: e.ChannelID, Email: e.Email, FirstName: e.FirstName,
		Phone: e.Phone, IsActive: e.IsActive, CustomerGroupID: e.GroupID, CustomerGroupSlug: e.GroupSlug,
		MarketingOptedIn: e.MarketingOptedIn, SmsOptedIn: e.SMSOptedIn,
		Consents: json.RawMessage(consentsJSON(e.Consents)), ReadAt: e.ReadAt,
		OrderCount: e.OrderCount, MonetaryCents: e.MonetaryCents,
		FirstOrderAt: nullTime(e.FirstOrderAt), LastOrderAt: nullTime(e.LastOrderAt),
	})
}

// DeleteRosterEntry drops one customer ahead of the sweep, taking their traits and memberships with them
// The tombstone lands first and whether or not a row existed, so a sweep page read before the delete cannot bring them back
func (r *Roster) DeleteRosterEntry(ctx context.Context, customerID uuid.UUID) error {
	if r.qs == nil {
		return nil
	}
	if err := r.qs.TombstoneCustomerRosterEntry(ctx, customerID); err != nil {
		return fmt.Errorf("tombstone roster entry: %w", err)
	}
	if _, err := r.qs.DeleteCustomerRosterEntry(ctx, customerID); err != nil {
		return err
	}
	if err := r.qs.DeleteCustomerTraits(ctx, customerID); err != nil {
		return fmt.Errorf("delete traits: %w", err)
	}
	if err := r.qs.DeleteCustomerMemberships(ctx, customerID); err != nil {
		return fmt.Errorf("delete memberships: %w", err)
	}
	if err := r.qs.DeleteCustomerFlowRuns(ctx, customerID); err != nil {
		return fmt.Errorf("delete flow runs: %w", err)
	}
	if err := r.qs.DetachCustomerCampaignRecipients(ctx, customerID); err != nil {
		return fmt.Errorf("detach campaign recipients: %w", err)
	}
	return nil
}

// A live run for a customer core deleted would keep trying to reach them - only the ids this prune removed are touched
func (r *Roster) pruneOrphanedMarketing(ctx context.Context, customerIDs []uuid.UUID) error {
	if _, err := r.qs.DeleteFlowRunsForCustomers(ctx, customerIDs); err != nil {
		return fmt.Errorf("prune orphaned flow runs: %w", err)
	}
	if _, err := r.qs.DetachCampaignRecipientsForCustomers(ctx, customerIDs); err != nil {
		return fmt.Errorf("detach orphaned campaign recipients: %w", err)
	}
	return nil
}

func (r *Roster) RosterChannels(ctx context.Context) ([]uuid.UUID, error) {
	if r.qs == nil {
		return nil, nil
	}
	return r.qs.RosterChannels(ctx)
}

func (r *Roster) TraitComputeChannels(ctx context.Context) ([]uuid.UUID, error) {
	if r.qs == nil {
		return nil, nil
	}
	return r.qs.TraitComputeChannels(ctx)
}

func (r *Roster) MarkTraitsComputed(ctx context.Context, channelID uuid.UUID, at time.Time) error {
	if r.qs == nil {
		return nil
	}
	return r.qs.MarkTraitComputeRun(ctx, signalsdb.MarkTraitComputeRunParams{ChannelID: channelID, FinishedAt: at})
}

// An attempt counts toward staleness, so a channel that never finishes in time cannot lead every run
func (r *Roster) MarkTraitsAttempted(ctx context.Context, channelID uuid.UUID, at time.Time) error {
	if r.qs == nil {
		return nil
	}
	return r.qs.MarkTraitComputeAttempt(ctx, signalsdb.MarkTraitComputeAttemptParams{ChannelID: channelID, AttemptedAt: at})
}

// The account stays in core as a redacted anchor, so its rows here are scrubbed in place rather than deleted
func (r *Roster) EraseCustomer(ctx context.Context, customerID uuid.UUID, email string, erasedAt time.Time) error {
	if r == nil || r.qs == nil {
		return nil
	}
	if err := r.qs.ScrubErasedRosterEntry(ctx, signalsdb.ScrubErasedRosterEntryParams{
		CustomerID: customerID, ErasedAt: sql.NullTime{Time: erasedAt, Valid: !erasedAt.IsZero()},
	}); err != nil {
		return fmt.Errorf("scrub roster entry: %w", err)
	}
	if err := r.qs.ScrubErasedFlowRuns(ctx, customerID); err != nil {
		return fmt.Errorf("scrub flow runs: %w", err)
	}
	if err := r.qs.ScrubErasedCampaignRecipients(ctx, customerID); err != nil {
		return fmt.Errorf("scrub campaign recipients: %w", err)
	}
	if email != "" {
		if err := r.qs.ScrubErasedGuestFlowRuns(ctx, email); err != nil {
			return fmt.Errorf("scrub guest flow runs: %w", err)
		}
		if err := r.qs.ScrubErasedGuestCampaignRecipients(ctx, email); err != nil {
			return fmt.Errorf("scrub guest campaign recipients: %w", err)
		}
	}
	if err := r.qs.DetachErasedLensSessions(ctx, customerID); err != nil {
		return fmt.Errorf("detach lens sessions: %w", err)
	}
	return nil
}

func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
