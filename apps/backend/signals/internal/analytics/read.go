package analytics

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
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

type ExperimentArm struct {
	Experiment    string
	Variation     string
	LastExposedAt time.Time
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

type AffinityValueCount struct { // one observed value of a tracked attribute plus the distinct identified customers carrying it - the reach of a would-be segment
	Value  string
	Actors int
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

type FunnelStep struct { // one step of a funnel result - the step's event and how many people reached it, in order from step 1
	EventName string
	Count     int64
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
	Aggregate   int64 // of Total, the events counted as nobody - visitors who hadn't allowed analytics
}

const insightsTopSeries = 8

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

type SegmentCondition struct { // one behavioral criterion - did, or when Negate did NOT, perform Event at least MinCount times within the last Days
	Event    string
	Negate   bool
	MinCount int
	Days     int
}

func (r *Recorder) ChannelCustomerIDs(ctx context.Context, channelID uuid.UUID) ([]uuid.UUID, error) { // the universe a segment-definition tree draws from, needed to evaluate negated or standalone-OR nodes, where NOT X is the universe minus X
	return r.qs.ChannelRosterCustomerIDs(ctx, channelID)
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
	// Ordered here, since neither half can order across the other
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
	// The membership join is app-side, since the two halves sit in different databases - commerce answers who is in the segment, and those ids feed the spine scan for their affinity props
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
