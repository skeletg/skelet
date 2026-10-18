package lens

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func lensDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.RequireSignals(t)
}

func signalsdbNew(conn *sql.DB) *signalsdb.Queries { return signalsdb.New(conn) }

type memBlobs struct {
	mu      sync.Mutex
	objects map[string][]byte
	puts    int
	gets    int
	failPut bool
}

func (m *memBlobs) PutBytes(_ context.Context, key string, data []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failPut {
		return errors.New("storage down")
	}
	if m.objects == nil {
		m.objects = map[string][]byte{}
	}
	m.puts++
	m.objects[key] = data
	return nil
}

func (m *memBlobs) GetBytes(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gets++
	b, ok := m.objects[key]
	if !ok {
		return nil, errors.New("no such key")
	}
	return b, nil
}

func (m *memBlobs) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func lensService(t *testing.T, conn *sql.DB, cfg Settings) (*Service, *memBlobs) {
	t.Helper()
	svc := NewService(signalsdb.New(conn), analytics.NewRecorder(nil, &fakeSpine{}), cfg, nil)
	blobs := &memBlobs{}
	svc.SetBlobStore(blobs)
	return svc, blobs
}

func rrweb(t *testing.T, at time.Time, n int) json.RawMessage {
	t.Helper()
	evs := make([]map[string]any, n)
	for i := range evs {
		evs[i] = map[string]any{"type": 3, "timestamp": at.Add(time.Duration(i) * time.Second).UnixMilli()}
	}
	raw, _ := json.Marshal(evs)
	return raw
}

func sessionRow(t *testing.T, conn *sql.DB, sid uuid.UUID) (events int64, chunks int) {
	t.Helper()
	if err := conn.QueryRow(`SELECT event_count, (SELECT count(*) FROM lens_session_chunks WHERE session_id = $1) FROM lens_sessions WHERE id = $1`, sid).Scan(&events, &chunks); err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	return events, chunks
}

