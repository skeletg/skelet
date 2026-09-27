package outbox

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// Every subsystem's "durable, at-least-once" promise is this relay: order emails, search indexing, webhooks, referral payouts.
// The failure modes are both expensive - a row claimed but never dispatched loses the event for good, and a row dispatched twice bills, mails or pays twice

type dbProbe struct {
	Kind string `json:"kind"` // a per-test event type, so a shared database can't cross-wire the assertions
	Note string `json:"note"`
}

func (p *dbProbe) Type() string { return p.Kind }

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Require(t)
}

type relay struct {
	*Outbox
	conn *sql.DB
	bus  *events.Bus
	kind string

	mu    sync.Mutex
	seen  []*dbProbe
	times []time.Time
	ids   []uuid.UUID
}

func newRelay(t *testing.T, conn *sql.DB) *relay {
	t.Helper()
	r := &relay{conn: conn, bus: events.NewBus(), kind: "outbox.dbprobe." + uuid.NewString()}
	r.Outbox = New(dbsqlc.New(conn), r.bus)
	r.RegisterType(r.kind, func() events.Event { return &dbProbe{} })
	r.bus.Subscribe(r.kind, func(ctx context.Context, e events.Event) error {
		p, ok := e.(*dbProbe)
		if !ok {
			t.Errorf("handler got %T, want *dbProbe", e)
			return nil
		}
		at, _ := events.EmittedAt(ctx)
		id, _ := events.OutboxID(ctx)
		r.mu.Lock()
		r.seen, r.times, r.ids = append(r.seen, p), append(r.times, at), append(r.ids, id)
		r.mu.Unlock()
		return nil
	})
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM event_outbox WHERE event_type = $1`, r.kind) })
	return r
}

func (r *relay) delivered() []*dbProbe {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*dbProbe(nil), r.seen...)
}

// Enqueue writes through the caller's transaction, so the commit decision belongs to the test
func (r *relay) enqueue(t *testing.T, note string, commit bool) {
	t.Helper()
	sqlTx, err := r.conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := r.Enqueue(context.Background(), sqlTx, &dbProbe{Kind: r.kind, Note: note}); err != nil {
		_ = sqlTx.Rollback()
		t.Fatalf("Enqueue: %v", err)
	}
	if commit {
		if err := sqlTx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
		return
	}
	if err := sqlTx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
}

// Writes a row directly, for the states a healthy Enqueue can never produce
func (r *relay) insertRaw(t *testing.T, eventType, payload string, attempts int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := r.conn.QueryRow(
		`INSERT INTO event_outbox (event_type, payload, attempts) VALUES ($1, $2::jsonb, $3) RETURNING id`,
		eventType, payload, attempts).Scan(&id); err != nil {
		t.Fatalf("insert raw row: %v", err)
	}
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM event_outbox WHERE id = $1`, id) })
	return id
}

func (r *relay) rowState(t *testing.T, id uuid.UUID) (attempts int, delivered bool) {
	t.Helper()
	var deliveredAt sql.NullTime
	if err := r.conn.QueryRow(`SELECT attempts, delivered_at FROM event_outbox WHERE id = $1`, id).
		Scan(&attempts, &deliveredAt); err != nil {
		t.Fatalf("read row state: %v", err)
	}
	return attempts, deliveredAt.Valid
}

