package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// Gated on SIGNALS_DB_URL - exercises the pgSpine Lens reads against real rows so
// the any/[]byte unwrapping is proven, not assumed
func TestPgSpineLensReadsUnwrapLabels(t *testing.T) {
	conn := testdb.RequireSignals(t)
	sp := newPGSpine(signalsdb.New(conn), nil)
	ctx := context.Background()

	var ch uuid.UUID
	var page string
	row := conn.QueryRow(`SELECT channel_id, COALESCE(properties->>'page_key','') FROM analytics_events
		WHERE source='web' AND event_name='page_view' AND channel_id IS NOT NULL
		  AND COALESCE(properties->>'page_key','') <> '' LIMIT 1`)
	if err := row.Scan(&ch, &page); err != nil {
		t.Skipf("no web page_view rows to check against: %v", err)
	}

	pages, err := sp.LensListPages(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 || pages[0].Value == "" {
		t.Fatalf("LensListPages returned %d rows, first value %q - empty means the unwrap is broken", len(pages), func() string {
			if len(pages) > 0 {
				return pages[0].Value
			}
			return "<none>"
		}())
	}
	refs, err := sp.LensPageReferrers(ctx, ch, page)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range refs {
		if r.Value == "" {
			t.Fatal("LensPageReferrers produced an empty label - the COALESCE guarantees '(direct)' at minimum")
		}
	}
	devs, err := sp.LensPageDeviceSplit(ctx, ch, page)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range devs {
		if d.Value == "" {
			t.Fatal("LensPageDeviceSplit produced an empty label")
		}
	}
	if _, err := sp.LensVitalsOverall(ctx, ch, time.Now().AddDate(0, 0, -3650)); err != nil {
		t.Fatal(err)
	}
	t.Logf("ok: %d pages, %d referrers, %d devices for %q", len(pages), len(refs), len(devs), page)
}
