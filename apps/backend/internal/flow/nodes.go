package flow

import (
	"context"
	"log"
	"sync"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/sendlog"
	"github.com/skeletg/skelet/apps/backend/internal/signalsclient"
)

// A side-effect node the engine runs and then moves past - control flow (wait, split, remote_action) stays in the engine
type NodeExecutor interface {
	Type() string
	Execute(ctx context.Context, node Node, run FlowRun)
}

type NodeRegistry struct {
	mu sync.RWMutex
	m  map[string]NodeExecutor
}

func NewNodeRegistry() *NodeRegistry { return &NodeRegistry{m: map[string]NodeExecutor{}} }

func (r *NodeRegistry) Register(e NodeExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[e.Type()] = e
}

func (r *NodeRegistry) Lookup(t string) (NodeExecutor, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.m[t]
	return e, ok
}

// Node types the engine knows - one with no executor is skipped rather than failed, so uninstalling a suite can't strand runs on flows already using its node
var knownNodeTypes = map[string]bool{
	NodeTrigger: true, NodeSendEmail: true, NodeSendSMS: true, NodeWait: true,
	NodeSplit: true, NodeAddSegment: true, NodeWebhook: true, NodeRemoteAction: true,
}

// RegisterNode adds an executor for a side-effect node - a suite supplies the one for the node it owns
func (s *Service) RegisterNode(e NodeExecutor) { s.nodes.Register(e) }

// runSideEffect reports whether the engine should carry on past this node
func (s *Service) runSideEffect(ctx context.Context, node Node, run FlowRun) bool {
	if e, ok := s.nodes.Lookup(node.Type); ok {
		e.Execute(ctx, node, run)
		return true
	}
	if knownNodeTypes[node.Type] {
		log.Printf("[flow] node %s (%s) has no executor registered, skipping", node.ID, node.Type)
		return true
	}
	return false
}

// The add_segment node - Synapse owns segment membership, so the executor is registered rather than built in
type segmentAssigner interface {
	AssignCustomers(ctx context.Context, segmentID uuid.UUID, customerIDs []uuid.UUID) error
}

type addSegmentNode struct{ segments segmentAssigner }

func NewAddSegmentNode(a segmentAssigner) NodeExecutor { return addSegmentNode{segments: a} }

func (addSegmentNode) Type() string { return NodeAddSegment }

func (n addSegmentNode) Execute(ctx context.Context, node Node, run FlowRun) {
	if run.IsGuest() || n.segments == nil {
		return // no customer to add to a segment
	}
	sid, err := uuid.Parse(node.Data.SegmentID)
	if err != nil {
		return
	}
	if err := n.segments.AssignCustomers(ctx, sid, []uuid.UUID{run.CustomerID}); err != nil {
		log.Printf("[flow] add_segment %s: %v", sid, err)
	}
}

// The spine event written beside a send - optional, since the log itself is core's and a send is recorded either way
// Satisfied by *signalsclient.Client - flow declares the shape rather than importing the suite
type spineRecorder interface {
	RecordEmailSent(ctx context.Context, s signalsclient.SendRef) error
}

func (s *Service) recordSent(ctx context.Context, send sendlog.Send) {
	if s.spine == nil {
		return
	}
	_ = s.spine.RecordEmailSent(ctx, signalsclient.SendRef{
		ID: send.ID, ChannelID: send.ChannelID, CustomerID: send.CustomerID, Source: send.Source,
	})
}
