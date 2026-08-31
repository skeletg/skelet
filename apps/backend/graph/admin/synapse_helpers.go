package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	"github.com/skeletg/skelet/apps/backend/internal/subscription"
)

// Needs the permission globally AND on the channel (mirrors lensViewScope) - RequirePermission alone ignores channel grants, letting a channel-A admin read channel B analytics by passing B id
func synapseScope(ctx context.Context, r *Resolver, perm, channelID string) (uuid.UUID, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, perm); err != nil {
		return uuid.Nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return uuid.Nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, perm, cid, nil); err != nil {
		return uuid.Nil, err
	}
	return cid, nil
}

// Gates on the channel in the segment's stored definition - shared by the detail query and the per-segment mutations so a segment id can never reach another channel's members
func synapseSegmentScope(ctx context.Context, r *Resolver, segmentID, perm string) (segment.Segment, segmentDef, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, perm); err != nil {
		return segment.Segment{}, segmentDef{}, err
	}
	sid, err := uuid.Parse(segmentID)
	if err != nil {
		return segment.Segment{}, segmentDef{}, errInvalidID("segmentId", err)
	}
	seg, err := r.SegmentSvc.Get(ctx, sid)
	if err != nil {
		return segment.Segment{}, segmentDef{}, errNotFound("segment")
	}
	def, ok := parseSegmentDef(seg.RefreshSQL)
	if !ok {
		return segment.Segment{}, segmentDef{}, fmt.Errorf("not a behavioral segment")
	}
	cid, err := uuid.Parse(def.ChannelID)
	if err != nil {
		return segment.Segment{}, segmentDef{}, fmt.Errorf("segment has no valid channel")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, perm, cid, nil); err != nil {
		return segment.Segment{}, segmentDef{}, err
	}
	return seg, def, nil
}

// The spine queries walk raw events, so an uncapped window is a full-table scan on demand
func synapseDays(sinceDays *int32, def, max int32) int32 {
	days := def
	if sinceDays != nil && *sinceDays > 0 {
		days = *sinceDays
	}
	if days > max {
		days = max
	}
	return days
}

func condsFromInput(in []*model.SegmentConditionInput) []segCond {
	out := make([]segCond, 0, len(in))
	for _, c := range in {
		sc := segCond{Event: c.Event}
		if c.Negate != nil {
			sc.Negate = *c.Negate
		}
		if c.Count != nil {
			sc.Count = int(*c.Count)
		}
		if c.Days != nil {
			sc.Days = int(*c.Days)
		}
		out = append(out, sc)
	}
	return out
}

func segmentConditionModels(d segmentDef) []*model.SynapseSegmentCondition {
	out := make([]*model.SynapseSegmentCondition, 0, len(d.Conditions))
	for _, c := range d.Conditions {
		cnt := max(c.Count, 1)
		days := c.Days
		if days <= 0 {
			days = 30
		}
		out = append(out, &model.SynapseSegmentCondition{Event: c.Event, Negate: c.Negate, Count: int32(cnt), Days: int32(days)})
	}
	return out
}

func synapseSegmentModel(seg segment.Segment, memberCount int) *model.SynapseSegment {
	def, _ := parseSegmentDef(seg.RefreshSQL)
	return &model.SynapseSegment{
		ID:               seg.ID.String(),
		ChannelID:        def.ChannelID,
		Name:             seg.Name,
		Slug:             seg.Slug,
		MemberCount:      int32(memberCount),
		Definition:       segNodeModel(def.tree()),
		Conditions:       segmentConditionModels(def),
		Trait:            segmentTraitModel(def.Trait),
		Affinity:         segmentAffinityModel(def.Affinity),
		LifecycleWindows: segmentLifecycleWindowsModel(seg),
	}
}

// The tree `root` wins when supplied, else the flat conditions/trait/affinity, all AND'd
func defFromInputs(channelID string, root *model.SegmentNodeInput, conditions []*model.SegmentConditionInput, trait *model.SegmentTraitInput, affinity *model.SegmentAffinityInput) segmentDef {
	if root != nil {
		return segmentDef{ChannelID: channelID, Root: nodeFromInput(root)}
	}
	return segmentDef{ChannelID: channelID, Conditions: condsFromInput(conditions), Trait: traitFromInput(trait), Affinity: affinityFromInput(affinity)}
}

