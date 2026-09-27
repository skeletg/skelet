package wiring

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestADeletedChannelTakesItsRowsAndItsQueuedJobsWithIt(t *testing.T) {
	conn := testdb.Require(t)
	ch := uuid.New()
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'Gone','USD',true)`, ch, "gone-"+ch.String()[:8]); err != nil {
		t.Fatal(err)
	}
	var job uuid.UUID
	for _, q := range []string{
		`INSERT INTO web_forms (channel_id, slug, name) VALUES ($1, 'contact', 'Contact')`,
		`INSERT INTO back_in_stock_requests (channel_id, product_id, email) VALUES ($1, gen_random_uuid(), 'w@example.test')`,
		`INSERT INTO admin_notifications (channel_id, category, title, required_permission, dedupe_key) VALUES ($1, 'x', 'x', 'orders.view', gen_random_uuid()::text)`,
	} {
		if _, err := conn.Exec(q, ch); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := conn.QueryRow(`INSERT INTO data_jobs (direction, resource, channel_id) VALUES ('export', 'customers', $1) RETURNING id`, ch).Scan(&job); err != nil {
		t.Fatal(err)
	}

	if _, err := conn.Exec(`DELETE FROM channels WHERE id = $1`, ch); err != nil {
		t.Fatalf("delete channel: %v", err)
	}
	for _, table := range []string{"web_forms", "back_in_stock_requests", "admin_notifications"} {
		var n int
		if err := conn.QueryRow(`SELECT count(*) FROM `+table+` WHERE channel_id = $1`, ch).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s kept %d rows of the deleted channel", table, n)
		}
	}
	var jobs int
	if err := conn.QueryRow(`SELECT count(*) FROM data_jobs WHERE id = $1`, job).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Error("the deleted channel's export survived, and with its channel cleared it would run over every channel")
	}
}

func TestTwoChannelsCanEachHaveARuleUnderOneSlug(t *testing.T) {
	conn := testdb.Require(t)
	slug := "summer-" + uuid.NewString()[:8]
	a, b := uuid.New(), uuid.New()
	for _, ch := range []uuid.UUID{a, b} {
		if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'Rules','USD',true)`, ch, "rules-"+ch.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM promotion_rules WHERE slug = $1`, slug)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1 OR id = $2`, a, b)
	})
	for _, ch := range []any{a, b, nil} {
		if _, err := conn.Exec(`INSERT INTO promotion_rules (slug, name, channel_id) VALUES ($1, 'Summer', $2)`, slug, ch); err != nil {
			t.Fatalf("rule on %v: %v - a slug is only unique within its channel", ch, err)
		}
	}
	if _, err := conn.Exec(`INSERT INTO promotion_rules (slug, name, channel_id) VALUES ($1, 'Again', NULL)`, slug); err == nil {
		t.Fatal("a second global rule under one slug was accepted")
	}
}
