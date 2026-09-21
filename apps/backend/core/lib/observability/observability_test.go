package observability_test

import (
	"context"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/observability"
)

func TestInitReturnsUsableShutdown(t *testing.T) {
	tel := observability.Init(context.Background(), "dev")
	if tel.Shutdown == nil {
		t.Fatal("Init returned nil Shutdown; must always return a callable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := tel.Shutdown(ctx); err != nil {
		t.Fatalf("no-op Shutdown should return nil, got %v", err)
	}
}

func TestInitWithoutSentryReturnsNilReporter(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	tel := observability.Init(context.Background(), "dev")
	if tel.Reporter != nil {
		t.Fatalf("Reporter should be nil when SENTRY_DSN is unset; got %T", tel.Reporter)
	}
}

func TestShutdownIdempotent(t *testing.T) {
	tel := observability.Init(context.Background(), "dev")
	for i := range 3 {
		if err := tel.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown call %d returned %v; should be idempotent and nil", i, err)
		}
	}
}
