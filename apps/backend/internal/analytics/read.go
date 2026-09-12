package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
)

type Engagement struct { // the channel-scoped marketing-email engagement summary on the Pulse dashboard
	OpensRaw    int64 // every logged open (incl. machine/proxy)
	OpensHuman  int64 // machine-flagged excluded
	Clicks      int64 // human clicks
	Conversions int64
	Revenue     []RevenueByCurrency // attributed revenue, by currency
	TopLinks    []LinkClicks
}

type RevenueByCurrency struct {
	Currency string
	Cents    int64
	Orders   int64
}

type LinkClicks struct {
	URL    string
	Clicks int64
}

func (s *pgSpine) Engagement(ctx context.Context, channelID uuid.UUID, since time.Time) (Engagement, error) { // queries the spine directly - the dashboard never scans it without these aggregates
	cid := uuid.NullUUID{UUID: channelID, Valid: true}
	sum, err := s.read().EngagementSummary(ctx, signalsdb.EngagementSummaryParams{ChannelID: cid, Since: since})
	if err != nil {
		return Engagement{}, err
	}
	out := Engagement{OpensRaw: sum.OpensRaw, OpensHuman: sum.OpensHuman, Clicks: sum.Clicks, Conversions: sum.Conversions}

	rev, err := s.read().AttributedRevenue(ctx, signalsdb.AttributedRevenueParams{ChannelID: cid, Since: since})
	if err != nil {
		return Engagement{}, err
	}
	for _, row := range rev {
		out.Revenue = append(out.Revenue, RevenueByCurrency{Currency: asString(row.Currency), Cents: row.Cents, Orders: row.Orders})
	}

	links, err := s.read().TopClickedLinks(ctx, signalsdb.TopClickedLinksParams{ChannelID: cid, Since: since})
	if err != nil {
		return Engagement{}, err
	}
	for _, row := range links {
		if u := asString(row.Url); u != "" {
			out.TopLinks = append(out.TopLinks, LinkClicks{URL: u, Clicks: row.Clicks})
		}
	}
	return out, nil
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}

// --- per-customer behavioural profile ---

type EventCount struct {
	Name  string
	Count int64
}

type ActivityItem struct {
	EventName  string
	OccurredAt time.Time
	Source     string
	Summary    string          // best-effort one-line detail (page, link, custom name, order)
	Props      json.RawMessage // the raw event properties - drives rich timeline rows
}

type Profile struct { // a customer's behavioural profile - cross-channel aggregates plus a recent timeline, stitched across their identified anon ids and any events recorded straight against their customer_id
	FirstSeen   *time.Time
	LastSeen    *time.Time
	TotalEvents int64
	PageViews   int64
	Conversions int64
	Revenue     []RevenueByCurrency
	Counts      []EventCount
	Timeline    []ActivityItem
	// Top distinct device / browser / referrer / ip the profile was seen with - which device, which IP, where they came from
	Devices          []ValueCount
	Browsers         []ValueCount
	Referrers        []ValueCount
	IPs              []ValueCount
	OSes             []ValueCount
	Screens          []ValueCount  // viewport bucket (mobile/tablet/desktop by width)
	Countries        []ValueCount  // GeoIP country (when GEOIP_DB_PATH is configured)
	Sources          []ValueCount  // utm_source on landing
	Campaigns        []ValueCount  // utm_campaign on landing
	TopPages         []PageView    // most-viewed pages (with dwell)
	PagesViewedTotal int           // distinct pages viewed (TopPages is the top slice)
	TopProducts      []ProductView // most-viewed products (with image/price)
	Searches         []ValueCount  // storefront search terms
	Sessions         int64         // distinct visits (derived: >30-min gap)
}

type ProductView struct { // a product the profile viewed, carrying the display metadata off the page_view for visual product cards
	Name  string
	Image string
	Price string
	SKU   string
	Views int
}

type PageView struct {
	Title           string
	PageKey         string
	Views           int
	AvgDwellSeconds int
}

type ValueCount struct {
	Value string
	Count int64
}

// Resolves the customer's anon ids, then rolls up every event for them since the cutoff - direct customer_id OR any stitched anon_id
func (s *pgSpine) CustomerProfile(ctx context.Context, customerID uuid.UUID, since time.Time) (Profile, error) {
	anonIDs, err := s.AnonIDsForCustomer(ctx, customerID)
	if err != nil {
		return Profile{}, err
	}
	return s.profileFor(ctx, uuid.NullUUID{UUID: customerID, Valid: true}, anonIDs, since)
}

// The not-yet-identified counterpart of CustomerProfile, rolling up one anonymous visitor id (the lens_aid cookie)
// Reuses the same reads with an invalid customer id, so the customer_id clause matches nothing and only the anon id counts
func (s *pgSpine) VisitorProfile(ctx context.Context, anonID string, since time.Time) (Profile, error) {
	if anonID == "" {
		return Profile{}, nil
	}
	return s.profileFor(ctx, uuid.NullUUID{}, []string{anonID}, since)
}

type ExperimentArm struct {
	Experiment    string
	Variation     string
	LastExposedAt time.Time
}

