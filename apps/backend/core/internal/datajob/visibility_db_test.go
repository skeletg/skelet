package datajob

import (
	"context"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestAScopedJobListCountsOnlyTheRunsItShows(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	var own, other uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&own); err != nil {
		t.Skipf("no channel: %v", err)
	}
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`, "dj-"+uuid.NewString()[:8], own).Scan(&other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	resource := "vis-" + uuid.NewString()[:8]
	for _, ch := range []uuid.NullUUID{{UUID: own, Valid: true}, {UUID: other, Valid: true}, {}, {UUID: own, Valid: true}} {
		if _, err := conn.Exec(`INSERT INTO data_jobs (direction, resource, channel_id) VALUES ('export', $1, $2)`, resource, ch); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM data_jobs WHERE resource = $1`, resource) })
	svc := New(db.New(conn), nil, nil)

	jobs, total, err := svc.List(ctx, ListFilter{Resources: []string{resource}, Limit: 1,
		Visibility: &Visibility{Pairs: []VisiblePair{{Resource: resource, ChannelID: own}}}})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(jobs) != 1 || jobs[0].ChannelID != own {
		t.Fatalf("scoped page = %d rows of %d, want 1 of 2 on the admin's channel - another channel's or an unscoped run leaked into the count", len(jobs), total)
	}
	if _, total, _ := svc.List(ctx, ListFilter{Resources: []string{resource}, Visibility: &Visibility{AnyChannel: []string{resource}}}); total != 4 {
		t.Fatalf("a global view counted %d, want all 4", total)
	}
	if _, total, _ := svc.List(ctx, ListFilter{Resources: []string{resource}}); total != 4 {
		t.Fatalf("no visibility counted %d, want all 4", total)
	}
}
