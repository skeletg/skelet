package spinech

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// ArchiveEvents is PruneEvents' archive twin: every aged slice is exported through open before it's removed
// The global sweep (source ”) keeps the partition-drop economics - one file per stale day partition, dropped only after its upload commits - while a source-scoped sweep exports occurred_at day slices and finishes with the same single lightweight DELETE the prune path uses
func (s *Store) ArchiveEvents(ctx context.Context, source string, before time.Time, open analytics.ArchiveOpen) (int64, error) {
	if source == "" {
		return s.archivePartitions(ctx, before, open)
	}
	return s.archiveBySource(ctx, source, before, open)
}

func (s *Store) archivePartitions(ctx context.Context, before time.Time, open analytics.ArchiveOpen) (int64, error) {
	parts, err := s.stalePartitions(ctx, before)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, p := range parts {
		day, err := time.ParseInLocation("2006-01-02", p, time.UTC)
		if err != nil {
			return total, err
		}
		n, err := s.archiveSlice(ctx, "", day, open)
		if err != nil {
			return total, err
		}
		if err := s.conn.Exec(ctx, "ALTER TABLE analytics_events DROP PARTITION ?", p); err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (s *Store) archiveBySource(ctx context.Context, source string, before time.Time, open analytics.ArchiveOpen) (int64, error) {
	var total int64
	var archivedThrough time.Time
	for {
		var oldest time.Time
		var found uint8
		if err := s.conn.QueryRow(ctx,
			"SELECT min(occurred_at), toUInt8(count() > 0) FROM analytics_events WHERE source = ? AND occurred_at >= ? AND occurred_at < ?",
			source, archivedThrough, before,
		).Scan(&oldest, &found); err != nil {
			return total, err
		}
		if found == 0 {
			break
		}
		day := oldest.UTC().Truncate(24 * time.Hour)
		dayEnd := day.Add(24 * time.Hour)
		if dayEnd.After(before) {
			break
		}
		n, err := s.archiveSlice(ctx, source, day, open)
		if err != nil {
			return total, err
		}
		total += n
		archivedThrough = dayEnd
	}
	if archivedThrough.IsZero() {
		return 0, nil
	}
	// One DELETE for every archived day, like the prune path - per-day mutations would stack
	if err := s.conn.Exec(ctx, "DELETE FROM analytics_events WHERE source = ? AND occurred_at < ?", source, archivedThrough); err != nil {
		return total, err
	}
	return total, nil
}

// Streams one occurred_at day through the archive file - FINAL so unmerged duplicates collapse in the export exactly as reads see them
func (s *Store) archiveSlice(ctx context.Context, source string, day time.Time, open analytics.ArchiveOpen) (int64, error) {
	rows, err := s.conn.Query(ctx,
		`SELECT id, event_name, occurred_at, received_at, channel_id, source, send_id, properties
		 FROM analytics_events FINAL
		 WHERE occurred_at >= ? AND occurred_at < ? AND (? = '' OR source = ?)`,
		day, day.Add(24*time.Hour), source, source)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var f *analytics.ArchiveFile
	var n int64
	for rows.Next() {
		var (
			e                 analytics.ArchivedEvent
			channelID, sendID uuid.UUID
			props             string
		)
		if err := rows.Scan(&e.ID, &e.EventName, &e.OccurredAt, &e.ReceivedAt, &channelID, &e.Source, &sendID, &props); err != nil {
			abortArchive(f)
			return 0, err
		}
		if f == nil {
			if f, err = open(day, e.ID); err != nil {
				return 0, err
			}
		}
		if channelID != uuid.Nil { // zero uuid is this schema's "none" sentinel
			e.ChannelID = &channelID
		}
		if sendID != uuid.Nil {
			e.SendID = &sendID
		}
		e.Properties = json.RawMessage(props)
		if err := f.Write(e); err != nil {
			abortArchive(f)
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		abortArchive(f)
		return 0, err
	}
	if f == nil {
		return 0, nil
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	return n, nil
}

func abortArchive(f *analytics.ArchiveFile) {
	if f != nil {
		f.Abort()
	}
}

// The active day partitions wholly before the cutoff, shared by the prune and archive sweeps
func (s *Store) stalePartitions(ctx context.Context, before time.Time) ([]string, error) {
	parts, err := s.conn.Query(ctx, `SELECT DISTINCT partition FROM system.parts
		WHERE database = currentDatabase() AND table = 'analytics_events'
		AND active AND toDate(partition) < toDate(?) ORDER BY partition`, before)
	if err != nil {
		return nil, err
	}
	defer parts.Close()
	var names []string
	for parts.Next() {
		var p string
		if err := parts.Scan(&p); err != nil {
			return nil, err
		}
		names = append(names, p)
	}
	return names, parts.Err()
}
