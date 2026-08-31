package urlrewrite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	dbsqlc "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// Cache invalidation feeds these straight to the storefront's revalidatePath, which treats its
// argument as a literal route: a missing leading slash, a redirect row, or a skipped per-channel
// canonical all mean a page keeps serving stale HTML with nothing to show it failed
//
// DB-backed because the ordering and the redirect_type filter are the repository's behaviour

func pathsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Require(t)
}

func TestCanonicalPathsReturnsEveryChannelWithLeadingSlash(t *testing.T) {
	conn := pathsTestDB(t)
	ctx := context.Background()
	svc := NewService(NewRepository(dbsqlc.New(conn)))

	entityID := uuid.New()
	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to scope against: %v", err)
	}

	rows := []struct {
		path         string
		redirectType int
		channel      *uuid.UUID
	}{
		{"global/pretty-path", 0, nil},
		{"channel/pretty-path", 0, &channelID},
		{"old/renamed-path", 301, nil}, // a redirect must never be purged as if it were the live page
	}
	for _, r := range rows {
		_, err := conn.ExecContext(ctx,
			`INSERT INTO url_rewrites (id, entity_type, entity_id, request_path, redirect_type, channel_id)
			 VALUES ($1,'product',$2,$3,$4,$5)`,
			uuid.New(), entityID, r.path, r.redirectType, r.channel)
		if err != nil {
			t.Fatalf("seed %q: %v", r.path, err)
		}
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM url_rewrites WHERE entity_id = $1`, entityID)
	})

	got := svc.CanonicalPaths(ctx, EntityTypeProduct, entityID)

	want := map[string]bool{"/global/pretty-path": false, "/channel/pretty-path": false}
	for _, p := range got {
		if p == "/old/renamed-path" {
			t.Errorf("CanonicalPaths returned the redirect row %q - purging it evicts the wrong page and leaves the live one stale", p)
			continue
		}
		if _, ok := want[p]; !ok {
			t.Errorf("CanonicalPaths returned unexpected %q", p)
			continue
		}
		want[p] = true
	}
	for p, seen := range want {
		if !seen {
			t.Errorf("CanonicalPaths missed %q - that channel keeps serving the stale page", p)
		}
	}
}

func TestEnsureLeadingSlash(t *testing.T) {
	cases := map[string]string{
		"shop/tees":  "/shop/tees",
		"/shop/tees": "/shop/tees",
		"":           "",
	}
	for in, want := range cases {
		if got := EnsureLeadingSlash(in); got != want {
			t.Errorf("EnsureLeadingSlash(%q) = %q, want %q", in, got, want)
		}
	}
}
