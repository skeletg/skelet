package flow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// Models what ClaimRunnableFlowRuns does in SQL: due runs, oldest first, capped, and only on a flow that
// will accept them - insertion order stands in for created_at
type claimRepo struct {
	*fakeRepo
	order []uuid.UUID
}

func (r *claimRepo) ClaimRunnable(_ context.Context, limit int32, marketingEnabled bool) ([]FlowRun, error) {
	var out []FlowRun
	for _, id := range r.order {
		if int32(len(out)) >= limit {
			break
		}
		run := r.runs[id]
		if run.Status != RunActive {
			continue
		}
		f := r.flows[run.FlowID]
		if f.Status != StatusActive || f.DeletedAt != nil {
			continue
		}
		if !marketingEnabled && !flowTransactional(f) {
			continue
		}
		out = append(out, run)
	}
	return out, nil
}

func (r *claimRepo) addRun(flowID uuid.UUID) uuid.UUID {
	id := uuid.New()
	r.runs[id] = FlowRun{ID: id, FlowID: flowID, CurrentNodeID: "w", Status: RunActive}
	r.order = append(r.order, id)
	return id
}

// A single pass-through node with no outgoing edge, so one advance runs the run to completion
func endingFlow(repo *claimRepo, status string, transactional bool) Flow {
	f := Flow{
		ChannelID: uuid.New(), Name: "f", TriggerType: TriggerOrderPlaced, Status: status,
		Graph: Graph{Nodes: []Node{{ID: "w", Type: NodeTrigger}}},
	}
	if transactional {
		f.TriggerConfig = json.RawMessage(`{"transactional":true}`)
	}
	created, _ := repo.Create(context.Background(), f)
	return created
}

func newClaimRepo() *claimRepo { return &claimRepo{fakeRepo: newFakeRepo()} }

// The engine claims the oldest runs up to a cap, and a run it would refuse used to be claimed anyway and
// left untouched - so a few hundred on a paused flow permanently starved every newer run behind them,
// which on this engine means order receipts and auth links stop going out
func TestPausedFlowRunsDoNotStarveTheQueue(t *testing.T) {
	repo := newClaimRepo()
	svc := &Service{repo: repo}

	paused := endingFlow(repo, StatusPaused, false)
	for range 200 {
		repo.addRun(paused.ID)
	}
	live := endingFlow(repo, StatusActive, true)
	receipt := repo.addRun(live.ID)

	if err := svc.ProcessRuns(context.Background()); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := repo.runs[receipt].Status; got != RunCompleted {
		t.Errorf("receipt run is %q after one tick, want it advanced - 200 parked runs must not hold the claim budget", got)
	}
}

// Same starvation, reached through the Signals switch instead of a pause
func TestParkedMarketingRunsDoNotStarveTransactionalOnes(t *testing.T) {
	repo := newClaimRepo()
	svc := &Service{repo: repo, disabled: true}

	marketing := endingFlow(repo, StatusActive, false)
	for range 200 {
		repo.addRun(marketing.ID)
	}
	live := endingFlow(repo, StatusActive, true)
	receipt := repo.addRun(live.ID)

	if err := svc.ProcessRuns(context.Background()); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := repo.runs[receipt].Status; got != RunCompleted {
		t.Errorf("receipt run is %q with Signals off, want it advanced past the parked marketing backlog", got)
	}
}

// Parking still has to be non-destructive - a paused flow's runs wait where they are and resume on reactivation
func TestPausedFlowRunsAreLeftUntouched(t *testing.T) {
	repo := newClaimRepo()
	svc := &Service{repo: repo}

	paused := endingFlow(repo, StatusPaused, false)
	parked := repo.addRun(paused.ID)

	if err := svc.ProcessRuns(context.Background()); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := repo.runs[parked].Status; got != RunActive {
		t.Errorf("parked run is %q, want it left active so it resumes when the flow is reactivated", got)
	}

	if _, err := repo.SetStatus(context.Background(), paused.ID, StatusActive); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if err := svc.ProcessRuns(context.Background()); err != nil {
		t.Fatalf("process after reactivate: %v", err)
	}
	if got := repo.runs[parked].Status; got != RunCompleted {
		t.Errorf("run is %q after reactivation, want it to resume", got)
	}
}
