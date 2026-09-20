package signalsgrpc

// The behavioural-segment half of Synapse. A segment's own definition says which channel it belongs
// to, so every one of these gates on that rather than on a channel the caller passes

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/behavioralsegment"
)

func (s *synapseServer) ListBehavioralSegments(ctx context.Context, req *signalsv1.ListBehavioralSegmentsRequest) (*signalsv1.ListBehavioralSegmentsResponse, error) {
	cid, err := channelScope(ctx, "customers.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	if s.behavioral == nil {
		return nil, status.Error(codes.FailedPrecondition, "the segment engine is not configured")
	}
	segs, err := s.behavioral.List(ctx, cid.String())
	if err != nil {
		return nil, err
	}
	limit, offset, err := page(req.GetPageSize(), req.GetPageToken(), 50)
	if err != nil {
		return nil, err
	}
	total := int32(len(segs))
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	out := make([]*signalsv1.BehavioralSegment, 0, end-offset)
	for _, seg := range segs[offset:end] {
		out = append(out, behavioralSegmentProto(seg))
	}
	return &signalsv1.ListBehavioralSegmentsResponse{
		Segments: out, NextPageToken: nextToken(offset, limit, end-offset, total),
	}, nil
}

func (s *synapseServer) GetBehavioralSegment(ctx context.Context, req *signalsv1.GetBehavioralSegmentRequest) (*signalsv1.GetBehavioralSegmentResponse, error) {
	seg, err := s.scopedSegment(ctx, req.GetId(), "customers.view")
	if err != nil {
		return nil, err
	}
	const memberCap = 200
	d, err := s.behavioral.Detail(ctx, seg.ID, memberCap, 90, 12)
	if err != nil {
		return nil, err
	}
	members := make([]*signalsv1.BehavioralSegmentMember, 0, len(d.Members))
	for _, m := range d.Members {
		row := &signalsv1.BehavioralSegmentMember{
			CustomerId: m.CustomerID.String(), Lifecycle: m.Lifecycle,
			RecencyDays: int32(m.RecencyDays), Orders: int32(m.Orders), SpendCents: m.SpendCents,
		}
		if !m.LastOrderAt.IsZero() {
			row.LastOrderAt = ts(m.LastOrderAt)
		}
		members = append(members, row)
	}
	growth := make([]*signalsv1.SegmentGrowthPoint, 0, len(d.Growth))
	for _, g := range d.Growth {
		growth = append(growth, &signalsv1.SegmentGrowthPoint{Day: g.Day.Format("2006-01-02"), Count: int32(g.Count)})
	}
	mix := make([]*signalsv1.SegmentStageCount, 0, len(d.LifecycleMix))
	for _, m := range d.LifecycleMix {
		mix = append(mix, &signalsv1.SegmentStageCount{Stage: m.Stage, Count: int32(m.Count)})
	}
	rfm := make([]*signalsv1.SegmentRfmBucket, 0, len(d.RfmDistribution))
	for _, b := range d.RfmDistribution {
		rfm = append(rfm, &signalsv1.SegmentRfmBucket{Dimension: b.Dimension, Score: int32(b.Score), Count: int32(b.Count)})
	}
	affs := make([]*signalsv1.SegmentAffinityCount, 0, len(d.TopAffinities))
	for _, a := range d.TopAffinities {
		affs = append(affs, &signalsv1.SegmentAffinityCount{Attr: a.Attr, Value: a.Value, Members: int32(a.Members)})
	}
	return &signalsv1.GetBehavioralSegmentResponse{
		Segment:          behavioralSegmentProto(d.Segment),
		RevenueCents:     d.RevenueCents,
		Orders:           int32(d.Orders),
		AovCents:         d.AOVCents,
		Members:          members,
		MembersTruncated: d.Segment.MemberCount > len(members),
		Growth:           growth,
		LifecycleMix:     mix,
		RfmDistribution:  rfm,
		TopAffinities:    affs,
	}, nil
}