func cleanupSession(t *testing.T, conn *sql.DB, sid uuid.UUID) {
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM lens_sessions WHERE id = $1`, sid) })
}

func TestARetransmittedChunkWritesNothingTwice(t *testing.T) {
	conn := lensDB(t)
	svc, blobs := lensService(t, conn, nil)
	ch, sid := uuid.New(), uuid.New()
	cleanupSession(t, conn, sid)
	b := ReplayBatch{SessionID: sid.String(), AnonID: "anon-1", Seq: 0, Page: "/", Viewport: "desktop", Events: rrweb(t, time.Now(), 3)}
	svc.IngestReplay(context.Background(), ch, "ua", b, uuid.Nil)
	b.Events = rrweb(t, time.Now(), 7)
	svc.IngestReplay(context.Background(), ch, "ua", b, uuid.Nil)
	if events, chunks := sessionRow(t, conn, sid); events != 3 || chunks != 1 {
		t.Fatalf("after a retransmit: %d events in %d chunks, want 3 in 1", events, chunks)
	}
	if blobs.puts != 1 {
		t.Errorf("the retransmit uploaded again (%d puts), replacing the stored chunk", blobs.puts)
	}
}

func TestAFlushNamingAnotherVisitorsSessionIsDropped(t *testing.T) {
	conn := lensDB(t)
	svc, _ := lensService(t, conn, nil)
	ch, sid := uuid.New(), uuid.New()
	cleanupSession(t, conn, sid)
	svc.IngestReplay(context.Background(), ch, "ua", ReplayBatch{SessionID: sid.String(), AnonID: "owner", Seq: 0, Events: rrweb(t, time.Now(), 2)}, uuid.Nil)
	svc.IngestReplay(context.Background(), ch, "ua", ReplayBatch{SessionID: sid.String(), AnonID: "intruder", Seq: 1, Events: rrweb(t, time.Now(), 2)}, uuid.Nil)
	if _, chunks := sessionRow(t, conn, sid); chunks != 1 {
		t.Fatalf("another visitor appended to the recording: %d chunks", chunks)
	}
}

func TestReplayLimitsHoldOnTheServer(t *testing.T) {
	conn := lensDB(t)
	svc, _ := lensService(t, conn, stubSettings{values: map[string]string{cfgReplayMaxMinutes: "1"}})
	ch, sid := uuid.New(), uuid.New()
	cleanupSession(t, conn, sid)
	start := time.Now().Add(-time.Hour)
	ingest := func(seq int32, at time.Time) {
		svc.IngestReplay(context.Background(), ch, strings.Repeat("ž", 300), ReplayBatch{SessionID: sid.String(), AnonID: "anon-1", Seq: seq, Events: rrweb(t, at, 1)}, uuid.Nil)
	}
	ingest(0, start)
	ingest(1, start.Add(30*time.Second))
	ingest(2, start.Add(1*time.Minute+replayGrace+time.Second))
	ingest(maxReplayChunks, start.Add(40*time.Second))
	if _, chunks := sessionRow(t, conn, sid); chunks != 2 {
		t.Fatalf("%d chunks kept, want the two inside the length cap and none past the seq bound", chunks)
	}
	var ua string
	if err := conn.QueryRow(`SELECT user_agent FROM lens_sessions WHERE id = $1`, sid).Scan(&ua); err != nil {
		t.Fatalf("a multibyte user agent cost the session its row: %v", err)
	}
	if len(ua) > maxUserAgent {
		t.Errorf("user agent kept %d bytes", len(ua))
	}
}

func TestThePlayerGetsABoundedRecording(t *testing.T) {
	conn := lensDB(t)
	svc, blobs := lensService(t, conn, nil)
	ch, sid := uuid.New(), uuid.New()
	cleanupSession(t, conn, sid)
	for seq := range int32(4) {
		svc.IngestReplay(context.Background(), ch, "ua", ReplayBatch{SessionID: sid.String(), AnonID: "anon-1", Seq: seq, Events: rrweb(t, time.Now().Add(time.Duration(seq)*time.Second), 50)}, uuid.Nil)
	}
	full, err := svc.SessionEvents(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	defer func(prev int) { maxReplayReadBytes = prev }(maxReplayReadBytes)
	maxReplayReadBytes = len(full) / 3
	blobs.gets = 0
	capped, err := svc.SessionEvents(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	var evs []json.RawMessage
	if err := json.Unmarshal(capped, &evs); err != nil || len(evs) == 0 || len(capped) >= len(full) {
		t.Fatalf("capped read = %d bytes (%d events), full = %d - want a valid, shorter recording", len(capped), len(evs), len(full))
	}
	if blobs.gets == 4 {
		t.Error("every chunk was still downloaded after the budget ran out")
	}
}

func snapshotEvent(layout, html string) Event {
	return Event{Type: "snapshot", PageKey: "/p", Viewport: "desktop", LayoutHash: layout, HTML: html, Width: 1200, Height: 900}
}

func TestAKnownLayoutsBackdropIsNeverReplaced(t *testing.T) {
	conn := lensDB(t)
	svc, blobs := lensService(t, conn, nil)
	ch := uuid.New()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM lens_page_snapshots WHERE channel_id = $1`, ch) })
	svc.Ingest(context.Background(), ch, "anon-1", analytics.Hit{}, Batch{Events: []Event{snapshotEvent("abc", `{"real":true}`)}}, uuid.Nil)
	svc.Ingest(context.Background(), ch, "intruder", analytics.Hit{}, Batch{Events: []Event{snapshotEvent("abc", `{"forged":true}`)}}, uuid.Nil)
	hm, ok, err := svc.Heatmap(context.Background(), ch, "/p", "desktop", "")
	if err != nil || !ok {
		t.Fatalf("heatmap ok=%v err=%v", ok, err)
	}
	if !strings.Contains(hm.SnapshotJSON, "real") || blobs.puts != 1 {
		t.Fatalf("backdrop = %s after %d uploads, want the first capture kept", hm.SnapshotJSON, blobs.puts)
	}

	for i := range maxSnapshotLayouts + 5 {
		svc.Ingest(context.Background(), ch, "anon-1", analytics.Hit{}, Batch{Events: []Event{snapshotEvent(fmt.Sprintf("l%d", i), `{}`)}}, uuid.Nil)
	}
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM lens_page_snapshots WHERE channel_id = $1`, ch).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != maxSnapshotLayouts {
		t.Errorf("%d layouts stored for one page state, want the %d cap", n, maxSnapshotLayouts)
	}
}

func TestAFailedUploadLeavesNoRowBehind(t *testing.T) {
	conn := lensDB(t)
	svc, blobs := lensService(t, conn, nil)
	blobs.failPut = true
	ch := uuid.New()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM lens_page_snapshots WHERE channel_id = $1`, ch) })
	svc.Ingest(context.Background(), ch, "anon-1", analytics.Hit{}, Batch{Events: []Event{snapshotEvent("abc", `{}`)}}, uuid.Nil)
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM lens_page_snapshots WHERE channel_id = $1`, ch).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a snapshot row survived its failed upload")
	}
}