func (r *relay) rowsOfKind(t *testing.T) int {
	t.Helper()
	var n int
	if err := r.conn.QueryRow(`SELECT count(*) FROM event_outbox WHERE event_type = $1`, r.kind).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func TestEnqueueLivesAndDiesWithTheCallersTransaction(t *testing.T) {
	// This is the whole point of an outbox: the event and the state change it describes commit
	// together, so a rolled-back order can never leave a "your order shipped" event behind
	conn := testDB(t)
	r := newRelay(t, conn)

	r.enqueue(t, "rolled back", false)
	if n := r.rowsOfKind(t); n != 0 {
		t.Errorf("%d rows survived a rollback", n)
	}
	r.enqueue(t, "committed", true)
	if n := r.rowsOfKind(t); n != 1 {
		t.Errorf("rows after commit = %d, want 1", n)
	}
}

func TestADrainHandsSubscribersTheSameEventThenMarksTheRow(t *testing.T) {
	conn := testDB(t)
	r := newRelay(t, conn)
	r.enqueue(t, "hello", true)

	r.DrainPending(context.Background())

	got := r.delivered()
	if len(got) != 1 || got[0].Note != "hello" {
		t.Fatalf("handler saw %+v, want one event carrying its payload", got)
	}
	var id uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM event_outbox WHERE event_type = $1`, r.kind).Scan(&id); err != nil {
		t.Fatalf("find row: %v", err)
	}
	attempts, delivered := r.rowState(t, id)
	if !delivered {
		t.Error("the row is still undelivered, so it will replay once the lease expires")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want the single claim counted", attempts)
	}
}

func TestADeliveredRowIsNeverClaimedAgain(t *testing.T) {
	// The relay polls every second for the life of the process, so leases expire constantly - what stops a delivered row coming back round is
	// the delivered_at guard, not the lease clock, and without it every order confirmation the store ever sent would go out again
	conn := testDB(t)
	r := newRelay(t, conn)
	r.enqueue(t, "once", true)

	r.DrainPending(context.Background())
	if got := r.delivered(); len(got) != 1 {
		t.Fatalf("handler fired %d times on the first drain, want once", len(got))
	}
	// Expiring the lease by hand is what the passage of time does in production
	if _, err := conn.Exec(
		`UPDATE event_outbox SET next_attempt_at = NOW() - interval '1 hour' WHERE event_type = $1`, r.kind,
	); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	for range 3 {
		r.DrainPending(context.Background())
	}
	if got := r.delivered(); len(got) != 1 {
		t.Errorf("handler fired %d times once the lease lapsed, want the delivered row left alone", len(got))
	}
}

func TestTheEventTimeSurvivesToTheHandler(t *testing.T) {
	// Handlers derive dedupe keys and reporting buckets from the emit time - taking "now" at dispatch
	// would give a replayed row a different identity than the original delivery
	conn := testDB(t)
	r := newRelay(t, conn)
	r.enqueue(t, "backdated", true)
	emitted := time.Now().Add(-72 * time.Hour).UTC().Truncate(time.Second)
	if _, err := conn.Exec(`UPDATE event_outbox SET created_at = $1 WHERE event_type = $2`, emitted, r.kind); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	r.DrainPending(context.Background())

	r.mu.Lock()
	times := append([]time.Time(nil), r.times...)
	r.mu.Unlock()
	if len(times) != 1 {
		t.Fatalf("handler fired %d times, want once", len(times))
	}
	if !times[0].UTC().Truncate(time.Second).Equal(emitted) {
		t.Errorf("handler saw emit time %v, want the row's created_at %v", times[0].UTC(), emitted)
	}
}

func TestTwoIdenticalRowsReachTheHandlerAsTwoRows(t *testing.T) {
	conn := testDB(t)
	r := newRelay(t, conn)
	r.enqueue(t, "same", true)
	r.enqueue(t, "same", true)

	r.DrainPending(context.Background())

	r.mu.Lock()
	ids := append([]uuid.UUID(nil), r.ids...)
	r.mu.Unlock()
	if len(ids) != 2 || ids[0] == uuid.Nil || ids[0] == ids[1] {
		t.Fatalf("handler saw outbox ids %v, want each row's own - identical rows would share a webhook dedupe key", ids)
	}
}

func TestOverlappingRelaysDoNotDoubleDeliver(t *testing.T) {
	// Every replica polls on its own timer, and a slow tick overlaps the next one - without SKIP LOCKED
	// exclusion each row would be dispatched once per claimer
	conn := testDB(t)
	r := newRelay(t, conn)
	const rows = 20
	for range rows {
		r.enqueue(t, "burst", true)
	}

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { r.DrainPending(context.Background()) })
	}
	wg.Wait()

	if got := r.delivered(); len(got) != rows {
		t.Errorf("handler fired %d times for %d rows across four concurrent relays", len(got), rows)
	}
}

func TestABurstLargerThanOneBatchIsFullyDrained(t *testing.T) {
	// A claim takes at most claimBatchSize rows, so stopping after one batch would leave the rest
	// waiting a poll interval each - a flash sale's events would trickle out over minutes
	conn := testDB(t)
	r := newRelay(t, conn)
	rows := claimBatchSize + 10
	for range rows {
		r.enqueue(t, "flood", true)
	}

	r.DrainPending(context.Background())

	if got := r.delivered(); len(got) != rows {
		t.Errorf("handler fired %d times, want all %d rows drained in one pass", len(got), rows)
	}
}

func TestAnUndispatchableRowRetriesThenDropsRatherThanPoisoningTheQueue(t *testing.T) {
	// A row nothing can decode - a plugin's type after the plugin is gone, or a payload from an older schema - must not be dropped on its first miss (a redeploy may fix it) and must not be retried forever either, since the claim batch is ordered oldest-first and a permanent failure at the head would starve everything behind it
	conn := testDB(t)
	r := newRelay(t, conn)

	for name, tc := range map[string]struct {
		eventType     string
		payload       string
		attempts      int
		wantDelivered bool
		wantAttempts  int
	}{
		"an unregistered type, first misses": {"outbox.gone." + uuid.NewString(), `{}`, 0, false, 1},
		"an unregistered type, out of tries": {"outbox.gone." + uuid.NewString(), `{}`, maxAttempts - 1, true, maxAttempts},
		"a payload that will not decode":     {r.kind, `"not an object"`, maxAttempts - 1, true, maxAttempts},
	} {
		t.Run(name, func(t *testing.T) {
			id := r.insertRaw(t, tc.eventType, tc.payload, tc.attempts)
			r.DrainPending(context.Background())

			attempts, delivered := r.rowState(t, id)
			if delivered != tc.wantDelivered {
				t.Errorf("delivered = %v, want %v", delivered, tc.wantDelivered)
			}
			if attempts != tc.wantAttempts {
				t.Errorf("attempts = %d, want %d", attempts, tc.wantAttempts)
			}
		})
	}
	if got := r.delivered(); len(got) != 0 {
		t.Errorf("handler fired %d times on rows it could never decode", len(got))
	}
}

func TestALeasedRowIsInvisibleUntilTheLeaseExpires(t *testing.T) {
	// The lease is what keeps a dispatch that is still running from being restarted underneath itself,
	// and it is also the retry timer for anything that failed
	conn := testDB(t)
	r := newRelay(t, conn)
	id := r.insertRaw(t, "outbox.gone."+uuid.NewString(), `{}`, 0)

	r.DrainPending(context.Background())
	if attempts, _ := r.rowState(t, id); attempts != 1 {
		t.Fatalf("attempts = %d after the first claim, want 1", attempts)
	}
	r.DrainPending(context.Background())
	if attempts, _ := r.rowState(t, id); attempts != 1 {
		t.Errorf("attempts = %d, want the leased row skipped by the second drain", attempts)
	}

	// Expiring the lease by hand rather than waiting out leaseSeconds
	if _, err := conn.Exec(`UPDATE event_outbox SET next_attempt_at = NOW() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	r.DrainPending(context.Background())
	if attempts, _ := r.rowState(t, id); attempts != 2 {
		t.Errorf("attempts = %d after the lease expired, want the row re-claimed", attempts)
	}
}

func TestDeliveredRowsAreSweptOnlyOnceTheyAreOldEnough(t *testing.T) {
	// The sweeper keeps the table from growing forever, but taking a freshly delivered row would
	// destroy the only record that the event was already handled
	conn := testDB(t)
	r := newRelay(t, conn)
	queries := dbsqlc.New(conn)
	ctx := context.Background()

	fresh := r.insertRaw(t, r.kind, `{}`, 1)
	stale := r.insertRaw(t, r.kind, `{}`, 1)
	undelivered := r.insertRaw(t, r.kind, `{}`, 1)
	for _, id := range []uuid.UUID{fresh, stale} {
		if err := queries.MarkOutboxDelivered(ctx, id); err != nil {
			t.Fatalf("mark delivered: %v", err)
		}
	}
	if _, err := conn.Exec(
		`UPDATE event_outbox SET delivered_at = NOW() - $1::interval WHERE id = $2`,
		"48 hours", stale); err != nil {
		t.Fatalf("age the stale row: %v", err)
	}

	if _, err := queries.DeleteDeliveredOutboxBefore(ctx, time.Now().Add(-deliveredRetention)); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	for name, tc := range map[string]struct {
		id       uuid.UUID
		wantGone bool
	}{
		"delivered long ago":     {stale, true},
		"delivered just now":     {fresh, false},
		"never delivered at all": {undelivered, false},
	} {
		t.Run(name, func(t *testing.T) {
			var n int
			if err := conn.QueryRow(`SELECT count(*) FROM event_outbox WHERE id = $1`, tc.id).Scan(&n); err != nil {
				t.Fatalf("read back: %v", err)
			}
			if gone := n == 0; gone != tc.wantGone {
				t.Errorf("swept = %v, want %v", gone, tc.wantGone)
			}
		})
	}
}
