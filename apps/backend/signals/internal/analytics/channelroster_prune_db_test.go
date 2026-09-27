package analytics

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func countWhere(t *testing.T, r *ChannelRoster, q string, args ...any) int {
	t.Helper()
	var n int
	if err := r.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestOverlappingPushesNeverPruneAChannelBothCarry(t *testing.T) {
	r := newRoster(t)
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	a, b := uuid.New(), uuid.New()
	for _, ch := range []uuid.UUID{a, b} {
		if _, err := conn.Exec(`INSERT INTO flows (channel_id, name, trigger_type) VALUES ($1, 'Test', 'order_placed')`, ch); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM flows WHERE channel_id = ANY($1)`, pqArray(a, b)) })
	push := []ChannelEntry{{ID: a, IsActive: true}, {ID: b, IsActive: true}}

	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for range 8 {
		wg.Go(func() {
			for range 25 {
				if _, _, err := r.SyncChannels(ctx, push); err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("sync: %v", err)
	}
	if n := countWhere(t, r, `SELECT count(*) FROM channel_roster WHERE channel_id = ANY($1)`, pqArray(a, b)); n != 2 {
		t.Errorf("roster holds %d of the two pushed channels, want both", n)
	}
	if n := countWhere(t, r, `SELECT count(*) FROM flows WHERE channel_id = ANY($1)`, pqArray(a, b)); n != 2 {
		t.Errorf("%d of the two live channels' flows survived the overlapping pushes, want both", n)
	}
}

// The trigger restamps every write an hour back, the way an overlapping older push landing after this one does
func TestAPushRestampedByAnOlderOneLandingLatePrunesNothing(t *testing.T) {
	r := newRoster(t)
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	a, b := uuid.New(), uuid.New()
	if _, err := conn.Exec(`INSERT INTO flows (channel_id, name, trigger_type) VALUES ($1, 'Test', 'order_placed')`, a); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM flows WHERE channel_id = $1`, a) })
	push := []ChannelEntry{{ID: a, IsActive: true}, {ID: b, IsActive: true}}
	if _, _, err := r.SyncChannels(ctx, push); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE FUNCTION test_late_push() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.synced_at := NEW.synced_at - interval '1 hour'; RETURN NEW; END $$`,
		`CREATE TRIGGER test_late_push BEFORE INSERT OR UPDATE ON channel_roster FOR EACH ROW EXECUTE FUNCTION test_late_push()`,
	} {
		if _, err := conn.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DROP TRIGGER IF EXISTS test_late_push ON channel_roster`)
		_, _ = conn.Exec(`DROP FUNCTION IF EXISTS test_late_push()`)
	})

	_, pruned, err := r.SyncChannels(ctx, push)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 0 || countWhere(t, r, `SELECT count(*) FROM channel_roster WHERE channel_id = ANY($1)`, pqArray(a, b)) != 2 {
		t.Errorf("pruned %d channels the push carried", pruned)
	}
	if n := countWhere(t, r, `SELECT count(*) FROM flows WHERE channel_id = $1`, a); n != 1 {
		t.Errorf("a live channel's flows = %d after the push, want the one it had", n)
	}
}

type flakyBlobs struct {
	mu      sync.Mutex
	fail    bool
	deleted []string
}

func (f *flakyBlobs) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("bucket unreachable")
	}
	f.deleted = append(f.deleted, key)
	return nil
}

func TestARetiredChannelsRecordingsLeaveTheBucketEvenAfterAFailedSweep(t *testing.T) {
	r := newRoster(t)
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	keep, retire := uuid.New(), uuid.New()
	session := uuid.New()
	chunkKey, snapKey := "lens/replay/"+session.String()+"/0", "lens/snap/"+retire.String()
	for _, seed := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO lens_sessions (id, channel_id, anon_id, started_at, last_event_at) VALUES ($1, $2, 'a', now(), now())`, []any{session, retire}},
		{`INSERT INTO lens_session_chunks (session_id, seq, first_ts, last_ts, blob_key) VALUES ($1, 0, 1, 2, $2)`, []any{session, chunkKey}},
		{`INSERT INTO lens_page_snapshots (channel_id, page_key, viewport_bucket, layout_hash, width, height, blob_key) VALUES ($1, '/', 'd', 'h', 1, 1, $2)`, []any{retire, snapKey}},
		{`INSERT INTO event_destinations (channel_id, name, target_url) VALUES ($1, 'hook', 'https://example.test/hook')`, []any{retire}},
	} {
		if _, err := conn.Exec(seed.q, seed.args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, seed.q)
		}
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM lens_sessions WHERE channel_id = $1`, retire)
		_, _ = conn.Exec(`DELETE FROM lens_page_snapshots WHERE channel_id = $1`, retire)
		_, _ = conn.Exec(`DELETE FROM event_destinations WHERE channel_id = $1`, retire)
		_, _ = conn.Exec(`DELETE FROM channel_roster_tombstones`)
	})
	blobs := &flakyBlobs{fail: true}
	r.SetBlobStore(blobs)
	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: keep, IsActive: true}, {ID: retire, IsActive: true}}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: keep, IsActive: true}}); err == nil {
		t.Error("a sweep that could not delete the recordings reported success")
	}
	if n := countWhere(t, r, `SELECT count(*) FROM event_destinations WHERE channel_id = $1`, retire); n != 0 {
		t.Errorf("a failing step stopped the sweep: %d destinations still forward for the retired channel", n)
	}
	if n := countWhere(t, r, `SELECT count(*) FROM lens_sessions WHERE channel_id = $1`, retire); n != 1 {
		t.Errorf("recording rows = %d while their objects are still in the bucket, want the row kept to find them again", n)
	}

	blobs.fail = false
	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: keep, IsActive: true}}); err != nil {
		t.Fatalf("the next push, pruning nothing new: %v", err)
	}
	if !slices.Contains(blobs.deleted, chunkKey) || !slices.Contains(blobs.deleted, snapKey) {
		t.Errorf("deleted objects %v, want the retired channel's replay chunk and snapshot", blobs.deleted)
	}
	for _, table := range []string{"lens_sessions", "lens_page_snapshots"} {
		if n := countWhere(t, r, `SELECT count(*) FROM `+table+` WHERE channel_id = $1`, retire); n != 0 {
			t.Errorf("%s left on the retired channel = %d after the retried sweep", table, n)
		}
	}
	if n := countWhere(t, r, `SELECT count(*) FROM channel_roster_tombstones`); n != 0 {
		t.Errorf("%d tombstones left after a complete sweep, want none", n)
	}
}
