package signals

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/signals/model"
	"github.com/skeletg/skelet/apps/backend/internal/behavioralsegment"
	"github.com/skeletg/skelet/apps/backend/internal/config"
)

// The definition (channel + conditions) rides as JSON in the segment's reserved refresh_sql column - what comes out is a normal customer segment, targetable by campaigns + flows
// The update path only accepts behavioral segments in the same channel, so a segment id can't hijack a hand-managed audience or another channel's
func (r *mutationResolver) SynapseSaveSegment(ctx context.Context, channelID string, name string, segmentID *string, root *model.SegmentNodeInput, conditions []*model.SegmentConditionInput, trait *model.SegmentTraitInput, affinity *model.SegmentAffinityInput) (*model.SynapseSegment, error) {
	if _, err := synapseScope(ctx, r.Resolver, "customers.manage", channelID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	in := behavioralsegment.SaveInput{ChannelID: channelID, Name: name, Definition: defFromInputs(channelID, root, conditions, trait, affinity)}
	if segmentID != nil && *segmentID != "" {
		sid, err := uuid.Parse(*segmentID)
		if err != nil {
			return nil, errInvalidID("segmentId", err)
		}
		in.SegmentID = &sid
	}
	seg, err := r.BehavioralSvc.Save(ctx, in, time.Now())
	if err != nil {
		return nil, err
	}
	return synapseSegmentModel(seg), nil
}

func (r *mutationResolver) SynapseRefreshSegment(ctx context.Context, segmentID string) (*model.SynapseSegment, error) {
	seg, err := synapseSegmentScope(ctx, r.Resolver, segmentID, "customers.manage") // .manage not .view - a refresh rewrites campaign/flow targeting
	if err != nil {
		return nil, err
	}
	seg, err = r.BehavioralSvc.Refresh(ctx, seg.ID, time.Now())
	if err != nil {
		return nil, err
	}
	return synapseSegmentModel(seg), nil
}

// nil windows clears the override - members otherwise get these in place of the channel default, shortest wins across segments
func (r *mutationResolver) SynapseSetSegmentLifecycleWindows(ctx context.Context, segmentID string, windows *model.SegmentLifecycleWindowsInput) (*model.SynapseSegment, error) {
	seg, err := synapseSegmentScope(ctx, r.Resolver, segmentID, "customers.manage")
	if err != nil {
		return nil, err
	}
	var newDays, activeDays, atRiskDays *int32
	if windows != nil {
		n, a, ar := windows.NewDays, windows.ActiveDays, windows.AtRiskDays
		newDays, activeDays, atRiskDays = &n, &a, &ar
	}
	seg, err = r.BehavioralSvc.SetLifecycleWindows(ctx, seg.ID, newDays, activeDays, atRiskDays)
	if err != nil {
		return nil, err
	}
	return synapseSegmentModel(seg), nil
}

func (r *queryResolver) CustomerProfile(ctx context.Context, customerID string, sinceDays *int32) (*model.CustomerProfile, error) {
	cid, err := r.scopedCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	days := synapseDays(sinceDays, 90, 365)
	since := time.Now().AddDate(0, 0, -int(days))
	p, err := r.AnalyticsSvc.CustomerProfile(ctx, cid, since)
	if err != nil {
		return nil, err
	}
	m := profileToModel(p)
	if t, ok, terr := r.AnalyticsSvc.CustomerTraits(ctx, cid); terr == nil && ok { // RFM / lifecycle / engagement, present only once a trait-compute run has covered this customer
		m.Traits = customerTraitsModel(t)
	}
	if arms, aerr := r.AnalyticsSvc.CustomerExperiments(ctx, cid, since); aerr == nil { // Prism arms reach a customer only through identity stitching
		m.Experiments = experimentArmsModel(arms)
	}
	if affs, aerr := r.AnalyticsSvc.CustomerAffinities(ctx, cid, since, synapseAffinityAttrs(ctx, r.Resolver)); aerr == nil {
		m.Affinities = affinityModels(affs)
	}
	return m, nil
}

// Member names are resolved here because the analytics package can't import customer
// Gated on the channel in the stored definition, not globally - the member list carries names/emails/spend, so a bare global check would let any admin enumerate segment ids across channels
func (r *queryResolver) SynapseSegment(ctx context.Context, id string) (*model.SynapseSegmentDetail, error) {
	seg, err := synapseSegmentScope(ctx, r.Resolver, id, "customers.view")
	if err != nil {
		return nil, err
	}
	segChannel, _ := seg.Definition.Channel() // validated by synapseSegmentScope
	const memberCap = 200
	d, err := r.BehavioralSvc.Detail(ctx, seg.ID, memberCap, 90, 12)
	if err != nil {
		return nil, err
	}
	members := make([]*model.SegmentMember, 0, len(d.Members))
	for _, m := range d.Members {
		var last *time.Time
		if !m.LastOrderAt.IsZero() {
			t := m.LastOrderAt
			last = &t
		}
		members = append(members, &model.SegmentMember{
			CustomerID: m.CustomerID.String(), Lifecycle: m.Lifecycle,
			RecencyDays: int32(m.RecencyDays), Orders: int32(m.Orders), SpendCents: int(m.SpendCents), LastOrderAt: last,
		})
	}
	gpts := make([]*model.SegmentGrowthPoint, 0, len(d.Growth))
	for _, g := range d.Growth {
		day := g.Day
		gpts = append(gpts, &model.SegmentGrowthPoint{Day: day, Count: int32(g.Count)})
	}
	lifecycleMix := make([]*model.SegmentStageCount, 0, len(d.LifecycleMix))
	for _, s := range d.LifecycleMix {
		lifecycleMix = append(lifecycleMix, &model.SegmentStageCount{Stage: s.Stage, Count: int32(s.Count)})
	}
	rfmDist := make([]*model.SegmentRfmBucket, 0, len(d.RfmDistribution))
	for _, b := range d.RfmDistribution {
		rfmDist = append(rfmDist, &model.SegmentRfmBucket{Dimension: b.Dimension, Score: int32(b.Score), Count: int32(b.Count)})
	}
	topAff := make([]*model.SegmentAffinityCount, 0, len(d.TopAffinities))
	for _, a := range d.TopAffinities {
		topAff = append(topAff, &model.SegmentAffinityCount{Attr: a.Attr, Value: a.Value, Members: int32(a.Members)})
	}

	currency := "USD" // channel currency for the money fields, never a hardcoded USD
	if ch, cerr := r.ChannelSvc.GetByID(ctx, segChannel); cerr == nil && ch.CurrencyCode != "" {
		currency = ch.CurrencyCode
	}

	return &model.SynapseSegmentDetail{
		Segment:          synapseSegmentModel(d.Segment),
		Currency:         currency,
		RevenueCents:     int(d.RevenueCents),
		Orders:           int32(d.Orders),
		AovCents:         int(d.AOVCents),
		Members:          members,
		MembersTruncated: d.Segment.MemberCount > len(members),
		Growth:           gpts,
		LifecycleMix:     lifecycleMix,
		RfmDistribution:  rfmDist,
		TopAffinities:    topAff,
	}, nil
}

// Active-customer names are resolved here - the analytics package is a leaf and can't import customer
func (r *queryResolver) SynapseOverview(ctx context.Context, channelID string, sinceDays *int32) (*model.SynapseOverview, error) {
	cid, err := synapseScope(ctx, r.Resolver, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	days := synapseDays(sinceDays, 30, 365)
	ov, err := r.AnalyticsSvc.SynapseOverview(ctx, cid, time.Now().AddDate(0, 0, -int(days)))
	if err != nil {
		return nil, err
	}

	out := &model.SynapseOverview{
		TotalEvents:         int32(ov.TotalEvents),
		PageViews:           int32(ov.PageViews),
		Conversions:         int32(ov.Conversions),
		IdentifiedCustomers: int32(ov.IdentifiedCustomers),
		AnonymousVisitors:   int32(ov.AnonymousVisitors),
		Revenue:             []*model.AttributedRevenue{},
		TopEvents:           []*model.CustomerEventCount{},
		ActiveCustomers:     []*model.SynapseActiveCustomer{},
		ActiveVisitors:      []*model.SynapseVisitor{},
	}
	for _, rv := range ov.Revenue {
		out.Revenue = append(out.Revenue, &model.AttributedRevenue{Currency: rv.Currency, Cents: int(rv.Cents), Orders: int32(rv.Orders)})
	}
	for _, e := range ov.TopEvents {
		out.TopEvents = append(out.TopEvents, &model.CustomerEventCount{Name: e.Name, Count: int32(e.Count)})
	}
	for _, c := range ov.ActiveCustomers {
		seen := c.LastSeen
		out.ActiveCustomers = append(out.ActiveCustomers, &model.SynapseActiveCustomer{
			CustomerID: c.CustomerID.String(),
			Events:     int32(c.Events),
			LastSeen:   &seen,
		})
	}
	for _, v := range ov.ActiveVisitors {
		seen := v.LastSeen
		out.ActiveVisitors = append(out.ActiveVisitors, &model.SynapseVisitor{
			AnonID:   v.AnonID,
			Events:   int32(v.Events),
			LastSeen: &seen,
			LastPage: v.LastPage,
		})
	}
	return out, nil
}

// Values observed over the last 90d, so the segment builder's affinity picker offers real attributes/values rather than free text
func (r *queryResolver) SynapseTrackedAttributes(ctx context.Context, channelID string) ([]*model.SynapseTrackedAttribute, error) {
	cid, err := synapseScope(ctx, r.Resolver, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	attrs := synapseAffinityAttrs(ctx, r.Resolver)
	since := time.Now().AddDate(0, 0, -90)
	out := make([]*model.SynapseTrackedAttribute, 0, len(attrs))
	for _, a := range attrs {
		values := []string{}
		if vals, verr := r.AnalyticsSvc.AffinityValueCounts(ctx, cid, a.Slug, since, 30); verr == nil {
			for _, v := range vals {
				values = append(values, v.Value)
			}
		}
		out = append(out, &model.SynapseTrackedAttribute{Slug: a.Slug, Name: a.Name, Values: values})
	}
	return out, nil
}

func (r *queryResolver) SynapseSegments(ctx context.Context, channelID string) ([]*model.SynapseSegment, error) {
	if _, err := synapseScope(ctx, r.Resolver, "customers.view", channelID); err != nil {
		return nil, err
	}
	segs, err := r.BehavioralSvc.List(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.SynapseSegment, 0, len(segs))
	for _, s := range segs {
		out = append(out, synapseSegmentModel(s))
	}
	return out, nil
}

func (r *queryResolver) SynapseSegmentPreview(ctx context.Context, channelID string, root *model.SegmentNodeInput, conditions []*model.SegmentConditionInput, trait *model.SegmentTraitInput, affinity *model.SegmentAffinityInput) (int32, error) {
	if _, err := synapseScope(ctx, r.Resolver, "customers.view", channelID); err != nil {
		return 0, err
	}
	ids, err := r.BehavioralSvc.Evaluate(ctx, defFromInputs(channelID, root, conditions, trait, affinity), time.Now())
	if err != nil {
		return 0, err
	}
	return int32(len(ids)), nil
}

func (r *queryResolver) SynapseFunnel(ctx context.Context, channelID string, steps []string, sinceDays *int32) (*model.SynapseFunnel, error) {
	cid, err := synapseScope(ctx, r.Resolver, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	if len(steps) < 2 {
		return nil, fmt.Errorf("a funnel needs at least 2 steps")
	}
	if len(steps) > 8 {
		steps = steps[:8]
	}
	days := synapseDays(sinceDays, 30, 180) // the walker streams raw step events and the per-actor walk is Go-side, so the window is the only real cost lever
	fs, err := r.AnalyticsSvc.Funnel(ctx, cid, steps, time.Now().AddDate(0, 0, -int(days)))
	if err != nil {
		return nil, err
	}
	out := &model.SynapseFunnel{Steps: make([]*model.SynapseFunnelStep, 0, len(fs))}
	for _, s := range fs {
		out.Steps = append(out.Steps, &model.SynapseFunnelStep{EventName: s.EventName, Count: int32(s.Count)})
	}
	return out, nil
}

func (r *queryResolver) SynapseFlow(ctx context.Context, channelID string, event string, sinceDays *int32, depth *int32) (*model.SynapseFlow, error) {
	cid, err := synapseScope(ctx, r.Resolver, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	if event == "" {
		return nil, fmt.Errorf("event is required")
	}
	days := synapseDays(sinceDays, 30, 90) // the walker streams every event of every focus-event actor - paths beyond 90d are rarely meaningful anyway
	d := 3
	if depth != nil && *depth > 0 {
		d = int(*depth)
	}
	if d > 6 {
		d = 6
	}
	res, err := r.AnalyticsSvc.Flow(ctx, cid, event, time.Now().AddDate(0, 0, -int(days)), d, 5)
	if err != nil {
		return nil, err
	}
	out := &model.SynapseFlow{
		Focus:     event,
		Cohort:    int32(res.Cohort),
		Continued: int32(res.Continued),
		AvgSteps:  res.AvgSteps,
		Nodes:     make([]*model.SynapseFlowNode, 0, len(res.Nodes)),
		TopPaths:  make([]*model.SynapseFlowPath, 0, len(res.TopPaths)),
	}
	for _, n := range res.Nodes {
		out.Nodes = append(out.Nodes, &model.SynapseFlowNode{ID: n.ID, ParentID: n.ParentID, Event: n.Event, Depth: int32(n.Depth), Count: int32(n.Count)})
	}
	for _, p := range res.TopPaths {
		out.TopPaths = append(out.TopPaths, &model.SynapseFlowPath{Events: p.Events, Count: int32(p.Count)})
	}
	return out, nil
}

func (r *queryResolver) SynapseStickiness(ctx context.Context, channelID string, event *string, sinceDays *int32, windowDays *int32) (*model.SynapseStickiness, error) {
	cid, err := synapseScope(ctx, r.Resolver, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	ev := ""
	if event != nil {
		ev = *event
	}
	days := synapseDays(sinceDays, 30, 365)
	window := 0 // return window: explicit arg wins, else the channel's configured cadence, else the registered 30d default
	if windowDays != nil && *windowDays > 0 {
		window = int(*windowDays)
	} else if v, cerr := r.ConfigSvc.GetInt(ctx, "pulse/synapse/return_window_days", config.Scope{Kind: config.ScopeChannel, ChannelID: &cid}); cerr == nil && v > 0 {
		window = int(v)
	}
	now := time.Now()
	s, err := r.AnalyticsSvc.Stickiness(ctx, cid, ev, now.AddDate(0, 0, -int(days)), now, window)
	if err != nil {
		return nil, err
	}
	out := &model.SynapseStickiness{
		WindowDays: int32(s.WindowDays), PriorActive: int32(s.PriorActive), ActiveNow: int32(s.ActiveNow),
		Returning: int32(s.Returning), ReturnRate: s.ReturnRate, Wau: int32(s.WAU), Mau: int32(s.MAU),
		Days: make([]*model.ActivePoint, 0, len(s.Days)),
	}
	for _, d := range s.Days {
		day := d.Day
		out.Days = append(out.Days, &model.ActivePoint{Day: day, Count: int32(d.Count)})
	}
	return out, nil
}

func (r *queryResolver) SynapseInsights(ctx context.Context, channelID string, event *string, breakdown *string, measure *string, unit *string, sinceDays *int32) (*model.SynapseInsights, error) {
	cid, err := synapseScope(ctx, r.Resolver, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	deref := func(p *string) string {
		if p != nil {
			return *p
		}
		return ""
	}
	days := synapseDays(sinceDays, 30, 365)
	now := time.Now()
	ins, err := r.AnalyticsSvc.Insights(ctx, cid, deref(event), deref(breakdown), deref(measure), deref(unit), now.AddDate(0, 0, -int(days)), now)
	if err != nil {
		return nil, err
	}
	out := &model.SynapseInsights{
		Points:      make([]*model.InsightPoint, 0, len(ins.Points)),
		Series:      make([]*model.InsightSeries, 0, len(ins.Series)),
		Total:       int32(ins.Total),
		Uniques:     int32(ins.Uniques),
		PrevTotal:   int32(ins.PrevTotal),
		PrevUniques: int32(ins.PrevUniques),
	}
	for _, p := range ins.Points {
		b := p.Bucket
		out.Points = append(out.Points, &model.InsightPoint{Bucket: b, Key: p.Key, Value: int32(p.Value)})
	}
	for _, s := range ins.Series {
		out.Series = append(out.Series, &model.InsightSeries{Key: s.Key, Total: int32(s.Total)})
	}
	return out, nil
}

func (r *queryResolver) SynapseRetention(ctx context.Context, channelID string, startEvent string, returnEvent string, sinceDays *int32, periods *int32, unit *string) (*model.SynapseRetention, error) {
	cid, err := synapseScope(ctx, r.Resolver, "customers.view", channelID)
	if err != nil {
		return nil, err
	}
	if startEvent == "" || returnEvent == "" {
		return nil, fmt.Errorf("startEvent and returnEvent are required")
	}
	days := synapseDays(sinceDays, 30, 365)
	n := 14
	if periods != nil && *periods > 0 {
		n = int(*periods)
	}
	if n > 60 {
		n = 60
	}
	step := 24 * time.Hour
	if unit != nil && *unit == "week" {
		step = 7 * 24 * time.Hour
	}
	now := time.Now()
	ret, err := r.AnalyticsSvc.Retention(ctx, cid, startEvent, returnEvent, now.AddDate(0, 0, -int(days)), now, n, step)
	if err != nil {
		return nil, err
	}
	out := &model.SynapseRetention{
		Cohort:  int32(ret.Cohort),
		Buckets: make([]*model.RetentionBucket, 0, len(ret.Buckets)),
		Cohorts: make([]*model.RetentionCohort, 0, len(ret.Cohorts)),
	}
	for _, b := range ret.Buckets {
		out.Buckets = append(out.Buckets, &model.RetentionBucket{Period: int32(b.Period), Count: int32(b.Count), Eligible: int32(b.Eligible)})
	}
	for _, c := range ret.Cohorts {
		rc := &model.RetentionCohort{StartAt: c.StartAt, Size: int32(c.Size), Buckets: make([]*model.RetentionBucket, 0, len(c.Buckets))}
		for _, b := range c.Buckets {
			rc.Buckets = append(rc.Buckets, &model.RetentionBucket{Period: int32(b.Period), Count: int32(b.Count), Eligible: int32(b.Eligible)})
		}
		out.Cohorts = append(out.Cohorts, rc)
	}
	return out, nil
}

// Gated on the channel the operator reached the visitor from, not globally - the profile carries IPs/pages, so a bare global check would expose any visitor to any channel-scoped admin
func (r *queryResolver) VisitorProfile(ctx context.Context, channelID string, anonID string, sinceDays *int32) (*model.CustomerProfile, error) {
	if _, err := synapseScope(ctx, r.Resolver, "customers.view", channelID); err != nil {
		return nil, err
	}
	if anonID == "" {
		return nil, fmt.Errorf("anonId is required")
	}
	days := synapseDays(sinceDays, 90, 365)
	since := time.Now().AddDate(0, 0, -int(days))
	p, err := r.AnalyticsSvc.VisitorProfile(ctx, anonID, since)
	if err != nil {
		return nil, err
	}
	m := profileToModel(p)
	m.Affinities = affinityModels(r.AnalyticsSvc.VisitorAffinities(ctx, anonID, since, synapseAffinityAttrs(ctx, r.Resolver)))
	return m, nil
}

func (r *queryResolver) CustomerAudiences(ctx context.Context, customerID string) ([]*model.SignalSegment, error) {
	cid, err := r.scopedCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	segs, err := r.SegmentSvc.SegmentsForCustomer(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.SignalSegment, 0, len(segs))
	for _, s := range segs {
		out = append(out, &model.SignalSegment{ID: s.ID.String(), Name: s.Name, Slug: s.Slug})
	}
	return out, nil
}
