// The behavioral-segment engine: definition types, the boolean-tree evaluator, and the scheduled-sync entry points - GraphQL converters and authz helpers live in synapse_helpers.go

package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/analytics"
	"github.com/skeletg/skelet/internal/segment"
)

// Pages rather than one capped List - once auto-affinity segments pushed the total past the cap, behavioral segments silently stopped refreshing
func listAllSegments(ctx context.Context, segs *segment.Service) ([]segment.Segment, error) {
	const pageSize = 500
	var all []segment.Segment
	for off := int32(0); ; off += pageSize {
		page, _, err := segs.List(ctx, pageSize, off)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			return all, nil
		}
	}
}

// Scheduler entry point - one segment's failure is logged and skipped, never fatal
// onEntered fires only for customers NEWLY joining on this refresh - it wires Synapse entry to Pulse's segment_entered flow trigger
func RefreshDefinedSegments(ctx context.Context, rec *analytics.Recorder, segs *segment.Service, now time.Time, onEntered func(channelID, customerID uuid.UUID, slug string)) (int, error) {
	list, err := listAllSegments(ctx, segs)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, s := range list {
		def, ok := parseSegmentDef(s.RefreshSQL)
		if !ok {
			continue
		}
		ids, err := evalSegmentDef(ctx, rec, def, now)
		if err != nil {
			log.Printf("synapse: refresh segment %s (%s): eval: %v", s.Slug, s.ID, err)
			continue
		}
		var prior map[uuid.UUID]bool
		if onEntered != nil {
			pm, _ := segs.MemberIDs(ctx, s.ID)
			prior = make(map[uuid.UUID]bool, len(pm))
			for _, id := range pm {
				prior[id] = true
			}
		}
		if err := materializeSegment(ctx, &Resolver{SegmentSvc: segs}, s.ID, ids); err != nil {
			log.Printf("synapse: refresh segment %s (%s): materialize: %v", s.Slug, s.ID, err)
			continue
		}
		if onEntered != nil {
			cid, _ := uuid.Parse(def.ChannelID)
			for _, id := range ids {
				if !prior[id] {
					onEntered(cid, id, s.Slug)
				}
			}
		}
		n++
	}
	return n, nil
}