// Unknown/empty kinds yield a node the evaluator rejects
func nodeFromInput(in *model.SegmentNodeInput) *segNode {
	if in == nil {
		return nil
	}
	n := &segNode{Kind: strings.ToLower(string(in.Kind))}
	switch n.Kind {
	case nodeGroup:
		n.Op = "and"
		if in.Op != nil {
			n.Op = strings.ToLower(string(*in.Op))
		}
		for _, c := range in.Children {
			if child := nodeFromInput(c); child != nil {
				n.Children = append(n.Children, *child)
			}
		}
	case nodeEvent:
		if in.Event != nil {
			n.Event = *in.Event
		}
		if in.Negate != nil {
			n.Negate = *in.Negate
		}
		if in.Count != nil {
			n.Count = int(*in.Count)
		}
		if in.Days != nil {
			n.Days = int(*in.Days)
		}
	case nodeTrait:
		n.Trait = traitFromInput(in.Trait)
	case nodeAffinity:
		n.Affinity = affinityFromInput(in.Affinity)
	}
	return n
}

func segNodeModel(n *segNode) *model.SegmentNode {
	if n == nil {
		return nil
	}
	out := &model.SegmentNode{Kind: model.SegmentNodeKind(strings.ToUpper(n.Kind))}
	switch n.Kind {
	case nodeGroup:
		op := model.SegmentBoolOp(strings.ToUpper(n.Op))
		if op != model.SegmentBoolOpOr {
			op = model.SegmentBoolOpAnd
		}
		out.Op = &op
		for i := range n.Children {
			out.Children = append(out.Children, segNodeModel(&n.Children[i]))
		}
	case nodeEvent:
		ev, neg, cnt, d := n.Event, n.Negate, int32(n.Count), int32(n.Days)
		out.Event, out.Negate, out.Count, out.Days = &ev, &neg, &cnt, &d
	case nodeTrait:
		out.Trait = segmentTraitModel(n.Trait)
	case nodeAffinity:
		out.Affinity = segmentAffinityModel(n.Affinity)
	}
	return out
}

// nil unless the full triple is set - a partial override is not a cadence
func segmentLifecycleWindowsModel(seg segment.Segment) *model.SegmentLifecycleWindows {
	if seg.LifecycleNewDays == nil || seg.LifecycleActiveDays == nil || seg.LifecycleAtRiskDays == nil {
		return nil
	}
	return &model.SegmentLifecycleWindows{
		NewDays:    *seg.LifecycleNewDays,
		ActiveDays: *seg.LifecycleActiveDays,
		AtRiskDays: *seg.LifecycleAtRiskDays,
	}
}

func segmentAffinityModel(a *segAffinity) *model.SynapseSegmentAffinity {
	if !a.active() {
		return nil
	}
	return &model.SynapseSegmentAffinity{Attr: a.Attr, Value: a.Value, Days: int32(a.window()), Auto: a.Auto}
}

func affinityFromInput(in *model.SegmentAffinityInput) *segAffinity {
	if in == nil || strings.TrimSpace(in.Attr) == "" || strings.TrimSpace(in.Value) == "" {
		return nil
	}
	a := &segAffinity{Attr: in.Attr, Value: in.Value}
	if in.Days != nil {
		a.Days = int(*in.Days)
	}
	return a
}

func segmentTraitModel(t *segTrait) *model.SynapseSegmentTrait {
	if !t.active() {
		return nil
	}
	return &model.SynapseSegmentTrait{
		Lifecycle: t.Lifecycle, MinRecencyScore: int32(t.MinRecencyScore), MinFrequencyScore: int32(t.MinFrequencyScore),
		MinMonetaryScore: int32(t.MinMonetaryScore), MinEngagement: int32(t.MinEngagement),
	}
}

