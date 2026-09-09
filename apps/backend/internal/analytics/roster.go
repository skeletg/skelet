package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

// The customer roster, copied into the signals database so trait compute produces a row per customer without joining across - a bulk sync rather than a spine projection, since a roster is current state and deriving it from events loses a customer on any gap
const rosterPage = 5000

// SyncRoster copies every customer across then drops the ones that went away, and runs before the recompute since a customer missing here gets no trait row
func (r *Recorder) SyncRoster(ctx context.Context) error {
	if r.qs == nil {
		return nil // no signals database wired, so nothing computes traits there either
	}
	// Stamped once for the whole run, so the prune can tell this sync's rows from the last one's without a second pass
	startedAt := time.Now()
	for off := int32(0); ; off += rosterPage {
		rows, err := r.read().CustomerRosterForSync(ctx, db.CustomerRosterForSyncParams{Lim: rosterPage, Off: off})
		if err != nil {
			return fmt.Errorf("read roster: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		ids := make([]uuid.UUID, len(rows))
		channels := make([]uuid.UUID, len(rows))
		for i, c := range rows {
			ids[i], channels[i] = c.CustomerID, c.ChannelID
		}
		if err := r.qs.UpsertCustomerRoster(ctx, signalsdb.UpsertCustomerRosterParams{
			CustomerIds: ids, ChannelIds: channels, SyncedAt: startedAt,
		}); err != nil {
			return fmt.Errorf("upsert roster: %w", err)
		}
		if len(rows) < rosterPage {
			break
		}
	}
	gone, err := r.qs.PruneCustomerRoster(ctx, startedAt)
	if err != nil {
		return fmt.Errorf("prune roster: %w", err)
	}
	if gone > 0 {
		// Their traits and memberships go with them - the FK cascades that used to do this cannot cross databases
		if _, err := r.qs.PruneOrphanedTraits(ctx); err != nil {
			return fmt.Errorf("prune orphaned traits: %w", err)
		}
		if _, err := r.qs.PruneOrphanedMemberships(ctx); err != nil {
			return fmt.Errorf("prune orphaned memberships: %w", err)
		}
	}
	return nil
}