func (s *pgSpine) CustomerExperiments(ctx context.Context, customerID uuid.UUID, since time.Time) ([]ExperimentArm, error) { // bridges the anon-keyed $experiment exposures to the customer through identity stitching, making the A/B arm a visible Synapse trait
	anonIDs, err := s.AnonIDsForCustomer(ctx, customerID)
	if err != nil || len(anonIDs) == 0 {
		return nil, err
	}
	rows, err := s.read().CustomerExperimentArms(ctx, signalsdb.CustomerExperimentArmsParams{Since: since, AnonIds: anonIDs})
	if err != nil {
		return nil, err
	}
	out := make([]ExperimentArm, 0, len(rows))
	for _, row := range rows {
		exp, _ := row.Experiment.(string)
		v, _ := row.Variation.(string)
		at, _ := row.LastExposedAt.(time.Time)
		out = append(out, ExperimentArm{Experiment: exp, Variation: v, LastExposedAt: at})
	}
	return out, nil
}

type AffinityAttr struct { // names a Synapse-tracked attribute the caller wants affinities for - supplied by the catalog, which owns the flag
	Slug string
	Name string
}

type AttributeAffinity struct { // a profile's top values for one tracked attribute (tire size to [{205/55R16, 8}, …]) - high counts mean high intent
	Slug   string
	Name   string
	Values []ValueCount
}

// Aggregates the top values of each tracked attribute's affinity prop (attr:<slug>) over a profile's events, populated from PDP views and filter selections - empty dimensions drop out
func (s *pgSpine) affinities(ctx context.Context, cid uuid.NullUUID, anonIDs []string, since time.Time, attrs []AffinityAttr) []AttributeAffinity {
	out := make([]AttributeAffinity, 0, len(attrs))
	for _, a := range attrs {
		vals := s.topValues(ctx, "attr:"+a.Slug, cid, anonIDs, since)
		if len(vals) > 0 {
			out = append(out, AttributeAffinity{Slug: a.Slug, Name: a.Name, Values: vals})
		}
	}
	return out
}

func (s *pgSpine) CustomerAffinities(ctx context.Context, customerID uuid.UUID, since time.Time, attrs []AffinityAttr) ([]AttributeAffinity, error) {
	if len(attrs) == 0 {
		return nil, nil
	}
	anonIDs, err := s.AnonIDsForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	return s.affinities(ctx, uuid.NullUUID{UUID: customerID, Valid: true}, anonIDs, since, attrs), nil
}

func (s *pgSpine) VisitorAffinities(ctx context.Context, anonID string, since time.Time, attrs []AffinityAttr) []AttributeAffinity {
	if anonID == "" || len(attrs) == 0 {
		return nil
	}
	return s.affinities(ctx, uuid.NullUUID{}, []string{anonID}, since, attrs)
}

func (s *pgSpine) AffinityActors(ctx context.Context, channelID uuid.UUID, attrSlug, value string, since time.Time) ([]uuid.UUID, error) { // the identity-resolved customer ids showing an affinity for attrSlug=value in the window - the actor set behind an affinity segment, targetable by Pulse and Prism
	return s.read().AffinityActors(ctx, signalsdb.AffinityActorsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		Since:     since, PropKey: "attr:" + attrSlug, Value: value,
	})
}

type AffinityValueCount struct { // one observed value of a tracked attribute plus the distinct identified customers carrying it - the reach of a would-be segment
	Value  string
	Actors int
}

func (s *pgSpine) AffinityValueCounts(ctx context.Context, channelID uuid.UUID, attrSlug string, since time.Time, limit int) ([]AffinityValueCount, error) { // the most-reached values of one tracked attribute across a channel, with distinct-customer counts - the discovery feed for auto-generated affinity segments, limit caps how many come back
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.read().AffinityValueActorCounts(ctx, signalsdb.AffinityValueActorCountsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		Since:     since, PropKey: "attr:" + attrSlug, Lim: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]AffinityValueCount, 0, len(rows))
	for _, row := range rows {
		out = append(out, AffinityValueCount{Value: row.Value, Actors: int(row.Actors)})
	}
	return out, nil
}

func (s *pgSpine) CustomerTopAffinities(ctx context.Context, customerID uuid.UUID, since time.Time) (map[string]string, error) { // a customer's strongest value per tracked attribute, keyed by slug - powers {{customer.affinity.<slug>}}
	anonIDs, err := s.AnonIDsForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	rows, err := s.read().CustomerTopAffinities(ctx, signalsdb.CustomerTopAffinitiesParams{
		CustomerID: uuid.NullUUID{UUID: customerID, Valid: true}, AnonIds: anonIDs, Since: since,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		k, _ := row.AttrKey.(string)
		v, _ := row.AttrVal.(string)
		if v != "" && strings.HasPrefix(k, "attr:") {
			out[strings.TrimPrefix(k, "attr:")] = v
		}
	}
	return out, nil
}

func (s *pgSpine) topProducts(ctx context.Context, cid uuid.NullUUID, anonIDs []string, since time.Time) []ProductView { // most-viewed products with their display metadata (name, image, price, sku) for visual product cards
	rows, err := s.read().ProfileTopProducts(ctx, signalsdb.ProfileTopProductsParams{CustomerID: cid, AnonIds: anonIDs, Since: since})
	if err != nil {
		return nil
	}
	out := make([]ProductView, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProductView{
			Name: asString(row.Name), Image: asString(row.Image), Price: asString(row.Price),
			SKU: asString(row.Sku), Views: int(row.Views),
		})
	}
	return out
}

