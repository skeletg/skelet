package flow

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeRepo is an in-memory Repository for engine state-machine tests (the
// node executors that need live services are covered by the E2E)
type fakeRepo struct {
	flows map[uuid.UUID]Flow
	runs  map[uuid.UUID]FlowRun
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{flows: map[uuid.UUID]Flow{}, runs: map[uuid.UUID]FlowRun{}}
}

func (r *fakeRepo) Create(_ context.Context, f Flow) (Flow, error) {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	r.flows[f.ID] = f
	return f, nil
}
func (r *fakeRepo) Get(_ context.Context, id uuid.UUID) (Flow, bool, error) {
	f, ok := r.flows[id]
	return f, ok, nil
}
func (r *fakeRepo) List(context.Context, uuid.UUID, bool) ([]Flow, error) { return nil, nil }
func (r *fakeRepo) ListActiveByTrigger(_ context.Context, ch uuid.UUID, trig string) ([]Flow, error) {
	var out []Flow
	for _, f := range r.flows {
		if f.ChannelID == ch && f.TriggerType == trig && f.Status == StatusActive {
			out = append(out, f)
		}
	}
	return out, nil
}
func (r *fakeRepo) Update(_ context.Context, f Flow) (bool, error) {
	if _, ok := r.flows[f.ID]; !ok {
		return false, nil
	}
	r.flows[f.ID] = f
	return true, nil
}
func (r *fakeRepo) SetStatus(_ context.Context, id uuid.UUID, st string) (bool, error) {
	f, ok := r.flows[id]
	if !ok {
		return false, nil
	}
	f.Status = st
	r.flows[id] = f
	return true, nil
}
func (r *fakeRepo) SoftDelete(_ context.Context, id uuid.UUID) error {
	f, ok := r.flows[id]
	if !ok {
		return nil
	}
	now := time.Now()
	f.DeletedAt = &now
	r.flows[id] = f
	return nil
}

func (r *fakeRepo) Restore(_ context.Context, id uuid.UUID) error {
	f, ok := r.flows[id]
	if !ok {
		return nil
	}
	f.DeletedAt = nil
	r.flows[id] = f
	return nil
}
func (r *fakeRepo) CountLiveRuns(_ context.Context, flowID, custID uuid.UUID) (int64, error) {
	var n int64
	for _, run := range r.runs {
		if run.FlowID == flowID && run.CustomerID == custID && (run.Status == RunActive || run.Status == RunWaiting) {
			n++
		}
	}
	return n, nil
}
func (r *fakeRepo) CountLiveGuestRuns(_ context.Context, flowID uuid.UUID, email string) (int64, error) {
	var n int64
	for _, run := range r.runs {
		if run.FlowID == flowID && run.CustomerID == uuid.Nil && run.Email == email && (run.Status == RunActive || run.Status == RunWaiting) {
			n++
		}
	}
	return n, nil
}
func (r *fakeRepo) CreateRun(_ context.Context, run FlowRun) (FlowRun, error) {
	if run.EnrollmentKey != "" { // the partial unique index, in memory
		for _, have := range r.runs {
			if have.FlowID == run.FlowID && have.EnrollmentKey == run.EnrollmentKey {
				return FlowRun{}, ErrMomentTaken
			}
		}
	}
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	r.runs[run.ID] = run
	return run, nil
}
func (r *fakeRepo) ClaimRunnable(context.Context, int32) ([]FlowRun, error) { return nil, nil }
func (r *fakeRepo) UpdateRun(_ context.Context, run FlowRun) error          { r.runs[run.ID] = run; return nil }
func (r *fakeRepo) ListRuns(context.Context, uuid.UUID, int32) ([]FlowRun, error) {
	return nil, nil
}

func (r *fakeRepo) RunsForCustomer(context.Context, uuid.UUID) ([]CustomerRun, error) {
	return nil, nil
}

func mkFlow(repo *fakeRepo, nodes []Node, edges []Edge) Flow {
	f, _ := repo.Create(context.Background(), Flow{
		ChannelID: uuid.New(), Name: "T", TriggerType: TriggerOrderPlaced, Status: StatusActive,
		Graph: Graph{Nodes: nodes, Edges: edges},
	})
	return f
}

func TestEnrollAndWaitParkResumeComplete(t *testing.T) {
	repo := newFakeRepo()
	f := mkFlow(repo,
		[]Node{{ID: "t", Type: NodeTrigger}, {ID: "w", Type: NodeWait, Data: NodeData{DelaySeconds: 3600}}},
		[]Edge{{ID: "e1", Source: "t", Target: "w"}}, // wait has no outgoing edge → ends after it
	)
	svc := &Service{repo: repo}
	cust := uuid.New()

	// enroll → one active run positioned at the wait node
	if _, err := svc.enroll(context.Background(), f, cust, "", nil, ""); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if len(repo.runs) != 1 {
		t.Fatalf("want 1 run, got %d", len(repo.runs))
	}
	var run FlowRun
	for _, r := range repo.runs {
		run = r
	}
	if run.CurrentNodeID != "w" || run.Status != RunActive {
		t.Fatalf("run should start at 'w' active, got %s/%s", run.CurrentNodeID, run.Status)
	}

	// re-enroll while live → skipped (one-run guard)
	_, _ = svc.enroll(context.Background(), f, cust, "", nil, "")
	if len(repo.runs) != 1 {
		t.Fatalf("re-enroll should be skipped, got %d runs", len(repo.runs))
	}

	// advance → parks at wait (waiting, resume_at set)
	if err := svc.advance(context.Background(), run); err != nil {
		t.Fatalf("advance(park): %v", err)
	}
	run = repo.runs[run.ID]
	if run.Status != RunWaiting || run.ResumeAt == nil {
		t.Fatalf("expected waiting+resume_at, got %s resume=%v", run.Status, run.ResumeAt)
	}

	// advance again (as if due) → moves past wait; no next → completed
	if err := svc.advance(context.Background(), run); err != nil {
		t.Fatalf("advance(resume): %v", err)
	}
	run = repo.runs[run.ID]
	if run.Status != RunCompleted {
		t.Fatalf("expected completed, got %s", run.Status)
	}
}

func TestGuestEnrollmentKeyedByEmail(t *testing.T) {
	repo := newFakeRepo()
	f := mkFlow(repo,
		[]Node{{ID: "t", Type: NodeTrigger}, {ID: "e", Type: NodeSendEmail}},
		[]Edge{{ID: "e1", Source: "t", Target: "e"}},
	)
	svc := &Service{repo: repo}
	ctx := context.Background()

	// A guest (no customer id) enrolls keyed by email
	if _, err := svc.enroll(ctx, f, uuid.Nil, "guest@example.com", nil, ""); err != nil {
		t.Fatalf("guest enroll: %v", err)
	}
	// Re-enroll the same guest email while live → deduped (one run)
	if _, err := svc.enroll(ctx, f, uuid.Nil, "guest@example.com", nil, ""); err != nil {
		t.Fatalf("guest re-enroll: %v", err)
	}
	// A different guest email → a separate run
	if _, err := svc.enroll(ctx, f, uuid.Nil, "other@example.com", nil, ""); err != nil {
		t.Fatalf("other guest enroll: %v", err)
	}
	if len(repo.runs) != 2 {
		t.Fatalf("want 2 guest runs (deduped per email), got %d", len(repo.runs))
	}
	for _, r := range repo.runs {
		if !r.IsGuest() || r.Email == "" {
			t.Fatalf("guest run should have no customer + an email, got custID=%s email=%q", r.CustomerID, r.Email)
		}
	}
}

func TestSplitBranchAndMissingNode(t *testing.T) {
	repo := newFakeRepo()
	// trigger → split; split 'no' → end - No condition (nil) → 'no' branch
	f := mkFlow(repo,
		[]Node{{ID: "t", Type: NodeTrigger}, {ID: "s", Type: NodeSplit}, {ID: "n", Type: NodeTrigger}},
		[]Edge{{ID: "e1", Source: "t", Target: "s"}, {ID: "e2", Source: "s", Target: "n", SourceHandle: "no"}},
	)
	svc := &Service{repo: repo}
	if _, err := svc.enroll(context.Background(), f, uuid.New(), "", nil, ""); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	var run FlowRun
	for _, r := range repo.runs {
		run = r
	}
	// at split → nil condition picks 'no' → node 'n' (trigger pass-through) → no next → complete
	if err := svc.advance(context.Background(), run); err != nil {
		t.Fatalf("advance: %v", err)
	}
	if repo.runs[run.ID].Status != RunCompleted {
		t.Fatalf("expected completed after 'no' branch, got %s", repo.runs[run.ID].Status)
	}
}
