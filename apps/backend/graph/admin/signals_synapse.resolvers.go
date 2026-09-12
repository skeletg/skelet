package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

func (r *mutationResolver) SynapseSaveSegment(ctx context.Context, channelID string, name string, segmentID *string, root *model.SegmentNodeInput, conditions []*model.SegmentConditionInput, trait *model.SegmentTraitInput, affinity *model.SegmentAffinityInput) (*model.SynapseSegment, error) {
	authed, err := r.signalsChannel(ctx, "customers.manage", channelID)
	if err != nil {
		return nil, err
	}
	req := &signalsv1.SaveBehavioralSegmentRequest{
		ChannelId: channelID, Name: name,
		Definition: segmentDefinitionProto(root, conditions, trait, affinity),
	}
	if segmentID != nil && *segmentID != "" {
		if _, err := uuid.Parse(*segmentID); err != nil {
			return nil, errInvalidID("segmentId", err)
		}
		req.SegmentId = *segmentID
	}
	res, err := r.SignalsAPI.Synapse.SaveBehavioralSegment(authed, req)
	if err != nil {
		return nil, signalsErr(err)
	}
	return synapseSegmentModel(res.GetSegment()), nil
}

func (r *mutationResolver) SynapseRefreshSegment(ctx context.Context, segmentID string) (*model.SynapseSegment, error) {
	authed, err := r.signalsScoped(ctx, "customers.manage") // manage, not view - a refresh rewrites campaign and flow targeting
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(segmentID); err != nil {
		return nil, errInvalidID("segmentId", err)
	}
	res, err := r.SignalsAPI.Synapse.RefreshBehavioralSegment(authed, &signalsv1.RefreshBehavioralSegmentRequest{SegmentId: segmentID})
	if err != nil {
		return nil, signalsErr(err)
	}
	return synapseSegmentModel(res.GetSegment()), nil
}

func (r *mutationResolver) SynapseSetSegmentLifecycleWindows(ctx context.Context, segmentID string, windows *model.SegmentLifecycleWindowsInput) (*model.SynapseSegment, error) {
	authed, err := r.signalsScoped(ctx, "customers.manage")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(segmentID); err != nil {
		return nil, errInvalidID("segmentId", err)
	}
	req := &signalsv1.SetSegmentLifecycleWindowsRequest{SegmentId: segmentID}
	if windows != nil { // absent clears the override
		req.Windows = &signalsv1.SegmentLifecycleWindows{
			NewDays: windows.NewDays, ActiveDays: windows.ActiveDays, AtRiskDays: windows.AtRiskDays,
		}
	}
	res, err := r.SignalsAPI.Synapse.SetSegmentLifecycleWindows(authed, req)
	if err != nil {
		return nil, signalsErr(err)
	}
	return synapseSegmentModel(res.GetSegment()), nil
}