// Per (channel x auto-tracked attribute): a segment per value shared by >= minActors customers, capped at maxValues, pruning values that no longer qualify or attributes that were un-flagged
// Membership is materialised at creation so RefreshDefinedSegments doesn't read the whole initial cohort as new entrants - otherwise every new segment blasts segment_entered
// attrs are the catalog attributes flagged synapse_track && auto_segment; individual failures are skipped, not fatal - the scheduler runs this repeatedly
func SyncAffinitySegments(ctx context.Context, rec *analytics.Recorder, segs *segment.Service, channelIDs []uuid.UUID, attrs []analytics.AffinityAttr, minActors, maxValues, windowDays int, now time.Time) (int, int, error) {
	if len(channelIDs) == 0 || len(attrs) == 0 {
		return 0, 0, nil
	}
	if minActors < 1 {
		minActors = 1
	}
	if windowDays <= 0 {
		windowDays = 90
	}
	key := func(parts ...string) string { return strings.Join(parts, "|") }

	list, err := listAllSegments(ctx, segs)
	if err != nil {
		return 0, 0, err
	}
	existing := make(map[string]segment.Segment)
	for _, s := range list {
		def, ok := parseSegmentDef(s.RefreshSQL)
		if !ok || def.Affinity == nil || !def.Affinity.Auto {
			continue
		}
		existing[key(def.ChannelID, def.Affinity.Attr, def.Affinity.Value)] = s
	}

	since := now.AddDate(0, 0, -windowDays)
	seen := make(map[string]bool)    // channel|attr|value present this run
	scanned := make(map[string]bool) // channel|attr discovered without error
	created := 0
	res := &Resolver{SegmentSvc: segs}

	for _, cid := range channelIDs {
		for _, a := range attrs {
			vals, err := rec.AffinityValueCounts(ctx, cid, a.Slug, since, maxValues)
			if err != nil {
				log.Printf("synapse: affinity sync %s/%s: discover values: %v", cid, a.Slug, err)
				continue // transient discovery error - don't prune this (channel,attr)
			}
			scanned[key(cid.String(), a.Slug)] = true
			for _, v := range vals {
				if v.Actors < minActors || strings.TrimSpace(v.Value) == "" {
					continue
				}
				k := key(cid.String(), a.Slug, v.Value)
				seen[k] = true
				def := segmentDef{ChannelID: cid.String(), Affinity: &segAffinity{Attr: a.Slug, Value: v.Value, Days: windowDays, Auto: true}}
				ids, err := evalSegmentDef(ctx, rec, def, now)
				if err != nil {
					log.Printf("synapse: affinity sync %s/%s=%q: eval: %v", cid, a.Slug, v.Value, err)
					continue
				}
				defJSON, err := json.Marshal(def)
				if err != nil {
					continue
				}
				name := a.Name + ": " + v.Value
				if s, ok := existing[k]; ok {
					if _, err := segs.Update(ctx, segment.UpdateInput{ID: s.ID, Slug: s.Slug, Name: name, Description: s.Description, RefreshSQL: string(defJSON), IsActive: true}); err != nil {
						log.Printf("synapse: affinity sync %s/%s=%q: update: %v", cid, a.Slug, v.Value, err)
						continue
					}
					if err := materializeSegment(ctx, res, s.ID, ids); err != nil {
						log.Printf("synapse: affinity sync %s/%s=%q: materialize: %v", cid, a.Slug, v.Value, err)
					}
				} else {
					seg, err := segs.Create(ctx, segment.CreateInput{Slug: slugify(name) + "-" + uuid.NewString()[:6], Name: name, RefreshSQL: string(defJSON), IsActive: true})
					if err != nil {
						log.Printf("synapse: affinity sync %s/%s=%q: create: %v", cid, a.Slug, v.Value, err)
						continue
					}
					if err := materializeSegment(ctx, res, seg.ID, ids); err != nil {
						log.Printf("synapse: affinity sync %s/%s=%q: materialize: %v", cid, a.Slug, v.Value, err)
					}
					created++
				}
			}
		}
	}

	pruned := 0 // pruning is confined to (channel,attr) pairs actually scanned this run, so a transient discovery error can't delete a live segment
	for k, s := range existing {
		if seen[k] {
			continue
		}
		def, ok := parseSegmentDef(s.RefreshSQL)
		if !ok || def.Affinity == nil || !scanned[key(def.ChannelID, def.Affinity.Attr)] {
			continue
		}
		if err := segs.ClearMembership(ctx, s.ID); err != nil {
			log.Printf("synapse: affinity prune %s (%s): clear: %v", s.Slug, s.ID, err)
			continue
		}
		if err := segs.Delete(ctx, s.ID); err != nil {
			log.Printf("synapse: affinity prune %s (%s): delete: %v", s.Slug, s.ID, err)
			continue
		}
		pruned++
	}
	return created, pruned, nil
}

// A behavioral segment is a normal customer segment whose membership Synapse materialises from JSON in the reserved refresh_sql column - campaigns/flows target it like any other

type segCond struct {
	Event  string `json:"event"`
	Negate bool   `json:"negate"`
	Count  int    `json:"count"`
	Days   int    `json:"days"`
}

type segTrait struct { // persisted half of analytics.TraitRule - RFM / lifecycle / engagement
	Lifecycle         []string `json:"lifecycle,omitempty"`
	MinRecencyScore   int      `json:"minRecencyScore,omitempty"`
	MinFrequencyScore int      `json:"minFrequencyScore,omitempty"`
	MinMonetaryScore  int      `json:"minMonetaryScore,omitempty"`
	MinEngagement     int      `json:"minEngagement,omitempty"`
}

func (t *segTrait) active() bool {
	return t != nil && (len(t.Lifecycle) > 0 || t.MinRecencyScore > 0 || t.MinFrequencyScore > 0 ||
		t.MinMonetaryScore > 0 || t.MinEngagement > 0)
}

func (t *segTrait) toAnalytics() analytics.TraitRule {
	if t == nil {
		return analytics.TraitRule{}
	}
	return analytics.TraitRule{
		Lifecycle: t.Lifecycle, MinRecencyScore: t.MinRecencyScore, MinFrequencyScore: t.MinFrequencyScore,
		MinMonetaryScore: t.MinMonetaryScore, MinEngagement: t.MinEngagement,
	}
}

