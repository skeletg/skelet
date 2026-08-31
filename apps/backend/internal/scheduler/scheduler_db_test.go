package scheduler

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	dbsqlc "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// The engine's DB-backed guarantees: a tick must claim without waiting on running jobs, a row's timeout must bound its run and release the
// lease, and Seed must persist a registered timeout - all of it rides the real claim SQL, so these need a database

func newDBService(t *testing.T) (*Service, *sql.DB, *dbsqlc.Queries) {
	t.Helper()
	conn := testdb.Require(t)
	q := dbsqlc.New(conn)
	return New(q, true), conn, q
}

func markDue(t *testing.T, conn *sql.DB, name string) {
	t.Helper()
	if _, err := conn.Exec(`UPDATE scheduled_jobs SET next_run_at = now() - interval '1 minute', locked_until = NULL WHERE name = $1`, name); err != nil {
		t.Fatalf("mark due: %v", err)
	}
}

// tick must come back promptly even while a claimed handler is still running - a
// batch-wait shape is exactly what this rules out
func tickReturns(t *testing.T, svc *Service, ctx context.Context) {
	t.Helper()
	done := make(chan struct{})
	go func() { svc.tick(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tick blocked on a running job")
	}
}

func waitFor(t *testing.T, d time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestTickClaimsWhileLongJobRuns(t *testing.T) {
	svc, conn, _ := newDBService(t)
	ctx := context.Background()

	longName := "test-tick-long-" + uuid.NewString()
	quickName := "test-tick-quick-" + uuid.NewString()
	release := make(chan struct{})
	longStarted := make(chan struct{})
	quickRan := make(chan struct{})
	svc.Register(longName, "@every 1h", "test", func(ctx context.Context) error {
		close(longStarted)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	})
	svc.Register(quickName, "@every 1h", "test", func(context.Context) error {
		close(quickRan)
		return nil
	})
	if err := svc.Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		svc.runWG.Wait()
		_, _ = conn.Exec(`DELETE FROM scheduled_jobs WHERE name IN ($1, $2)`, longName, quickName)
	})

	markDue(t, conn, longName)
	waitFor(t, 10*time.Second, "long job never claimed", func() bool {
		tickReturns(t, svc, ctx)
		select {
		case <-longStarted:
			return true
		default:
			return false
		}
	})

	// The long handler is parked on `release`, holding its slot - a later tick must still
	// claim and finish the quick job
	markDue(t, conn, quickName)
	waitFor(t, 10*time.Second, "quick job starved behind the running long job", func() bool {
		tickReturns(t, svc, ctx)
		select {
		case <-quickRan:
			return true
		default:
			return false
		}
	})

	close(release)
	svc.runWG.Wait()
}

func TestRowTimeoutBoundsRunAndReleasesLease(t *testing.T) {
	svc, conn, q := newDBService(t)
	ctx := context.Background()

	name := "test-timeout-" + uuid.NewString()
	svc.RegisterWithTimeout(name, "@every 1h", "test", time.Second, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err := svc.Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM scheduled_jobs WHERE name = $1`, name) })

	var timeoutSeconds int32
	if err := conn.QueryRow(`SELECT timeout_seconds FROM scheduled_jobs WHERE name = $1`, name).Scan(&timeoutSeconds); err != nil {
		t.Fatalf("read timeout: %v", err)
	}
	if timeoutSeconds != 1 {
		t.Fatalf("Seed persisted timeout_seconds = %d, want 1", timeoutSeconds)
	}

	markDue(t, conn, name)
	rows, err := q.ClaimDueScheduledJobs(ctx, dbsqlc.ClaimDueScheduledJobsParams{Now: time.Now(), BatchSize: 50})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	var row *dbsqlc.ScheduledJob
	for i := range rows {
		if rows[i].Name == name {
			row = &rows[i]
			continue
		}
		// foreign due rows in a shared test DB - hand their leases straight back
		_, _ = conn.Exec(`UPDATE scheduled_jobs SET locked_until = NULL WHERE id = $1`, rows[i].ID)
	}
	if row == nil {
		t.Fatal("job was not claimed")
	}
	// the lease must outlast the row's own timeout (timeout + grace)
	if !row.LockedUntil.Valid || row.LockedUntil.Time.Before(time.Now().Add(30*time.Second)) {
		t.Fatalf("claim lease too short: %v", row.LockedUntil)
	}

	start := time.Now()
	svc.runOne(ctx, *row)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("run outlived its 1s timeout: %v", elapsed)
	}

	var status, lastErr string
	var locked sql.NullTime
	if err := conn.QueryRow(`SELECT last_status, last_error, locked_until FROM scheduled_jobs WHERE name = $1`, name).Scan(&status, &lastErr, &locked); err != nil {
		t.Fatalf("read outcome: %v", err)
	}
	if status != "error" || !strings.Contains(lastErr, "context deadline exceeded") {
		t.Fatalf("outcome = %q / %q, want a deadline-exceeded error", status, lastErr)
	}
	if locked.Valid {
		t.Fatal("lease not released after the timed-out run")
	}
}
