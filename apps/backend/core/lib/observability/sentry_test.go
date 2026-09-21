package observability

import (
	"context"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

func TestSentryInitWithoutDSNReturnsNoOp(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	reporter, shutdown, err := initSentry("test")
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

func TestSentryInitWithBadDSNReturnsError(t *testing.T) {
	t.Setenv("SENTRY_DSN", "not-a-valid-dsn")
	reporter, shutdown, err := initSentry("test")
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

// The graphqlx layer guards against a nil reporter, but a defensive ad-hoc call site shouldn't crash either
func TestSentryReporterNilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil-receiver Reporter calls should not panic: %v", r)
		}
	}()
	var r *sentryReporter
	r.CaptureAppError(context.Background(), apperr.New("X", apperr.ClassInternal, "boom"))
	r.CapturePanic(context.Background(), "boom")
}
