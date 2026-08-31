package sentryx_test

import (
	"context"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/sentryx"
)

func TestInitWithoutDSNReturnsNoOp(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	reporter, shutdown, err := sentryx.Init("test")
	if err != nil {
		t.Fatalf("Init with empty DSN should not error: %v", err)
	}
	if reporter != nil {
		t.Fatalf("expected nil reporter; got %v", reporter)
	}
	if shutdown == nil {
		t.Fatal("shutdown should always be non-nil (no-op when disabled)")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned %v", err)
	}
}

func TestInitWithBadDSNReturnsError(t *testing.T) {
	t.Setenv("SENTRY_DSN", "not-a-valid-dsn")
	reporter, shutdown, err := sentryx.Init("test")
	if err == nil {
		t.Fatal("Init with malformed DSN should return an error")
	}
	if reporter != nil {
		t.Fatalf("reporter should be nil after Init failure; got %v", reporter)
	}
	if shutdown == nil {
		t.Fatal("shutdown should be the no-op even on Init failure")
	}
	// no-op shutdown must still be callable without panicking
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown after failed Init returned %v", err)
	}
}

// TestReporterNilSafe documents that a nil *Reporter is safe to call The graphqlx layer guards against this, but
// defensive call sites (e.g. ad-hoc CaptureAppError in a worker) shouldn't crash either
func TestReporterNilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil-receiver Reporter calls should not panic: %v", r)
		}
	}()
	var r *sentryx.Reporter
	r.CaptureAppError(context.Background(), apperr.New("X", apperr.ClassInternal, "boom"))
	r.CapturePanic(context.Background(), "boom")
}
