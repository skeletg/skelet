package admin

// Converters between the Synapse contract and the admin models

import (
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func profileValues(in []*signalsv1.ValueCount) []*model.ProfileValue {
	out := make([]*model.ProfileValue, 0, len(in))
	for _, v := range in {
		out = append(out, &model.ProfileValue{Value: v.GetValue(), Count: v.GetCount()})
	}
	return out
}

// Shared by customerProfile and visitorProfile - the anonymous side carries no identity
func profileModel(p *signalsv1.Profile) *model.CustomerProfile {
	out := &model.CustomerProfile{
		TotalEvents: p.GetTotalEvents(),
		PageViews:   p.GetPageViews(),
		Conversions: p.GetConversions(),
		Revenue:     []*model.AttributedRevenue{},
		Counts:      []*model.CustomerEventCount{},
		Timeline:    []*model.CustomerActivityItem{},
		Experiments: []*model.CustomerExperimentArm{},
		Affinities:  []*model.AttributeAffinity{},
	}
	if p.GetFirstSeen() != nil {
		t := p.GetFirstSeen().AsTime()
		out.FirstSeen = &t
	}
	if p.GetLastSeen() != nil {
		t := p.GetLastSeen().AsTime()
		out.LastSeen = &t
	}
	for _, rv := range p.GetRevenue() {
		out.Revenue = append(out.Revenue, &model.AttributedRevenue{Currency: rv.GetCurrency(), Cents: int(rv.GetCents()), Orders: rv.GetOrders()})
	}
	for _, c := range p.GetCounts() {
		out.Counts = append(out.Counts, &model.CustomerEventCount{Name: c.GetName(), Count: c.GetCount()})
	}
	for _, t := range p.GetTimeline() {
		details := t.GetDetailsJson()
		if details == "" {
			details = "{}"
		}
		out.Timeline = append(out.Timeline, &model.CustomerActivityItem{
			EventName: t.GetEventName(), OccurredAt: t.GetOccurredAt().AsTime(),
			Source: t.GetSource(), Summary: t.GetSummary(), DetailsJSON: details,
		})
	}
	out.Devices = profileValues(p.GetDevices())
	out.Browsers = profileValues(p.GetBrowsers())
	out.Referrers = profileValues(p.GetReferrers())
	out.IPAddresses = profileValues(p.GetIpAddresses())
	out.OperatingSystems = profileValues(p.GetOperatingSystems())
	out.Screens = profileValues(p.GetScreens())
	out.Countries = profileValues(p.GetCountries())
	out.Sources = profileValues(p.GetSources())
	out.Campaigns = profileValues(p.GetCampaigns())
	out.Searches = profileValues(p.GetSearches())
	out.PagesViewedTotal = p.GetPagesViewedTotal()
	out.Sessions = p.GetSessions()
	for _, v := range p.GetTopPages() {
		out.TopPages = append(out.TopPages, &model.ProfilePageView{
			Title: v.GetTitle(), PageKey: v.GetPageKey(), Views: v.GetViews(), DwellSeconds: v.GetDwellSeconds(),
		})
	}
	for _, v := range p.GetTopProducts() {
		out.TopProducts = append(out.TopProducts, &model.ProfileProductView{
			Name: v.GetName(), Image: v.GetImage(), Price: v.GetPrice(), Sku: v.GetSku(), Views: v.GetViews(),
		})
	}
	for _, a := range p.GetAffinities() {
		out.Affinities = append(out.Affinities, &model.AttributeAffinity{Slug: a.GetSlug(), Name: a.GetName(), Values: profileValues(a.GetValues())})
	}
	for _, a := range p.GetExperiments() {
		out.Experiments = append(out.Experiments, &model.CustomerExperimentArm{
			Experiment: a.GetExperiment(), Variation: a.GetVariation(), LastExposedAt: a.GetLastExposedAt().AsTime(),
		})
	}
	if t := p.GetTraits(); t != nil {
		out.Traits = &model.CustomerTraits{
			Lifecycle: t.GetLifecycle(), RfmRecency: t.GetRfmRecency(), RfmFrequency: t.GetRfmFrequency(),
			RfmMonetary: t.GetRfmMonetary(), RecencyDays: t.GetRecencyDays(), Frequency: t.GetFrequency(),
			MonetaryCents: int(t.GetMonetaryCents()), EngagementScore: t.GetEngagementScore(),
			ChurnRisk: t.GetChurnRisk(), PredictedClvCents: int(t.GetPredictedClvCents()),
			NextOrderDays: t.GetNextOrderDays(), ComputedAt: t.GetComputedAt().AsTime(),
		}
	}
	return out
}

func synapseSegmentModel(seg *signalsv1.BehavioralSegment) *model.SynapseSegment {
	out := &model.SynapseSegment{
		ID: seg.GetId(), ChannelID: seg.GetChannelId(), Name: seg.GetName(), Slug: seg.GetSlug(),
		MemberCount: seg.GetMemberCount(),
		Definition:  segmentNodeModel(seg.GetDefinition()),
		Conditions:  make([]*model.SynapseSegmentCondition, 0, len(seg.GetConditions())),
		Trait:       segmentTraitModel(seg.GetTrait()),
		Affinity:    segmentAffinityModel(seg.GetAffinity()),
	}
	for _, c := range seg.GetConditions() {
		out.Conditions = append(out.Conditions, &model.SynapseSegmentCondition{
			Event: c.GetEvent(), Negate: c.GetNegate(), Count: c.GetCount(), Days: c.GetDays(),
		})
	}
	if w := seg.GetLifecycleWindows(); w != nil {
		out.LifecycleWindows = &model.SegmentLifecycleWindows{
			NewDays: w.GetNewDays(), ActiveDays: w.GetActiveDays(), AtRiskDays: w.GetAtRiskDays(),
		}
	}
	return out
}

func segmentNodeModel(n *signalsv1.SegmentNode) *model.SegmentNode {
	if n == nil {
		return nil
	}
	out := &model.SegmentNode{}
	switch n.GetKind() {
	case signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_GROUP:
		out.Kind = model.SegmentNodeKindGroup
		op := model.SegmentBoolOpAnd
		if n.GetOp() == signalsv1.SegmentBoolOp_SEGMENT_BOOL_OP_OR {
			op = model.SegmentBoolOpOr
		}
		out.Op = &op
		for _, c := range n.GetChildren() {
			out.Children = append(out.Children, segmentNodeModel(c))
		}
	case signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_EVENT:
		out.Kind = model.SegmentNodeKindEvent
		ev, neg, cnt, d := n.GetEvent(), n.GetNegate(), n.GetCount(), n.GetDays()
		out.Event, out.Negate, out.Count, out.Days = &ev, &neg, &cnt, &d
	case signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_TRAIT:
		out.Kind = model.SegmentNodeKindTrait
		out.Trait = segmentTraitModel(n.GetTrait())
	case signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_AFFINITY:
		out.Kind = model.SegmentNodeKindAffinity
		out.Affinity = segmentAffinityModel(n.GetAffinity())
	}
	return out
}

func segmentTraitModel(t *signalsv1.SegmentTrait) *model.SynapseSegmentTrait {
	if t == nil {
		return nil
	}
	return &model.SynapseSegmentTrait{
		Lifecycle: t.GetLifecycle(), MinRecencyScore: t.GetMinRecencyScore(), MinFrequencyScore: t.GetMinFrequencyScore(),
		MinMonetaryScore: t.GetMinMonetaryScore(), MinEngagement: t.GetMinEngagement(),
	}
}

func segmentAffinityModel(a *signalsv1.SegmentAffinity) *model.SynapseSegmentAffinity {
	if a == nil {
		return nil
	}
	return &model.SynapseSegmentAffinity{Attr: a.GetAttr(), Value: a.GetValue(), Days: a.GetDays(), Auto: a.GetAuto()}
}

// The tree wins when supplied, else the flat conditions, trait and affinity, all AND'd
func segmentDefinitionProto(root *model.SegmentNodeInput, conditions []*model.SegmentConditionInput, trait *model.SegmentTraitInput, affinity *model.SegmentAffinityInput) *signalsv1.SegmentDefinition {
	if root != nil {
		return &signalsv1.SegmentDefinition{Root: segmentNodeProto(root)}
	}
	out := &signalsv1.SegmentDefinition{Trait: segmentTraitProto(trait), Affinity: segmentAffinityProto(affinity)}
	for _, c := range conditions {
		sc := &signalsv1.SegmentCondition{Event: c.Event}
		if c.Negate != nil {
			sc.Negate = *c.Negate
		}
		if c.Count != nil {
			sc.Count = *c.Count
		}
		if c.Days != nil {
			sc.Days = *c.Days
		}
		out.Conditions = append(out.Conditions, sc)
	}
	return out
}

// An unknown kind yields a node the evaluator rejects
func segmentNodeProto(in *model.SegmentNodeInput) *signalsv1.SegmentNode {
	if in == nil {
		return nil
	}
	// The graph's enum maps straight onto the contract's - the engine's own spelling is its business
	out := &signalsv1.SegmentNode{}
	switch in.Kind {
	case model.SegmentNodeKindGroup:
		out.Kind = signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_GROUP
		out.Op = signalsv1.SegmentBoolOp_SEGMENT_BOOL_OP_AND
		if in.Op != nil && *in.Op == model.SegmentBoolOpOr {
			out.Op = signalsv1.SegmentBoolOp_SEGMENT_BOOL_OP_OR
		}
		for _, c := range in.Children {
			if child := segmentNodeProto(c); child != nil {
				out.Children = append(out.Children, child)
			}
		}
	case model.SegmentNodeKindEvent:
		out.Kind = signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_EVENT
		if in.Event != nil {
			out.Event = *in.Event
		}
		if in.Negate != nil {
			out.Negate = *in.Negate
		}
		if in.Count != nil {
			out.Count = *in.Count
		}
		if in.Days != nil {
			out.Days = *in.Days
		}
	case model.SegmentNodeKindTrait:
		out.Kind = signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_TRAIT
		out.Trait = segmentTraitProto(in.Trait)
	case model.SegmentNodeKindAffinity:
		out.Kind = signalsv1.SegmentNodeKind_SEGMENT_NODE_KIND_AFFINITY
		out.Affinity = segmentAffinityProto(in.Affinity)
	}
	return out
}

func segmentTraitProto(in *model.SegmentTraitInput) *signalsv1.SegmentTrait {
	if in == nil {
		return nil
	}
	out := &signalsv1.SegmentTrait{Lifecycle: in.Lifecycle}
	if in.MinRecencyScore != nil {
		out.MinRecencyScore = *in.MinRecencyScore
	}
	if in.MinFrequencyScore != nil {
		out.MinFrequencyScore = *in.MinFrequencyScore
	}
	if in.MinMonetaryScore != nil {
		out.MinMonetaryScore = *in.MinMonetaryScore
	}
	if in.MinEngagement != nil {
		out.MinEngagement = *in.MinEngagement
	}
	return out
}

func segmentAffinityProto(in *model.SegmentAffinityInput) *signalsv1.SegmentAffinity {
	if in == nil || strings.TrimSpace(in.Attr) == "" || strings.TrimSpace(in.Value) == "" {
		return nil
	}
	out := &signalsv1.SegmentAffinity{Attr: in.Attr, Value: in.Value}
	if in.Days != nil {
		out.Days = *in.Days
	}
	return out
}

// The contract carries a day bucket as YYYY-MM-DD, and this surface speaks Time - midnight UTC
func signalsDay(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// The spine queries walk raw events, so an uncapped window is a full-table scan on demand
func synapseWindow(sinceDays *int32, def, max int32) int32 {
	days := def
	if sinceDays != nil && *sinceDays > 0 {
		days = *sinceDays
	}
	if days > max {
		days = max
	}
	return days
}
