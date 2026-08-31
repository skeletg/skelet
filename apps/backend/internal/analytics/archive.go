// Retention archiving: instead of deleting aged rows, the retention sweep can export them to object storage first (retention_action=archive) - identity-stripped gzip JSONL, one file per aged day (spine) or month (send log)
// Stripping customer_id / anon_id / dedupe_key / email at export time is what keeps GDPR trivial: archives hold anonymous event streams, so an erasure request never has to rewrite a cold object
package analytics

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
)

// ArchiveSink is the object-storage seam, satisfied by the BACKUP_* destination adapter in signalsboot - Put must return nil only once the object is durably stored and verified, because the sweep deletes rows on its word
type ArchiveSink interface {
	Put(ctx context.Context, key, contentType string, body io.Reader) error
}

// ArchiveOpen opens the archive file for one aged slice - drivers call it lazily on the slice's first row and must not delete anything until Close succeeds
// The first row's id rides into the object key: a crash-retry re-reads the same rows and idempotently overwrites the same key, while a rare late arrival into an already-archived day gets its own file instead of clobbering the original
type ArchiveOpen func(day time.Time, first uuid.UUID) (*ArchiveFile, error)

// ArchivedEvent is one spine event as archived - no customer_id, anon_id or dedupe_key on purpose (identify's dedupe key embeds both ids)
type ArchivedEvent struct {
	ID         uuid.UUID       `json:"id"`
	EventName  string          `json:"event_name"`
	OccurredAt time.Time       `json:"occurred_at"`
	ReceivedAt time.Time       `json:"received_at"`
	ChannelID  *uuid.UUID      `json:"channel_id,omitempty"`
	Source     string          `json:"source"`
	SendID     *uuid.UUID      `json:"send_id,omitempty"`
	Properties json.RawMessage `json:"properties,omitempty"`
}

// One send-log row as archived - customer_id, email and recipient_id stripped, keeping campaign/flow/order linkage for aggregate stats
type archivedSend struct {
	ID           uuid.UUID  `json:"id"`
	ChannelID    uuid.UUID  `json:"channel_id"`
	Source       string     `json:"source"`
	CampaignID   *uuid.UUID `json:"campaign_id,omitempty"`
	Variant      string     `json:"variant,omitempty"`
	FlowID       *uuid.UUID `json:"flow_id,omitempty"`
	FlowRunID    *uuid.UUID `json:"flow_run_id,omitempty"`
	FlowNodeID   string     `json:"flow_node_id,omitempty"`
	MessageKey   string     `json:"message_key,omitempty"`
	OrderID      *uuid.UUID `json:"order_id,omitempty"`
	SentAt       time.Time  `json:"sent_at"`
	FirstOpenAt  *time.Time `json:"first_open_at,omitempty"`
	FirstClickAt *time.Time `json:"first_click_at,omitempty"`
}

const archivePageSize = 5000 // keyset page for archive exports - bounds memory while the gzip stream runs

var (
	// Fail-closed: archive mode with no destination must never fall back to plain deletion
	ErrNoArchiveSink  = errors.New("retention_action is archive but no archive destination is configured - set the BACKUP_S3_* environment variables or switch back to prune")
	errArchiveAborted = errors.New("archive export aborted")
)

// ArchiveFile streams gzip JSONL into the sink through a pipe, so a day never buffers in memory - Close commits the upload, Abort kills it without finalizing a partial object
type ArchiveFile struct {
	gz   *gzip.Writer
	pw   *io.PipeWriter
	done chan error
	enc  *json.Encoder
}

func newArchiveFile(ctx context.Context, sink ArchiveSink, key string) *ArchiveFile {
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		var err error
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					err = fmt.Errorf("archive upload panicked: %v", rec)
				}
			}()
			err = sink.Put(ctx, key, "application/gzip", pr)
		}()
		pr.CloseWithError(err) // a Put that returns early (failed upload) must unblock the writer side, not deadlock it
		done <- err
	}()
	gz := gzip.NewWriter(pw)
	return &ArchiveFile{gz: gz, pw: pw, done: done, enc: json.NewEncoder(gz)}
}

func (f *ArchiveFile) Write(v any) error { return f.enc.Encode(v) }

func (f *ArchiveFile) Close() error {
	gzErr := f.gz.Close()
	_ = f.pw.Close()
	if err := <-f.done; err != nil { // the upload's verdict outranks a pipe-side symptom of the same failure
		return err
	}
	return gzErr
}

func (f *ArchiveFile) Abort() {
	f.pw.CloseWithError(errArchiveAborted)
	<-f.done // reap the upload goroutine so an abort can't leak it
}

// SetArchiveSink wires the retention archive destination - unset leaves archive mode failing closed
func (r *Recorder) SetArchiveSink(s ArchiveSink) { r.archive = s }

func (r *Recorder) openArchive(ctx context.Context, dir string) ArchiveOpen {
	return func(day time.Time, first uuid.UUID) (*ArchiveFile, error) {
		key := dir + "/" + day.UTC().Format("2006-01-02") + "-" + first.String()[:8] + ".jsonl.gz"
		return newArchiveFile(ctx, r.archive, key), nil
	}
}

// ArchivePrune is Prune's archive twin: aged event days are exported to the sink before their rows delete, while identity mappings still just die - they're pure PII linkage, exactly what archives strip
func (r *Recorder) ArchivePrune(ctx context.Context, before time.Time) (int64, error) {
	if r.archive == nil {
		return 0, ErrNoArchiveSink
	}
	if n, err := r.spine.PruneIdentities(ctx, before); err != nil {
		log.Printf("[analytics] prune identities: %v", err)
	} else if n > 0 {
		log.Printf("[analytics] pruned %d stale identity mappings", n)
	}
	return r.spine.ArchiveEvents(ctx, "", before, r.openArchive(ctx, "spine"))
}