func (r *queryResolver) CustomerProfile(ctx context.Context, customerID string, sinceDays *int32) (*model.CustomerProfile, error) {
	authed, err := r.signalsScoped(ctx, "customers.view")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(customerID); err != nil {
		return nil, errInvalidID("customerId", err)
	}
	res, err := r.SignalsAPI.Synapse.GetCustomerProfile(authed, &signalsv1.GetCustomerProfileRequest{
		CustomerId: customerID, SinceDays: synapseWindow(sinceDays, 90, 365),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return profileModel(res.GetProfile()), nil
}

func (r *queryResolver) SynapseSegment(ctx context.Context, id string) (*model.SynapseSegmentDetail, error) {
	authed, err := r.signalsScoped(ctx, "customers.view")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, errInvalidID("id", err)
	}
	d, err := r.SignalsAPI.Synapse.GetBehavioralSegment(authed, &signalsv1.GetBehavioralSegmentRequest{Id: id})
	if err != nil {
		return nil, signalsErr(err)
	}
	members := make([]*model.SegmentMember, 0, len(d.GetMembers()))
	for _, m := range d.GetMembers() {
		row := &model.SegmentMember{
			CustomerID: m.GetCustomerId(), Lifecycle: m.GetLifecycle(),
			RecencyDays: m.GetRecencyDays(), Orders: m.GetOrders(), SpendCents: int(m.GetSpendCents()),
		}
		if m.GetLastOrderAt() != nil {
			t := m.GetLastOrderAt().AsTime()
			row.LastOrderAt = &t
		}
		members = append(members, row)
	}
	growth := make([]*model.SegmentGrowthPoint, 0, len(d.GetGrowth()))
	for _, g := range d.GetGrowth() {
		growth = append(growth, &model.SegmentGrowthPoint{Day: signalsDay(g.GetDay()), Count: g.GetCount()})
	}
	mix := make([]*model.SegmentStageCount, 0, len(d.GetLifecycleMix()))
	for _, s := range d.GetLifecycleMix() {
		mix = append(mix, &model.SegmentStageCount{Stage: s.GetStage(), Count: s.GetCount()})
	}
	rfm := make([]*model.SegmentRfmBucket, 0, len(d.GetRfmDistribution()))
	for _, b := range d.GetRfmDistribution() {
		rfm = append(rfm, &model.SegmentRfmBucket{Dimension: b.GetDimension(), Score: b.GetScore(), Count: b.GetCount()})
	}
	aff := make([]*model.SegmentAffinityCount, 0, len(d.GetTopAffinities()))
	for _, a := range d.GetTopAffinities() {
		aff = append(aff, &model.SegmentAffinityCount{Attr: a.GetAttr(), Value: a.GetValue(), Members: a.GetMembers()})
	}
	return &model.SynapseSegmentDetail{
		Segment: synapseSegmentModel(d.GetSegment()), Currency: d.GetCurrency(),
		RevenueCents: int(d.GetRevenueCents()), Orders: d.GetOrders(), AovCents: int(d.GetAovCents()),
		Members: members, MembersTruncated: d.GetMembersTruncated(),
		Growth: growth, LifecycleMix: mix, RfmDistribution: rfm, TopAffinities: aff,
	}, nil
}

func (r *queryResolver) SynapseOverview(ctx context.Context, channelID string, sinceDays *int32) (*model.SynapseOverview, error) {
	authed, err := r.signalsChannel(ctx, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	ov, err := r.SignalsAPI.Synapse.GetSynapseOverview(authed, &signalsv1.GetSynapseOverviewRequest{
		ChannelId: channelID, SinceDays: synapseWindow(sinceDays, 30, 365),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := &model.SynapseOverview{
		TotalEvents: ov.GetTotalEvents(), PageViews: ov.GetPageViews(), Conversions: ov.GetConversions(),
		IdentifiedCustomers: ov.GetIdentifiedCustomers(), AnonymousVisitors: ov.GetAnonymousVisitors(),
		Revenue: []*model.AttributedRevenue{}, TopEvents: []*model.CustomerEventCount{},
		ActiveCustomers: []*model.SynapseActiveCustomer{}, ActiveVisitors: []*model.SynapseVisitor{},
	}
	for _, rv := range ov.GetRevenue() {
		out.Revenue = append(out.Revenue, &model.AttributedRevenue{Currency: rv.GetCurrency(), Cents: int(rv.GetCents()), Orders: rv.GetOrders()})
	}
	for _, e := range ov.GetTopEvents() {
		out.TopEvents = append(out.TopEvents, &model.CustomerEventCount{Name: e.GetName(), Count: e.GetCount()})
	}
	for _, c := range ov.GetActiveCustomers() {
		row := &model.SynapseActiveCustomer{CustomerID: c.GetCustomerId(), Events: c.GetEvents()}
		if c.GetLastSeen() != nil {
			t := c.GetLastSeen().AsTime()
			row.LastSeen = &t
		}
		out.ActiveCustomers = append(out.ActiveCustomers, row)
	}
	for _, v := range ov.GetActiveVisitors() {
		row := &model.SynapseVisitor{AnonID: v.GetAnonId(), Events: v.GetEvents(), LastPage: v.GetLastPage()}
		if v.GetLastSeen() != nil {
			t := v.GetLastSeen().AsTime()
			row.LastSeen = &t
		}
		out.ActiveVisitors = append(out.ActiveVisitors, row)
	}
	return out, nil
}

func (r *queryResolver) SynapseTrackedAttributes(ctx context.Context, channelID string) ([]*model.SynapseTrackedAttribute, error) {
	authed, err := r.signalsChannel(ctx, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Synapse.ListTrackedAttributes(authed, &signalsv1.ListTrackedAttributesRequest{ChannelId: channelID})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.SynapseTrackedAttribute, 0, len(res.GetAttributes()))
	for _, a := range res.GetAttributes() {
		out = append(out, &model.SynapseTrackedAttribute{Slug: a.GetSlug(), Name: a.GetName(), Values: a.GetValues()})
	}
	return out, nil
}

func (r *queryResolver) SynapseSegments(ctx context.Context, channelID string) ([]*model.SynapseSegment, error) {
	authed, err := r.signalsChannel(ctx, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Synapse.ListBehavioralSegments(authed, &signalsv1.ListBehavioralSegmentsRequest{ChannelId: channelID, PageSize: 500})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.SynapseSegment, 0, len(res.GetSegments()))
	for _, s := range res.GetSegments() {
		out = append(out, synapseSegmentModel(s))
	}
	return out, nil
}

func (r *queryResolver) SynapseSegmentPreview(ctx context.Context, channelID string, root *model.SegmentNodeInput, conditions []*model.SegmentConditionInput, trait *model.SegmentTraitInput, affinity *model.SegmentAffinityInput) (int32, error) {
	authed, err := r.signalsChannel(ctx, "customers.view", channelID)
	if err != nil {
		return 0, err
	}
	res, err := r.SignalsAPI.Synapse.PreviewBehavioralSegment(authed, &signalsv1.PreviewBehavioralSegmentRequest{
		ChannelId: channelID, Definition: segmentDefinitionProto(root, conditions, trait, affinity),
	})
	if err != nil {
		return 0, signalsErr(err)
	}
	return res.GetMatched(), nil
}

func (r *queryResolver) SynapseFunnel(ctx context.Context, channelID string, steps []string, sinceDays *int32) (*model.SynapseFunnel, error) {
	authed, err := r.signalsChannel(ctx, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	if len(steps) < 2 {
		return nil, fmt.Errorf("a funnel needs at least 2 steps")
	}
	if len(steps) > 8 {
		steps = steps[:8]
	}
	// The walker streams raw step events and walks them per actor, so the window is the only real cost lever
	res, err := r.SignalsAPI.Synapse.GetFunnel(authed, &signalsv1.GetFunnelRequest{
		ChannelId: channelID, Steps: steps, SinceDays: synapseWindow(sinceDays, 30, 180),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := &model.SynapseFunnel{Steps: make([]*model.SynapseFunnelStep, 0, len(res.GetSteps()))}
	for _, s := range res.GetSteps() {
		out.Steps = append(out.Steps, &model.SynapseFunnelStep{EventName: s.GetEventName(), Count: s.GetCount()})
	}
	return out, nil
}

func (r *queryResolver) SynapseFlow(ctx context.Context, channelID string, event string, sinceDays *int32, depth *int32) (*model.SynapseFlow, error) {
	authed, err := r.signalsChannel(ctx, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	if event == "" {
		return nil, fmt.Errorf("event is required")
	}
	d := int32(3)
	if depth != nil && *depth > 0 {
		d = *depth
	}
	if d > 6 {
		d = 6
	}
	// The walker streams every event of every focus-event actor, and paths beyond 90 days are rarely meaningful
	res, err := r.SignalsAPI.Synapse.GetFlow(authed, &signalsv1.GetFlowRequest{
		ChannelId: channelID, Event: event, SinceDays: synapseWindow(sinceDays, 30, 90), Depth: d,
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := &model.SynapseFlow{
		Focus: res.GetFocus(), Cohort: res.GetCohort(), Continued: res.GetContinued(), AvgSteps: res.GetAvgSteps(),
		Nodes: make([]*model.SynapseFlowNode, 0, len(res.GetNodes())), TopPaths: make([]*model.SynapseFlowPath, 0, len(res.GetTopPaths())),
	}
	for _, n := range res.GetNodes() {
		out.Nodes = append(out.Nodes, &model.SynapseFlowNode{
			ID: n.GetId(), ParentID: n.GetParentId(), Event: n.GetEvent(), Depth: n.GetDepth(), Count: n.GetCount(),
		})
	}
	for _, p := range res.GetTopPaths() {
		out.TopPaths = append(out.TopPaths, &model.SynapseFlowPath{Events: p.GetEvents(), Count: p.GetCount()})
	}
	return out, nil
}

func (r *queryResolver) SynapseStickiness(ctx context.Context, channelID string, event *string, sinceDays *int32, windowDays *int32) (*model.SynapseStickiness, error) {
	authed, err := r.signalsChannel(ctx, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	ev := ""
	if event != nil {
		ev = *event
	}
	// The return window - an explicit argument wins, else the channel's cadence, else the suite's default
	window := int32(0)
	if windowDays != nil && *windowDays > 0 {
		window = *windowDays
	} else if cid, perr := uuid.Parse(channelID); perr == nil {
		if v, cerr := r.ConfigSvc.GetInt(ctx, "pulse/synapse/return_window_days", config.Scope{Kind: config.ScopeChannel, ChannelID: &cid}); cerr == nil && v > 0 {
			window = int32(v)
		}
	}
	s, err := r.SignalsAPI.Synapse.GetStickiness(authed, &signalsv1.GetStickinessRequest{
		ChannelId: channelID, Event: ev, SinceDays: synapseWindow(sinceDays, 30, 365), WindowDays: window,
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := &model.SynapseStickiness{
		WindowDays: s.GetWindowDays(), PriorActive: s.GetPriorActive(), ActiveNow: s.GetActiveNow(),
		Returning: s.GetReturning(), ReturnRate: s.GetReturnRate(), Wau: s.GetWau(), Mau: s.GetMau(),
		Days: make([]*model.ActivePoint, 0, len(s.GetDays())),
	}
	for _, d := range s.GetDays() {
		out.Days = append(out.Days, &model.ActivePoint{Day: signalsDay(d.GetDay()), Count: d.GetCount()})
	}
	return out, nil
}

func (r *queryResolver) SynapseInsights(ctx context.Context, channelID string, event *string, breakdown *string, measure *string, unit *string, sinceDays *int32) (*model.SynapseInsights, error) {
	authed, err := r.signalsChannel(ctx, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	deref := func(p *string) string {
		if p != nil {
			return *p
		}
		return ""
	}
	ins, err := r.SignalsAPI.Synapse.GetInsights(authed, &signalsv1.GetInsightsRequest{
		ChannelId: channelID, Event: deref(event), Breakdown: deref(breakdown),
		Measure: deref(measure), Unit: deref(unit), SinceDays: synapseWindow(sinceDays, 30, 365),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := &model.SynapseInsights{
		Points: make([]*model.InsightPoint, 0, len(ins.GetPoints())), Series: make([]*model.InsightSeries, 0, len(ins.GetSeries())),
		Total: ins.GetTotal(), Uniques: ins.GetUniques(), PrevTotal: ins.GetPrevTotal(), PrevUniques: ins.GetPrevUniques(),
	}
	for _, p := range ins.GetPoints() {
		out.Points = append(out.Points, &model.InsightPoint{Bucket: p.GetBucket().AsTime(), Key: p.GetKey(), Value: p.GetValue()})
	}
	for _, s := range ins.GetSeries() {
		out.Series = append(out.Series, &model.InsightSeries{Key: s.GetKey(), Total: s.GetTotal()})
	}
	return out, nil
}

func (r *queryResolver) SynapseRetention(ctx context.Context, channelID string, startEvent string, returnEvent string, sinceDays *int32, periods *int32, unit *string) (*model.SynapseRetention, error) {
	authed, err := r.signalsChannel(ctx, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	if startEvent == "" || returnEvent == "" {
		return nil, fmt.Errorf("startEvent and returnEvent are required")
	}
	n := int32(14)
	if periods != nil && *periods > 0 {
		n = *periods
	}
	if n > 60 {
		n = 60
	}
	step := "day"
	if unit != nil && *unit == "week" {
		step = "week"
	}
	ret, err := r.SignalsAPI.Synapse.GetRetention(authed, &signalsv1.GetRetentionRequest{
		ChannelId: channelID, StartEvent: startEvent, ReturnEvent: returnEvent,
		SinceDays: synapseWindow(sinceDays, 30, 365), Periods: n, Unit: step,
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	bucket := func(b *signalsv1.RetentionBucket) *model.RetentionBucket {
		return &model.RetentionBucket{Period: b.GetPeriod(), Count: b.GetCount(), Eligible: b.GetEligible()}
	}
	out := &model.SynapseRetention{
		Cohort:  ret.GetCohort(),
		Buckets: make([]*model.RetentionBucket, 0, len(ret.GetBuckets())),
		Cohorts: make([]*model.RetentionCohort, 0, len(ret.GetCohorts())),
	}
	for _, b := range ret.GetBuckets() {
		out.Buckets = append(out.Buckets, bucket(b))
	}
	for _, c := range ret.GetCohorts() {
		rc := &model.RetentionCohort{StartAt: c.GetStartAt().AsTime(), Size: c.GetSize(), Buckets: make([]*model.RetentionBucket, 0, len(c.GetBuckets()))}
		for _, b := range c.GetBuckets() {
			rc.Buckets = append(rc.Buckets, bucket(b))
		}
		out.Cohorts = append(out.Cohorts, rc)
	}
	return out, nil
}

// Gated on the channel the operator reached the visitor from, since the profile carries IPs and pages
func (r *queryResolver) VisitorProfile(ctx context.Context, channelID string, anonID string, sinceDays *int32) (*model.CustomerProfile, error) {
	authed, err := r.signalsChannel(ctx, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	if anonID == "" {
		return nil, fmt.Errorf("anonId is required")
	}
	res, err := r.SignalsAPI.Synapse.GetVisitorProfile(authed, &signalsv1.GetVisitorProfileRequest{
		ChannelId: channelID, AnonId: anonID, SinceDays: synapseWindow(sinceDays, 90, 365),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return profileModel(res.GetProfile()), nil
}

func (r *queryResolver) CustomerAudiences(ctx context.Context, customerID string) ([]*model.SignalSegment, error) {
	authed, err := r.signalsScoped(ctx, "customers.view")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(customerID); err != nil {
		return nil, errInvalidID("customerId", err)
	}
	res, err := r.SignalsAPI.Synapse.ListCustomerAudiences(authed, &signalsv1.ListCustomerAudiencesRequest{CustomerId: customerID})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.SignalSegment, 0, len(res.GetSegments()))
	for _, s := range res.GetSegments() {
		out = append(out, &model.SignalSegment{ID: s.GetId(), Name: s.GetName(), Slug: s.GetSlug()})
	}
	return out, nil
}
