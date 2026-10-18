package lens

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func TestASignalBumpsOnlyASessionOfItsOwnChannel(t *testing.T) {
	conn := testdb.RequireSignals(t)
	qs := signalsdb.New(conn)
	ctx := context.Background()
	mine, theirs := uuid.New(), uuid.New()
	own, foreign, now := uuid.New(), uuid.New(), time.Now()
	for _, s := range []struct{ id, channel uuid.UUID }{{own, mine}, {foreign, theirs}} {
		if _, err := qs.LensUpsertSession(ctx, signalsdb.LensUpsertSessionParams{
			ID: s.id, ChannelID: s.channel, AnonID: "visitor", StartedAt: now, LastEventAt: now, EventCount: 1,
		}); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	svc := NewService(qs, nil, nil, nil)
	rage := func(session uuid.UUID) {
		svc.recordSignal(ctx, mine, Event{Name: sigRageClick, PageKey: "/", Props: map[string]any{"sessionId": session.String()}})
	}
	rageCount := func(session uuid.UUID) int {
		var n int
		if err := conn.QueryRow(`SELECT rage_count FROM lens_sessions WHERE id = $1`, session).Scan(&n); err != nil {
			t.Fatalf("read session: %v", err)
		}
		return n
	}

	rage(foreign)
	rage(own)
	if n := rageCount(foreign); n != 0 {
		t.Errorf("a signal on one channel bumped another channel's session to %d", n)
	}
	if n := rageCount(own); n != 1 {
		t.Errorf("own session rage_count = %d, want 1", n)
	}
}