func (s *pgSpine) topPages(ctx context.Context, cid uuid.NullUUID, anonIDs []string, since time.Time) ([]PageView, int) {
	rows, err := s.read().ProfileTopPages(ctx, signalsdb.ProfileTopPagesParams{CustomerID: cid, AnonIds: anonIDs, Since: since})
	if err != nil {
		return nil, 0
	}
	out := make([]PageView, 0, len(rows))
	total := 0
	for _, row := range rows {
		total = int(row.TotalPages) // same on every row (window count)
		out = append(out, PageView{
			Title: asString(row.Title), PageKey: asString(row.PageKey),
			Views: int(row.Views), AvgDwellSeconds: int(row.AvgDwell),
		})
	}
	return out, total
}

// The shared core of CustomerProfile and VisitorProfile - rolls the spine into a Profile for an optional customer id and/or a set of anon ids
func (s *pgSpine) profileFor(ctx context.Context, cid uuid.NullUUID, anonIDs []string, since time.Time) (Profile, error) {
	ov, err := s.read().CustomerOverview(ctx, signalsdb.CustomerOverviewParams{CustomerID: cid, AnonIds: anonIDs, Since: since})
	if err != nil {
		return Profile{}, err
	}
	p := Profile{TotalEvents: ov.Total, FirstSeen: asTime(ov.FirstAt), LastSeen: asTime(ov.LastAt)}

	if counts, err := s.read().CustomerEventCounts(ctx, signalsdb.CustomerEventCountsParams{CustomerID: cid, AnonIds: anonIDs, Since: since}); err == nil {
		for _, c := range counts {
			p.Counts = append(p.Counts, EventCount{Name: c.EventName, Count: c.Count})
			switch c.EventName {
			case EventPageView:
				p.PageViews = c.Count
			case EventConversion:
				p.Conversions = c.Count
			}
		}
	}
	if conv, err := s.read().CustomerConversions(ctx, signalsdb.CustomerConversionsParams{CustomerID: cid, AnonIds: anonIDs, Since: since}); err == nil {
		for _, c := range conv {
			p.Revenue = append(p.Revenue, RevenueByCurrency{Currency: asString(c.Currency), Cents: c.Cents, Orders: c.Orders})
		}
	}
	if tl, err := s.read().CustomerTimeline(ctx, signalsdb.CustomerTimelineParams{CustomerID: cid, AnonIds: anonIDs, Since: since, Lim: 60}); err == nil {
		for _, t := range tl {
			p.Timeline = append(p.Timeline, ActivityItem{
				EventName: t.EventName, OccurredAt: t.OccurredAt, Source: t.Source,
				Summary: SummarizeProps(t.EventName, t.Properties),
				Props:   t.Properties,
			})
		}
	}
	p.Devices = s.topValues(ctx, "device", cid, anonIDs, since)
	p.Browsers = s.topValues(ctx, "client", cid, anonIDs, since)
	p.Referrers = s.topValues(ctx, "referrer", cid, anonIDs, since)
	p.IPs = s.topValues(ctx, "ip", cid, anonIDs, since)
	p.OSes = s.topValues(ctx, "os", cid, anonIDs, since)
	p.Screens = s.topValues(ctx, "viewport", cid, anonIDs, since)
	p.Countries = s.topValues(ctx, "country", cid, anonIDs, since)
	p.Sources = s.topValues(ctx, "utm_source", cid, anonIDs, since)
	p.Campaigns = s.topValues(ctx, "utm_campaign", cid, anonIDs, since)
	p.TopPages, p.PagesViewedTotal = s.topPages(ctx, cid, anonIDs, since)
	p.TopProducts = s.topProducts(ctx, cid, anonIDs, since)
	p.Searches = s.topValues(ctx, "search_query", cid, anonIDs, since)
	if n, err := s.read().ProfileSessionCount(ctx, signalsdb.ProfileSessionCountParams{CustomerID: cid, AnonIds: anonIDs, Since: since}); err == nil {
		p.Sessions = n
	}
	return p, nil
}

func (s *pgSpine) topValues(ctx context.Context, key string, cid uuid.NullUUID, anonIDs []string, since time.Time) []ValueCount { // the most frequent distinct values of one event-property key across the profile's events - the "context" dimensions
	rows, err := s.read().ProfileTopValues(ctx, signalsdb.ProfileTopValuesParams{PropKey: key, CustomerID: cid, AnonIds: anonIDs, Since: since})
	if err != nil {
		return nil
	}
	out := make([]ValueCount, 0, len(rows))
	for _, row := range rows {
		if v := asString(row.Value); v != "" {
			out = append(out, ValueCount{Value: v, Count: row.Count})
		}
	}
	return out
}

// --- Synapse: org-wide behavioural overview ---

