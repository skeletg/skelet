package datajob

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/dataio"
)

// A store that records what it was given, so a test can assert on what actually reached it
type fakeBlobs struct {
	mu      sync.Mutex
	objects map[string][]byte
	deleted []string
	// When set, UploadStream drains its reader and then fails - the shape of a store rejecting a finished body
	failUpload error
}

func newFakeBlobs() *fakeBlobs { return &fakeBlobs{objects: map[string][]byte{}} }

func (f *fakeBlobs) UploadStream(_ context.Context, key, _ string, body io.Reader) error {
	data, err := io.ReadAll(body)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		return err // a producer that died mid-stream surfaces here, and nothing is stored
	}
	if f.failUpload != nil {
		return f.failUpload
	}
	f.objects[key] = data
	return nil
}

func (f *fakeBlobs) GetStream(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[key]
	if !ok {
		return nil, errors.New("no such object")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeBlobs) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	f.deleted = append(f.deleted, key)
	return nil
}

func (f *fakeBlobs) stored(key string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	return b, ok
}

// The regression this guards: a producer that fails mid-stream must not leave a truncated object behind that
// looks complete. CloseWithError on the pipe is what makes the reader fail rather than see a clean EOF
func TestPipeFailureNeverStoresATruncatedObject(t *testing.T) {
	blobs := newFakeBlobs()
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("sku,name\nA,first\n"))
		pw.CloseWithError(errors.New("row 2 blew up"))
	}()

	err := blobs.UploadStream(context.Background(), "k", "text/csv", pr)
	if err == nil {
		t.Fatal("a producer failing mid-stream must fail the upload, not finalise a partial body")
	}
	if _, ok := blobs.stored("k"); ok {
		t.Fatal("a truncated object was stored - it would look like a complete export")
	}
}

// A plain Close is the trap: the reader sees EOF and the store happily finalises whatever arrived
func TestPlainCloseWouldHaveStoredTheTruncatedBody(t *testing.T) {
	blobs := newFakeBlobs()
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("sku\nA\n"))
		pw.Close()
	}()
	if err := blobs.UploadStream(context.Background(), "k", "text/csv", pr); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, ok := blobs.stored("k"); !ok {
		t.Fatal("expected the body to be stored, which is exactly why the error path must use CloseWithError")
	}
}

func TestCountingReaderTalliesEverythingItPassesOn(t *testing.T) {
	body := strings.Repeat("sku,name\n", 500)
	c := &countingReader{r: strings.NewReader(body)}
	n, err := io.Copy(io.Discard, c)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if c.n != n || c.n != int64(len(body)) {
		t.Fatalf("counted %d, copied %d, want %d", c.n, n, len(body))
	}
}

func TestSummariseDedupesAndCaps(t *testing.T) {
	errs := rowErrors("bad price", "bad price", "bad price", "missing sku", "bad date", "other")
	got := summarise(errs)
	if strings.Count(got, "bad price") != 1 {
		t.Fatalf("identical messages should collapse: %q", got)
	}
	if !strings.Contains(got, "and 3 more") {
		t.Fatalf("want a tail count for what was left out: %q", got)
	}
	if summarise(nil) != "" {
		t.Fatal("no errors should summarise to nothing")
	}
}

func TestStatusTerminal(t *testing.T) {
	for _, s := range []Status{StatusSucceeded, StatusFailed, StatusCancelled} {
		if !s.Terminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	for _, s := range []Status{StatusQueued, StatusRunning} {
		if s.Terminal() {
			t.Errorf("%s should not be terminal", s)
		}
	}
}

// The row outlives its artifact, so "has a file" and "exists" are different questions
func TestHasArtifactTracksTheKeyNotTheRow(t *testing.T) {
	if (Job{ObjectKey: "data/exports/x.csv"}).HasArtifact() != true {
		t.Fatal("a job holding a key has an artifact")
	}
	if (Job{}).HasArtifact() != false {
		t.Fatal("a swept job must report no artifact rather than a broken download")
	}
}

func rowErrors(messages ...string) []dataio.RowError {
	out := make([]dataio.RowError, len(messages))
	for i, m := range messages {
		out[i] = dataio.RowError{Line: i + 2, Message: m}
	}
	return out
}
