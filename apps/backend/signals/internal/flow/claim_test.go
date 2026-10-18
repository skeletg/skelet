package flow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

type claimRepo struct {
	*fakeRepo
	order []uuid.UUID
}

func (r *claimRepo) ClaimRunnable(_ context.Context, limit int32, lease time.Duration) ([]FlowRun, error) {
	var out []FlowRun
	now := time.Now()
	for _, id := range r.order {
		if int32(len(out)) >= limit {
			break
		}
		run := r.runs[id]
		due := run.ResumeAt == nil || !run.ResumeAt.After(now)
		if run.Status != RunActive || !due {
			continue
		}
		f := r.flows[run.FlowID]
		if f.Status != StatusActive || f.DeletedAt != nil {
			continue
		}
		until := now.Add(lease)
		run.ResumeAt = &until
		r.runs[id] = run
		out = append(out, run)
	}
	return out, nil
}

func (r *claimRepo) CreateRun(ctx context.Context, run FlowRun) (FlowRun, error) {
	created, err := r.fakeRepo.CreateRun(ctx, run)
	if err == nil {
		r.order = append(r.order, created.ID)
	}
	return created, err
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

// A run the engine would refuse must not fill a claim slot - a few hundred on a paused flow would starve every newer run
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

// A keyed moment asked twice starts one run and counts the flow both times - anything else sends twice or tells core to send its own
func TestAKeyedMomentStartsOneRunAndAnswersTheSameCount(t *testing.T) {
	repo := newFakeRepo()
	// A node nothing executes, so one advance runs the run to completion
	f := mkFlow(repo, []Node{{ID: "t", Type: NodeTrigger}, {ID: "n", Type: NodeAddSegment}}, []Edge{{ID: "e", Source: "t", Target: "n"}})
	svc := &Service{repo: repo}
	ctx := context.Background()
	key := "order_placed:" + uuid.NewString()

	first := svc.Enroll(ctx, f.TriggerType, f.ChannelID, uuid.Nil, "guest@example.test", nil, key)
	for _, run := range repo.runs { // completed, so the already-in-flow guard no longer covers a repeat
		if err := svc.advance(ctx, run); err != nil {
			t.Fatalf("advance: %v", err)
		}
	}
	second := svc.Enroll(ctx, f.TriggerType, f.ChannelID, uuid.Nil, "guest@example.test", nil, key)

	if first != 1 || second != 1 {
		t.Fatalf("answers were %d then %d, want 1 and 1 - the repeat must read exactly like the original", first, second)
	}
	if n := len(repo.runs); n != 1 {
		t.Fatalf("%d runs for one keyed moment, want 1", n)
	}

	// An unkeyed repeat of a finished run is a new moment, as before
	svc.Enroll(ctx, f.TriggerType, f.ChannelID, uuid.Nil, "guest@example.test", nil, "")
	if n := len(repo.runs); n != 2 {
		t.Fatalf("%d runs after an unkeyed repeat, want 2", n)
	}
}

// The drain keeps claiming while the budget lasts, and a zero budget is still the single batch
func TestDrainKeepsClaimingWithinItsBudget(t *testing.T) {
	repo := newClaimRepo()
	svc := &Service{repo: repo}
	live := endingFlow(repo, StatusActive, true)
	for range claimBatch*2 + 5 {
		repo.addRun(live.ID)
	}

	n, err := svc.Drain(context.Background(), 0)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n != claimBatch {
		t.Fatalf("a zero budget advanced %d runs, want exactly one batch of %d", n, claimBatch)
	}

	n, err = svc.Drain(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n != claimBatch+5 {
		t.Fatalf("the budgeted drain advanced %d runs, want the remaining %d across two batches", n, claimBatch+5)
	}
	for id, run := range repo.runs {
		if run.Status != RunCompleted {
			t.Fatalf("run %s is %q after the drain, want completed", id, run.Status)
		}
	}
}
