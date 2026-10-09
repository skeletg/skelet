package datajob

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

type rowFunc func(ctx context.Context, r *dataio.Row) (dataio.Outcome, error)

func (f rowFunc) Row(ctx context.Context, r *dataio.Row) (dataio.Outcome, error) { return f(ctx, r) }

type runnerRig struct {
	conn  *sql.DB
	svc   *Service
	blobs *fakeBlobs
}

// A registry with one importable resource whose rows run fn, and a runner whose lease is short enough to lapse inside a test
func newRunnerRig(t *testing.T, newImporter func() (dataio.Importer, error)) runnerRig {
	t.Helper()
	conn := testdb.Require(t)
	if _, err := conn.Exec(`DELETE FROM data_jobs`); err != nil {
		t.Fatal(err)
	}
	reg := dataio.NewRegistry()
	reg.Register(dataio.Resource{
		Slug: "slow", Label: "Slow", Group: "Test", ViewPermission: "test.view", ImportPermission: "test.manage",
		Fields:      []dataio.Field{{Name: "sku", Type: dataio.FieldText, Identifier: true, Required: true}},
		NewImporter: func(context.Context, dataio.Opts) (dataio.Importer, error) { return newImporter() },
	})
	blobs := newFakeBlobs()
	svc := New(db.New(conn), reg, blobs)
	svc.renewEvery = 50 * time.Millisecond
	svc.leaseSeconds = 1
	return runnerRig{conn: conn, svc: svc, blobs: blobs}
}

