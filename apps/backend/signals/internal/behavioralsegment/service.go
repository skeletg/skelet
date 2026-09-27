package behavioralsegment

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
)

type Service struct {
	rec  *analytics.Recorder
	segs *segment.Service
}

func New(rec *analytics.Recorder, segs *segment.Service) *Service {
	return &Service{rec: rec, segs: segs}
}

// Evaluate turns a definition into the customers it matches, without touching membership
func (s *Service) Evaluate(ctx context.Context, def Definition, now time.Time) ([]uuid.UUID, error) {
	cid, err := def.Channel()
	if err != nil {
		return nil, err
	}
	root := def.Tree()
	if root == nil {
		return nil, ErrEmptyDefinition
	}
	ev := &nodeEvaluator{rec: s.rec, cid: cid, now: now}
	set, err := ev.eval(ctx, root)
	if err != nil {
		return nil, err
	}
	return setToSlice(set), nil
}

// Get answers ErrNotBehavioral for a hand-managed segment
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Segment, error) {
	seg, err := s.segs.Get(ctx, id)
	if err != nil {
		return Segment{}, err
	}
	def, ok := Parse(seg.Definition)
	if !ok {
		return Segment{}, ErrNotBehavioral
	}
	count, _ := s.segs.MemberCount(ctx, id)
	return Segment{Segment: seg, Definition: def, MemberCount: int(count)}, nil
}

// List answers one channel's behavioral segments, member counts included
func (s *Service) List(ctx context.Context, channelID string) ([]Segment, error) {
	all, err := s.listAll(ctx)
	if err != nil {
		return nil, err
	}
	mine := make([]Segment, 0)
	ids := make([]uuid.UUID, 0)
	for _, seg := range all {
		def, ok := Parse(seg.Definition)
		if !ok || def.ChannelID != channelID {
			continue
		}
		mine = append(mine, Segment{Segment: seg, Definition: def})
		ids = append(ids, seg.ID)
	}
	counts, _ := s.segs.MemberCounts(ctx, ids)
	for i := range mine {
		mine[i].MemberCount = int(counts[mine[i].ID])
	}
	return mine, nil
}

// SaveInput creates when SegmentID is nil and updates that segment otherwise
type SaveInput struct {
	ChannelID  string
	Name       string
	SegmentID  *uuid.UUID
	Definition Definition
}

// Save evaluates first, so a definition that cannot run never reaches the store
// An update only accepts a behavioral segment in the same channel, so an id cannot hijack a hand-managed audience
func (s *Service) Save(ctx context.Context, in SaveInput, now time.Time) (Segment, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Segment{}, ErrNameRequired
	}
	def := in.Definition
	def.ChannelID = in.ChannelID
	ids, err := s.Evaluate(ctx, def, now)
	if err != nil {
		return Segment{}, err
	}
	defJSON, err := json.Marshal(def)
	if err != nil {
		return Segment{}, err
	}
	var seg segment.Segment
	if in.SegmentID != nil {
		existing, err := s.Get(ctx, *in.SegmentID)
		if err != nil {
			return Segment{}, err
		}
		if existing.Definition.ChannelID != in.ChannelID {
			return Segment{}, ErrOtherChannel
		}
		seg, err = s.segs.Update(ctx, segment.UpdateInput{
			ID: *in.SegmentID, Slug: existing.Slug, Name: in.Name,
			Description: existing.Description, Definition: string(defJSON), IsActive: true,
		})
		if err != nil {
			return Segment{}, err
		}
	} else {
		seg, err = s.segs.Create(ctx, segment.CreateInput{
			Slug: Slugify(in.Name) + "-" + uuid.NewString()[:6], Name: in.Name, Definition: string(defJSON), IsActive: true,
		})
		if err != nil {
			return Segment{}, err
		}
	}
	if err := s.materialize(ctx, seg.ID, ids); err != nil {
		return Segment{}, err
	}
	return Segment{Segment: seg, Definition: def, MemberCount: len(ids)}, nil
}

// Refresh re-evaluates one segment's full tree, so it matches what a save would have produced
func (s *Service) Refresh(ctx context.Context, id uuid.UUID, now time.Time) (Segment, error) {
	seg, err := s.Get(ctx, id)
	if err != nil {
		return Segment{}, err
	}
	ids, err := s.Evaluate(ctx, seg.Definition, now)
	if err != nil {
		return Segment{}, err
	}
	if err := s.materialize(ctx, id, ids); err != nil {
		return Segment{}, err
	}
	seg.MemberCount = len(ids)
	return seg, nil
}

