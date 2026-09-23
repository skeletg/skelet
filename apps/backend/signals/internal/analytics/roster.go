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
const RosterVersion int32 = 3

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
	}
	if err := s.r.qs.UpsertCustomerRoster(ctx, p); err != nil {
		return fmt.Errorf("upsert roster: %w", err)
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
	gone, err := s.r.qs.PruneCustomerRoster(ctx, s.startedAt)
	if err != nil {
		return s.synced, 0, fmt.Errorf("prune roster: %w", err)
	}
	if gone > 0 {
		// Their traits and memberships go with them, since an FK cascade cannot cross databases
		if _, err := s.r.qs.PruneOrphanedTraits(ctx); err != nil {
			return s.synced, int(gone), fmt.Errorf("prune orphaned traits: %w", err)
		}
		if _, err := s.r.qs.PruneOrphanedMemberships(ctx); err != nil {
			return s.synced, int(gone), fmt.Errorf("prune orphaned memberships: %w", err)
		}
		if err := s.r.pruneOrphanedMarketing(ctx); err != nil {
			return s.synced, int(gone), err
		}
	}
	if err := s.r.qs.SetRosterFilled(ctx, RosterVersion); err != nil {
		return s.synced, int(gone), fmt.Errorf("stamp roster version: %w", err)
	}
	return s.synced, int(gone), nil
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
		Consents: json.RawMessage(consentsJSON(e.Consents)),
	})
}

// DeleteRosterEntry drops one customer ahead of the sweep, taking their traits and memberships with them
func (r *Roster) DeleteRosterEntry(ctx context.Context, customerID uuid.UUID) error {
	if r.qs == nil {
		return nil
	}
	gone, err := r.qs.DeleteCustomerRosterEntry(ctx, customerID)
	if err != nil || gone == 0 {
		return err
	}
	if _, err := r.qs.PruneOrphanedTraits(ctx); err != nil {
		return fmt.Errorf("prune orphaned traits: %w", err)
	}
	if _, err := r.qs.PruneOrphanedMemberships(ctx); err != nil {
		return fmt.Errorf("prune orphaned memberships: %w", err)
	}
	return r.pruneOrphanedMarketing(ctx)
}

// A live run or a targeted row for a customer core deleted would otherwise sit here and keep trying to reach them
func (r *Roster) pruneOrphanedMarketing(ctx context.Context) error {
	if _, err := r.qs.PruneOrphanedFlowRuns(ctx); err != nil {
		return fmt.Errorf("prune orphaned flow runs: %w", err)
	}
	if _, err := r.qs.PruneOrphanedCampaignRecipients(ctx); err != nil {
		return fmt.Errorf("prune orphaned campaign recipients: %w", err)
	}
	return nil
}

// RosterChannels are the channels the roster covers, which is what the trait recompute iterates -
// a channel with no customers has nothing to recompute
func (r *Roster) RosterChannels(ctx context.Context) ([]uuid.UUID, error) {
	if r.qs == nil {
		return nil, nil
	}
	return r.qs.RosterChannels(ctx)
}