type ActiveCustomer struct { // one row of the most-active-customers ranking, resolving direct customer events plus web events via stitched anon ids
	CustomerID uuid.UUID
	Events     int64
	LastSeen   time.Time
}

type Visitor struct { // one row of the most-active ANONYMOUS visitor ranking, not yet stitched to a customer - LastPage is the most recent title and path
	AnonID   string
	Events   int64
	LastSeen time.Time
	LastPage string
}

type SynapseOverview struct {
	TotalEvents         int64
	PageViews           int64
	Conversions         int64
	IdentifiedCustomers int64
	AnonymousVisitors   int64
	Revenue             []RevenueByCurrency
	TopEvents           []EventCount
	ActiveCustomers     []ActiveCustomer
	ActiveVisitors      []Visitor
}

// Errors propagate on purpose - swallowing a failed sub-read renders an all-zeros dashboard, which operators read as "traffic died" rather than "the read pool is down"
func (s *pgSpine) SynapseOverview(ctx context.Context, channelID uuid.UUID, since time.Time) (SynapseOverview, error) {
	cid := uuid.NullUUID{UUID: channelID, Valid: true}
	var o SynapseOverview
	t, err := s.read().SynapseTotals(ctx, signalsdb.SynapseTotalsParams{ChannelID: cid, Since: since})
	if err != nil {
		return o, fmt.Errorf("synapse overview: totals: %w", err)
	}
	o.TotalEvents, o.PageViews, o.Conversions = t.TotalEvents, t.PageViews, t.Conversions
	n, err := s.read().SynapseIdentifiedCount(ctx, cid)
	if err != nil {
		return o, fmt.Errorf("synapse overview: identified: %w", err)
	}
	o.IdentifiedCustomers = n
	ev, err := s.read().SynapseEventCounts(ctx, signalsdb.SynapseEventCountsParams{ChannelID: cid, Since: since})
	if err != nil {
		return o, fmt.Errorf("synapse overview: event counts: %w", err)
	}
	for _, e := range ev {
		o.TopEvents = append(o.TopEvents, EventCount{Name: e.EventName, Count: e.Count})
	}
	rev, err := s.read().AttributedRevenue(ctx, signalsdb.AttributedRevenueParams{ChannelID: cid, Since: since})
	if err != nil {
		return o, fmt.Errorf("synapse overview: revenue: %w", err)
	}
	for _, rv := range rev {
		o.Revenue = append(o.Revenue, RevenueByCurrency{Currency: asString(rv.Currency), Cents: rv.Cents, Orders: rv.Orders})
	}
	ac, err := s.read().SynapseActiveCustomers(ctx, signalsdb.SynapseActiveCustomersParams{ChannelID: cid, Since: since, Lim: 15})
	if err != nil {
		return o, fmt.Errorf("synapse overview: active customers: %w", err)
	}
	for _, c := range ac {
		o.ActiveCustomers = append(o.ActiveCustomers, ActiveCustomer{CustomerID: c.CustomerID, Events: c.Events, LastSeen: c.LastSeen})
	}
	anon, err := s.read().SynapseAnonCount(ctx, signalsdb.SynapseAnonCountParams{ChannelID: cid, Since: since})
	if err != nil {
		return o, fmt.Errorf("synapse overview: anonymous count: %w", err)
	}
	o.AnonymousVisitors = anon
	av, err := s.read().SynapseActiveVisitors(ctx, signalsdb.SynapseActiveVisitorsParams{ChannelID: cid, Since: since, Lim: 15})
	if err != nil {
		return o, fmt.Errorf("synapse overview: active visitors: %w", err)
	}
	for _, v := range av {
		o.ActiveVisitors = append(o.ActiveVisitors, Visitor{
			AnonID: v.AnonID.String, Events: v.Events, LastSeen: v.LastSeen, LastPage: asString(v.LastPage),
		})
	}
	return o, nil
}

type FunnelStep struct { // one step of a funnel result - the step's event and how many people reached it, in order from step 1
	EventName string
	Count     int64
}

// How many distinct people performed steps[0], then steps[1] after it, in chronological order within the window
// Greedy earliest-match per person, the standard funnel semantics, with people resolved across identified and anonymous activity
func (s *pgSpine) Funnel(ctx context.Context, channelID uuid.UUID, steps []string, since time.Time) ([]FunnelStep, error) {
	out := make([]FunnelStep, len(steps))
	for i, step := range steps {
		out[i] = FunnelStep{EventName: step}
	}
	if len(steps) < 2 {
		return out, nil
	}
	rows, err := s.read().FunnelEvents(ctx, signalsdb.FunnelEventsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		Since:     since,
		Steps:     steps,
		FirstStep: steps[0], // only actors who entered the funnel are streamed
	})
	if err != nil {
		return out, err
	}
	counts := make([]int64, len(steps))
	curActor := ""
	idx := 0
	have := false
	flush := func() {
		for k := 0; k < idx; k++ {
			counts[k]++
		}
	}
	for _, row := range rows {
		actor := row.Actor.String
		if !have || actor != curActor {
			if have {
				flush()
			}
			curActor, idx, have = actor, 0, true
		}
		if idx < len(steps) && row.EventName == steps[idx] {
			idx++
		}
	}
	if have {
		flush()
	}
	for k := range steps {
		out[k].Count = counts[k]
	}
	return out, nil
}

