package flow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type countingNode struct {
	n      *int
	during func()
}

func (countingNode) Type() string { return "count" }
func (c countingNode) Execute(context.Context, Node, FlowRun) {
	*c.n++
	if c.during != nil {
		c.during()
	}
}

func countingFlow(repo *fakeRepo, trigger string) Flow {
	f, _ := repo.Create(context.Background(), Flow{
		ChannelID: uuid.New(), Name: "T", TriggerType: trigger, Status: StatusActive,
		Graph: Graph{
			Nodes: []Node{{ID: "t", Type: NodeTrigger}, {ID: "c", Type: "count"}},
			Edges: []Edge{{ID: "e", Source: "t", Target: "c"}},
		},
	})
	return f
}

func TestASyncRunIsAdvancedByExactlyOnePath(t *testing.T) {
	repo := newClaimRepo()
	svc := &Service{repo: repo, nodes: NewNodeRegistry()}
	sends := 0
	var tickErr error
	svc.RegisterNode(countingNode{n: &sends, during: func() {
		if sends == 1 {
			tickErr = svc.ProcessRuns(context.Background())
		}
	}})
	f := countingFlow(repo.fakeRepo, TriggerPasswordReset)

	n, err := svc.EnrollMoment(context.Background(), f.TriggerType, f.ChannelID, uuid.New(), "", nil, "password_reset:x")
	if err != nil || n != 1 {
		t.Fatalf("enroll = %d, %v, want 1 and no error", n, err)
	}
	if tickErr != nil {
		t.Fatalf("tick: %v", tickErr)
	}
	if sends != 1 {
		t.Fatalf("the email step ran %d times, want 1 - the engine claimed a run the inline advance held", sends)
	}
	for _, run := range repo.runs {
		if run.Status != RunCompleted {
			t.Fatalf("run is %q, want completed by the inline advance", run.Status)
		}
	}
}

func TestEnrollCountsOnlyTheFlowsThatTookTheMoment(t *testing.T) {
	ctx := context.Background()
	cust := uuid.New()

	t.Run("a live run from an earlier order does not take the next one", func(t *testing.T) {
		repo := newFakeRepo()
		f := mkFlow(repo, []Node{{ID: "t", Type: NodeTrigger}, {ID: "w", Type: NodeWait, Data: NodeData{DelaySeconds: 3600}}},
			[]Edge{{ID: "e", Source: "t", Target: "w"}})
		svc := &Service{repo: repo}
		first := "order_placed:" + uuid.NewString()
		if n, err := svc.EnrollMoment(ctx, f.TriggerType, f.ChannelID, cust, "", nil, first); n != 1 || err != nil {
			t.Fatalf("first order = %d, %v, want 1", n, err)
		}
		if n, err := svc.EnrollMoment(ctx, f.TriggerType, f.ChannelID, cust, "", nil, "order_placed:"+uuid.NewString()); n != 0 || err != nil {
			t.Fatalf("second order = %d, %v, want 0 so core sends its receipt", n, err)
		}
		if n, err := svc.EnrollMoment(ctx, f.TriggerType, f.ChannelID, cust, "", nil, first); n != 1 || err != nil {
			t.Fatalf("replay of the first order = %d, %v, want 1 - its run is still live and still owns it", n, err)
		}
	})

	t.Run("a filtered-out flow does not take it", func(t *testing.T) {
		repo := newFakeRepo()
		f := mkFlow(repo, []Node{{ID: "t", Type: NodeTrigger}, {ID: "n", Type: NodeAddSegment}}, []Edge{{ID: "e", Source: "t", Target: "n"}})
		f.TriggerConfig = json.RawMessage(`{"enrollmentFilter":{"kind":"lifecycle","lifecycle":["active"]}}`)
		repo.flows[f.ID] = f
		svc := &Service{repo: repo}
		if n, err := svc.EnrollMoment(ctx, f.TriggerType, f.ChannelID, uuid.Nil, "guest@example.test", nil, "order_placed:"+uuid.NewString()); n != 0 || err != nil {
			t.Fatalf("filtered guest = %d, %v, want 0", n, err)
		}
	})

	t.Run("a store failure is an error, not a zero", func(t *testing.T) {
		repo := newFakeRepo()
		f := mkFlow(repo, []Node{{ID: "t", Type: NodeTrigger}, {ID: "n", Type: NodeAddSegment}}, []Edge{{ID: "e", Source: "t", Target: "n"}})
		repo.createErr = errors.New("db down")
		svc := &Service{repo: repo}
		if _, err := svc.EnrollMoment(ctx, f.TriggerType, f.ChannelID, cust, "", nil, "order_placed:"+uuid.NewString()); err == nil {
			t.Fatal("a failed insert answered without an error, so the caller would neither send nor retry")
		}
	})
}
