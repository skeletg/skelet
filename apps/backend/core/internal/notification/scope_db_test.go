package notification

import (
	"context"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestTheInboxQueryAgreesWithCanSeePerPermission(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	var a uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&a); err != nil {
		t.Skipf("no channel: %v", err)
	}
	var b uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`, "nt-"+uuid.NewString()[:8], a).Scan(&b); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, b) })
	svc := New(db.New(conn), nil)
	tag := uuid.NewString()[:8]
	for _, n := range []Input{
		{Category: CategoryOrder, RequiredPermission: PermOrders, ChannelID: &a, Title: "own order " + tag},
		{Category: CategoryOrder, RequiredPermission: PermOrders, ChannelID: &b, Title: "their order " + tag},
		{Category: CategoryReview, RequiredPermission: PermReviews, ChannelID: &b, Title: "their review " + tag},
	} {
		n.DedupeKey = n.Title
		if err := svc.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM admin_notifications WHERE dedupe_key LIKE '%' || $1`, tag) })

	sc := Scope{AdminID: uuid.New(), Perms: []string{PermOrders, PermReviews}, GlobalPerms: []string{PermReviews},
		PermChannels: map[string][]uuid.UUID{PermOrders: {a}}}
	got, err := svc.List(ctx, sc, false, 200)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, n := range got {
		seen[n.Title] = true
	}
	if !seen["own order "+tag] || !seen["their review "+tag] {
		t.Errorf("the inbox hid a notification the scope covers: %v", seen)
	}
	if seen["their order "+tag] {
		t.Error("the inbox listed another channel's order notification for orders.view on one channel")
	}
	if n, err := svc.UnreadCount(ctx, sc); err != nil || n < 2 {
		t.Errorf("unread badge = %d, %v", n, err)
	}
}

// A notice for one admin, such as a request for access to their board, is theirs alone - a superuser's bypass doesn't
// reach it, and the badge and mark-all skip it the same way, or someone else's request would sit in their count
func TestANoticeForOneAdminReachesOnlyThem(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	var owner uuid.UUID
	if err := conn.QueryRow(`INSERT INTO admin_users (email, password_hash, first_name, last_name) VALUES ($1, 'x', 'A', 'B') RETURNING id`,
		"nt-"+uuid.NewString()[:8]+"@example.com").Scan(&owner); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM admin_users WHERE id = $1`, owner) })
	svc := New(db.New(conn), nil)
	title := "asked for access " + uuid.NewString()[:8]
	if err := svc.Create(ctx, Input{Category: CategoryBoard, RequiredPermission: PermBoards, Title: title, DedupeKey: title, RecipientAdminID: &owner}); err != nil {
		t.Fatal(err)
	}
	has := func(sc Scope) bool {
		got, err := svc.List(ctx, sc, false, 200)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range got {
			if n.Title == title {
				return true
			}
		}
		return false
	}
	if !has(Scope{AdminID: owner, Perms: []string{PermBoards}, GlobalPerms: []string{PermBoards}}) {
		t.Error("the recipient's inbox hid their notice")
	}
	other := Scope{AdminID: uuid.New(), Bypass: true}
	if has(other) {
		t.Error("a superuser saw a notice meant for someone else")
	}
	before, _ := svc.UnreadCount(ctx, other)
	if err := svc.MarkAllRead(ctx, other); err != nil {
		t.Fatal(err)
	}
	if after, _ := svc.UnreadCount(ctx, other); before < 0 || after != 0 {
		t.Errorf("unread after mark-all = %d, want 0", after)
	}
}
