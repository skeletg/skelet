package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
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

func TestRunNowByNamePullsTheJobForward(t *testing.T) {
	svc, conn, _ := newDBService(t)
	ctx := context.Background()
	name := "run-now-" + uuid.NewString()[:8]
	svc.Register(name, "@every 24h", "run-now fixture", func(context.Context) error { return nil })
	if err := svc.Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM scheduled_jobs WHERE name = $1`, name) })

	if err := svc.RunNowByName(ctx, name); err != nil {
		t.Fatalf("RunNowByName: %v", err)
	}
	var due time.Time
	if err := conn.QueryRow(`SELECT next_run_at FROM scheduled_jobs WHERE name = $1`, name).Scan(&due); err != nil {
		t.Fatalf("read due: %v", err)
	}
	if time.Until(due) > time.Minute {
		t.Errorf("next run is %s away, want it pulled to now", time.Until(due))
	}
	if err := svc.RunNowByName(ctx, "no-such-job-"+uuid.NewString()[:8]); err == nil {
		t.Error("an unknown job name was accepted")
	}
}

func TestRunNowRefusesADisabledJob(t *testing.T) {
	svc, conn, q := newDBService(t)
	ctx := context.Background()
	name := "run-now-off-" + uuid.NewString()[:8]
	svc.RegisterDisabled(name, "@every 24h", "disabled fixture", func(context.Context) error { return nil })
	if err := svc.Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM scheduled_jobs WHERE name = $1`, name) })
	var id uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM scheduled_jobs WHERE name = $1`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := svc.RunNow(ctx, id); !errors.Is(err, ErrJobDisabled) {
		t.Fatalf("RunNow on a disabled job = %v, want ErrJobDisabled", err)
	}
	row, err := q.GetScheduledJob(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if row.NextRunAt.Valid && time.Until(row.NextRunAt.Time) < time.Hour {
		t.Error("a refused run-now still pulled the job forward")
	}
}

func TestWebhookJobsCannotTakeACodeJobsName(t *testing.T) {
	svc, conn, _ := newDBService(t)
	name := "code-owned-" + uuid.NewString()[:8]
	svc.Register(name, "@every 24h", "fixture", func(context.Context) error { return nil })
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM scheduled_jobs WHERE name = $1`, name) })
	_, err := svc.CreateWebhook(context.Background(), CreateWebhookInput{Name: name, Schedule: "@every 1h", URL: "https://hooks.example.com/x"}, nil)
	if !errors.Is(err, ErrWebhookNameTaken) {
		t.Fatalf("CreateWebhook on a code job's name = %v, want ErrWebhookNameTaken", err)
	}
}

func TestWebhookSecretsAreSealedInTheRow(t *testing.T) {
	svc, conn, _ := newDBService(t)
	svc.SetCipher(testCipher(t))
	ctx := context.Background()
	job, err := svc.CreateWebhook(ctx, CreateWebhookInput{
		Name: "sealed-" + uuid.NewString()[:8], Schedule: "@every 1h", URL: "https://hooks.example.com/x?token=abc",
		Secret: "sign-me", Headers: map[string]string{"Authorization": "Bearer api-key"},
	}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM scheduled_jobs WHERE id = $1`, job.ID) })
	var stored string
	if err := conn.QueryRow(`SELECT payload::text FROM scheduled_jobs WHERE id = $1`, job.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "sign-me") || strings.Contains(stored, "api-key") {
		t.Fatalf("payload holds a plaintext secret: %s", stored)
	}
	if job.Webhook == nil || job.Webhook.Host != "hooks.example.com" || !job.Webhook.HasSecret {
		t.Errorf("list view = %+v, want the host alone and the secret flagged", job.Webhook)
	}
	detail, err := svc.WebhookDetail(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Headers["Authorization"] != "Bearer api-key" {
		t.Errorf("detail headers = %v, want them opened", detail.Headers)
	}
	if _, err := svc.UpdateWebhook(ctx, UpdateWebhookInput{ID: job.ID, URL: "https://hooks.example.com/y", Method: "POST"}, nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	after, err := svc.Get(ctx, job.ID)
	if err != nil || !after.Webhook.HasSecret {
		t.Errorf("update with a blank secret dropped it: %+v %v", after.Webhook, err)
	}
}

// A deploy interrupting a daily job must not cost a day - it stays due for the next replica or boot
func TestAnInterruptedRunStaysDue(t *testing.T) {
	svc, conn, q := newDBService(t)
	name := "interrupted-" + uuid.NewString()[:8]
	started := make(chan struct{})
	svc.Register(name, "@every 24h", "fixture", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if err := svc.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM scheduled_jobs WHERE name = $1`, name) })
	markDue(t, conn, name)
	var row dbsqlc.ScheduledJob
	rows, err := q.ClaimDueScheduledJobs(context.Background(), dbsqlc.ClaimDueScheduledJobsParams{Now: time.Now(), BatchSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Name == name {
			row = r
		} else {
			_, _ = conn.Exec(`UPDATE scheduled_jobs SET locked_until = NULL WHERE id = $1`, r.ID)
		}
	}
	if row.Name == "" {
		t.Fatal("job was not claimed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	svc.runOne(ctx, row)
	var due sql.NullTime
	if err := conn.QueryRow(`SELECT next_run_at FROM scheduled_jobs WHERE name = $1`, name).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if !due.Valid || time.Until(due.Time) > time.Minute {
		t.Errorf("an interrupted run was rescheduled to %v, want it still due", due)
	}
}
