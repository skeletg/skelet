package lens

import (
	"context"
	"time"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// Drops web spine events, heatmap aggregates and snapshots past the cutoff - used by the lens-prune scheduler job
// archive routes the raw spine events through the retention archive before they delete; the heatmap/snapshot aggregates always just die (screenshots and binned counters, not re-ingestable events)
func (s *Service) Prune(ctx context.Context, before time.Time, archive bool) (int64, error) {
	// Delete externalized snapshot objects before their rows go, best-effort
	if s.blobs != nil {
		if keys, err := s.q.LensSnapshotBlobKeysBefore(ctx, before); err == nil {
			s.deleteObjects(ctx, keys)
		}
	}
	// web spine events age out on the engine - under ClickHouse a Postgres DELETE would silently stop enforcing this window - and so do the event-derived heatmap increments (source=lens)
	spineRetire := func(source string) func(context.Context, time.Time) (int64, error) {
		return func(ctx context.Context, before time.Time) (int64, error) {
			if archive {
				return s.rec.ArchiveEventsBySource(ctx, source, before)
			}
			return s.rec.PruneEventsBySource(ctx, source, before)
		}
	}
	var total int64
	for _, del := range []func(context.Context, time.Time) (int64, error){
		spineRetire(analytics.SourceWeb),
		spineRetire(analytics.SourceLens),
		s.q.LensPruneStaleSnapshots,
		s.q.PruneLensVisitors,
		s.q.PruneBrowseSeen,
		s.q.PruneBrowseNotified,
	} {
		n, err := del(ctx, before)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// PruneRollups ages out the per-day aggregates on their own (much longer) window - they're a few rows per channel per day and what makes year-over-year trends possible after the raw events are gone, so they deliberately left Prune's shared cutoff
func (s *Service) PruneRollups(ctx context.Context, before time.Time) (int64, error) {
	var total int64
	for _, del := range []func(context.Context, time.Time) (int64, error){
		s.q.PruneTrafficRollup,
		s.q.PruneLensDaily,
	} {
		n, err := del(ctx, before)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// Separate from Prune because replay has its own shorter retention window - it's heavier and more PII-sensitive
func (s *Service) PruneReplay(ctx context.Context, before time.Time) (int64, error) {
	// Delete externalized chunk objects before the rows cascade away
	if s.blobs != nil {
		if keys, err := s.q.LensSessionBlobKeysBefore(ctx, before); err == nil {
			s.deleteObjects(ctx, keys)
		}
	}
	return s.q.LensPruneSessions(ctx, before)
}

// Best-effort - a failed delete just leaves an orphan for a later sweep or lifecycle rule to reap
func (s *Service) deleteObjects(ctx context.Context, keys []string) {
	for _, k := range keys {
		_ = s.blobs.Delete(ctx, k)
	}
}