type InsightPoint struct { // one (time bucket, breakdown series, value) cell of an Insights report - InsightSeriesTotal is a series plus its ranked window total
	Bucket time.Time
	Key    string
	Value  int64
}
type InsightSeriesTotal struct {
	Key   string
	Total int64
}
type Insights struct {
	Points []InsightPoint
	Series []InsightSeriesTotal
	// Totals and uniques for the current window and the prior equal-length one, for the headline and its period-over-period delta - a true COUNT(DISTINCT) rather than summed buckets, so uniques are exact
	Total       int64
	Uniques     int64
	PrevTotal   int64
	PrevUniques int64
}

const insightsTopSeries = 8

// The ad-hoc breakdown report - an event's measure ("total" events or "unique" people) over day or week buckets, broken down by a property key ("" for none), with values beyond insightsTopSeries folded into "(other)"
// For the unique measure each (bucket, series) value is exact, but cross-series and cross-bucket sums (the "(other)" fold, series totals) can over-count a person seen in several - acceptable for an at-a-glance breakdown
func (s *pgSpine) Insights(ctx context.Context, channelID uuid.UUID, event, breakdown, measure, unit string, since, now time.Time) (Insights, error) {
	u := "day"
	if unit == "week" {
		u = "week"
	}
	cid := uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}
	rows, err := s.read().InsightsSeries(ctx, signalsdb.InsightsSeriesParams{
		Unit: u, PropKey: breakdown, ChannelID: cid, Since: since, EventName: event,
	})
	if err != nil {
		return Insights{}, err
	}
	in := make([]InsightsSeriesInput, 0, len(rows))
	for _, r := range rows {
		in = append(in, InsightsSeriesInput{Bucket: r.Bucket, Breakdown: asString(r.Breakdown), Total: r.Total, Uniques: r.Uniques})
	}
	var out Insights
	out.Points, out.Series = RollupInsightsSeries(in, measure == "unique")

	// Exact window-level measures for the headline, plus the prior equal-length window for the delta
	if cur, err := s.read().EventWindowMeasure(ctx, signalsdb.EventWindowMeasureParams{ChannelID: cid, StartAt: since, EndAt: now, EventName: event}); err == nil {
		out.Total, out.Uniques = cur.Total, cur.Uniques
	}
	prevStart := since.Add(-now.Sub(since))
	if prev, err := s.read().EventWindowMeasure(ctx, signalsdb.EventWindowMeasureParams{ChannelID: cid, StartAt: prevStart, EndAt: since, EventName: event}); err == nil {
		out.PrevTotal, out.PrevUniques = prev.Total, prev.Uniques
	}
	return out, nil
}

func topKeys(totals map[string]int64, n int) map[string]bool {
	type kv struct {
		k string
		v int64
	}
	all := make([]kv, 0, len(totals))
	for k, v := range totals {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(a, b int) bool {
		if all[a].v != all[b].v {
			return all[a].v > all[b].v
		}
		return all[a].k < all[b].k
	})
	keep := map[string]bool{}
	for i, e := range all {
		if i >= n {
			break
		}
		keep[e.k] = true
	}
	return keep
}

type ActivePoint struct {
	Day   time.Time
	Count int64
}

// Channel engagement reframed for commerce around a return window rather than the daily DAU/MAU habit ratio - of the people active in the previous window, how many came back in this one
// WindowDays is the store's return cadence; the daily-active series and WAU/MAU are secondary context
type Stickiness struct {
	Days        []ActivePoint
	WAU         int64
	MAU         int64
	WindowDays  int
	PriorActive int64   // distinct active in the previous window
	ActiveNow   int64   // distinct active in the current window
	Returning   int64   // active in both windows
	ReturnRate  float64 // returning / priorActive * 100
}

// The DAU series spans since to now while WAU/MAU are fixed trailing 7- and 30-day distinct-active counts, with people resolved across anonymous and identified activity
func (s *pgSpine) Stickiness(ctx context.Context, channelID uuid.UUID, event string, since, now time.Time, windowDays int) (Stickiness, error) {
	cid := uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}
	if windowDays < 1 {
		windowDays = 30
	}
	out := Stickiness{WindowDays: windowDays}

	days, err := s.read().ActiveByDay(ctx, signalsdb.ActiveByDayParams{ChannelID: cid, Since: since, EventName: event})
	if err != nil {
		return out, err
	}
	for _, d := range days {
		out.Days = append(out.Days, ActivePoint{Day: d.Day, Count: d.Count})
	}
	if n, err := s.read().ActiveDistinct(ctx, signalsdb.ActiveDistinctParams{ChannelID: cid, Since: now.AddDate(0, 0, -7), EventName: event}); err == nil {
		out.WAU = n
	}
	if n, err := s.read().ActiveDistinct(ctx, signalsdb.ActiveDistinctParams{ChannelID: cid, Since: now.AddDate(0, 0, -30), EventName: event}); err == nil {
		out.MAU = n
	}

	// Return window - prior is [now-2W, now-W), current is [now-W, now)
	rw, err := s.read().ReturnWindowStats(ctx, signalsdb.ReturnWindowStatsParams{
		ChannelID: cid, EventName: event,
		PrevStart: now.AddDate(0, 0, -2*windowDays),
		CurStart:  now.AddDate(0, 0, -windowDays),
		CurEnd:    now,
	})
	if err != nil {
		return out, err
	}
	out.PriorActive, out.ActiveNow, out.Returning = rw.PriorActive, rw.ActiveNow, rw.Returning
	if rw.PriorActive > 0 {
		out.ReturnRate = float64(rw.Returning) / float64(rw.PriorActive) * 100
	}
	return out, nil
}

