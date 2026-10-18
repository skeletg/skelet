// Package behavioralsegment is Synapse's engine over customer segments - a definition rides as JSON in a segment's definition column, and what comes out is a normal segment that campaigns, flows and promotions target like any other
package behavioralsegment

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
)

var (
	ErrNotBehavioral   = errors.New("behavioralsegment: not a behavioral segment")
	ErrEmptyDefinition = errors.New("behavioralsegment: a segment needs at least one condition, trait, or affinity rule")
	ErrInvalidChannel  = errors.New("behavioralsegment: definition has no valid channel")
	ErrUnknownKind     = errors.New("behavioralsegment: unknown node kind")
	ErrEmptyGroup      = errors.New("behavioralsegment: a group needs at least one condition")
	ErrNameRequired    = errors.New("behavioralsegment: a segment needs a name")
	ErrOtherChannel    = errors.New("behavioralsegment: segment belongs to another channel")
	// The triple is a cadence, so it is all three ascending or nothing
	ErrLifecycleWindows = errors.New("behavioralsegment: lifecycle windows must be positive and ascending (new <= active <= at-risk)")
	ErrTooLarge         = errors.New("behavioralsegment: definition is too large")
	ErrBadCondition     = errors.New("behavioralsegment: invalid condition")
)

// Each node is a spine query per refresh, so a definition is bounded in breadth and depth
const (
	maxNodes = 50
	maxDepth = 4
	// The look-back an affinity uses when it names none, and the auto-affinity sync's default window
	DefaultAffinityWindowDays = 90
)

// Node kinds, as they are spelled in the stored JSON
const (
	KindGroup    = "group"
	KindEvent    = "event"
	KindTrait    = "trait"
	KindAffinity = "affinity"
)

const (
	OpAnd = "and"
	OpOr  = "or"
)

type Condition struct {
	Event  string `json:"event"`
	Negate bool   `json:"negate"`
	Count  int    `json:"count"`
	Days   int    `json:"days"`
}

// The persisted half of analytics.TraitRule - RFM, lifecycle and engagement
type Trait struct {
	Lifecycle         []string `json:"lifecycle,omitempty"`
	MinRecencyScore   int      `json:"minRecencyScore,omitempty"`
	MinFrequencyScore int      `json:"minFrequencyScore,omitempty"`
	MinMonetaryScore  int      `json:"minMonetaryScore,omitempty"`
	MinEngagement     int      `json:"minEngagement,omitempty"`
}

func (t *Trait) Active() bool {
	return t != nil && (len(t.Lifecycle) > 0 || t.MinRecencyScore > 0 || t.MinFrequencyScore > 0 ||
		t.MinMonetaryScore > 0 || t.MinEngagement > 0)
}

func (t *Trait) Rule() analytics.TraitRule {
	if t == nil {
		return analytics.TraitRule{}
	}
	return analytics.TraitRule{
		Lifecycle: t.Lifecycle, MinRecencyScore: t.MinRecencyScore, MinFrequencyScore: t.MinFrequencyScore,
		MinMonetaryScore: t.MinMonetaryScore, MinEngagement: t.MinEngagement,
	}
}

// A catalog attribute slug = value, e.g. tire-size = 205/55R16
type Affinity struct {
	Attr  string `json:"attr"`
	Value string `json:"value"`
	Days  int    `json:"days,omitempty"`
	// Set by SyncAffinity, which only manages and prunes its own - a hand-made affinity segment leaves it false
	Auto bool `json:"auto,omitempty"`
}

func (a *Affinity) Active() bool {
	return a != nil && strings.TrimSpace(a.Attr) != "" && strings.TrimSpace(a.Value) != ""
}

func (a *Affinity) Window() int {
	if a == nil || a.Days <= 0 {
		return DefaultAffinityWindowDays
	}
	return a.Days
}

type Definition struct {
	ChannelID string `json:"channelId"`
	// nil on flat definitions - Tree derives one from the flat fields, so callers always get a tree
	Root *Node `json:"root,omitempty"`
	// Flat fields, all AND'd - what the affinity sync writes
	Conditions []Condition `json:"conditions,omitempty"`
	Trait      *Trait      `json:"trait,omitempty"`
	Affinity   *Affinity   `json:"affinity,omitempty"`
}