// ArchiveEventsBySource is PruneEventsBySource's archive twin, for the shorter Lens window
func (r *Recorder) ArchiveEventsBySource(ctx context.Context, source string, before time.Time) (int64, error) {
	if r.archive == nil {
		return 0, ErrNoArchiveSink
	}
	return r.spine.ArchiveEvents(ctx, source, before, r.openArchive(ctx, "spine/"+source))
}

// PruneEmailSends ages out whole months of the send log - the one signals table nothing pruned before - archiving each month first when asked
// before should sit on a month boundary; email_sends lives on signals Postgres under every engine, so this goes through r.qs not the spine driver
func (r *Recorder) PruneEmailSends(ctx context.Context, before time.Time, archive bool) (int64, error) {
	if archive && r.archive == nil {
		return 0, ErrNoArchiveSink
	}
	months, err := r.qs.EmailSendMonthsBefore(ctx, before)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, m := range months {
		if archive {
			if err := r.archiveEmailSendMonth(ctx, m); err != nil {
				return total, err
			}
		}
		n, err := r.qs.DropEmailSendMonth(ctx, m)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (r *Recorder) archiveEmailSendMonth(ctx context.Context, month time.Time) error {
	var f *ArchiveFile // opened on the first row - a pre-created partition can be empty and deserves no object
	cursor := uuid.Nil
	from, to := month.UTC(), month.UTC().AddDate(0, 1, 0)
	for {
		rows, err := r.qs.ListEmailSendsForArchive(ctx, signalsdb.ListEmailSendsForArchiveParams{
			FromAt: from, ToAt: to, CursorID: cursor, Lim: archivePageSize,
		})
		if err != nil {
			abortArchive(f)
			return err
		}
		for _, row := range rows {
			if f == nil {
				f = newArchiveFile(ctx, r.archive, "email-sends/"+month.UTC().Format("2006-01")+"-"+row.ID.String()[:8]+".jsonl.gz")
			}
			if err := f.Write(archivedSend{
				ID: row.ID, ChannelID: row.ChannelID, Source: row.Source,
				CampaignID: pu(row.CampaignID), Variant: row.Variant,
				FlowID: pu(row.FlowID), FlowRunID: pu(row.FlowRunID), FlowNodeID: row.FlowNodeID,
				MessageKey: row.MessageKey, OrderID: pu(row.OrderID),
				SentAt: row.SentAt, FirstOpenAt: pt(row.FirstOpenAt), FirstClickAt: pt(row.FirstClickAt),
			}); err != nil {
				abortArchive(f)
				return err
			}
		}
		if len(rows) < archivePageSize {
			break
		}
		cursor = rows[len(rows)-1].ID
	}
	if f == nil {
		return nil
	}
	return f.Close()
}

func abortArchive(f *ArchiveFile) {
	if f != nil {
		f.Abort()
	}
}

// ArchiveEvents exports aged whole days (received_at, the pg prune axis) oldest-first, deleting each day only after its upload commits - a partial trailing day stays until it fully ages, mirroring the ClickHouse driver only releasing whole partitions
func (s *pgSpine) ArchiveEvents(ctx context.Context, source string, before time.Time, open ArchiveOpen) (int64, error) {
	var total int64
	for {
		oldest, err := s.w.OldestAnalyticsEventBefore(ctx, signalsdb.OldestAnalyticsEventBeforeParams{Before: before, Source: source})
		if errors.Is(err, sql.ErrNoRows) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
		day := oldest.UTC().Truncate(24 * time.Hour)
		dayEnd := day.Add(24 * time.Hour)
		if dayEnd.After(before) {
			return total, nil
		}
		n, err := s.archiveDay(ctx, source, day, dayEnd, open)
		if err != nil {
			return total, err
		}
		if n == 0 { // the probed day vanished under us - stop rather than spin on it
			return total, nil
		}
		total += n
	}
}

func (s *pgSpine) archiveDay(ctx context.Context, source string, day, dayEnd time.Time, open ArchiveOpen) (int64, error) {
	var f *ArchiveFile
	cursorAt, cursorID := day.Add(-time.Second), uuid.Nil
	for {
		rows, err := s.w.ListAnalyticsEventsForArchive(ctx, signalsdb.ListAnalyticsEventsForArchiveParams{
			FromAt: day, ToAt: dayEnd, Source: source, CursorAt: cursorAt, CursorID: cursorID, Lim: archivePageSize,
		})
		if err != nil {
			abortArchive(f)
			return 0, err
		}
		for _, row := range rows {
			if f == nil {
				if f, err = open(day, row.ID); err != nil {
					return 0, err
				}
			}
			if err := f.Write(ArchivedEvent{
				ID: row.ID, EventName: row.EventName, OccurredAt: row.OccurredAt, ReceivedAt: row.ReceivedAt,
				ChannelID: pu(row.ChannelID), Source: row.Source, SendID: pu(row.SendID), Properties: row.Properties,
			}); err != nil {
				abortArchive(f)
				return 0, err
			}
		}
		if len(rows) < archivePageSize {
			break
		}
		last := rows[len(rows)-1]
		cursorAt, cursorID = last.ReceivedAt, last.ID
	}
	if f == nil {
		return 0, nil
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	return s.w.PruneAnalyticsEventsRange(ctx, signalsdb.PruneAnalyticsEventsRangeParams{FromAt: day, ToAt: dayEnd, Source: source})
}

func pt(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time
	return &t
}
