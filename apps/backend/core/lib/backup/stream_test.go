package backup

import (
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// An upload failing without draining the pipe leaves pg_dump asleep in write() and Wait() asleep on pg_dump, forever
func TestStreamCommandUploadFailureDoesNotDeadlock(t *testing.T) {
	// An endless writer stands in for pg_dump - fills the 64KB pipe buffer instantly then blocks, exactly like the real thing
	cmd := exec.Command("sh", "-c", "cat /dev/zero")
	boom := errors.New("upload exploded before reading anything")

	type result struct{ uploadErr, runErr error }
	done := make(chan result, 1)
	go func() {
		up, run := streamCommand(cmd, func(io.Reader) error { return boom })
		done <- result{up, run}
	}()

	select {
	case res := <-done:
		if !errors.Is(res.uploadErr, boom) {
			t.Fatalf("uploadErr = %v, want the upload's own error", res.uploadErr)
		}
		if res.runErr == nil {
			t.Fatal("runErr = nil, want the killed child's exit error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("streamCommand deadlocked: upload failure did not kill/drain the child")
	}
}

func TestStreamCommandHappyPath(t *testing.T) {
	cmd := exec.Command("sh", "-c", "printf 'dump-bytes'")
	var got strings.Builder
	uploadErr, runErr := streamCommand(cmd, func(r io.Reader) error {
		_, err := io.Copy(&got, r)
		return err
	})
	if uploadErr != nil || runErr != nil {
		t.Fatalf("uploadErr=%v runErr=%v, want nil/nil", uploadErr, runErr)
	}
	if got.String() != "dump-bytes" {
		t.Fatalf("streamed %q, want %q", got.String(), "dump-bytes")
	}
}

// A child dying mid-stream EOFs the pipe, so the upload "succeeds" with truncated data - runErr is the only signal callers have to delete the object
func TestStreamCommandTruncatedStreamSurfacesAsRunErr(t *testing.T) {
	cmd := exec.Command("sh", "-c", "printf 'partial'; exit 3")
	uploadErr, runErr := streamCommand(cmd, func(r io.Reader) error {
		_, err := io.Copy(io.Discard, r)
		return err
	})
	if uploadErr != nil {
		t.Fatalf("uploadErr = %v, want nil (the stream itself read fine)", uploadErr)
	}
	if runErr == nil {
		t.Fatal("runErr = nil, want the exit-status error that flags truncation")
	}
}
