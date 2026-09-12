package signalsgrpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/behavioralsegment"
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

type synapseServer struct {
	signalsv1.UnimplementedSynapseServiceServer
	rec      *analytics.Recorder
	segments *segment.Service
	// Which attributes are flagged for affinity tracking - the suite's one commerce read
	commerce *db.Queries
	// A customer's channel comes off the synced roster, since this service has no customers table
	roster *signalsdb.Queries
	// Synapse's engine over segments - the definition grammar, the evaluator and the sync passes
	behavioral *behavioralsegment.Service
}

func (s *synapseServer) GetSynapseOverview(ctx context.Context, req *signalsv1.GetSynapseOverviewRequest) (*signalsv1.GetSynapseOverviewResponse, error) {
	cid, err := channelScope(ctx, "customers.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	ov, err := s.rec.SynapseOverview(ctx, cid, since(req.GetSinceDays(), 30))
	if err != nil {
		return nil, err
	}
	customers := make([]*signalsv1.SynapseActiveCustomer, 0, len(ov.ActiveCustomers))
	for _, c := range ov.ActiveCustomers {
		customers = append(customers, &signalsv1.SynapseActiveCustomer{CustomerId: c.CustomerID.String(), Events: int32(c.Events), LastSeen: ts(c.LastSeen)})
	}
	visitors := make([]*signalsv1.SynapseVisitor, 0, len(ov.ActiveVisitors))
	for _, v := range ov.ActiveVisitors {
		visitors = append(visitors, &signalsv1.SynapseVisitor{AnonId: v.AnonID, Events: int32(v.Events), LastSeen: ts(v.LastSeen), LastPage: v.LastPage})
	}
	return &signalsv1.GetSynapseOverviewResponse{
		TotalEvents: int32(ov.TotalEvents), PageViews: int32(ov.PageViews), Conversions: int32(ov.Conversions),
		IdentifiedCustomers: int32(ov.IdentifiedCustomers), AnonymousVisitors: int32(ov.AnonymousVisitors),
		Revenue: revenueProto(ov.Revenue), TopEvents: eventCounts(ov.TopEvents),
		ActiveCustomers: customers, ActiveVisitors: visitors,
	}, nil
}

func (s *synapseServer) GetFunnel(ctx context.Context, req *signalsv1.GetFunnelRequest) (*signalsv1.GetFunnelResponse, error) {
	cid, err := channelScope(ctx, "customers.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	steps := make([]string, 0, len(req.GetSteps()))
	for _, s := range req.GetSteps() {
		if s != "" {
			steps = append(steps, s)
		}
	}
	if len(steps) < 2 {
		return nil, invalidArgument("a funnel needs at least two steps")
	}
	rows, err := s.rec.Funnel(ctx, cid, steps, since(req.GetSinceDays(), 30))
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.FunnelStep, 0, len(rows))
	for _, st := range rows {
		out = append(out, &signalsv1.FunnelStep{EventName: st.EventName, Count: int32(st.Count)})
	}
	return &signalsv1.GetFunnelResponse{Steps: out}, nil
}

func (s *synapseServer) GetFlow(ctx context.Context, req *signalsv1.GetFlowRequest) (*signalsv1.GetFlowResponse, error) {
	cid, err := channelScope(ctx, "customers.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	if req.GetEvent() == "" {
		return nil, invalidArgument("event is required")
	}
	depth := int(topN(req.GetDepth(), 4, 8))
	fl, err := s.rec.Flow(ctx, cid, req.GetEvent(), since(req.GetSinceDays(), 30), depth, 6)
	if err != nil {
		return nil, err
	}
	nodes := make([]*signalsv1.FlowNode, 0, len(fl.Nodes))
	for _, n := range fl.Nodes {
		nodes = append(nodes, &signalsv1.FlowNode{Id: n.ID, ParentId: n.ParentID, Event: n.Event, Depth: int32(n.Depth), Count: int32(n.Count)})
	}
	paths := make([]*signalsv1.FlowPath, 0, len(fl.TopPaths))
	for _, p := range fl.TopPaths {
		paths = append(paths, &signalsv1.FlowPath{Events: p.Events, Count: int32(p.Count)})
	}
	return &signalsv1.GetFlowResponse{
		Focus: req.GetEvent(), Cohort: int32(fl.Cohort), Continued: int32(fl.Continued),
		AvgSteps: fl.AvgSteps, Nodes: nodes, TopPaths: paths,
	}, nil
}

func (s *synapseServer) GetRetention(ctx context.Context, req *signalsv1.GetRetentionRequest) (*signalsv1.GetRetentionResponse, error) {
	cid, err := channelScope(ctx, "customers.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	if req.GetStartEvent() == "" || req.GetReturnEvent() == "" {
		return nil, invalidArgument("start_event and return_event are both required")
	}
	unit := 24 * time.Hour
	if req.GetUnit() == "week" {
		unit = 7 * 24 * time.Hour
	}
	periods := int(topN(req.GetPeriods(), 8, 12))
	res, err := s.rec.Retention(ctx, cid, req.GetStartEvent(), req.GetReturnEvent(), since(req.GetSinceDays(), 30), time.Now(), periods, unit)
	if err != nil {
		return nil, err
	}
	cohorts := make([]*signalsv1.RetentionCohort, 0, len(res.Cohorts))
	for _, c := range res.Cohorts {
		cohorts = append(cohorts, &signalsv1.RetentionCohort{StartAt: ts(c.StartAt), Size: int32(c.Size), Buckets: buckets(c.Buckets)})
	}
	return &signalsv1.GetRetentionResponse{Cohort: int32(res.Cohort), Buckets: buckets(res.Buckets), Cohorts: cohorts}, nil
}

func (s *synapseServer) GetStickiness(ctx context.Context, req *signalsv1.GetStickinessRequest) (*signalsv1.GetStickinessResponse, error) {
	cid, err := channelScope(ctx, "customers.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	st, err := s.rec.Stickiness(ctx, cid, req.GetEvent(), since(req.GetSinceDays(), 30), time.Now(), int(windowDays(req.GetWindowDays(), 30)))
	if err != nil {
		return nil, err
	}
	days := make([]*signalsv1.ActivePoint, 0, len(st.Days))
	for _, d := range st.Days {
		days = append(days, &signalsv1.ActivePoint{Day: d.Day.Format(time.DateOnly), Count: int32(d.Count)})
	}
	return &signalsv1.GetStickinessResponse{
		Days: days, WindowDays: int32(st.WindowDays), PriorActive: int32(st.PriorActive),
		ActiveNow: int32(st.ActiveNow), Returning: int32(st.Returning), ReturnRate: st.ReturnRate,
		Wau: int32(st.WAU), Mau: int32(st.MAU),
	}, nil
}

func (s *synapseServer) GetInsights(ctx context.Context, req *signalsv1.GetInsightsRequest) (*signalsv1.GetInsightsResponse, error) {
	cid, err := channelScope(ctx, "customers.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	ins, err := s.rec.Insights(ctx, cid, req.GetEvent(), req.GetBreakdown(), req.GetMeasure(), req.GetUnit(), since(req.GetSinceDays(), 30), time.Now())
	if err != nil {
		return nil, err
	}
	points := make([]*signalsv1.InsightPoint, 0, len(ins.Points))
	for _, p := range ins.Points {
		points = append(points, &signalsv1.InsightPoint{Bucket: ts(p.Bucket), Key: p.Key, Value: int32(p.Value)})
	}
	series := make([]*signalsv1.InsightSeries, 0, len(ins.Series))
	for _, sr := range ins.Series {
		series = append(series, &signalsv1.InsightSeries{Key: sr.Key, Total: int32(sr.Total)})
	}
	return &signalsv1.GetInsightsResponse{
		Points: points, Series: series, Total: int32(ins.Total), Uniques: int32(ins.Uniques),
		PrevTotal: int32(ins.PrevTotal), PrevUniques: int32(ins.PrevUniques),
	}, nil
}

func (s *synapseServer) GetCustomerProfile(ctx context.Context, req *signalsv1.GetCustomerProfileRequest) (*signalsv1.GetCustomerProfileResponse, error) {
	cid, err := s.scopedCustomer(ctx, req.GetCustomerId())
	if err != nil {
		return nil, err
	}
	window := since(req.GetSinceDays(), 90)
	p, err := s.rec.CustomerProfile(ctx, cid, window)
	if err != nil {
		return nil, err
	}
	out := profileProto(p)
	if t, ok, terr := s.rec.CustomerTraits(ctx, cid); terr == nil && ok {
		out.Traits = traitsProto(t)
	}
	if arms, aerr := s.rec.CustomerExperiments(ctx, cid, window); aerr == nil {
		out.Experiments = armsProto(arms)
	}
	if affs, aerr := s.rec.CustomerAffinities(ctx, cid, window, s.trackedAttrs(ctx)); aerr == nil {
		out.Affinities = affinitiesProto(affs)
	}
	return &signalsv1.GetCustomerProfileResponse{Profile: out}, nil
}

func (s *synapseServer) GetVisitorProfile(ctx context.Context, req *signalsv1.GetVisitorProfileRequest) (*signalsv1.GetVisitorProfileResponse, error) {
	// A visitor's activity is channel-agnostic - the scope decides who may look, not what is returned
	if _, err := channelScope(ctx, "customers.view", req.GetChannelId()); err != nil {
		return nil, err
	}
	if req.GetAnonId() == "" {
		return nil, invalidArgument("anon_id is required")
	}
	window := since(req.GetSinceDays(), 90)
	p, err := s.rec.VisitorProfile(ctx, req.GetAnonId(), window)
	if err != nil {
		return nil, err
	}
	out := profileProto(p)
	out.Affinities = affinitiesProto(s.rec.VisitorAffinities(ctx, req.GetAnonId(), window, s.trackedAttrs(ctx)))
	return &signalsv1.GetVisitorProfileResponse{Profile: out}, nil
}

func (s *synapseServer) ListCustomerAudiences(ctx context.Context, req *signalsv1.ListCustomerAudiencesRequest) (*signalsv1.ListCustomerAudiencesResponse, error) {
	cid, err := s.scopedCustomer(ctx, req.GetCustomerId())
	if err != nil {
		return nil, err
	}
	segs, err := s.segments.SegmentsForCustomer(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.SignalSegment, 0, len(segs))
	for _, sg := range segs {
		out = append(out, &signalsv1.SignalSegment{Id: sg.ID.String(), Name: sg.Name, Slug: sg.Slug})
	}
	return &signalsv1.ListCustomerAudiencesResponse{Segments: out}, nil
}

func (s *synapseServer) ListTrackedAttributes(ctx context.Context, req *signalsv1.ListTrackedAttributesRequest) (*signalsv1.ListTrackedAttributesResponse, error) {
	cid, err := channelScope(ctx, "customers.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	attrs := s.trackedAttrs(ctx)
	out := make([]*signalsv1.TrackedAttribute, 0, len(attrs))
	for _, a := range attrs {
		vals, _ := s.rec.AffinityValueCounts(ctx, cid, a.Slug, since(0, 90), 20)
		names := make([]string, 0, len(vals))
		for _, v := range vals {
			names = append(names, v.Value)
		}
		out = append(out, &signalsv1.TrackedAttribute{Slug: a.Slug, Name: a.Name, Values: names})
	}
	return &signalsv1.ListTrackedAttributesResponse{Attributes: out}, nil
}

func (s *synapseServer) ListAffinityValues(ctx context.Context, req *signalsv1.ListAffinityValuesRequest) (*signalsv1.ListAffinityValuesResponse, error) {
	cid, err := channelScope(ctx, "customers.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	if req.GetAttribute() == "" {
		return nil, invalidArgument("attribute is required")
	}
	vals, err := s.rec.AffinityValueCounts(ctx, cid, req.GetAttribute(), since(req.GetSinceDays(), 90), int(topN(req.GetLimit(), 15, 50)))
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.AffinityValue, 0, len(vals))
	for _, v := range vals {
		out = append(out, &signalsv1.AffinityValue{Value: v.Value, Actors: int32(v.Actors)})
	}
	return &signalsv1.ListAffinityValuesResponse{Values: out}, nil
}

// customers.view on the customer's own channel, which this service reads off the synced roster
func (s *synapseServer) scopedCustomer(ctx context.Context, raw string) (uuid.UUID, error) {
	if err := requirePermission(ctx, "customers.view"); err != nil {
		return uuid.Nil, err
	}
	id, err := parseID("customer_id", raw)
	if err != nil {
		return uuid.Nil, err
	}
	if s.roster == nil {
		return uuid.Nil, status.Error(codes.FailedPrecondition, "the customer roster is not configured")
	}
	channelID, err := s.roster.ChannelForCustomer(ctx, id)
	if err != nil {
		return uuid.Nil, status.Error(codes.NotFound, "customer not found")
	}
	if err := requireChannel(ctx, "customers.view", channelID); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// The attributes flagged for affinity tracking, which is a catalog setting rather than a suite one
func (s *synapseServer) trackedAttrs(ctx context.Context) []analytics.AffinityAttr {
	if s.commerce == nil {
		return nil
	}
	rows, err := s.commerce.ListSynapseTrackedAttributes(ctx)
	if err != nil {
		return nil
	}
	out := make([]analytics.AffinityAttr, 0, len(rows))
	for _, a := range rows {
		out = append(out, analytics.AffinityAttr{Slug: a.Slug, Name: a.Name})
	}
	return out
}

func profileProto(p analytics.Profile) *signalsv1.Profile {
	timeline := make([]*signalsv1.CustomerActivityItem, 0, len(p.Timeline))
	for _, t := range p.Timeline {
		timeline = append(timeline, &signalsv1.CustomerActivityItem{
			EventName: t.EventName, OccurredAt: ts(t.OccurredAt), Source: t.Source,
			Summary: t.Summary, DetailsJson: string(t.Props),
		})
	}
	pages := make([]*signalsv1.ProfilePageView, 0, len(p.TopPages))
	for _, pg := range p.TopPages {
		pages = append(pages, &signalsv1.ProfilePageView{Title: pg.Title, PageKey: pg.PageKey, Views: int32(pg.Views), DwellSeconds: int32(pg.AvgDwellSeconds)})
	}
	products := make([]*signalsv1.ProfileProductView, 0, len(p.TopProducts))
	for _, pr := range p.TopProducts {
		products = append(products, &signalsv1.ProfileProductView{Name: pr.Name, Image: pr.Image, Price: pr.Price, Sku: pr.SKU, Views: int32(pr.Views)})
	}
	out := &signalsv1.Profile{
		TotalEvents: int32(p.TotalEvents), PageViews: int32(p.PageViews), Conversions: int32(p.Conversions),
		Revenue: revenueProto(p.Revenue), Counts: eventCounts(p.Counts), Timeline: timeline,
		Devices: valueCounts(p.Devices), Browsers: valueCounts(p.Browsers), Referrers: valueCounts(p.Referrers),
		IpAddresses: valueCounts(p.IPs), OperatingSystems: valueCounts(p.OSes), Screens: valueCounts(p.Screens),
		Countries: valueCounts(p.Countries), Sources: valueCounts(p.Sources), Campaigns: valueCounts(p.Campaigns),
		TopPages: pages, PagesViewedTotal: int32(p.PagesViewedTotal), TopProducts: products,
		Searches: valueCounts(p.Searches), Sessions: int32(p.Sessions),
	}
	if p.FirstSeen != nil {
		out.FirstSeen = ts(*p.FirstSeen)
	}
	if p.LastSeen != nil {
		out.LastSeen = ts(*p.LastSeen)
	}
	return out
}

func traitsProto(t analytics.CustomerTraits) *signalsv1.CustomerTraits {
	return &signalsv1.CustomerTraits{
		Lifecycle: t.Lifecycle, RfmRecency: int32(t.RFMRecency), RfmFrequency: int32(t.RFMFrequency),
		RfmMonetary: int32(t.RFMMonetary), RecencyDays: int32(t.RecencyDays), Frequency: int32(t.Frequency),
		MonetaryCents: t.MonetaryCents, EngagementScore: int32(t.EngagementScore),
		ChurnRisk: int32(t.ChurnRisk), PredictedClvCents: t.PredictedCLVCents,
		NextOrderDays: int32(t.NextOrderDays), ComputedAt: ts(t.ComputedAt),
	}
}

func armsProto(arms []analytics.ExperimentArm) []*signalsv1.CustomerExperimentArm {
	out := make([]*signalsv1.CustomerExperimentArm, 0, len(arms))
	for _, a := range arms {
		out = append(out, &signalsv1.CustomerExperimentArm{Experiment: a.Experiment, Variation: a.Variation, LastExposedAt: ts(a.LastExposedAt)})
	}
	return out
}

func affinitiesProto(affs []analytics.AttributeAffinity) []*signalsv1.AttributeAffinity {
	out := make([]*signalsv1.AttributeAffinity, 0, len(affs))
	for _, a := range affs {
		out = append(out, &signalsv1.AttributeAffinity{Slug: a.Slug, Name: a.Name, Values: valueCounts(a.Values)})
	}
	return out
}

func valueCounts(rows []analytics.ValueCount) []*signalsv1.ValueCount {
	out := make([]*signalsv1.ValueCount, 0, len(rows))
	for _, v := range rows {
		out = append(out, &signalsv1.ValueCount{Value: v.Value, Count: int32(v.Count)})
	}
	return out
}

func eventCounts(rows []analytics.EventCount) []*signalsv1.EventCount {
	out := make([]*signalsv1.EventCount, 0, len(rows))
	for _, e := range rows {
		out = append(out, &signalsv1.EventCount{Name: e.Name, Count: int32(e.Count)})
	}
	return out
}

func buckets(rows []analytics.RetentionBucket) []*signalsv1.RetentionBucket {
	out := make([]*signalsv1.RetentionBucket, 0, len(rows))
	for _, b := range rows {
		out = append(out, &signalsv1.RetentionBucket{Period: int32(b.Period), Count: int32(b.Count), Eligible: int32(b.Eligible)})
	}
	return out
}