type FlowNode struct { // one node of a flow path tree - the event reached at this depth, its full path id for tree linkage, its parent's id, and how many people took it
	ID       string // path key (focus + each step)
	ParentID string
	Event    string
	Depth    int
	Count    int64
}

const flowSep = "\x1f"

type FlowPath struct { // one complete journey people took after the focus event, the collapsed event sequence with focus excluded, and how many took it exactly
	Events []string
	Count  int64
}

type FlowResult struct { // the cohort, the drill-down edge tree, and the headline comparisons - continued versus dropped after the focus event, average journey depth among those who continued, and the most common complete paths
	Cohort    int64
	Nodes     []FlowNode
	Continued int64 // people who took >= 1 tracked step after the focus event
	AvgSteps  float64
	TopPaths  []FlowPath
}

// A forward path tree from a focus event: per person, from their first focus event, the next events with consecutive duplicates collapsed, up to depth, aggregated into parent-to-child edge counts and pruned to the top topK children per node
// cohort is everyone who did the focus event, people are resolved across anonymous and identified activity, and it reuses FlowEvents
func (s *pgSpine) Flow(ctx context.Context, channelID uuid.UUID, focus string, since time.Time, depth, topK int) (FlowResult, error) {
	if depth < 1 {
		depth = 1
	}
	if topK < 1 {
		topK = 1
	}
	rows, err := s.read().FlowEvents(ctx, signalsdb.FlowEventsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		Since:     since,
		Focus:     focus, // only focus-event actors' rows are streamed
	})
	if err != nil {
		return FlowResult{}, err
	}
	events := make([]SpineActorEvent, 0, len(rows))
	for _, r := range rows {
		ev, _ := r.EventName.(string) // CASE expr; always a string
		events = append(events, SpineActorEvent{Actor: r.Actor.String, EventName: ev, OccurredAt: r.OccurredAt})
	}
	return FlowFromEvents(events, focus, depth, topK), nil
}

type SegmentCondition struct { // one behavioral criterion - did, or when Negate did NOT, perform Event at least MinCount times within the last Days
	Event    string
	Negate   bool
	MinCount int
	Days     int
}

func (s *pgSpine) EventActors(ctx context.Context, channelID uuid.UUID, event string, minCount, days int, now time.Time) ([]uuid.UUID, error) { // one positive "did X" leaf of a segment definition tree - unlike SegmentActors it imposes no "needs a positive anchor" rule, so the tree evaluator can combine it freely with AND/OR/NOT
	if minCount < 1 {
		minCount = 1
	}
	if days <= 0 {
		days = 30
	}
	return s.read().SegmentEventActors(ctx, signalsdb.SegmentEventActorsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		EventName: event, Since: now.AddDate(0, 0, -days), MinCount: int64(minCount),
	})
}

func (r *Recorder) ChannelCustomerIDs(ctx context.Context, channelID uuid.UUID) ([]uuid.UUID, error) { // the universe a segment-definition tree draws from, needed to evaluate negated or standalone-OR nodes, where NOT X is the universe minus X
	return r.read().ChannelCustomerIDs(ctx, channelID)
}

// Distinct non-cancelled orders and total spend in cents for the detail page, off the spine projection
func (r *Recorder) SegmentMemberStats(ctx context.Context, segmentID uuid.UUID) (orders int64, revenueCents int64, err error) {
	ids, err := r.qs.SegmentMemberCustomerIDs(ctx, segmentID)
	if err != nil || len(ids) == 0 {
		return 0, 0, err
	}
	row, err := r.qs.SegmentOrderRollup(ctx, ids)
	if err != nil {
		return 0, 0, err
	}
	return row.Orders, row.RevenueCents, nil
}

type SegmentMemberRow struct { // one segment member with traits and commerce for the detail page's table - LastOrderAt is the zero time when they have no orders
	CustomerID      uuid.UUID
	Lifecycle       string
	RecencyDays     int
	EngagementScore int
	Orders          int
	SpendCents      int64
	LastOrderAt     time.Time
}

