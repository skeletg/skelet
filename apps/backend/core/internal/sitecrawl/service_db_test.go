package sitecrawl

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestStartRefusesAnUnvouchedOriginAndASecondRun(t *testing.T) {
	conn := testdb.Require(t)
	srv := testSite(t)
	defer srv.Close()
	var ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale, public_url)
		SELECT $1, $1, currency_code, default_locale, $2 FROM channels ORDER BY created_at LIMIT 1 RETURNING id`, "crawl-"+uuid.NewString()[:8], srv.URL).Scan(&ch); err != nil {
		t.Skipf("no channel to clone: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch) })
	q := db.New(conn)
	svc := NewService(q, slog.Default())
	ctx := context.Background()

	if _, err := svc.Start(ctx, StartInput{ChannelID: ch, BaseURL: "http://attacker.test"}); !errors.Is(err, errBaseURLRefused) {
		t.Fatalf("Start(foreign origin) = %v, want %v", err, errBaseURLRefused)
	}
	if rows, _ := svc.List(ctx, ch, 10); len(rows) != 0 {
		t.Fatalf("a refused start left %d crawl rows", len(rows))
	}

	if _, err := q.CreateSeoCrawl(ctx, db.CreateSeoCrawlParams{ChannelID: ch, BaseUrl: srv.URL, MaxPages: 1, MaxDepth: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateSeoCrawl(ctx, db.CreateSeoCrawlParams{ChannelID: ch, BaseUrl: srv.URL, MaxPages: 1, MaxDepth: 1}); !pgdb.IsUniqueViolation(err) {
		t.Fatalf("a second running crawl = %v, want a unique violation", err)
	}
	if _, err := svc.Start(ctx, StartInput{ChannelID: ch}); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Start beside a running crawl = %v, want %v", err, ErrAlreadyRunning)
	}
}
