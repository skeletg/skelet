package subscription

import (
	"context"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestAChannelScopedListIsFilteredBeforeTheLimit(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	var own uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&own); err != nil {
		t.Skipf("no channel: %v", err)
	}
	var other uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`, "sub-"+uuid.NewString()[:8], own).Scan(&other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	insert := func(ch uuid.UUID, age string) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO subscriptions (product_id, interval_unit, price, currency_code, current_period_start, current_period_end, next_billing_at, channel_id, created_at)
			VALUES ($1, 'month', 100, 'USD', now(), now(), now(), $2, now() - $3::interval) RETURNING id`, uuid.New(), ch, age).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM subscriptions WHERE id = $1`, id) })
		return id
	}
	mine := insert(own, "1 hour")
	insert(other, "1 minute")

	svc := NewService(NewRepository(db.New(conn)), nil)
	got, err := svc.ListForChannels(ctx, "", []uuid.UUID{own}, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != mine {
		t.Fatalf("page of one = %v, want the scoped channel's subscription - the newer foreign one must not use up the limit", got)
	}
	if none, _ := svc.ListForChannels(ctx, "", []uuid.UUID{}, 10, 0); len(none) != 0 {
		t.Errorf("no permitted channels listed %d subscriptions", len(none))
	}
}