func (r *Recorder) SegmentMembers(ctx context.Context, segmentID uuid.UUID, limit int) ([]SegmentMemberRow, error) { // richest-spender first - names and emails are resolved by the caller through the customer service
	if limit <= 0 {
		limit = 200
	}
	ids, err := r.qs.SegmentMemberCustomerIDs(ctx, segmentID)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	// Orders and traits come off the same recompute, so the two halves of a row agree
	stats, err := r.qs.SegmentMemberOrderStats(ctx, ids)
	if err != nil {
		return nil, err
	}
	traits, err := r.qs.TraitsForCustomers(ctx, ids)
	if err != nil {
		return nil, err
	}
	byTraits := make(map[uuid.UUID]signalsdb.TraitsForCustomersRow, len(traits))
	for _, t := range traits {
		byTraits[t.CustomerID] = t
	}
	out := make([]SegmentMemberRow, 0, len(ids))
	for _, id := range ids {
		row := SegmentMemberRow{CustomerID: id, RecencyDays: -1}
		if t, ok := byTraits[id]; ok {
			row.Lifecycle, row.RecencyDays, row.EngagementScore = t.LifecycleStage, int(t.RecencyDays), int(t.EngagementScore)
		}
		out = append(out, row)
	}
	byID := make(map[uuid.UUID]int, len(out))
	for i, m := range out {
		byID[m.CustomerID] = i
	}
	for _, st := range stats {
		if i, ok := byID[st.CustomerID]; ok {
			out[i].Orders, out[i].SpendCents = int(st.Orders), st.SpendCents
			if st.LastOrderAt.Valid { // the projection holds a row per customer, so one who has never ordered has no date rather than no row
				out[i].LastOrderAt = st.LastOrderAt.Time
			}
		}
	}
	// The ordering the single query used to do, now that both halves are in hand
	sort.Slice(out, func(i, j int) bool {
		if out[i].SpendCents != out[j].SpendCents {
			return out[i].SpendCents > out[j].SpendCents
		}
		return out[i].CustomerID.String() < out[j].CustomerID.String()
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// SegmentStageCount / SegmentRfmBucket / SegmentAffinityCount feed the segment detail page's composition charts
type SegmentStageCount struct {
	Stage string
	Count int
}
type SegmentRfmBucket struct {
	Dimension string
	Score     int
	Count     int
}
type SegmentAffinityCount struct {
	Attr    string
	Value   string
	Members int
}

func (r *Recorder) SegmentLifecycleMix(ctx context.Context, segmentID uuid.UUID) ([]SegmentStageCount, error) {
	ids, err := r.qs.SegmentMemberCustomerIDs(ctx, segmentID)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	rows, err := r.qs.LifecycleMixForCustomers(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]SegmentStageCount, 0, len(rows))
	for _, m := range rows {
		out = append(out, SegmentStageCount{Stage: m.Stage, Count: int(m.Count)})
	}
	return out, nil
}

func (r *Recorder) SegmentRfmDistribution(ctx context.Context, segmentID uuid.UUID) ([]SegmentRfmBucket, error) {
	ids, err := r.qs.SegmentMemberCustomerIDs(ctx, segmentID)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	rows, err := r.qs.RfmDistributionForCustomers(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]SegmentRfmBucket, 0, len(rows))
	for _, m := range rows {
		out = append(out, SegmentRfmBucket{Dimension: m.Dimension, Score: int(m.Score), Count: int(m.Count)})
	}
	return out, nil
}

func (r *Recorder) SegmentTopAffinities(ctx context.Context, segmentID, channelID uuid.UUID, since time.Time, limit int) ([]SegmentAffinityCount, error) { // the top attribute-affinity values among a segment's members by distinct-member count, slug stripped of its "attr:" prefix - channel and window bound the scan, since the query jsonb-expands every row it touches
	if limit <= 0 {
		limit = 12
	}
	// The spine moved to the signals db, so the membership join is now app-side - commerce answers who's in the segment, and those member ids feed the signals scan for their affinity props
	memberIDs, err := r.qs.SegmentMemberIDs(ctx, segmentID) // commerce: who's in the segment
	if err != nil {
		return nil, err
	}
	if len(memberIDs) == 0 {
		return nil, nil
	}
	return r.spine.SegmentAffinitiesForMembers(ctx, channelID, memberIDs, since, limit)
}

type SegmentGrowthPoint struct {
	Day   time.Time
	Count int
}

func (r *Recorder) SegmentGrowth(ctx context.Context, segmentID uuid.UUID, days int) ([]SegmentGrowthPoint, error) { // a segment's daily member-count series over the last `days` - accrues forward from when the segment first materialised
	if days <= 0 {
		days = 90
	}
	rows, err := r.qs.SegmentGrowth(ctx, signalsdb.SegmentGrowthParams{SegmentID: segmentID, Days: int32(days)})
	if err != nil {
		return nil, err
	}
	out := make([]SegmentGrowthPoint, 0, len(rows))
	for _, g := range rows {
		out = append(out, SegmentGrowthPoint{Day: g.Day, Count: int(g.MemberCount)})
	}
	return out, nil
}

// Evaluates a behavioral segment definition - the intersection of all positive conditions, minus anyone matched by a negated one
// Requires at least one positive condition, since a pure "did not" segment is the whole customer base and v1 doesn't materialize that - Each condition resolves to a customer, so anonymous-only people can't be members
func (s *pgSpine) SegmentActors(ctx context.Context, channelID uuid.UUID, conds []SegmentCondition, now time.Time) ([]uuid.UUID, error) {
	cid := uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}
	actors := func(c SegmentCondition) ([]uuid.UUID, error) {
		minCount := max(c.MinCount, 1)
		days := c.Days
		if days <= 0 {
			days = 30
		}
		return s.read().SegmentEventActors(ctx, signalsdb.SegmentEventActorsParams{
			ChannelID: cid, EventName: c.Event, Since: now.AddDate(0, 0, -days), MinCount: int64(minCount),
		})
	}

	var result map[uuid.UUID]bool
	positives := 0
	for _, c := range conds {
		if c.Negate {
			continue
		}
		ids, err := actors(c)
		if err != nil {
			return nil, err
		}
		set := make(map[uuid.UUID]bool, len(ids))
		for _, id := range ids {
			set[id] = true
		}
		if positives == 0 {
			result = set
		} else {
			for id := range result {
				if !set[id] {
					delete(result, id)
				}
			}
		}
		positives++
	}
	if positives == 0 {
		return nil, fmt.Errorf("a segment needs at least one positive (did) condition")
	}
	for _, c := range conds {
		if !c.Negate {
			continue
		}
		ids, err := actors(c)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			delete(result, id)
		}
	}
	out := make([]uuid.UUID, 0, len(result))
	for id := range result {
		out = append(out, id)
	}
	return out, nil
}

