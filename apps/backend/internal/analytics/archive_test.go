package analytics

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Collects committed objects; failKey simulates a destination outage for one key
type fakeSink struct {
	mu      sync.Mutex
	objects map[string][]byte
	failKey string
}

func newFakeSink() *fakeSink { return &fakeSink{objects: map[string][]byte{}} }

func (s *fakeSink) Put(_ context.Context, key, contentType string, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if contentType != "application/gzip" {
		return errors.New("unexpected content type " + contentType)
	}
	if key == s.failKey {
		return errors.New("sink down")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = data
	return nil
}

func decodeArchive(t *testing.T, data []byte) []ArchivedEvent {
	t.Helper()
	gz, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	var out []ArchivedEvent
	sc := bufio.NewScanner(gz)
	for sc.Scan() {
		var e ArchivedEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("decode line: %v", err)
		}
		out = append(out, e)
	}
	return out
}

func TestArchiveFileRoundTrip(t *testing.T) {
	sink := newFakeSink()
	rec := &Recorder{archive: sink}
	day := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	first := uuid.MustParse("0196b9a1-0000-7000-8000-000000000001")

	f, err := rec.openArchive(context.Background(), "spine")(day, first)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	events := []ArchivedEvent{
		{ID: first, EventName: "page_view", OccurredAt: day.Add(time.Hour), ReceivedAt: day.Add(time.Hour), Source: "web", Properties: json.RawMessage(`{"page_key":"/"}`)},
		{ID: uuid.New(), EventName: "purchase", OccurredAt: day.Add(2 * time.Hour), ReceivedAt: day.Add(2 * time.Hour), Source: "commerce"},
	}
	for _, e := range events {
		if err := f.Write(e); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	wantKey := "spine/2026-05-01-0196b9a1.jsonl.gz"
	data, ok := sink.objects[wantKey]
	if !ok {
		t.Fatalf("object %q not stored, have %v", wantKey, keys(sink))
	}
	got := decodeArchive(t, data)
	if len(got) != 2 || got[0].EventName != "page_view" || got[1].EventName != "purchase" {
		t.Fatalf("decoded %+v", got)
	}
	if got[0].ChannelID != nil || string(got[0].Properties) != `{"page_key":"/"}` {
		t.Fatalf("row content drifted: %+v", got[0])
	}
}

func TestArchiveFileCloseSurfacesSinkFailure(t *testing.T) {
	sink := newFakeSink()
	sink.failKey = "spine/2026-05-01-00000000.jsonl.gz"
	rec := &Recorder{archive: sink}
	day := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	f, err := rec.openArchive(context.Background(), "spine")(day, uuid.Nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := f.Write(ArchivedEvent{ID: uuid.New(), EventName: "page_view"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err == nil {
		t.Fatal("close must fail when the sink refused the object - the sweep deletes on its word")
	}
	if len(sink.objects) != 0 {
		t.Fatalf("failed upload stored an object: %v", keys(sink))
	}
}

func TestArchiveFileAbortDiscardsObject(t *testing.T) {
	sink := newFakeSink()
	f := newArchiveFile(context.Background(), sink, "spine/2026-05-01-deadbeef.jsonl.gz")
	if err := f.Write(ArchivedEvent{ID: uuid.New(), EventName: "page_view"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Abort() // must not hang, and nothing may commit
	if len(sink.objects) != 0 {
		t.Fatalf("aborted upload stored an object: %v", keys(sink))
	}
}

func TestArchiveModeFailsClosedWithoutSink(t *testing.T) {
	rec := NewRecorder(nil, nil)
	ctx := context.Background()
	if _, err := rec.ArchivePrune(ctx, time.Now()); !errors.Is(err, ErrNoArchiveSink) {
		t.Fatalf("ArchivePrune err = %v", err)
	}
	if _, err := rec.ArchiveEventsBySource(ctx, SourceWeb, time.Now()); !errors.Is(err, ErrNoArchiveSink) {
		t.Fatalf("ArchiveEventsBySource err = %v", err)
	}
	if _, err := rec.PruneEmailSends(ctx, time.Now(), true); !errors.Is(err, ErrNoArchiveSink) {
		t.Fatalf("PruneEmailSends err = %v", err)
	}
}

func keys(s *fakeSink) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.objects {
		out = append(out, k)
	}
	return out
}
