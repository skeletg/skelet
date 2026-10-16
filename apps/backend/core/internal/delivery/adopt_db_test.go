package delivery

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Adopting the defaults keeps what checkout was offering: the same ids carts chose them by, the pickup billing rule, and both switched on even though nothing is set up to pass their checklists - twice at once makes them once
func TestAdoptDefaultsKeepsWhatCheckoutOffered(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	var ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels ORDER BY created_at LIMIT 1 RETURNING id`, "adopt-"+uuid.NewString()[:8]).Scan(&ch); err != nil {
		t.Skipf("no channel to clone: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch) })
	svc := NewService(NewRepository(db.New(conn), txrunner.New(conn)))

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() { errs[i] = svc.AdoptDefaults(ctx, ch, true) })
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("AdoptDefaults: %v", err)
		}
	}

	saved, err := svc.ListOptions(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultOptions(ch)
	if len(saved) != len(want) {
		t.Fatalf("saved %d options, want the %d defaults once", len(saved), len(want))
	}
	for i, o := range saved {
		if o.ID != want[i].ID || !o.Enabled || o.Implicit() {
			t.Errorf("option %d = %+v, want saved, enabled, under the default id %s", i, o, want[i].ID)
		}
		if o.Kind == KindStorePickup && o.BlockModes[BlockBillingAddress] != ModeRequired {
			t.Errorf("pickup block modes = %v, want billing required as the store asks it", o.BlockModes)
		}
	}
}
