package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

// The customer roster, copied into the signals database so trait compute produces a row per customer without joining across - a bulk sync rather than a spine projection, since a roster is current state and deriving it from events loses a customer on any gap
const rosterPage = 5000

// SyncRoster copies every customer and their resolved lifecycle override then drops the ones that went away, and runs before the recompute since a customer missing here gets no trait row
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
		newDays := make([]int32, len(rows))
		activeDays := make([]int32, len(rows))
		atRiskDays := make([]int32, len(rows))
		for i, c := range rows {
			ids[i], channels[i] = c.CustomerID, c.ChannelID
			// 0 stands in for absent, matching the recompute's COALESCE onto the channel default
			newDays[i], activeDays[i], atRiskDays[i] = nullInt(c.NewDays), nullInt(c.ActiveDays), nullInt(c.AtRiskDays)
		}
		if err := r.qs.UpsertCustomerRoster(ctx, signalsdb.UpsertCustomerRosterParams{
			CustomerIds: ids, ChannelIds: channels,
			NewDays: newDays, ActiveDays: activeDays, AtRiskDays: atRiskDays, SyncedAt: startedAt,
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
		// Their traits go with them - the FK cascade that used to do this cannot cross databases
		if _, err := r.qs.PruneOrphanedTraits(ctx); err != nil {
			return fmt.Errorf("prune orphaned traits: %w", err)
		}
	}
	return nil
}

func nullInt(n sql.NullInt32) int32 {
	if !n.Valid {
		return 0
	}
	return n.Int32
}
