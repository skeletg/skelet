package spinech

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/chtest"
)

func TestPurgeSeedRemovesOnlySeededRowsOnTheChannel(t *testing.T) {
	ctx := context.Background()
	st, err := New(ctx, chtest.DSN(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer st.Close()

	seeded, other := uuid.New(), uuid.New()
	cust := uuid.New()
	add := func(ch uuid.UUID, key, anon string) {
		if _, err := st.AppendEvent(ctx, analytics.SpineEvent{EventName: "page_view", OccurredAt: time.Now(), ChannelID: &ch,
			AnonID: anon, Source: "web", DedupeKey: key}); err != nil {
			t.Fatal(err)
		}
		if err := st.UpsertIdentity(ctx, anon, cust, ch); err != nil {
			t.Fatal(err)
		}
	}
	add(seeded, "seed:"+seeded.String()+":1", "seed-"+uuid.NewString())
	add(seeded, "web:"+uuid.NewString(), "anon-"+uuid.NewString())    // real traffic on the seeded shop stays
	add(other, "seed:"+other.String()+":1", "seed-"+uuid.NewString()) // another shop's seed stays

	n, err := st.PurgeSeed(ctx, "seed:", "seed-", []uuid.UUID{seeded})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("purged %d events, want 1", n)
	}
	count := func(q string, args ...any) uint64 {
		var c uint64
		if err := st.conn.QueryRow(ctx, q, args...).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if c := count("SELECT count() FROM analytics_events WHERE channel_id IN ?", []uuid.UUID{seeded, other}); c != 2 {
		t.Fatalf("%d events left, want 2", c)
	}
	if c := count("SELECT count() FROM analytics_identities FINAL WHERE customer_id = ?", cust); c != 2 {
		t.Fatalf("%d identities left, want 2", c)
	}
}
