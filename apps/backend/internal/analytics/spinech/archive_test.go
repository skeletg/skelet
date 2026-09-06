package spinech

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
)

type fakeSink struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeSink() *fakeSink { return &fakeSink{objects: map[string][]byte{}} }

func (s *fakeSink) Put(_ context.Context, key, _ string, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = data
	return nil
}

func (s *fakeSink) rows(t *testing.T) map[string][]analytics.ArchivedEvent {
	t.Helper()
	out := map[string][]analytics.ArchivedEvent{}
	for key, data := range s.objects {
		gz, err := gzip.NewReader(strings.NewReader(string(data)))
		if err != nil {
			t.Fatalf("gzip %s: %v", key, err)
		}
		sc := bufio.NewScanner(gz)
		for sc.Scan() {
			var e analytics.ArchivedEvent
			if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
				t.Fatalf("decode %s: %v", key, err)
			}
			out[key] = append(out[key], e)
		}
	}
	return out
}

// The source-scoped archive sweep on this engine - safe against a shared database because it only ever touches its own unique source
func TestArchiveEventsBySourceCH(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	rec := analytics.NewRecorder(nil, nil)
	rec.SetSpine(st)
	sink := newFakeSink()
	rec.SetArchiveSink(sink)

	ch := uuid.New()
	src := "archtest-" + uuid.NewString()[:8]
	ev := func(at time.Time) analytics.SpineEvent {
		return analytics.SpineEvent{
			EventName: "page_view", OccurredAt: at, ChannelID: &ch, Source: src,
			Props: map[string]any{"page_key": "/t"}, DedupeKey: "archsrc:" + uuid.NewString(),
		}
	}
	now := time.Now().UTC()
	for _, e := range []analytics.SpineEvent{ev(now.AddDate(0, 0, -10)), ev(now.AddDate(0, 0, -10)), ev(now.AddDate(0, 0, -9)), ev(now)} {
		if _, err := st.AppendEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}

	n, err := rec.ArchiveEventsBySource(ctx, src, now.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n != 3 {
		t.Fatalf("archived %d rows, want 3", n)
	}
	files := sink.rows(t)
	var archived int
	for key, events := range files {
		if !strings.HasPrefix(key, "spine/"+src+"/") {
			t.Fatalf("unexpected key %s", key)
		}
		for _, e := range events {
			if e.Source != src || e.ChannelID == nil || *e.ChannelID != ch {
				t.Fatalf("row content drifted in %s: %+v", key, e)
			}
		}
		archived += len(events)
	}
	if len(files) != 2 || archived != 3 {
		t.Fatalf("stored %d files with %d rows, want 2 with 3", len(files), archived)
	}
	var left uint64
	if err := st.conn.QueryRow(ctx, "SELECT count() FROM analytics_events FINAL WHERE source = ?", src).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Fatalf("%d rows left for %s, want just the fresh one", left, src)
	}
}

// The global sweep's partition-drop path, in a throwaway database so dropping real partitions can't touch shared data
func TestArchiveEventsDropsPartitionsCH(t *testing.T) {
	dsn := os.Getenv("SIGNALS_CH_TEST_DSN")
	if dsn == "" {
		t.Skip("set SIGNALS_CH_TEST_DSN")
	}
	ctx := context.Background()
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := clickhouse.Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	const testDB = "skelet_archtest"
	if err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+testDB); err != nil {
		t.Fatal(err)
	}
	if err := admin.Exec(ctx, "CREATE DATABASE "+testDB); err != nil {
		t.Skipf("cannot create a throwaway database (%v)", err)
	}
	t.Cleanup(func() { _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+testDB) })

	// the real schema, straight from the migration - the file keeps semicolons out of comments exactly so it splits like this
	ddl, err := os.ReadFile("migrations/000001_spine.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	dbOpts := *opts
	dbOpts.Auth.Database = testDB
	raw, err := clickhouse.Open(&dbOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for stmt := range strings.SplitSeq(string(ddl), ";") {
		if strings.TrimSpace(stripComments(stmt)) == "" {
			continue
		}
		if err := raw.Exec(ctx, stmt); err != nil {
			t.Fatalf("apply schema: %v", err)
		}
	}

	st, err := New(ctx, strings.Replace(dsn, "/"+opts.Auth.Database, "/"+testDB, 1))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rec := analytics.NewRecorder(nil, nil)
	rec.SetSpine(st)
	sink := newFakeSink()
	rec.SetArchiveSink(sink)

	ch := uuid.New()
	now := time.Now().UTC()
	ev := func(source string, at time.Time) analytics.SpineEvent {
		return analytics.SpineEvent{
			EventName: "page_view", OccurredAt: at, ChannelID: &ch, Source: source,
			Props: map[string]any{}, DedupeKey: "archpart:" + uuid.NewString(),
		}
	}
	for _, e := range []analytics.SpineEvent{
		ev("web", now.AddDate(0, 0, -10)), ev("commerce", now.AddDate(0, 0, -10)),
		ev("commerce", now.AddDate(0, 0, -9)), ev("web", now),
	} {
		if _, err := st.AppendEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}

	n, err := rec.ArchivePrune(ctx, now.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n != 3 {
		t.Fatalf("archived %d rows, want 3", n)
	}
	var archived int
	for _, events := range sink.rows(t) {
		archived += len(events)
	}
	if len(sink.objects) != 2 || archived != 3 {
		t.Fatalf("stored %d files with %d rows, want one per dropped day partition", len(sink.objects), archived)
	}
	var staleParts, left uint64
	if err := st.conn.QueryRow(ctx, `SELECT count() FROM system.parts WHERE database = currentDatabase()
		AND table = 'analytics_events' AND active AND toDate(partition) < toDate(?)`, now.AddDate(0, 0, -1)).Scan(&staleParts); err != nil {
		t.Fatal(err)
	}
	if staleParts != 0 {
		t.Fatalf("%d stale partitions survived the archive sweep", staleParts)
	}
	if err := st.conn.QueryRow(ctx, "SELECT count() FROM analytics_events FINAL").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Fatalf("%d rows left, want just the fresh one", left)
	}
}

func stripComments(stmt string) string {
	var out []string
	for line := range strings.SplitSeq(stmt, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
