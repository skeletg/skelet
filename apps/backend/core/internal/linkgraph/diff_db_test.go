package linkgraph

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func seedCrawl(t *testing.T, conn *sql.DB, channelID uuid.UUID, status, age string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(`INSERT INTO seo_crawls (channel_id, base_url, status, max_pages, max_depth, started_at)
		VALUES ($1, 'https://shop.test', $2, 100, 5, now() - $3::interval) RETURNING id`, channelID, status, age).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM seo_crawls WHERE id = $1`, id) })
	return id
}

func TestDiffRefusesABaselineThatCannotAnswer(t *testing.T) {
	conn := testdb.Require(t)
	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel: %v", err)
	}
	svc := New(db.New(conn))
	ctx := context.Background()
	current := seedCrawl(t, conn, channelID, "completed", "1 hour")
	failed := seedCrawl(t, conn, channelID, "failed", "2 hours")

	unknown := uuid.New()
	for name, base := range map[string]*uuid.UUID{"itself": &current, "unknown id": &unknown} {
		if _, err := svc.Diff(ctx, channelID, base, &current); !errors.Is(err, ErrNoBaseline) {
			t.Errorf("base = %s: %v, want ErrNoBaseline", name, err)
		}
	}
	d, err := svc.Diff(ctx, channelID, &failed, &current)
	if err != nil {
		t.Fatal(err)
	}
	if d.CoverageComparable {
		t.Error("a failed baseline was treated as full coverage, so its missing pages would read as GONE")
	}
}

func TestBuildReadsAPinnedCrawl(t *testing.T) {
	conn := testdb.Require(t)
	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel: %v", err)
	}
	crawl := seedCrawl(t, conn, channelID, "completed", "1 hour")
	svc := New(db.New(conn))
	g, err := svc.Build(context.Background(), channelID, &crawl, 0)
	if err != nil || g.Crawl.ID != crawl {
		t.Fatalf("build = %+v, %v - want the pinned crawl", g.Crawl, err)
	}
	if _, err := svc.Build(context.Background(), uuid.New(), &crawl, 0); !errors.Is(err, ErrNoCrawl) {
		t.Fatalf("another channel's crawl = %v, want ErrNoCrawl", err)
	}
}
