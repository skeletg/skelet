package lens

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func TestABrowsePageReadsOnlyTheAskedChannel(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	mine, theirs := uuid.New(), uuid.New()
	at := time.Now().Add(-time.Hour)
	for _, ch := range []uuid.UUID{mine, theirs, theirs} {
		if _, err := conn.Exec(`INSERT INTO browse_seen (channel_id, anon_id, last_browse_at) VALUES ($1, $2, $3)`, ch, "anon-"+uuid.NewString(), at); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM browse_seen WHERE channel_id = ANY($1)`, []uuid.UUID{mine, theirs})
	})
	page := func(ch uuid.NullUUID) map[uuid.UUID]int {
		rows, err := signalsdb.New(conn).BrowseSeenSincePage(ctx, signalsdb.BrowseSeenSincePageParams{
			MaxAge: at.Add(-time.Minute), AfterAt: time.Now().Add(time.Hour), AfterAnon: "", Lim: 100, ChannelID: ch,
		})
		if err != nil {
			t.Fatal(err)
		}
		out := map[uuid.UUID]int{}
		for _, r := range rows {
			out[r.ChannelID]++
		}
		return out
	}
	if got := page(uuid.NullUUID{UUID: mine, Valid: true}); got[mine] != 1 || got[theirs] != 0 {
		t.Fatalf("one channel's page = %v, want only its own browse - another store's backlog would fill the page", got)
	}
	if got := page(uuid.NullUUID{}); got[mine] != 1 || got[theirs] != 2 {
		t.Fatalf("an unscoped page = %v, want every channel", got)
	}
}