func (s *synapseServer) PreviewBehavioralSegment(ctx context.Context, req *signalsv1.PreviewBehavioralSegmentRequest) (*signalsv1.PreviewBehavioralSegmentResponse, error) {
	cid, err := channelScope(ctx, "customers.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	if s.behavioral == nil {
		return nil, status.Error(codes.FailedPrecondition, "the segment engine is not configured")
	}
	ids, err := s.behavioral.Evaluate(ctx, definitionFromProto(cid.String(), req.GetDefinition()), time.Now())
	if err != nil {
		return nil, definitionError(err)
	}
	return &signalsv1.PreviewBehavioralSegmentResponse{Matched: int32(len(ids))}, nil
}

func (s *synapseServer) SaveBehavioralSegment(ctx context.Context, req *signalsv1.SaveBehavioralSegmentRequest) (*signalsv1.SaveBehavioralSegmentResponse, error) {
	cid, err := channelScope(ctx, "customers.manage", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	if s.behavioral == nil {
		return nil, status.Error(codes.FailedPrecondition, "the segment engine is not configured")
	}
	in := behavioralsegment.SaveInput{
		ChannelID:  cid.String(),
		Name:       req.GetName(),
		Definition: definitionFromProto(cid.String(), req.GetDefinition()),
	}
	if req.GetSegmentId() != "" {
		existing, err := s.scopedSegment(ctx, req.GetSegmentId(), "customers.manage")
		if err != nil {
			return nil, err
		}
		in.SegmentID = &existing.ID
	}
	seg, err := s.behavioral.Save(ctx, in, time.Now())
	if err != nil {
		return nil, definitionError(err)
	}
	return &signalsv1.SaveBehavioralSegmentResponse{Segment: behavioralSegmentProto(seg)}, nil
}

func (s *synapseServer) RefreshBehavioralSegment(ctx context.Context, req *signalsv1.RefreshBehavioralSegmentRequest) (*signalsv1.RefreshBehavioralSegmentResponse, error) {
	seg, err := s.scopedSegment(ctx, req.GetSegmentId(), "customers.manage") // manage, not view - a refresh rewrites campaign and flow targeting
	if err != nil {
		return nil, err
	}
	refreshed, err := s.behavioral.Refresh(ctx, seg.ID, time.Now())
	if err != nil {
		return nil, definitionError(err)
	}
	return &signalsv1.RefreshBehavioralSegmentResponse{Segment: behavioralSegmentProto(refreshed)}, nil
}

func (s *synapseServer) SetSegmentLifecycleWindows(ctx context.Context, req *signalsv1.SetSegmentLifecycleWindowsRequest) (*signalsv1.SetSegmentLifecycleWindowsResponse, error) {
	seg, err := s.scopedSegment(ctx, req.GetSegmentId(), "customers.manage")
	if err != nil {
		return nil, err
	}
	var newDays, activeDays, atRiskDays *int32
	if w := req.GetWindows(); w != nil { // absent clears the override
		n, a, ar := w.GetNewDays(), w.GetActiveDays(), w.GetAtRiskDays()
		newDays, activeDays, atRiskDays = &n, &a, &ar
	}
	updated, err := s.behavioral.SetLifecycleWindows(ctx, seg.ID, newDays, activeDays, atRiskDays)
	if err != nil {
		return nil, definitionError(err)
	}
	return &signalsv1.SetSegmentLifecycleWindowsResponse{Segment: behavioralSegmentProto(updated)}, nil
}

// Gates on the channel in the segment's stored definition, never on one the caller names
func (s *synapseServer) scopedSegment(ctx context.Context, raw, perm string) (behavioralsegment.Segment, error) {
	if err := requirePermission(ctx, perm); err != nil {
		return behavioralsegment.Segment{}, err
	}
	id, err := parseID("segment_id", raw)
	if err != nil {
		return behavioralsegment.Segment{}, err
	}
	if s.behavioral == nil {
		return behavioralsegment.Segment{}, status.Error(codes.FailedPrecondition, "the segment engine is not configured")
	}
	seg, err := s.behavioral.Get(ctx, id)
	if err != nil {
		if errors.Is(err, behavioralsegment.ErrNotBehavioral) {
			return behavioralsegment.Segment{}, status.Error(codes.FailedPrecondition, "not a behavioral segment")
		}
		return behavioralsegment.Segment{}, status.Error(codes.NotFound, "segment not found")
	}
	cid, err := seg.Definition.Channel()
	if err != nil {
		return behavioralsegment.Segment{}, status.Error(codes.FailedPrecondition, "segment has no valid channel")
	}
	if err := requireChannel(ctx, perm, cid); err != nil {
		return behavioralsegment.Segment{}, err
	}
	return seg, nil
}

// A malformed definition is the caller's fault, not the server's
func definitionError(err error) error {
	switch {
	case errors.Is(err, behavioralsegment.ErrEmptyDefinition),
		errors.Is(err, behavioralsegment.ErrInvalidChannel),
		errors.Is(err, behavioralsegment.ErrUnknownKind),
		errors.Is(err, behavioralsegment.ErrEmptyGroup),
		errors.Is(err, behavioralsegment.ErrNameRequired),
		errors.Is(err, behavioralsegment.ErrOtherChannel),
		errors.Is(err, behavioralsegment.ErrLifecycleWindows):
		return status.Error(codes.InvalidArgument, wireMessage(err))
	case errors.Is(err, behavioralsegment.ErrNotBehavioral):
		return status.Error(codes.FailedPrecondition, wireMessage(err))
	}
	return err
}

// The package prefix is for a Go reader, not for a caller on the other side of the wire
func wireMessage(err error) string {
	return strings.TrimPrefix(err.Error(), "behavioralsegment: ")
}

func behavioralSegmentProto(seg behavioralsegment.Segment) *signalsv1.BehavioralSegment {
	out := &signalsv1.BehavioralSegment{
		Id:          seg.ID.String(),
		ChannelId:   seg.Definition.ChannelID,
		Name:        seg.Name,
		Slug:        seg.Slug,
		MemberCount: int32(seg.MemberCount),
		Definition:  nodeProto(seg.Definition.Tree()),
		Trait:       traitProto(seg.Definition.Trait),
		Affinity:    affinityProto(seg.Definition.Affinity),
	}
	for _, c := range seg.Definition.Conditions {
		out.Conditions = append(out.Conditions, &signalsv1.SegmentCondition{
			Event: c.Event, Negate: c.Negate, Count: int32(max(c.Count, 1)), Days: int32(conditionDays(c.Days)),
		})
	}
	// Only the full triple is an override - a partial one is not a cadence
	if seg.LifecycleNewDays != nil && seg.LifecycleActiveDays != nil && seg.LifecycleAtRiskDays != nil {
		out.LifecycleWindows = &signalsv1.SegmentLifecycleWindows{
			NewDays: *seg.LifecycleNewDays, ActiveDays: *seg.LifecycleActiveDays, AtRiskDays: *seg.LifecycleAtRiskDays,
		}
	}
	return out
}

func conditionDays(d int) int {
	if d <= 0 {
		return 30
	}
	return d
}

func nodeProto(n *behavioralsegment.Node) *signalsv1.SegmentNode {
	if n == nil {
		return nil
	}
	out := &signalsv1.SegmentNode{}
	switch n.Kind {
	case behavioralsegment.KindGroup:
		out.Kind = signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_GROUP
		out.Op = signalsv1.SegmentBoolOp_SEGMENT_BOOL_OP_AND
		if strings.EqualFold(n.Op, behavioralsegment.OpOr) {
			out.Op = signalsv1.SegmentBoolOp_SEGMENT_BOOL_OP_OR
		}
		for i := range n.Children {
			out.Children = append(out.Children, nodeProto(&n.Children[i]))
		}
	case behavioralsegment.KindEvent:
		out.Kind = signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_EVENT
		out.Event, out.Negate, out.Count, out.Days = n.Event, n.Negate, int32(n.Count), int32(n.Days)
	case behavioralsegment.KindTrait:
		out.Kind = signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_TRAIT
		out.Trait = traitProto(n.Trait)
	case behavioralsegment.KindAffinity:
		out.Kind = signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_AFFINITY
		out.Affinity = affinityProto(n.Affinity)
	}
	return out
}

func traitProto(t *behavioralsegment.Trait) *signalsv1.SegmentTrait {
	if !t.Active() {
		return nil
	}
	return &signalsv1.SegmentTrait{
		Lifecycle: t.Lifecycle, MinRecencyScore: int32(t.MinRecencyScore), MinFrequencyScore: int32(t.MinFrequencyScore),
		MinMonetaryScore: int32(t.MinMonetaryScore), MinEngagement: int32(t.MinEngagement),
	}
}

func affinityProto(a *behavioralsegment.Affinity) *signalsv1.SegmentAffinity {
	if !a.Active() {
		return nil
	}
	return &signalsv1.SegmentAffinity{Attr: a.Attr, Value: a.Value, Days: int32(a.Window()), Auto: a.Auto}
}

// An absent root leaves the flat criteria to stand, which is what the evaluator falls back to
func definitionFromProto(channelID string, d *signalsv1.SegmentDefinition) behavioralsegment.Definition {
	out := behavioralsegment.Definition{ChannelID: channelID}
	if d == nil {
		return out
	}
	if d.GetRoot() != nil {
		out.Root = nodeFromProto(d.GetRoot())
		return out
	}
	for _, c := range d.GetConditions() {
		out.Conditions = append(out.Conditions, behavioralsegment.Condition{
			Event: c.GetEvent(), Negate: c.GetNegate(), Count: int(c.GetCount()), Days: int(c.GetDays()),
		})
	}
	out.Trait = traitFromProto(d.GetTrait())
	out.Affinity = affinityFromProto(d.GetAffinity())
	return out
}

func nodeFromProto(n *signalsv1.SegmentNode) *behavioralsegment.Node {
	if n == nil {
		return nil
	}
	out := &behavioralsegment.Node{}
	switch n.GetKind() {
	case signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_GROUP:
		out.Kind = behavioralsegment.KindGroup
		out.Op = behavioralsegment.OpAnd
		if n.GetOp() == signalsv1.SegmentBoolOp_SEGMENT_BOOL_OP_OR {
			out.Op = behavioralsegment.OpOr
		}
		for _, c := range n.GetChildren() {
			if child := nodeFromProto(c); child != nil {
				out.Children = append(out.Children, *child)
			}
		}
	case signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_EVENT:
		out.Kind = behavioralsegment.KindEvent
		out.Event, out.Negate, out.Count, out.Days = n.GetEvent(), n.GetNegate(), int(n.GetCount()), int(n.GetDays())
	case signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_TRAIT:
		out.Kind = behavioralsegment.KindTrait
		out.Trait = traitFromProto(n.GetTrait())
	case signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_AFFINITY:
		out.Kind = behavioralsegment.KindAffinity
		out.Affinity = affinityFromProto(n.GetAffinity())
	}
	return out // an unspecified kind yields a node the evaluator rejects
}

func traitFromProto(t *signalsv1.SegmentTrait) *behavioralsegment.Trait {
	if t == nil {
		return nil
	}
	return &behavioralsegment.Trait{
		Lifecycle: t.GetLifecycle(), MinRecencyScore: int(t.GetMinRecencyScore()),
		MinFrequencyScore: int(t.GetMinFrequencyScore()), MinMonetaryScore: int(t.GetMinMonetaryScore()),
		MinEngagement: int(t.GetMinEngagement()),
	}
}

func affinityFromProto(a *signalsv1.SegmentAffinity) *behavioralsegment.Affinity {
	if a == nil || strings.TrimSpace(a.GetAttr()) == "" || strings.TrimSpace(a.GetValue()) == "" {
		return nil
	}
	return &behavioralsegment.Affinity{Attr: a.GetAttr(), Value: a.GetValue(), Days: int(a.GetDays())}
}
