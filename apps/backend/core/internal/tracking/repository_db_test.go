package tracking

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// One event bound for two destinations is stored once, a replay adds nothing, a browser event can't take a server
// event's id, and the claim still hands each destination the event's payload
func TestAnEventIsStoredOnceForEveryDestination(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	var ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'trk', 'USD') RETURNING id`, "trk-"+uuid.NewString()[:8]).Scan(&ch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch) })
	repo := &repository{q: db.New(conn)}
	var dests []uuid.UUID
	for _, name := range []string{"a", "b"} {
		d, err := repo.Create(ctx, Destination{ChannelID: ch, Kind: "test", Name: name, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		dests = append(dests, d.ID)
	}
	cid := uuid.New()
	purchase := Event{ID: "order-1", Name: Purchase, ChannelID: ch, User: User{CustomerID: &cid, Email: " Ana@Example.COM "}}
	for range 2 { // the second is an outbox replay
		if err := repo.Enqueue(ctx, dests, purchase); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Enqueue(ctx, dests[:1], Event{ID: "order-1", Name: PageView, ChannelID: ch}); err != nil {
		t.Fatal(err)
	}

	count := func(q string) int {
		t.Helper()
		var n int
		if err := conn.QueryRow(q, ch).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM tracking_events WHERE channel_id = $1 AND email = 'ana@example.com' AND customer_id IS NOT NULL`); n != 1 {
		t.Errorf("%d stored purchases with their identity columns, want 1", n)
	}
	if n := count(`SELECT count(*) FROM tracking_events WHERE channel_id = $1`); n != 2 {
		t.Errorf("%d events, want the purchase and the page view that reused its id", n)
	}
	due, err := repo.ClaimDue(ctx, 10, 60)
	if err != nil {
		t.Fatal(err)
	}
	var purchases int
	for _, d := range due {
		if d.EventName == Purchase && d.EventID == "order-1" && len(d.Payload) > 0 {
			purchases++
		}
	}
	if len(due) != 3 || purchases != 2 {
		t.Errorf("claimed %d (%d purchases), want a purchase per destination and the one page view", len(due), purchases)
	}

	// Settled and old, the deliveries go, and their events with them
	if _, err := conn.Exec(`UPDATE tracking_deliveries SET status = 'sent', updated_at = now() - interval '40 days'
		WHERE destination_id = ANY($1)`, dests); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE tracking_events SET created_at = now() - interval '40 days' WHERE channel_id = $1`, ch); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Prune(ctx, time.Now().Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT count(*) FROM tracking_events WHERE channel_id = $1`); n != 0 {
		t.Errorf("%d events outlived their deliveries", n)
	}
}
