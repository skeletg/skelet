package datapatch

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type rig struct {
	conn *sql.DB
	q    *db.Queries
	tx   *txrunner.Manager
}

func newRig(t *testing.T) rig {
	t.Helper()
	conn := testdb.Require(t)
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS dp_test_rows (patch text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	return rig{conn: conn, q: db.New(conn), tx: txrunner.New(conn)}
}

func (r rig) count(t *testing.T, query string, arg string) int {
	t.Helper()
	var n int
	if err := r.conn.QueryRow(query, arg).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r rig) rows(t *testing.T, id string) int {
	return r.count(t, `SELECT count(*) FROM dp_test_rows WHERE patch = $1`, id)
}

func (r rig) marked(t *testing.T, id string) bool {
	return r.count(t, `SELECT count(*) FROM data_patches WHERE id = $1`, id) == 1
}

func writes(id string) Patch {
	return Patch{ID: id, Namespace: "test", Apply: func(ctx context.Context, pc *Context) error {
		_, err := pc.Tx.ExecContext(ctx, `INSERT INTO dp_test_rows VALUES ($1)`, id)
		return err
	}}
}

func TestAPatchAndItsMarkerCommitTogetherAndRunOnce(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	p := writes("dbtest:once")
	res, err := run(ctx, r.q, r.tx, []Patch{p})
	if err != nil || len(res.Applied) != 1 {
		t.Fatalf("first run: %+v %v", res, err)
	}
	if r.rows(t, p.ID) != 1 || !r.marked(t, p.ID) {
		t.Fatal("the patch's write or its marker is missing")
	}
	res, err = run(ctx, r.q, r.tx, []Patch{p})
	if err != nil || len(res.Applied) != 0 || res.Skipped != 1 {
		t.Fatalf("second run: %+v %v", res, err)
	}
	if r.rows(t, p.ID) != 1 {
		t.Fatal("an applied patch ran again")
	}
}

func TestAFailedPatchLeavesNothingAndRetries(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	const id = "dbtest:retry"
	failing := writes(id)
	inner := failing.Apply
	failing.Apply = func(ctx context.Context, pc *Context) error {
		if err := inner(ctx, pc); err != nil {
			return err
		}
		return errors.New("half way")
	}
	if _, err := run(ctx, r.q, r.tx, []Patch{failing}); err == nil || !strings.Contains(err.Error(), id) {
		t.Fatalf("a failed patch: %v, want an error naming it", err)
	}
	if r.rows(t, id) != 0 || r.marked(t, id) {
		t.Fatal("a failed patch left its write or its marker behind")
	}
	if res, err := run(ctx, r.q, r.tx, []Patch{writes(id)}); err != nil || len(res.Applied) != 1 {
		t.Fatalf("the fixed patch did not run on the next deploy: %+v %v", res, err)
	}
}

func TestAPanickingPatchFailsWithItsName(t *testing.T) {
	r := newRig(t)
	const id = "dbtest:panic"
	p := Patch{ID: id, Namespace: "test", Apply: func(context.Context, *Context) error { panic("boom") }}
	if _, err := run(context.Background(), r.q, r.tx, []Patch{p}); err == nil || !strings.Contains(err.Error(), id) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("a panicking patch: %v", err)
	}
	if r.marked(t, id) {
		t.Fatal("a panicking patch was marked applied")
	}
}

// A hand-run `data-patch up` during the deploy one-shot - both plan the same patch, only one may run it
func TestOverlappingRunsApplyAPatchOnce(t *testing.T) {
	r := newRig(t)
	const id = "dbtest:overlap"
	var calls atomic.Int32
	p := writes(id)
	inner := p.Apply
	p.Apply = func(ctx context.Context, pc *Context) error {
		calls.Add(1)
		time.Sleep(200 * time.Millisecond)
		return inner(ctx, pc)
	}
	var wg sync.WaitGroup
	results := make([]Result, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() { results[i], errs[i] = run(context.Background(), r.q, r.tx, []Patch{p}) })
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("an overlapping run failed: %v", err)
		}
	}
	if calls.Load() != 1 || r.rows(t, id) != 1 {
		t.Fatalf("Apply ran %d times, want once", calls.Load())
	}
	if len(results[0].Applied)+len(results[1].Applied) != 1 || results[0].Skipped+results[1].Skipped != 1 {
		t.Fatalf("results %+v, want one applied and one skipped", results)
	}
}
