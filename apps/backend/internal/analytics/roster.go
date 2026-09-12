package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
)

// The customer roster, kept in the signals database so trait compute produces a row per customer
// without joining across - core owns customers and streams them here, and a customer gone from the
// stream is what tells the recompute to drop their traits, replacing a cascade that cannot span two databases

// RosterSync accumulates one push. Every entry is stamped with the run's start, so the sweep at the
// end can tell this run's rows from the last one's without a second pass
type RosterSync struct {
	r         *Recorder
	startedAt time.Time
	synced    int
}

// BeginRosterSync opens a push - nothing is pruned until Commit, so a stream that dies partway leaves the roster as it was
func (r *Recorder) BeginRosterSync() *RosterSync {
	return &RosterSync{r: r, startedAt: time.Now()}
}

// Page upserts one batch of the roster
func (s *RosterSync) Page(ctx context.Context, customerIDs, channelIDs []uuid.UUID) error {
	if s.r.qs == nil || len(customerIDs) == 0 {
		return nil
	}
	if len(customerIDs) != len(channelIDs) {
		return fmt.Errorf("roster page: %d customers against %d channels", len(customerIDs), len(channelIDs))
	}
	if err := s.r.qs.UpsertCustomerRoster(ctx, signalsdb.UpsertCustomerRosterParams{
		CustomerIds: customerIDs, ChannelIds: channelIDs, SyncedAt: s.startedAt,
	}); err != nil {
		return fmt.Errorf("upsert roster: %w", err)
	}
	s.synced += len(customerIDs)
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
		// Their traits and memberships go with them - the FK cascades that used to do this cannot cross databases
		if _, err := s.r.qs.PruneOrphanedTraits(ctx); err != nil {
			return s.synced, int(gone), fmt.Errorf("prune orphaned traits: %w", err)
		}
		if _, err := s.r.qs.PruneOrphanedMemberships(ctx); err != nil {
			return s.synced, int(gone), fmt.Errorf("prune orphaned memberships: %w", err)
		}
	}
	return s.synced, int(gone), nil
}

// RosterChannels are the channels the roster covers, which is what the trait recompute iterates -
// a channel with no customers has nothing to recompute
func (r *Recorder) RosterChannels(ctx context.Context) ([]uuid.UUID, error) {
	if r.qs == nil {
		return nil, nil
	}
	return r.qs.RosterChannels(ctx)
}