type segAffinity struct { // catalog attribute slug = value, e.g. tire-size = 205/55R16
	Attr  string `json:"attr"`
	Value string `json:"value"`
	Days  int    `json:"days,omitempty"`
	Auto  bool   `json:"auto,omitempty"` // set by SyncAffinitySegments - manual affinity segments leave it false and the sync only manages/prunes Auto ones
}

func (a *segAffinity) active() bool {
	return a != nil && strings.TrimSpace(a.Attr) != "" && strings.TrimSpace(a.Value) != ""
}

func (a *segAffinity) window() int {
	if a == nil || a.Days <= 0 {
		return 90
	}
	return a.Days
}

type segmentDef struct {
	ChannelID  string       `json:"channelId"`
	Root       *segNode     `json:"root,omitempty"`       // nil on flat definitions - parseSegmentDef derives one from the flat fields below so callers always get a tree
	Conditions []segCond    `json:"conditions,omitempty"` // flat fields, all AND'd - what the auto-affinity sync writes
	Trait      *segTrait    `json:"trait,omitempty"`
	Affinity   *segAffinity `json:"affinity,omitempty"`
}

type segNode struct {
	Kind     string       `json:"kind"`         // group | event | trait | affinity
	Op       string       `json:"op,omitempty"` // group only: and | or
	Children []segNode    `json:"children,omitempty"`
	Event    string       `json:"event,omitempty"` // event leaf, with Negate/Count/Days
	Negate   bool         `json:"negate,omitempty"`
	Count    int          `json:"count,omitempty"`
	Days     int          `json:"days,omitempty"`
	Trait    *segTrait    `json:"trait,omitempty"`    // trait leaf
	Affinity *segAffinity `json:"affinity,omitempty"` // affinity leaf
}

const (
	nodeGroup    = "group"
	nodeEvent    = "event"
	nodeTrait    = "trait"
	nodeAffinity = "affinity"
)

// AND-group tree over the flat fields, so a flat definition evaluates and displays through the same path as a tree one
func (d segmentDef) legacyRoot() *segNode {
	g := &segNode{Kind: nodeGroup, Op: "and"}
	for _, c := range d.Conditions {
		g.Children = append(g.Children, segNode{Kind: nodeEvent, Event: c.Event, Negate: c.Negate, Count: c.Count, Days: c.Days})
	}
	if d.Trait.active() {
		t := *d.Trait
		g.Children = append(g.Children, segNode{Kind: nodeTrait, Trait: &t})
	}
	if d.Affinity.active() {
		a := *d.Affinity
		g.Children = append(g.Children, segNode{Kind: nodeAffinity, Affinity: &a})
	}
	if len(g.Children) == 0 {
		return nil
	}
	return g
}

func (d segmentDef) tree() *segNode {
	if d.Root != nil {
		return d.Root
	}
	return d.legacyRoot()
}

