package analytics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	signalsdb "github.com/skeletg/skelet/internal/db/signals/sqlc"
	"github.com/skeletg/skelet/internal/testdb"
)

// Gated on SIGNALS_DB_URL (disposable marker) - proves the pg archive sweep exports whole aged days and deletes exactly what it exported
// Scoped to a unique test source so it never touches the database's other rows
func TestPgSpineArchiveEventsExportsThenDeletes(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	src := "archtest-" + uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM analytics_events WHERE source = $1`, src) })

	now := time.Now().UTC()
	seed := func(receivedAt time.Time) {
		if _, err := conn.Exec(`INSERT INTO analytics_events (event_name, occurred_at, received_at, source, properties, dedupe_key)
			VALUES ('page_view', $1, $1, $2, '{"page_key":"/t"}', $3)`, receivedAt, src, "archtest:"+uuid.NewString()); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	for range 3 {
		seed(now.AddDate(0, 0, -10))
	}
	for range 2 {
		seed(now.AddDate(0, 0, -9))
	}
	seed(now.Add(-time.Hour)) // its day hasn't fully aged, so it must survive the sweep

	sink := newFakeSink()
	rec := NewRecorder(nil, signalsdb.New(conn))
	rec.SetArchiveSink(sink)

	n, err := rec.ArchiveEventsBySource(ctx, src, now.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n != 5 {
		t.Fatalf("archived %d rows, want 5", n)
	}
	var archived int
	for key, data := range sink.objects {
		events := decodeArchive(t, data)
		archived += len(events)
		for _, e := range events {
			if e.Source != src || string(e.Properties) != `{"page_key":"/t"}` {
				t.Fatalf("object %s carries a drifted row: %+v", key, e)
			}
		}
	}
	if len(sink.objects) != 2 || archived != 5 {
		t.Fatalf("stored %d objects with %d rows, want 2 with 5: %v", len(sink.objects), archived, keys(sink))
	}
	var left int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM analytics_events WHERE source = $1`, src).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Fatalf("%d rows left, want just the un-aged one", left)
	}
}

// A refusing destination must abort the sweep with every row intact - archive mode is fail-closed
func TestPgSpineArchiveEventsKeepsRowsWhenSinkFails(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	src := "archtest-" + uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM analytics_events WHERE source = $1`, src) })

	old := time.Now().UTC().AddDate(0, 0, -10)
	if _, err := conn.Exec(`INSERT INTO analytics_events (event_name, occurred_at, received_at, source, properties, dedupe_key)
		VALUES ('page_view', $1, $1, $2, '{}', $3)`, old, src, "archtest:"+uuid.NewString()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := NewRecorder(nil, signalsdb.New(conn))
	rec.SetArchiveSink(downSink{})
	if _, err := rec.ArchiveEventsBySource(ctx, src, time.Now().UTC().AddDate(0, 0, -1)); err == nil {
		t.Fatal("archive against a dead sink must error")
	}
	var left int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM analytics_events WHERE source = $1`, src).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Fatalf("%d rows left, want 1 - a failed upload must never cost data", left)
	}
}

type downSink struct{}

func (downSink) Put(_ context.Context, _, _ string, body io.Reader) error {
	_, _ = io.Copy(io.Discard, body)
	return errors.New("sink down")
}

// Exercises the two send-log retention functions inside one rolled-back transaction, so partitions and rows are untouched afterwards (DDL is transactional in Postgres)
func TestEmailSendRetentionFunctions(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	q := signalsdb.New(tx)

	now := time.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -4, 0)
	for i := range 3 {
		if _, err := tx.Exec(`INSERT INTO email_sends (channel_id, source, email, sent_at)
			VALUES ($1, 'campaign', $2, $3)`, uuid.New(), fmt.Sprintf("t%d@example.test", i), month.AddDate(0, 0, i)); err != nil {
			t.Fatalf("seed send: %v", err)
		}
	}

	months, err := q.EmailSendMonthsBefore(ctx, month.AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("months: %v", err)
	}
	found := false
	for _, m := range months {
		if m.UTC().Format("2006-01") == month.Format("2006-01") {
			found = true
		}
	}
	if !found {
		t.Fatalf("months %v does not include the seeded %s", months, month.Format("2006-01"))
	}

	n, err := q.DropEmailSendMonth(ctx, month)
	if err != nil {
		t.Fatalf("drop: %v", err)
	}
	if n < 3 {
		t.Fatalf("dropped %d rows, want at least the 3 seeded", n)
	}
	var left int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM email_sends WHERE sent_at >= $1 AND sent_at < $2`,
		month, month.AddDate(0, 1, 0)).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d rows survived the month drop", left)
	}
}