type Node struct {
	Kind     string    `json:"kind"`
	Op       string    `json:"op,omitempty"`
	Children []Node    `json:"children,omitempty"`
	Event    string    `json:"event,omitempty"`
	Negate   bool      `json:"negate,omitempty"`
	Count    int       `json:"count,omitempty"`
	Days     int       `json:"days,omitempty"`
	Trait    *Trait    `json:"trait,omitempty"`
	Affinity *Affinity `json:"affinity,omitempty"`
}

func (d Definition) Channel() (uuid.UUID, error) {
	id, err := uuid.Parse(d.ChannelID)
	if err != nil {
		return uuid.Nil, ErrInvalidChannel
	}
	return id, nil
}

// An AND group over the flat fields, so a flat definition evaluates and displays through the same path as a tree one
func (d Definition) legacyRoot() *Node {
	g := &Node{Kind: KindGroup, Op: OpAnd}
	for _, c := range d.Conditions {
		g.Children = append(g.Children, Node{Kind: KindEvent, Event: c.Event, Negate: c.Negate, Count: c.Count, Days: c.Days})
	}
	if d.Trait.Active() {
		t := *d.Trait
		g.Children = append(g.Children, Node{Kind: KindTrait, Trait: &t})
	}
	if d.Affinity.Active() {
		a := *d.Affinity
		g.Children = append(g.Children, Node{Kind: KindAffinity, Affinity: &a})
	}
	if len(g.Children) == 0 {
		return nil
	}
	return g
}

// validateTree bounds a definition before it runs: node count, depth, known kinds and ops, and sane event conditions
func validateTree(root *Node) error {
	nodes := 0
	var walk func(n *Node, depth int) error
	walk = func(n *Node, depth int) error {
		if nodes++; nodes > maxNodes {
			return fmt.Errorf("%w: more than %d conditions", ErrTooLarge, maxNodes)
		}
		if depth > maxDepth {
			return fmt.Errorf("%w: groups nest deeper than %d", ErrTooLarge, maxDepth)
		}
		switch n.Kind {
		case KindGroup:
			if n.Op != "" && !strings.EqualFold(n.Op, OpAnd) && !strings.EqualFold(n.Op, OpOr) {
				return fmt.Errorf("%w: group op %q", ErrBadCondition, n.Op)
			}
			for i := range n.Children {
				if err := walk(&n.Children[i], depth+1); err != nil {
					return err
				}
			}
		case KindEvent:
			if strings.TrimSpace(n.Event) == "" || n.Count < 0 || n.Days < 0 {
				return fmt.Errorf("%w: an event condition needs an event name and a count and days of 0 or more", ErrBadCondition)
			}
		case KindAffinity:
			if n.Affinity != nil && n.Affinity.Days < 0 {
				return fmt.Errorf("%w: affinity days can't be negative", ErrBadCondition)
			}
		case KindTrait:
		default:
			return ErrUnknownKind
		}
		return nil
	}
	return walk(root, 1)
}

func (d Definition) Tree() *Node {
	if d.Root != nil {
		return d.Root
	}
	return d.legacyRoot()
}

// ok=false for a segment that is not behavioral (empty, non-JSON, or no criteria), leaving the rest of the segment system alone
func Parse(refreshSQL string) (Definition, bool) {
	if strings.TrimSpace(refreshSQL) == "" {
		return Definition{}, false
	}
	var d Definition
	if err := json.Unmarshal([]byte(refreshSQL), &d); err != nil {
		return Definition{}, false
	}
	// A tree-only definition has no flat fields, so the tree check comes first or the scheduler reads it as non-behavioral
	if d.Root == nil && len(d.Conditions) == 0 && !d.Trait.Active() && !d.Affinity.Active() {
		return Definition{}, false
	}
	return d, true
}

// Segment is a behavioral segment: the stored row, its parsed definition and its member count
type Segment struct {
	segment.Segment
	Definition  Definition
	MemberCount int
}

// Detail is the segment analytics behind the detail screen - money is cents, and the currency is the caller's to add
type Detail struct {
	Segment         Segment
	Orders          int64
	RevenueCents    int64
	AOVCents        int64
	Members         []analytics.SegmentMemberRow
	Growth          []analytics.SegmentGrowthPoint
	LifecycleMix    []analytics.SegmentStageCount
	RfmDistribution []analytics.SegmentRfmBucket
	TopAffinities   []analytics.SegmentAffinityCount
}

// Callers append a short unique suffix - segment slugs are unique
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "segment"
	}
	return out
}
