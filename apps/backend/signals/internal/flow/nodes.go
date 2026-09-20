package flow

import (
	"context"
	"log"
	"sync"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
)

// A side-effect node the engine runs and then moves past - control flow (wait, split) stays in the engine
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

// Node types the engine knows - one with no executor is skipped rather than failed, so a node whose
// executor is not wired can't strand runs on flows already using it
var knownNodeTypes = map[string]bool{
	NodeTrigger: true, NodeSendEmail: true, NodeSendSMS: true, NodeWait: true,
	NodeSplit: true, NodeAddSegment: true, NodeWebhook: true,
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

// The record of a send - the log the tracking token names and the spine event beside it, satisfied by *engagement.Tracker
// Best-effort: the mail goes out either way, and a token the log never heard of is ignored when the pixel loads
type sendTracker interface {
	Sent(ctx context.Context, spec sendlog.Spec) error
}

func (s *Service) recordSent(ctx context.Context, send sendlog.Spec) {
	if s.sends == nil {
		return
	}
	if err := s.sends.Sent(ctx, send); err != nil {
		log.Printf("[flow] record send %s: %v", send.ID, err)
	}
}