// AND groups intersect their children, OR groups union them, and negated event leaves take the channel-universe complement - shared by save and the scheduled refresh
func evalSegmentDef(ctx context.Context, rec *analytics.Recorder, def segmentDef, now time.Time) ([]uuid.UUID, error) {
	cid, err := uuid.Parse(def.ChannelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	root := def.tree()
	if root == nil {
		return nil, fmt.Errorf("a segment needs at least one condition, trait, or affinity rule")
	}
	ev := &nodeEvaluator{rec: rec, cid: cid, now: now}
	set, err := ev.eval(ctx, root)
	if err != nil {
		return nil, err
	}
	return setToSlice(set), nil
}

// Caches the channel universe - fetched at most once, and only when a complement is actually needed
type nodeEvaluator struct {
	rec      *analytics.Recorder
	cid      uuid.UUID
	now      time.Time
	universe map[uuid.UUID]bool
}

func (ev *nodeEvaluator) channelUniverse(ctx context.Context) (map[uuid.UUID]bool, error) {
	if ev.universe != nil {
		return ev.universe, nil
	}
	ids, err := ev.rec.ChannelCustomerIDs(ctx, ev.cid)
	if err != nil {
		return nil, err
	}
	ev.universe = sliceToSet(ids)
	return ev.universe, nil
}

func (ev *nodeEvaluator) eval(ctx context.Context, n *segNode) (map[uuid.UUID]bool, error) {
	switch n.Kind {
	case nodeGroup:
		return ev.evalGroup(ctx, n)
	case nodeEvent:
		ids, err := ev.rec.EventActors(ctx, ev.cid, n.Event, n.Count, n.Days, ev.now)
		if err != nil {
			return nil, err
		}
		set := sliceToSet(ids)
		if n.Negate {
			uni, err := ev.channelUniverse(ctx)
			if err != nil {
				return nil, err
			}
			return complementSet(uni, set), nil
		}
		return set, nil
	case nodeTrait:
		ids, err := ev.rec.TraitActors(ctx, ev.cid, n.Trait.toAnalytics())
		if err != nil {
			return nil, err
		}
		return sliceToSet(ids), nil
	case nodeAffinity:
		if !n.Affinity.active() {
			return map[uuid.UUID]bool{}, nil
		}
		ids, err := ev.rec.AffinityActors(ctx, ev.cid, n.Affinity.Attr, n.Affinity.Value, ev.now.AddDate(0, 0, -n.Affinity.window()))
		if err != nil {
			return nil, err
		}
		return sliceToSet(ids), nil
	default:
		return nil, fmt.Errorf("unknown segment node kind %q", n.Kind)
	}
}

func (ev *nodeEvaluator) evalGroup(ctx context.Context, n *segNode) (map[uuid.UUID]bool, error) {
	if len(n.Children) == 0 {
		return nil, fmt.Errorf("a group needs at least one condition")
	}
	or := strings.EqualFold(n.Op, "or")
	var acc map[uuid.UUID]bool
	for i := range n.Children {
		child := n.Children[i]
		set, err := ev.eval(ctx, &child)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			acc = set
			continue
		}
		if or {
			unionInto(acc, set)
		} else {
			acc = intersectSets(acc, set)
		}
	}
	return acc, nil
}

func sliceToSet(ids []uuid.UUID) map[uuid.UUID]bool {
	s := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}

func setToSlice(s map[uuid.UUID]bool) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(s))
	for id := range s {
		out = append(out, id)
	}
	return out
}

func unionInto(dst, src map[uuid.UUID]bool) {
	for id := range src {
		dst[id] = true
	}
}

func intersectSets(a, b map[uuid.UUID]bool) map[uuid.UUID]bool {
	out := make(map[uuid.UUID]bool)
	small, large := a, b
	if len(b) < len(a) {
		small, large = b, a
	}
	for id := range small {
		if large[id] {
			out[id] = true
		}
	}
	return out
}

func complementSet(universe, exclude map[uuid.UUID]bool) map[uuid.UUID]bool {
	out := make(map[uuid.UUID]bool, len(universe))
	for id := range universe {
		if !exclude[id] {
			out[id] = true
		}
	}
	return out
}

// ok=false for non-behavioral segments (empty / non-JSON / no conditions), so the rest of the segment system is unaffected
func parseSegmentDef(refreshSQL string) (segmentDef, bool) {
	if strings.TrimSpace(refreshSQL) == "" {
		return segmentDef{}, false
	}
	var d segmentDef
	if err := json.Unmarshal([]byte(refreshSQL), &d); err != nil {
		return segmentDef{}, false
	}
	// Behavioral iff it has a tree OR any flat criterion - a tree-only def has no flat fields, so the tree check has to come first or the scheduler treats tree segments as non-behavioral
	if d.Root == nil && len(d.Conditions) == 0 && !d.Trait.active() && !d.Affinity.active() {
		return segmentDef{}, false
	}
	return d, true
}

// The replace is atomic - campaign/flow readers never see the cleared in-between state; the snapshot is best-effort and only feeds the growth chart
func materializeSegment(ctx context.Context, r *Resolver, segID uuid.UUID, ids []uuid.UUID) error {
	if err := r.SegmentSvc.ReplaceMembership(ctx, segID, ids); err != nil {
		return err
	}
	_ = r.SegmentSvc.RecordDailySnapshot(ctx, segID)
	return nil
}

// Callers append a short unique suffix - segment slugs are unique
func slugify(s string) string {
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