// SetLifecycleWindows takes nil to clear the override, and members fall back to the channel default
func (s *Service) SetLifecycleWindows(ctx context.Context, id uuid.UUID, newDays, activeDays, atRiskDays *int32) (Segment, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return Segment{}, err
	}
	if newDays != nil || activeDays != nil || atRiskDays != nil {
		if newDays == nil || activeDays == nil || atRiskDays == nil ||
			*newDays <= 0 || *activeDays < *newDays || *atRiskDays < *activeDays {
			return Segment{}, ErrLifecycleWindows
		}
	}
	seg, err := s.segs.SetLifecycleWindows(ctx, id, newDays, activeDays, atRiskDays)
	if err != nil {
		return Segment{}, err
	}
	def, _ := Parse(seg.Definition)
	count, _ := s.segs.MemberCount(ctx, id)
	return Segment{Segment: seg, Definition: def, MemberCount: int(count)}, nil
}

// Members are capped at memberCap, and growth and affinities both look back growthDays
func (s *Service) Detail(ctx context.Context, id uuid.UUID, memberCap, growthDays, affinityLimit int) (Detail, error) {
	seg, err := s.Get(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	cid, err := seg.Definition.Channel()
	if err != nil {
		return Detail{}, err
	}
	orders, revenue, err := s.rec.SegmentMemberStats(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	members, err := s.rec.SegmentMembers(ctx, id, memberCap)
	if err != nil {
		return Detail{}, err
	}
	growth, err := s.rec.SegmentGrowth(ctx, id, growthDays)
	if err != nil {
		return Detail{}, err
	}
	mix, err := s.rec.SegmentLifecycleMix(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	rfm, err := s.rec.SegmentRfmDistribution(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	affs, err := s.rec.SegmentTopAffinities(ctx, id, cid, time.Now().AddDate(0, 0, -growthDays), affinityLimit)
	if err != nil {
		return Detail{}, err
	}
	var aov int64
	if orders > 0 {
		aov = revenue / orders
	}
	return Detail{
		Segment: seg, Orders: orders, RevenueCents: revenue, AOVCents: aov,
		Members: members, Growth: growth, LifecycleMix: mix, RfmDistribution: rfm, TopAffinities: affs,
	}, nil
}

// RefreshAll re-materialises every behavioral segment - one segment's failure is logged and skipped, never fatal
// onEntered fires only for customers newly joining on this pass, which is what drives Pulse's segment_entered trigger
func (s *Service) RefreshAll(ctx context.Context, now time.Time, onEntered func(channelID, customerID uuid.UUID, slug string)) (int, error) {
	list, err := s.listAll(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, seg := range list {
		def, ok := Parse(seg.Definition)
		if !ok {
			continue
		}
		ids, err := s.Evaluate(ctx, def, now)
		if err != nil {
			log.Printf("synapse: refresh segment %s (%s): eval: %v", seg.Slug, seg.ID, err)
			continue
		}
		var prior map[uuid.UUID]bool
		if onEntered != nil {
			pm, _ := s.segs.MemberIDs(ctx, seg.ID)
			prior = make(map[uuid.UUID]bool, len(pm))
			for _, id := range pm {
				prior[id] = true
			}
		}
		if err := s.materialize(ctx, seg.ID, ids); err != nil {
			log.Printf("synapse: refresh segment %s (%s): materialize: %v", seg.Slug, seg.ID, err)
			continue
		}
		if onEntered != nil {
			cid, _ := uuid.Parse(def.ChannelID)
			for _, id := range ids {
				if !prior[id] {
					onEntered(cid, id, seg.Slug)
				}
			}
		}
		n++
	}
	return n, nil
}

// AffinitySyncOptions bounds one sync pass - zero values take the defaults
type AffinitySyncOptions struct {
	Channels   []uuid.UUID
	Attributes []analytics.AffinityAttr
	MinActors  int
	MaxValues  int
	WindowDays int
}

// SyncAffinity keeps one segment per (channel x tracked attribute x value) shared by at least MinActors customers, pruning values that no longer qualify and attributes that were un-flagged Membership is materialised at creation, so RefreshAll does not read the whole initial cohort as new entrants
func (s *Service) SyncAffinity(ctx context.Context, opts AffinitySyncOptions, now time.Time) (created, pruned int, err error) {
	if len(opts.Channels) == 0 || len(opts.Attributes) == 0 {
		return 0, 0, nil
	}
	if opts.MinActors < 1 {
		opts.MinActors = 1
	}
	if opts.WindowDays <= 0 {
		opts.WindowDays = 90
	}
	key := func(parts ...string) string { return strings.Join(parts, "|") }

	list, err := s.listAll(ctx)
	if err != nil {
		return 0, 0, err
	}
	existing := make(map[string]segment.Segment)
	for _, seg := range list {
		def, ok := Parse(seg.Definition)
		if !ok || def.Affinity == nil || !def.Affinity.Auto {
			continue
		}
		existing[key(def.ChannelID, def.Affinity.Attr, def.Affinity.Value)] = seg
	}

	since := now.AddDate(0, 0, -opts.WindowDays)
	seen := make(map[string]bool)
	scanned := make(map[string]bool)

	for _, cid := range opts.Channels {
		for _, a := range opts.Attributes {
			vals, err := s.rec.AffinityValueCounts(ctx, cid, a.Slug, since, opts.MaxValues)
			if err != nil {
				log.Printf("synapse: affinity sync %s/%s: discover values: %v", cid, a.Slug, err)
				continue // a transient discovery error must not prune this (channel, attr)
			}
			scanned[key(cid.String(), a.Slug)] = true
			for _, v := range vals {
				if v.Actors < opts.MinActors || strings.TrimSpace(v.Value) == "" {
					continue
				}
				k := key(cid.String(), a.Slug, v.Value)
				seen[k] = true
				def := Definition{ChannelID: cid.String(), Affinity: &Affinity{Attr: a.Slug, Value: v.Value, Days: opts.WindowDays, Auto: true}}
				ids, err := s.Evaluate(ctx, def, now)
				if err != nil {
					log.Printf("synapse: affinity sync %s/%s=%q: eval: %v", cid, a.Slug, v.Value, err)
					continue
				}
				defJSON, err := json.Marshal(def)
				if err != nil {
					continue
				}
				name := a.Name + ": " + v.Value
				seg, ok := existing[k]
				if ok {
					if _, err := s.segs.Update(ctx, segment.UpdateInput{ID: seg.ID, Slug: seg.Slug, Name: name, Description: seg.Description, Definition: string(defJSON), IsActive: true}); err != nil {
						log.Printf("synapse: affinity sync %s/%s=%q: update: %v", cid, a.Slug, v.Value, err)
						continue
					}
				} else {
					seg, err = s.segs.Create(ctx, segment.CreateInput{Slug: Slugify(name) + "-" + uuid.NewString()[:6], Name: name, Definition: string(defJSON), IsActive: true})
					if err != nil {
						log.Printf("synapse: affinity sync %s/%s=%q: create: %v", cid, a.Slug, v.Value, err)
						continue
					}
					created++
				}
				if err := s.materialize(ctx, seg.ID, ids); err != nil {
					log.Printf("synapse: affinity sync %s/%s=%q: materialize: %v", cid, a.Slug, v.Value, err)
				}
			}
		}
	}

	// Confined to the (channel, attr) pairs actually scanned this run
	for k, seg := range existing {
		if seen[k] {
			continue
		}
		def, ok := Parse(seg.Definition)
		if !ok || def.Affinity == nil || !scanned[key(def.ChannelID, def.Affinity.Attr)] {
			continue
		}
		if err := s.segs.ClearMembership(ctx, seg.ID); err != nil {
			log.Printf("synapse: affinity prune %s (%s): clear: %v", seg.Slug, seg.ID, err)
			continue
		}
		if err := s.segs.Delete(ctx, seg.ID); err != nil {
			log.Printf("synapse: affinity prune %s (%s): delete: %v", seg.Slug, seg.ID, err)
			continue
		}
		pruned++
	}
	return created, pruned, nil
}

// Pages rather than one capped list - auto-affinity segments alone can push the total past any fixed cap, and a capped list silently stops refreshing the rest
func (s *Service) listAll(ctx context.Context) ([]segment.Segment, error) {
	const pageSize = 500
	var all []segment.Segment
	for off := int32(0); ; off += pageSize {
		page, _, err := s.segs.List(ctx, pageSize, off)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			return all, nil
		}
	}
}

// The replace is atomic, so campaign and flow readers never see the cleared in-between state
func (s *Service) materialize(ctx context.Context, segID uuid.UUID, ids []uuid.UUID) error {
	if err := s.segs.ReplaceMembership(ctx, segID, ids); err != nil {
		return err
	}
	_ = s.segs.RecordDailySnapshot(ctx, segID) // best-effort - it only feeds the growth chart
	return nil
}