func traitFromInput(in *model.SegmentTraitInput) *segTrait {
	if in == nil {
		return nil
	}
	t := &segTrait{Lifecycle: in.Lifecycle}
	if in.MinRecencyScore != nil {
		t.MinRecencyScore = int(*in.MinRecencyScore)
	}
	if in.MinFrequencyScore != nil {
		t.MinFrequencyScore = int(*in.MinFrequencyScore)
	}
	if in.MinMonetaryScore != nil {
		t.MinMonetaryScore = int(*in.MinMonetaryScore)
	}
	if in.MinEngagement != nil {
		t.MinEngagement = int(*in.MinEngagement)
	}
	return t
}

// The tracked flag lives on the catalog - analytics only aggregates the values
func synapseAffinityAttrs(ctx context.Context, r *Resolver) []analytics.AffinityAttr {
	if r.CatalogSvc == nil {
		return nil
	}
	attrs, err := r.CatalogSvc.ListAttributes(ctx)
	if err != nil {
		return nil
	}
	out := make([]analytics.AffinityAttr, 0)
	for _, a := range attrs {
		if a.SynapseTrack {
			out = append(out, analytics.AffinityAttr{Slug: a.Slug, Name: a.Name})
		}
	}
	return out
}

func affinityModels(affs []analytics.AttributeAffinity) []*model.AttributeAffinity {
	out := make([]*model.AttributeAffinity, 0, len(affs))
	for _, a := range affs {
		out = append(out, &model.AttributeAffinity{Slug: a.Slug, Name: a.Name, Values: valueCounts(a.Values)})
	}
	return out
}

func experimentArmsModel(arms []analytics.ExperimentArm) []*model.CustomerExperimentArm {
	out := make([]*model.CustomerExperimentArm, 0, len(arms))
	for _, a := range arms {
		out = append(out, &model.CustomerExperimentArm{Experiment: a.Experiment, Variation: a.Variation, LastExposedAt: a.LastExposedAt})
	}
	return out
}

func customerTraitsModel(t analytics.CustomerTraits) *model.CustomerTraits {
	return &model.CustomerTraits{
		Lifecycle: t.Lifecycle, RfmRecency: int32(t.RFMRecency), RfmFrequency: int32(t.RFMFrequency),
		RfmMonetary: int32(t.RFMMonetary), RecencyDays: int32(t.RecencyDays), Frequency: int32(t.Frequency),
		MonetaryCents: int(t.MonetaryCents), EngagementScore: int32(t.EngagementScore),
		ChurnRisk: int32(t.ChurnRisk), PredictedClvCents: int(t.PredictedCLVCents), NextOrderDays: int32(t.NextOrderDays),
		ComputedAt: t.ComputedAt,
	}
}

const marketingConsentKind = "marketing_email" // slug registered in internal/consent/defaults.go

// Composed from existing services rather than new SQL - lifetime spend is per-currency and excludes cancelled/draft
func buildCustomerCommerce(ctx context.Context, r *Resolver, cid uuid.UUID, cust customer.Customer) (*model.CustomerCommerce, error) {
	out := &model.CustomerCommerce{Spend: []*model.AttributedRevenue{}, MemberSince: cust.CreatedAt}
	if cust.Group != nil {
		out.Group = cust.Group.Name
	}

	orders, err := r.OrderSvc.ListOrdersByCustomer(ctx, cid)
	if err != nil {
		return nil, err
	}
	type acc struct {
		cents int64
		n     int32
	}
	byCur := map[string]*acc{}
	for _, o := range orders {
		if o.Status == order.OrderStatusCancelled || o.Status == order.OrderStatusDraft {
			continue
		}
		out.Orders++
		a := byCur[o.CurrencyCode]
		if a == nil {
			a = &acc{}
			byCur[o.CurrencyCode] = a
		}
		a.cents += o.TotalAmount
		a.n++
		t := o.CreatedAt
		if out.FirstOrderAt == nil || t.Before(*out.FirstOrderAt) {
			ft := t
			out.FirstOrderAt = &ft
		}
		if out.LastOrderAt == nil || t.After(*out.LastOrderAt) {
			lt := t
			out.LastOrderAt = &lt
		}
		if rets, e := r.OrderSvc.ListReturns(ctx, o.ID); e == nil {
			out.Returns += int32(len(rets))
		}
	}
	for cur, a := range byCur {
		out.Spend = append(out.Spend, &model.AttributedRevenue{Currency: cur, Cents: int(a.cents), Orders: a.n})
	}

	if subs, e := r.SubscriptionSvc.ListForCustomer(ctx, cid); e == nil {
		for _, s := range subs {
			if s.Status == subscription.StatusActive {
				out.ActiveSubscriptions++
			}
		}
	}
	out.MarketingOptedIn, _ = r.ConsentSvc.IsOptedIn(ctx, cid, marketingConsentKind)
	return out, nil
}

