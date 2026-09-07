package flow

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/sendlog"
)

// A split condition the engine can evaluate, registered rather than switched so a suite that owns its data contributes the condition instead of core declaring one it can't answer
type ConditionEvaluator interface {
	Kind() string
	Matches(ctx context.Context, customerID uuid.UUID, c *Condition) bool
}

// An unregistered kind is unknown to validation too, so a flow can't be saved against a condition nothing can answer
type ConditionRegistry struct {
	mu sync.RWMutex
	m  map[string]ConditionEvaluator
}

func NewConditionRegistry() *ConditionRegistry {
	return &ConditionRegistry{m: map[string]ConditionEvaluator{}}
}

// Last registration wins, so a suite can replace a core default rather than collide with it
func (r *ConditionRegistry) Register(e ConditionEvaluator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[e.Kind()] = e
}

func (r *ConditionRegistry) Lookup(kind string) (ConditionEvaluator, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.m[kind]
	return e, ok
}

func (r *ConditionRegistry) Kinds() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.m))
	for k := range r.m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// --- the reads each condition needs, narrow so flow declares them rather than importing the suite that serves them ---

type segmentMembers interface {
	MemberIDs(ctx context.Context, segmentID uuid.UUID) ([]uuid.UUID, error)
}

// The trait slice a split reads, in flow's own shape - the bool is load-bearing, since a customer with no traits row must not match an rfm condition whose minimums are all zero
type CustomerTraits struct {
	RFMRecency      int
	RFMFrequency    int
	RFMMonetary     int
	Lifecycle       string
	EngagementScore int
}

type traitReader interface {
	CustomerTraits(ctx context.Context, customerID uuid.UUID) (CustomerTraits, bool, error)
}

type engagementReader interface {
	CustomerEngagement(ctx context.Context, customerID uuid.UUID) (sendlog.Engagement, error)
}

type consentReader interface {
	IsOptedIn(ctx context.Context, customerID uuid.UUID, kind string) (bool, error)
}

// --- core's own conditions: neither needs the analytics stack ---

type consentCondition struct{ consent consentReader }

func (consentCondition) Kind() string { return "consent" }

func (c consentCondition) Matches(ctx context.Context, customerID uuid.UUID, cond *Condition) bool {
	kind := strings.TrimSpace(cond.ConsentKind)
	if kind == "" || c.consent == nil {
		return false
	}
	optedIn, _ := c.consent.IsOptedIn(ctx, customerID, kind)
	return optedIn == cond.OptedIn
}

// Counts off the send log, which is core's - no Signals needed to know whether a customer opens their mail
type emailActivityCondition struct{ sends engagementReader }

func (emailActivityCondition) Kind() string { return "email_activity" }

func (c emailActivityCondition) Matches(ctx context.Context, customerID uuid.UUID, cond *Condition) bool {
	if c.sends == nil {
		return false
	}
	eng, err := c.sends.CustomerEngagement(ctx, customerID)
	if err != nil {
		return false
	}
	return eng.Opened >= cond.MinOpens && eng.Clicked >= cond.MinClicks
}

// --- Synapse-backed conditions: registered only when the trait and segment data behind them exists ---

type segmentCondition struct{ segments segmentMembers }

func (segmentCondition) Kind() string { return "segment" }

func (c segmentCondition) Matches(ctx context.Context, customerID uuid.UUID, cond *Condition) bool {
	if c.segments == nil {
		return false
	}
	sid, err := uuid.Parse(cond.SegmentID)
	if err != nil {
		return false
	}
	ids, err := c.segments.MemberIDs(ctx, sid)
	if err != nil {
		return false
	}
	return slices.Contains(ids, customerID)
}

// Saved flows still carry the older spelling, so it resolves to the same evaluator
type segmentAliasCondition struct{ segmentCondition }

func (segmentAliasCondition) Kind() string { return conditionInSegmentAlias }

type lifecycleCondition struct{ traits traitReader }

func (lifecycleCondition) Kind() string { return "lifecycle" }

func (c lifecycleCondition) Matches(ctx context.Context, customerID uuid.UUID, cond *Condition) bool {
	if len(cond.Lifecycle) == 0 || c.traits == nil {
		return false
	}
	t, ok, err := c.traits.CustomerTraits(ctx, customerID)
	if err != nil || !ok {
		return false
	}
	return slices.Contains(cond.Lifecycle, t.Lifecycle)
}

type rfmCondition struct{ traits traitReader }

func (rfmCondition) Kind() string { return "rfm" }

func (c rfmCondition) Matches(ctx context.Context, customerID uuid.UUID, cond *Condition) bool {
	if c.traits == nil {
		return false
	}
	t, ok, err := c.traits.CustomerTraits(ctx, customerID)
	if err != nil || !ok {
		return false
	}
	return t.RFMRecency >= cond.MinRecency &&
		t.RFMFrequency >= cond.MinFrequency &&
		t.RFMMonetary >= cond.MinMonetary &&
		t.EngagementScore >= cond.MinEngagement
}

// A kind nothing has registered can't be evaluated, so a flow must not be saveable against it
func (s *Service) validConditionKind(k string) bool {
	_, ok := s.conditions.Lookup(k)
	return ok
}

// Constructors for the conditions a suite registers - the evaluator types stay unexported so the registry is the only way in
func NewSegmentCondition(m segmentMembers) ConditionEvaluator {
	return segmentCondition{segments: m}
}

func NewSegmentAliasCondition(m segmentMembers) ConditionEvaluator {
	return segmentAliasCondition{segmentCondition{segments: m}}
}

func NewLifecycleCondition(t traitReader) ConditionEvaluator { return lifecycleCondition{traits: t} }

func NewRFMCondition(t traitReader) ConditionEvaluator { return rfmCondition{traits: t} }