func (r runnerRig) queue(t *testing.T) Job {
	t.Helper()
	job, err := r.svc.QueueImport(context.Background(), ImportRequest{Resource: "slow", Format: dataio.FormatCSV}, strings.NewReader("sku\nA\n"))
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func (r runnerRig) status(t *testing.T, id uuid.UUID) Job {
	t.Helper()
	job, err := r.svc.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

// A row slower than the lease used to let another runner claim the same job and apply it a second time
func TestTheHeartbeatHoldsALeaseThroughASlowRow(t *testing.T) {
	var runs atomic.Int32
	rig := newRunnerRig(t, func() (dataio.Importer, error) {
		return rowFunc(func(context.Context, *dataio.Row) (dataio.Outcome, error) {
			runs.Add(1)
			time.Sleep(2500 * time.Millisecond) // two and a half leases with no progress callback
			return dataio.OutcomeCreated, nil
		}), nil
	})
	job := rig.queue(t)
	done := make(chan error, 1)
	go func() { done <- rig.svc.tick(context.Background()) }()

	time.Sleep(1800 * time.Millisecond)
	stolen, err := rig.svc.q.ClaimDueDataJobs(context.Background(), db.ClaimDueDataJobsParams{LeaseSeconds: 1, MaxAttempts: maxAttempts, BatchSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(stolen) != 0 {
		t.Fatalf("a second runner claimed the job mid-run: %d rows", len(stolen))
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := rig.status(t, job.ID); got.Status != StatusSucceeded || runs.Load() != 1 {
		t.Fatalf("status %s after %d runs, want one successful run", got.Status, runs.Load())
	}
}

// One claim per tick, so a second queued job is never held without a heartbeat
func TestATickClaimsOneJob(t *testing.T) {
	rig := newRunnerRig(t, func() (dataio.Importer, error) {
		return rowFunc(func(context.Context, *dataio.Row) (dataio.Outcome, error) { return dataio.OutcomeCreated, nil }), nil
	})
	first, second := rig.queue(t), rig.queue(t)
	if err := rig.svc.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := rig.status(t, second.ID); got.Status != StatusQueued || got.Attempts != 0 {
		t.Fatalf("the second job was claimed alongside the first: %s, %d attempts", got.Status, got.Attempts)
	}
	if got := rig.status(t, first.ID); got.Status != StatusSucceeded {
		t.Fatalf("first job = %s", got.Status)
	}
}

// A runner that lost the job must not write its result over the current owner's
func TestOnlyTheCurrentAttemptCanFinishAJob(t *testing.T) {
	rig := newRunnerRig(t, nil)
	job := rig.queue(t)
	claimed, err := rig.svc.q.ClaimDueDataJobs(context.Background(), db.ClaimDueDataJobsParams{LeaseSeconds: 60, MaxAttempts: maxAttempts, BatchSize: 1})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v %d", err, len(claimed))
	}
	stale := toJob(claimed[0])
	if _, err := rig.conn.Exec(`UPDATE data_jobs SET attempts = attempts + 1 WHERE id = $1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := rig.svc.finish(context.Background(), stale, db.FinishDataJobParams{Status: string(StatusSucceeded), Warnings: []string{}}); !errors.Is(err, errLostLease) {
		t.Fatalf("a stale attempt finished the job: %v", err)
	}
	if got := rig.status(t, job.ID); got.Status != StatusRunning {
		t.Fatalf("status = %s, want the new owner's run left alone", got.Status)
	}
	if _, err := rig.svc.Cancel(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	current := stale
	current.Attempts++
	if err := rig.svc.finish(context.Background(), current, db.FinishDataJobParams{Status: string(StatusSucceeded), Warnings: []string{}}); !errors.Is(err, errLostLease) {
		t.Fatalf("a cancelled job was finished: %v", err)
	}
	if got := rig.status(t, job.ID); got.Status != StatusCancelled {
		t.Fatalf("status = %s, want it to stay cancelled", got.Status)
	}
}

// Cancelling stops a run within a heartbeat even while a row is still working, and the job stays cancelled
func TestCancellingARunningImportStopsItAndStaysCancelled(t *testing.T) {
	var rows atomic.Int32
	rig := newRunnerRig(t, func() (dataio.Importer, error) {
		return rowFunc(func(ctx context.Context, _ *dataio.Row) (dataio.Outcome, error) {
			rows.Add(1)
			<-ctx.Done()
			return dataio.OutcomeSkipped, ctx.Err()
		}), nil
	})
	job := rig.queue(t)
	done := make(chan error, 1)
	go func() { done <- rig.svc.tick(context.Background()) }()
	for rows.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := rig.svc.Cancel(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the run kept going after it was cancelled")
	}
	if got := rig.status(t, job.ID); got.Status != StatusCancelled {
		t.Fatalf("status = %s, want cancelled", got.Status)
	}
}

// A panic outside the row loop used to take the whole process down, once per attempt
func TestAPanicInImportSetupFailsTheJob(t *testing.T) {
	rig := newRunnerRig(t, func() (dataio.Importer, error) { panic("setup blew up") })
	job := rig.queue(t)
	if err := rig.svc.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := rig.status(t, job.ID)
	if got.Status != StatusFailed || !strings.Contains(got.ErrorSummary, "panicked") {
		t.Fatalf("status %s summary %q, want a failed job naming the panic", got.Status, got.ErrorSummary)
	}
}

type failingDelete struct{ *fakeBlobs }

func (f failingDelete) Delete(context.Context, string) error { return errors.New("store down") }

// The key is cleared only once the object is gone, and a run still waiting for its upload is never swept
func TestTheSweepKeepsWhatItCouldNotDeleteAndSparesActiveRuns(t *testing.T) {
	rig := newRunnerRig(t, nil)
	finished, waiting := rig.queue(t), rig.queue(t)
	if _, err := rig.conn.Exec(`UPDATE data_jobs SET status = 'succeeded', finished_at = NOW(), expires_at = NOW() - interval '1 day' WHERE id = $1`, finished.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.conn.Exec(`UPDATE data_jobs SET expires_at = NOW() - interval '1 day' WHERE id = $1`, waiting.ID); err != nil {
		t.Fatal(err)
	}
	broken := *rig.svc
	broken.blobs = failingDelete{rig.blobs}
	if err := broken.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := rig.status(t, finished.ID); !got.HasArtifact() {
		t.Fatal("a file the store refused to delete lost its key, orphaning it")
	}
	if err := rig.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := rig.status(t, finished.ID); got.HasArtifact() {
		t.Fatal("an expired finished run kept its file")
	}
	if got := rig.status(t, waiting.ID); !got.HasArtifact() {
		t.Fatal("a queued import lost its upload to the sweep")
	}
}

// A row whose stored options can't be read fails instead of running with none
func TestUnreadableOptionsFailTheJob(t *testing.T) {
	rig := newRunnerRig(t, func() (dataio.Importer, error) {
		return rowFunc(func(context.Context, *dataio.Row) (dataio.Outcome, error) { return dataio.OutcomeCreated, nil }), nil
	})
	job := rig.queue(t)
	if _, err := rig.conn.Exec(`UPDATE data_jobs SET options = '"not an object"' WHERE id = $1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := rig.svc.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := rig.status(t, job.ID); got.Status != StatusFailed {
		t.Fatalf("status = %s, want failed", got.Status)
	}
}
