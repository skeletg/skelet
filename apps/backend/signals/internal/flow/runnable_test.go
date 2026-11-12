package flow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

func node(id, typ string, data NodeData) Node { return Node{ID: id, Type: typ, Data: data} }
func edge(from, to string, handle ...string) Edge {
	e := Edge{ID: from + ">" + to, Source: from, Target: to}
	if len(handle) > 0 {
		e.SourceHandle = handle[0]
	}
	return e
}

var (
	trigger = node("t", NodeTrigger, NodeData{})
	mail    = node("mail", NodeSendEmail, NodeData{EmailID: uuid.NewString()})
	wait    = node("wait", NodeWait, NodeData{DelaySeconds: 60})
)

func TestAFlowThatIsWiredAndFilledInCanRun(t *testing.T) {
	own := node("own", NodeSendEmail, NodeData{Subject: "Shipped", Blocks: []emailtemplate.Block{{Type: "heading"}}})
	split := node("split", NodeSplit, NodeData{Condition: &Condition{Kind: "lifecycle", Lifecycle: []string{"active"}}})
	g := Graph{
		Nodes: []Node{trigger, mail, split, wait, own, node("loose", NodeWebhook, NodeData{})},
		Edges: []Edge{edge("t", "mail"), edge("mail", "split"), edge("split", "wait", "yes"), edge("split", "own", "no")},
	}
	if err := g.runnable(); err != nil {
		t.Errorf("runnable() = %v - a step no run can reach must not hold the flow back", err)
	}
}

func TestActivatingNamesTheFirstThingThatWouldGoWrong(t *testing.T) {
	for name, tc := range map[string]struct {
		g    Graph
		want string
	}{
		"no trigger":            {Graph{Nodes: []Node{mail}}, "needs a trigger node"},
		"trigger leads nowhere": {Graph{Nodes: []Node{trigger, mail}}, "nothing follows the trigger"},
		"no email": {
			Graph{Nodes: []Node{trigger, node("m", NodeSendEmail, NodeData{})}, Edges: []Edge{edge("t", "m")}},
			"a send-email step has no email picked",
		},
		"segment split with no segment": {
			Graph{Nodes: []Node{trigger, node("s", NodeSplit, NodeData{Condition: &Condition{Kind: "segment"}})}, Edges: []Edge{edge("t", "s")}},
			"a split has no segment to branch on",
		},
		"add to no segment": {
			Graph{Nodes: []Node{trigger, wait, node("seg", NodeAddSegment, NodeData{})}, Edges: []Edge{edge("t", "wait"), edge("wait", "seg")}},
			"an add-to-segment step has no segment picked",
		},
		"webhook with no url": {
			Graph{Nodes: []Node{trigger, node("h", NodeWebhook, NodeData{URL: " "})}, Edges: []Edge{edge("t", "h")}},
			"a webhook step has no URL to call",
		},
		"a loop": {
			Graph{Nodes: []Node{trigger, mail, wait}, Edges: []Edge{edge("t", "mail"), edge("mail", "wait"), edge("wait", "mail")}},
			"loops back on itself",
		},
		"a connection to a step that is gone": {
			Graph{Nodes: []Node{trigger, mail}, Edges: []Edge{edge("t", "mail"), edge("mail", "gone")}},
			"no longer there",
		},
		"a problem down the no branch": {
			Graph{
				Nodes: []Node{trigger, node("s", NodeSplit, NodeData{Condition: &Condition{Kind: "lifecycle"}}), node("sms", NodeSendSMS, NodeData{})},
				Edges: []Edge{edge("t", "s"), edge("s", "sms", "no")},
			},
			"a send-SMS step has no message",
		},
	} {
		err := tc.g.runnable()
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: runnable() = %v, want ErrInvalid naming %q", name, err, tc.want)
		}
	}
}

func TestOnlyActivatingAsksAFlowToBeWhole(t *testing.T) {
	repo := newFakeRepo()
	f := Flow{ID: uuid.New(), Status: StatusDraft, Graph: Graph{Nodes: []Node{trigger, node("m", NodeSendEmail, NodeData{})}, Edges: []Edge{edge("t", "m")}}}
	repo.flows[f.ID] = f
	svc := &Service{repo: repo}
	ctx := context.Background()

	if _, err := svc.SetStatus(ctx, f.ID, StatusActive); !errors.Is(err, ErrInvalid) {
		t.Fatalf("activate: err = %v, want ErrInvalid", err)
	}
	if repo.flows[f.ID].Status != StatusDraft {
		t.Errorf("status = %q after a refused activation, want draft", repo.flows[f.ID].Status)
	}
	if _, err := svc.SetStatus(ctx, f.ID, StatusPaused); err != nil {
		t.Errorf("pause: %v - only activating checks the graph", err)
	}

	f.Graph.Nodes[1].Data.EmailID = uuid.NewString()
	repo.flows[f.ID] = f
	if got, err := svc.SetStatus(ctx, f.ID, StatusActive); err != nil || got.Status != StatusActive {
		t.Errorf("activate once whole: status %q, err %v", got.Status, err)
	}
}
