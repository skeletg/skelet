package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// Gated on SIGNALS_DB_URL - an incremental refresh must land the projection on exactly what a full recompute would produce: aged-out customers deleted, fresh ones upserted, untouched ones untouched
func TestIncrementalEngagementMatchesFull(t *testing.T) {
	sconn := testdb.RequireSignals(t)

	ctx := context.Background()
	rec := NewRecorder(signalsdb.New(sconn))
	ch := uuid.New()
	agedC, freshC, stableC := uuid.New(), uuid.New(), uuid.New()
	t1 := time.Now().UTC()
	t2 := t1.Add(48 * time.Hour) // the second run happens "two days later"

	cleanup := func() {
		_, _ = sconn.Exec(`DELETE FROM analytics_events WHERE channel_id = $1`, ch)
		_, _ = sconn.Exec(`DELETE FROM customer_engagement_30d WHERE channel_id = $1`, ch)
		_, _ = sconn.Exec(`DELETE FROM customer_engagement_refresh WHERE channel_id = $1`, ch)
	}
	cleanup()
	defer cleanup()

	seed := func(cust uuid.UUID, at time.Time, key string) {
		if _, err := sconn.Exec(`INSERT INTO analytics_events (event_name, occurred_at, channel_id, customer_id, source, properties, dedupe_key)
			VALUES ('page_view', $1, $2, $3, 'commerce', '{}', $4)`, at, ch, cust, "engtest:"+key+":"+uuid.NewString()); err != nil {
			t.Fatal(err)
		}
	}
	seed(agedC, t1.Add(-29*24*time.Hour), "aged") // agedC's only event is in t1's window but out of t2's, while stableC sits inside both
	seed(stableC, t1.Add(-5*24*time.Hour), "stable")
	seed(stableC, t1.Add(-4*24*time.Hour), "stable")

	if err := rec.refreshEngagement(ctx, ch, t1); err != nil {
		t.Fatalf("full refresh: %v", err)
	}
	seed(freshC, t1.Add(12*time.Hour), "fresh") // freshC appears between the runs
	if err := rec.refreshEngagement(ctx, ch, t2); err != nil {
		t.Fatalf("incremental refresh: %v", err)
	}

	got := map[uuid.UUID]int32{}
	rows, err := sconn.Query(`SELECT customer_id, engagement FROM customer_engagement_30d WHERE channel_id = $1`, ch)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var n int32
		if err := rows.Scan(&id, &n); err != nil {
			t.Fatal(err)
		}
		got[id] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if _, ok := got[agedC]; ok {
		t.Errorf("aged-out customer still in the projection: %v", got)
	}
	if got[freshC] != 1 {
		t.Errorf("fresh customer engagement = %d, want 1", got[freshC])
	}
	if got[stableC] != 2 {
		t.Errorf("stable customer engagement = %d, want 2", got[stableC])
	}

	want, err := rec.spine.ChannelEngagementCounts(ctx, ch, t2.Add(-engagementWindow)) // byte-for-byte what a full recompute at t2 says
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != len(got) {
		t.Fatalf("projection has %d rows, full recompute says %d", len(got), len(want))
	}
	for _, w := range want {
		if got[w.CustomerID] != w.Count {
			t.Errorf("customer %s projection %d, full recompute %d", w.CustomerID, got[w.CustomerID], w.Count)
		}
	}
}
