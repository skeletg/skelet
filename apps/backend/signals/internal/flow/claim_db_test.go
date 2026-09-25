package flow

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func mine(runs []FlowRun, flowID uuid.UUID) int {
	n := 0
	for _, r := range runs {
		if r.FlowID == flowID {
			n++
		}
	}
	return n
}

func TestClaimLeasesEachRunToOneAdvancer(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	r := &repo{q: db.New(conn)}

	var flowID uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO flows (channel_id, name, trigger_type, status) VALUES ($1, 'claim-test', 'password_reset', 'active') RETURNING id`,
		uuid.New()).Scan(&flowID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM flows WHERE id = $1`, flowID) })

	now := time.Now()
	if _, err := r.CreateRun(ctx, FlowRun{FlowID: flowID, CustomerID: uuid.New(), CurrentNodeID: "n", Status: RunActive, ResumeAt: &now}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	got := make([]int, 8)
	for i := range got {
		wg.Go(func() {
			runs, err := r.ClaimRunnable(ctx, 100, runLease)
			if err != nil {
				t.Error(err)
			}
			got[i] = mine(runs, flowID)
		})
	}
	wg.Wait()
	total := 0
	for _, n := range got {
		total += n
	}
	if total != 1 {
		t.Fatalf("%d advancers claimed the one run, want exactly 1", total)
	}

	leased := now.Add(runLease)
	cust := uuid.New()
	if _, err := r.CreateRun(ctx, FlowRun{FlowID: flowID, CustomerID: cust, CurrentNodeID: "n", Status: RunActive, ResumeAt: &leased, EnrollmentKey: "k"}); err != nil {
		t.Fatal(err)
	}
	runs, err := r.ClaimRunnable(ctx, 100, runLease)
	if err != nil {
		t.Fatal(err)
	}
	if n := mine(runs, flowID); n != 0 {
		t.Fatalf("the claim took %d leased runs, want none", n)
	}

	if held, err := r.HasRunForKey(ctx, flowID, "k"); err != nil || !held {
		t.Fatalf("HasRunForKey = %v, %v, want the keyed run found", held, err)
	}
	if _, err := r.CreateRun(ctx, FlowRun{FlowID: flowID, CustomerID: cust, CurrentNodeID: "n", Status: RunActive, ResumeAt: &now, EnrollmentKey: "k2"}); !errors.Is(err, ErrAlreadyInFlow) {
		t.Fatalf("a second live run for one customer = %v, want ErrAlreadyInFlow", err)
	}
}

func TestOneChannelsBacklogDoesNotStarveAnothersRuns(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	r := &repo{q: db.New(conn)}
	flowOn := func(name string) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO flows (channel_id, name, trigger_type, status) VALUES ($1, $2, 'customer_registered', 'active') RETURNING id`, uuid.New(), name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM flows WHERE id = $1`, id) })
		return id
	}
	busy, quiet := flowOn("busy"), flowOn("quiet")
	now := time.Now()
	seed := func(flowID uuid.UUID, age time.Duration) {
		run, err := r.CreateRun(ctx, FlowRun{FlowID: flowID, CustomerID: uuid.New(), CurrentNodeID: "n", Status: RunActive, ResumeAt: &now})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`UPDATE flow_runs SET created_at = now() - $2::interval WHERE id = $1`, run.ID, age.String()); err != nil {
			t.Fatal(err)
		}
	}
	for range 5 {
		seed(busy, 240*time.Hour)
	}
	seed(quiet, 216*time.Hour)

	runs, err := r.ClaimRunnable(ctx, 2, runLease)
	if err != nil {
		t.Fatal(err)
	}
	if mine(runs, quiet) != 1 {
		t.Fatalf("claimed %d of the busy channel's runs and %d of the quiet one's, want the quiet channel to get its turn", mine(runs, busy), mine(runs, quiet))
	}
}