type RetentionBucket struct { // one period offset of a retention curve - how many cohort members came back in period N out of how many were eligible, meaning enough time had elapsed since their start event - retention% is count/eligible
	Period   int
	Count    int64
	Eligible int64
}

type RetentionCohort struct { // one start-period cohort's row in the retention grid - the people who first did the start event within that bucket, and how many returned in each later period
	StartAt time.Time
	Size    int64
	Buckets []RetentionBucket
}

type Retention struct {
	Cohort  int64             // total people across all cohorts
	Buckets []RetentionBucket // overall (summed) curve - the blended view
	Cohorts []RetentionCohort // per-start-period rows (the cohort grid)
}

// Classic cohort retention - of the people who did startEvent in the window, how many did returnEvent in each period offset, where period 0 is the same period as their first start event
// Each bucket carries its own eligible denominator, a member only counting toward period N once now - anchor >= N*unit, so immature cohorts don't deflate later periods - Reuses FunnelEvents
func (s *pgSpine) Retention(ctx context.Context, channelID uuid.UUID, startEvent, returnEvent string, since, now time.Time, periods int, unit time.Duration) (Retention, error) {
	out := Retention{Buckets: make([]RetentionBucket, periods)}
	for i := range out.Buckets {
		out.Buckets[i].Period = i
	}
	if periods <= 0 || unit <= 0 {
		return out, nil
	}
	steps := []string{startEvent}
	if returnEvent != startEvent {
		steps = append(steps, returnEvent)
	}
	rows, err := s.read().FunnelEvents(ctx, signalsdb.FunnelEventsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		Since:     since,
		Steps:     steps,
		FirstStep: startEvent, // only actors who started a cohort are streamed
	})
	if err != nil {
		return out, err
	}
	events := make([]SpineActorEvent, 0, len(rows))
	for _, r := range rows {
		events = append(events, SpineActorEvent{Actor: r.Actor.String, EventName: r.EventName, OccurredAt: r.OccurredAt})
	}
	return RetentionFromEvents(events, startEvent, returnEvent, now, periods, unit), nil
}

func asTime(v any) *time.Time {
	if t, ok := v.(time.Time); ok {
		return &t
	}
	return nil
}

type EmailEngagement struct { // a customer's lifetime Pulse email engagement, read from the authoritative email_sends projection where opens and clicks are bot-filtered at write time
	Sent       int
	Opened     int
	Clicked    int
	LastSentAt *time.Time
	LastOpenAt *time.Time
}

func (r *Recorder) CustomerEmailEngagement(ctx context.Context, customerID uuid.UUID) (EmailEngagement, error) { // every email sent to a customer plus distinct opens and clicks - zero-valued with no error when they've never been sent one
	row, err := r.read().CustomerEmailEngagement(ctx, uuid.NullUUID{UUID: customerID, Valid: true})
	if err != nil {
		return EmailEngagement{}, err
	}
	return EmailEngagement{
		Sent:       int(row.Sent),
		Opened:     int(row.Opened),
		Clicked:    int(row.Clicked),
		LastSentAt: asTime(row.LastSentAt),
		LastOpenAt: asTime(row.LastOpenAt),
	}, nil
}

// Pulls the single most descriptive value out of an event's properties for the timeline line
// Event-aware: a page view shows the page TITLE (the product, category or search name) rather than the raw path, a filter event shows the chosen facets, commerce events show order/sku/status
func SummarizeProps(eventName string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	var prefer []string
	switch eventName {
	case EventPageView:
		// title is the page's <title>, which is the product name on a PDP and the category, collection or search name on a listing - falls back to path
		prefer = []string{"title", "page_key", "url"}
	case EventProductFilter:
		prefer = []string{"filters"}
	default:
		prefer = []string{"order_number", "sku", "status", "action", "changed_fields", "return_id", "name", "url", "order_id"}
	}
	for _, k := range prefer {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