// Shared by customerProfile and visitorProfile - the anonymous side reuses the same aggregates+timeline shape
func profileToModel(p analytics.Profile) *model.CustomerProfile {
	out := &model.CustomerProfile{
		FirstSeen:   p.FirstSeen,
		LastSeen:    p.LastSeen,
		TotalEvents: int32(p.TotalEvents),
		PageViews:   int32(p.PageViews),
		Conversions: int32(p.Conversions),
		Revenue:     []*model.AttributedRevenue{},
		Counts:      []*model.CustomerEventCount{},
		Timeline:    []*model.CustomerActivityItem{},
		Experiments: []*model.CustomerExperimentArm{},
		Affinities:  []*model.AttributeAffinity{},
	}
	for _, rv := range p.Revenue {
		out.Revenue = append(out.Revenue, &model.AttributedRevenue{Currency: rv.Currency, Cents: int(rv.Cents), Orders: int32(rv.Orders)})
	}
	for _, c := range p.Counts {
		out.Counts = append(out.Counts, &model.CustomerEventCount{Name: c.Name, Count: int32(c.Count)})
	}
	for _, t := range p.Timeline {
		details := "{}"
		if len(t.Props) > 0 {
			details = string(t.Props)
		}
		out.Timeline = append(out.Timeline, &model.CustomerActivityItem{
			EventName: t.EventName, OccurredAt: t.OccurredAt, Source: t.Source, Summary: t.Summary, DetailsJSON: details,
		})
	}
	out.Devices = valueCounts(p.Devices)
	out.Browsers = valueCounts(p.Browsers)
	out.Referrers = valueCounts(p.Referrers)
	out.IPAddresses = valueCounts(p.IPs)
	out.OperatingSystems = valueCounts(p.OSes)
	out.Screens = valueCounts(p.Screens)
	out.Countries = valueCounts(p.Countries)
	out.Sources = valueCounts(p.Sources)
	out.Campaigns = valueCounts(p.Campaigns)
	out.TopPages = pageViews(p.TopPages)
	out.PagesViewedTotal = int32(p.PagesViewedTotal)
	out.TopProducts = productViews(p.TopProducts)
	out.Searches = valueCounts(p.Searches)
	out.Sessions = int32(p.Sessions)
	return out
}

func valueCounts(in []analytics.ValueCount) []*model.ProfileValue {
	out := make([]*model.ProfileValue, 0, len(in))
	for _, v := range in {
		out = append(out, &model.ProfileValue{Value: v.Value, Count: int32(v.Count)})
	}
	return out
}

func productViews(in []analytics.ProductView) []*model.ProfileProductView {
	out := make([]*model.ProfileProductView, 0, len(in))
	for _, v := range in {
		out = append(out, &model.ProfileProductView{Name: v.Name, Image: v.Image, Price: v.Price, Sku: v.SKU, Views: int32(v.Views)})
	}
	return out
}

func pageViews(in []analytics.PageView) []*model.ProfilePageView {
	out := make([]*model.ProfilePageView, 0, len(in))
	for _, v := range in {
		out = append(out, &model.ProfilePageView{Title: v.Title, PageKey: v.PageKey, Views: int32(v.Views), DwellSeconds: int32(v.AvgDwellSeconds)})
	}
	return out
}
