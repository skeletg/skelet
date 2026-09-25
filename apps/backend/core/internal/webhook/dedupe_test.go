package webhook

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

func TestTwoOutboxRowsOfOneShapeAreTwoDeliveries(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	ka := replayDedupeKey(events.WithOutboxID(context.Background(), a))
	if ka == "" || ka != replayDedupeKey(events.WithOutboxID(context.Background(), a)) {
		t.Fatalf("a replay of row a keyed %q, want the same non-empty key as its first delivery", ka)
	}
	if ka == replayDedupeKey(events.WithOutboxID(context.Background(), b)) {
		t.Fatal("two outbox rows share a dedupe key, so the second event's delivery is dropped")
	}
	if k := replayDedupeKey(context.Background()); k != "" {
		t.Fatalf("the fire-and-forget path keyed %q, want none", k)
	}
}
