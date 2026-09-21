package flow

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type recordingAssigner struct{ calls int }

func (r *recordingAssigner) AssignCustomers(context.Context, uuid.UUID, []uuid.UUID) error {
	r.calls++
	return nil
}

// Uninstalling a suite must not strand customers mid-journey on flows that already use its node
func TestKnownNodeWithNoExecutorIsSkippedNotFailed(t *testing.T) {
	s := &Service{nodes: NewNodeRegistry()}
	if !s.runSideEffect(context.Background(), Node{ID: "n", Type: NodeAddSegment}, FlowRun{}) {
		t.Error("add_segment with no executor stopped the run, want it skipped so the journey continues")
	}
}

// A type the engine has never heard of is still a broken graph, not an absent suite
func TestUnknownNodeTypeStillStopsTheRun(t *testing.T) {
	s := &Service{nodes: NewNodeRegistry()}
	if s.runSideEffect(context.Background(), Node{ID: "n", Type: "vibes"}, FlowRun{}) {
		t.Error("an unknown node type was skipped, want the run failed")
	}
}

func TestRegisteredNodeRuns(t *testing.T) {
	a := &recordingAssigner{}
	s := &Service{nodes: NewNodeRegistry()}
	s.RegisterNode(NewAddSegmentNode(a))
	run := FlowRun{CustomerID: uuid.New()}
	if !s.runSideEffect(context.Background(), Node{ID: "n", Type: NodeAddSegment, Data: NodeData{SegmentID: uuid.New().String()}}, run) {
		t.Fatal("registered node did not report progress")
	}
	if a.calls != 1 {
		t.Errorf("assigner called %d times, want 1", a.calls)
	}
}

// A guest run has no customer to assign, so the node must do nothing rather than assign the zero uuid
func TestAddSegmentSkipsGuests(t *testing.T) {
	a := &recordingAssigner{}
	n := NewAddSegmentNode(a)
	n.Execute(context.Background(), Node{Data: NodeData{SegmentID: uuid.New().String()}}, FlowRun{})
	if a.calls != 0 {
		t.Errorf("assigner called %d times for a guest run, want 0", a.calls)
	}
}
