package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
)

// The pluggable engine behind the event spine, defined at the level of DOMAIN RESULTS rather than raw rows, so each driver computes however its engine does best - the Postgres driver streams events and walks them in Go where the ClickHouse one calls native windowFunnel, retention and uniq, and the abstraction being the Go method boundary rather than shared SQL means no lowest-common-denominator penalty
// It grows a slice at a time as the Recorder's methods move behind it - commerce-coupled reads (segment membership, email_sends attribution, trait projection) stay on the Recorder, which orchestrates the SpineStore plus the commerce db
type SpineStore interface {
	// Appends one event, idempotent on DedupeKey - returns whether a genuinely new row was written
	AppendEvent(ctx context.Context, e SpineEvent) (bool, error)
	PruneEvents(ctx context.Context, before time.Time) (int64, error)
	// Source-scoped retention - Lens prunes web events on a shorter window than the global sweep, and that window must hold on whichever engine stores them
	PruneEventsBySource(ctx context.Context, source string, before time.Time) (int64, error)
	// Archive retention: export each aged whole day through open (source '' = every source), deleting a day only after its upload commits - ClickHouse turns the global sweep into partition drops, Postgres into day-range deletes
	ArchiveEvents(ctx context.Context, source string, before time.Time, open ArchiveOpen) (int64, error)
	// Batch ingest - one statement (Postgres), buffer feed (ClickHouse) or async publish window (queue) per batch, and every decorator must override this alongside AppendEvent or writes bypass it
	AppendEvents(ctx context.Context, events []SpineEvent) error
	// Stitches an anonymous visitor to a customer, last-write-wins
	UpsertIdentity(ctx context.Context, anonID string, customerID, channelID uuid.UUID) error
	AnonIDsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error)
	// GDPR erasure - a leftover mapping would keep re-linking web events to the scrubbed customer, so it dies on whichever engine holds it - batched because a ClickHouse delete is a mutation and erasures arrive in bursts
	DeleteCustomerIdentities(ctx context.Context, customerIDs []uuid.UUID) error
	// Retention twin for the mapping table - a stitch shouldn't outlive its last confirmation by more than the events it stitched
	PruneIdentities(ctx context.Context, before time.Time) (int64, error)
	// Health panel: newest says whether ingest is flowing, oldest whether retention is
	Stats(ctx context.Context) (SpineStats, error)
	// Bulk resolution for batch scans (browse abandonment) - unstitched ids are absent from the map
	CustomersForAnonIDs(ctx context.Context, anonIDs []string) (map[string]uuid.UUID, error)

	// Event-analytics reads - each returns a domain result, with the Postgres driver streaming events and computing in Go where a ClickHouse driver would use windowFunnel, retention or sequenceMatch natively - People resolve across anonymous and identified activity

	Funnel(ctx context.Context, channelID uuid.UUID, steps []string, since time.Time) ([]FunnelStep, error)
	Insights(ctx context.Context, channelID uuid.UUID, event, breakdown, measure, unit string, since, now time.Time) (Insights, error)
	Stickiness(ctx context.Context, channelID uuid.UUID, event string, since, now time.Time, windowDays int) (Stickiness, error)
	Flow(ctx context.Context, channelID uuid.UUID, focus string, since time.Time, depth, topK int) (FlowResult, error)
	Retention(ctx context.Context, channelID uuid.UUID, startEvent, returnEvent string, since, now time.Time, periods int, unit time.Duration) (Retention, error)

	// Pulse engagement, the Synapse overview, per-profile and affinity reads, and segment actor lists - all pure-spine, since identity resolution lives there too - Commerce-coupled reads stay on the Recorder
	Engagement(ctx context.Context, channelID uuid.UUID, since time.Time) (Engagement, error)
	SynapseOverview(ctx context.Context, channelID uuid.UUID, since time.Time) (SynapseOverview, error)
	CustomerProfile(ctx context.Context, customerID uuid.UUID, since time.Time) (Profile, error)
	VisitorProfile(ctx context.Context, anonID string, since time.Time) (Profile, error)
	CustomerExperiments(ctx context.Context, customerID uuid.UUID, since time.Time) ([]ExperimentArm, error)
	CustomerAffinities(ctx context.Context, customerID uuid.UUID, since time.Time, attrs []AffinityAttr) ([]AttributeAffinity, error)
	VisitorAffinities(ctx context.Context, anonID string, since time.Time, attrs []AffinityAttr) []AttributeAffinity
	AffinityActors(ctx context.Context, channelID uuid.UUID, attrSlug, value string, since time.Time) ([]uuid.UUID, error)
	AffinityValueCounts(ctx context.Context, channelID uuid.UUID, attrSlug string, since time.Time, limit int) ([]AffinityValueCount, error)
	CustomerTopAffinities(ctx context.Context, customerID uuid.UUID, since time.Time) (map[string]string, error)
	EventActors(ctx context.Context, channelID uuid.UUID, event string, minCount, days int, now time.Time) ([]uuid.UUID, error)
	SegmentActors(ctx context.Context, channelID uuid.UUID, conds []SegmentCondition, now time.Time) ([]uuid.UUID, error)

	// Lens web-behaviour reads over the spine - Lens keeps its own heatmap/session tables on Postgres, only these raw-event reads are engine-pluggable
	LensPageTotals(ctx context.Context, channelID uuid.UUID, pageKey string) (views, visitors int64, err error)
	LensPageReferrers(ctx context.Context, channelID uuid.UUID, pageKey string) ([]ValueCount, error)
	LensPageClients(ctx context.Context, channelID uuid.UUID, pageKey string) ([]ValueCount, error)
	LensPageDeviceSplit(ctx context.Context, channelID uuid.UUID, pageKey string) ([]ValueCount, error)
	LensPageTimeline(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]LensDayCount, error)
	LensListPages(ctx context.Context, channelID uuid.UUID) ([]ValueCount, error)
	LensTopRageElements(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]LensRageElement, error)
	LensTopErrors(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]LensErrorStat, error)
	LensVitalsByPage(ctx context.Context, channelID uuid.UUID, since time.Time) ([]LensVital, error)
	LensVitalsOverall(ctx context.Context, channelID uuid.UUID, since time.Time) ([]LensVital, error)
	LensSessionSignals(ctx context.Context, channelID uuid.UUID, sessionID string, from, to time.Time) ([]LensSessionSignal, error)
	// The ClickHouse heatmap path - grid increments live as raw lens_cell/lens_element events there (hot-row upserts don't scale), aggregated at read time
	LensCellsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport, state, kind string) ([]LensCellCount, error)
	LensElementsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport string, limit int32) ([]LensElementCount, error)
	LensPageGridFromEvents(ctx context.Context, channelID uuid.UUID, pageKey string) (LensGridTotals, error)

	// Frontend A/B: pickable goal-event names, and per-variation exposed/converted/revenue for one experiment
	ExperimentGoalEvents(ctx context.Context, channelID uuid.UUID, since time.Time) ([]string, error)
	ExperimentResults(ctx context.Context, channelID uuid.UUID, key, goalEvent string, since time.Time) ([]VariationResult, error)

	// A keyset page of raw events for outbound forwarding, ordered by (created_at, id) - the destination service pages through this with its stored cursor
	EventsForForwarding(ctx context.Context, channelID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]ForwardEvent, error)

	// Spine primitives for commerce-orchestrated reads - the Recorder joins these with commerce data for segment membership, email_sends and trait projection
	SegmentAffinitiesForMembers(ctx context.Context, channelID uuid.UUID, memberIDs []uuid.UUID, since time.Time, limit int) ([]SegmentAffinityCount, error)
	RecentEngagementEvents(ctx context.Context, customerID uuid.UUID, since time.Time) ([]EngagementCandidate, error)
	ChannelEngagementCounts(ctx context.Context, channelID uuid.UUID, since time.Time) ([]CustomerEngagement, error)
	// The incremental-refresh trio: who changed in an occurred_at slice (aged-out edge), who ARRIVED in an ingest-time slice (fresh edge keys on arrival time, so queue-backlog drains and outbox redeliveries whose occurred_at predates the window still surface), then exact counts for just those ids
	ActiveCustomerIDs(ctx context.Context, channelID uuid.UUID, from, to time.Time) ([]uuid.UUID, error)
	IngestedCustomerIDs(ctx context.Context, channelID uuid.UUID, from, to time.Time) ([]uuid.UUID, error)
	ChannelEngagementCountsFor(ctx context.Context, channelID uuid.UUID, since time.Time, customerIDs []uuid.UUID) ([]CustomerEngagement, error)

	// Lifetime order aggregates per customer, netted against cancellations - the RFM inputs, so trait compute reads the spine rather than joining the orders table
	ChannelOrderAggregates(ctx context.Context, channelID uuid.UUID) ([]CustomerOrders, error)
}

// Lens raw-event read results - the shapes the Lens service folds into its own view models
type (
	LensDayCount struct { // YYYY-MM-DD bucket for a page sparkline
		Date  string
		Views int64
	}
	LensRageElement struct {
		Selector string
		Text     string
		Count    int64
	}
	LensErrorStat struct {
		Message  string
		Count    int64
		Pages    int64
		LastSeen time.Time
	}
	LensVital struct { // PageKey is empty for the site-wide rollup
		PageKey string
		Metric  string
		P75     float64
		Samples int64
	}
	LensSessionSignal struct {
		Name       string
		Message    string
		Metric     string
		Value      string
		OccurredAt time.Time
	}
)

// One arm of an A/B experiment - Exposed counts distinct exposed visitors, Converted those who then hit the goal
type VariationResult struct {
	Variation string
	Exposed   int
	Converted int
	Revenue   int64
}

// One raw spine event on its way to an external destination - CreatedAt is the ingest order the forwarding cursor advances through, which is not OccurredAt
type ForwardEvent struct {
	ID         uuid.UUID
	Name       string
	OccurredAt time.Time
	CustomerID *uuid.UUID
	AnonID     string
	Source     string
	Properties json.RawMessage
	CreatedAt  time.Time
}

// One of a customer's recent marketing opens or clicks, best-first by click over open then recency, for conversion attribution
type EngagementCandidate struct {
	EventName  string
	OccurredAt time.Time
	SendID     uuid.UUID
	// The send's origin, written onto the event so attribution needs no second lookup
	Source     string
	CampaignID string
	FlowID     string
}

// A customer's windowed spine event count, for the trait projection
type CustomerEngagement struct {
	CustomerID uuid.UUID
	Count      int32
}

// A customer's lifetime order history as the spine knows it, for the RFM projection - cancelled orders are already excluded
type CustomerOrders struct {
	CustomerID    uuid.UUID
	OrderCount    int32
	MonetaryCents int64
	FirstOrderAt  time.Time
	LastOrderAt   time.Time
}

// One event to append in engine-neutral form - the driver owns how it's stored, a jsonb row on Postgres or columns on ClickHouse
type SpineEvent struct {
	EventName  string
	OccurredAt time.Time // already resolved by the caller (orNow)
	ChannelID  *uuid.UUID
	CustomerID *uuid.UUID
	AnonID     string
	Source     string
	SendID     *uuid.UUID
	Props      map[string]any // the driver marshals
	DedupeKey  string
}

// The Postgres (signalsdb) implementation and default engine - w is the write pool, r an optional read pool for aggregations
type pgSpine struct {
	w *signalsdb.Queries
	r *signalsdb.Queries
}

func newPGSpine(write, read *signalsdb.Queries) *pgSpine { return &pgSpine{w: write, r: read} } // builds the Postgres spine driver - read may be nil, falling back to w

var _ SpineStore = (*pgSpine)(nil) // pgSpine must satisfy the engine contract, as would any future ClickHouse driver

func (s *pgSpine) read() *signalsdb.Queries { // the read pool when configured, else the write pool
	if s.r != nil {
		return s.r
	}
	return s.w
}

func (s *pgSpine) AppendEvent(ctx context.Context, e SpineEvent) (bool, error) {
	raw, err := json.Marshal(e.Props)
	if err != nil {
		return false, err
	}
	n, err := s.w.RecordAnalyticsEvent(ctx, signalsdb.RecordAnalyticsEventParams{
		EventName:  e.EventName,
		OccurredAt: e.OccurredAt,
		ChannelID:  nu(e.ChannelID),
		CustomerID: nu(e.CustomerID),
		AnonID:     sql.NullString{String: e.AnonID, Valid: e.AnonID != ""},
		Source:     e.Source,
		SendID:     nu(e.SendID),
		Properties: raw,
		DedupeKey:  e.DedupeKey,
	})
	return n > 0 && err == nil, err
}

// Lands the whole batch as one statement - the queue consumer's drain rate was one round-trip per event, which a backlog could outrun
func (s *pgSpine) AppendEvents(ctx context.Context, events []SpineEvent) error {
	if len(events) == 0 {
		return nil
	}
	p := signalsdb.RecordAnalyticsEventsBatchParams{
		EventNames:  make([]string, 0, len(events)),
		OccurredAts: make([]time.Time, 0, len(events)),
		ChannelIds:  make([]uuid.UUID, 0, len(events)),
		CustomerIds: make([]uuid.UUID, 0, len(events)),
		AnonIds:     make([]string, 0, len(events)),
		Sources:     make([]string, 0, len(events)),
		SendIds:     make([]uuid.UUID, 0, len(events)),
		Properties:  make([]string, 0, len(events)),
		DedupeKeys:  make([]string, 0, len(events)),
	}
	for _, e := range events {
		raw, err := json.Marshal(e.Props)
		if err != nil {
			return err
		}
		p.EventNames = append(p.EventNames, e.EventName)
		p.OccurredAts = append(p.OccurredAts, e.OccurredAt)
		p.ChannelIds = append(p.ChannelIds, zu(e.ChannelID))
		p.CustomerIds = append(p.CustomerIds, zu(e.CustomerID))
		p.AnonIds = append(p.AnonIds, e.AnonID)
		p.Sources = append(p.Sources, e.Source)
		p.SendIds = append(p.SendIds, zu(e.SendID))
		p.Properties = append(p.Properties, string(raw))
		p.DedupeKeys = append(p.DedupeKeys, e.DedupeKey)
	}
	_, err := s.w.RecordAnalyticsEventsBatch(ctx, p)
	return err
}

func (s *pgSpine) PruneEvents(ctx context.Context, before time.Time) (int64, error) {
	return s.w.PruneAnalyticsEvents(ctx, before)
}

func (s *pgSpine) PruneEventsBySource(ctx context.Context, source string, before time.Time) (int64, error) {
	return s.w.PruneAnalyticsEventsBySource(ctx, signalsdb.PruneAnalyticsEventsBySourceParams{Source: source, Before: before})
}

func (s *pgSpine) UpsertIdentity(ctx context.Context, anonID string, customerID, channelID uuid.UUID) error {
	return s.w.UpsertIdentity(ctx, signalsdb.UpsertIdentityParams{
		AnonID:     anonID,
		CustomerID: customerID,
		ChannelID:  uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
	})
}

func (s *pgSpine) AnonIDsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error) {
	return s.read().AnonIDsForCustomer(ctx, customerID)
}

func (s *pgSpine) DeleteCustomerIdentities(ctx context.Context, customerIDs []uuid.UUID) error {
	_, err := s.w.DeleteCustomerAnalyticsIdentities(ctx, customerIDs)
	return err
}

func (s *pgSpine) PruneIdentities(ctx context.Context, before time.Time) (int64, error) {
	return s.w.PruneAnalyticsIdentities(ctx, before)
}

type SpineStats struct { // arrival-time bounds of the spine - deliberately no row count, see the query's note
	Oldest *time.Time
	Newest *time.Time
}

func (s *pgSpine) Stats(ctx context.Context) (SpineStats, error) {
	row, err := s.read().SpineStats(ctx)
	if err != nil {
		return SpineStats{}, err
	}
	// asTime (read.go) unwraps the interface{} sqlc emits for a bare MIN/MAX
	return SpineStats{Oldest: asTime(row.Oldest), Newest: asTime(row.Newest)}, nil
}

func (s *pgSpine) CustomersForAnonIDs(ctx context.Context, anonIDs []string) (map[string]uuid.UUID, error) {
	out := make(map[string]uuid.UUID, len(anonIDs))
	if len(anonIDs) == 0 {
		return out, nil
	}
	rows, err := s.read().CustomersForAnonIDs(ctx, anonIDs)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.AnonID] = r.CustomerID
	}
	return out, nil
}

// Segments up to this size take the index-probing query variant - above it per-member probes lose to the channel-window scan the plain variant gets, and the resolved anon-id array itself stops being cheap to build and bind
const segmentAffinityProbeLimit = 50_000

func (s *pgSpine) SegmentAffinitiesForMembers(ctx context.Context, channelID uuid.UUID, memberIDs []uuid.UUID, since time.Time, limit int) ([]SegmentAffinityCount, error) {
	nullCh := uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}
	if len(memberIDs) <= segmentAffinityProbeLimit {
		anonIDs, err := s.read().AnonIDsForCustomerSet(ctx, memberIDs)
		if err != nil {
			return nil, err
		}
		rows, err := s.read().SegmentAffinitiesForMembersSmall(ctx, signalsdb.SegmentAffinitiesForMembersSmallParams{
			ChannelID: nullCh, Since: since, MemberIds: memberIDs, MemberAnonIds: anonIDs, Lim: int32(limit),
		})
		if err != nil {
			return nil, err
		}
		out := make([]SegmentAffinityCount, 0, len(rows))
		for _, m := range rows {
			appendAffinity(&out, m.AttrKey, m.AttrVal, int(m.Members))
		}
		return out, nil
	}
	rows, err := s.read().SegmentAffinitiesForMembers(ctx, signalsdb.SegmentAffinitiesForMembersParams{
		ChannelID: nullCh,
		Since:     since,
		MemberIds: memberIDs,
		Lim:       int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]SegmentAffinityCount, 0, len(rows))
	for _, m := range rows {
		appendAffinity(&out, m.AttrKey, m.AttrVal, int(m.Members))
	}
	return out, nil
}

func appendAffinity(out *[]SegmentAffinityCount, attrKey, attrVal any, members int) {
	k, _ := attrKey.(string)
	v, _ := attrVal.(string)
	if !strings.HasPrefix(k, "attr:") {
		return
	}
	*out = append(*out, SegmentAffinityCount{Attr: strings.TrimPrefix(k, "attr:"), Value: v, Members: members})
}

func (s *pgSpine) ExperimentGoalEvents(ctx context.Context, channelID uuid.UUID, since time.Time) ([]string, error) {
	rows, err := s.read().ExperimentGoalEvents(ctx, signalsdb.ExperimentGoalEventsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		Since:     since,
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if name, ok := r.Name.(string); ok && name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}

func (s *pgSpine) ExperimentResults(ctx context.Context, channelID uuid.UUID, key, goalEvent string, since time.Time) ([]VariationResult, error) {
	rows, err := s.read().ExperimentResults(ctx, signalsdb.ExperimentResultsParams{
		Key: key, ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		GoalEvent: goalEvent, Since: since,
	})
	if err != nil {
		return nil, err
	}
	out := make([]VariationResult, 0, len(rows))
	for _, r := range rows {
		v, _ := r.Variation.(string)
		out = append(out, VariationResult{Variation: v, Exposed: int(r.Exposed), Converted: int(r.Converted), Revenue: r.RevenueCents})
	}
	return out, nil
}

func (s *pgSpine) EventsForForwarding(ctx context.Context, channelID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]ForwardEvent, error) {
	rows, err := s.read().ListEventsForForwarding(ctx, signalsdb.ListEventsForForwardingParams{
		ChannelID:       uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		CursorCreatedAt: afterCreatedAt,
		CursorEventID:   afterID,
		BatchLimit:      int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ForwardEvent, 0, len(rows))
	for _, r := range rows {
		e := ForwardEvent{
			ID: r.ID, Name: r.EventName, OccurredAt: r.OccurredAt, Source: r.Source,
			Properties: r.Properties, CreatedAt: r.CreatedAt,
		}
		if r.CustomerID.Valid {
			id := r.CustomerID.UUID
			e.CustomerID = &id
		}
		if r.AnonID.Valid {
			e.AnonID = r.AnonID.String
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *pgSpine) RecentEngagementEvents(ctx context.Context, customerID uuid.UUID, since time.Time) ([]EngagementCandidate, error) {
	rows, err := s.read().CustomerEngagementEvents(ctx, signalsdb.CustomerEngagementEventsParams{
		CustomerID: uuid.NullUUID{UUID: customerID, Valid: true},
		Since:      since,
	})
	if err != nil {
		return nil, err
	}
	out := make([]EngagementCandidate, 0, len(rows))
	for _, c := range rows {
		if !c.SendID.Valid {
			continue
		}
		out = append(out, EngagementCandidate{
			EventName: c.EventName, OccurredAt: c.OccurredAt, SendID: c.SendID.UUID,
			Source: c.Source, CampaignID: propString(c.Properties, "campaign_id"), FlowID: propString(c.Properties, "flow_id"),
		})
	}
	return out, nil
}

func (s *pgSpine) ChannelOrderAggregates(ctx context.Context, channelID uuid.UUID) ([]CustomerOrders, error) {
	rows, err := s.read().ChannelOrderAggregates(ctx, uuid.NullUUID{UUID: channelID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]CustomerOrders, 0, len(rows))
	for _, r := range rows {
		out = append(out, CustomerOrders{
			CustomerID:    r.CustomerID,
			OrderCount:    r.OrderCount,
			MonetaryCents: r.MonetaryCents,
			FirstOrderAt:  r.FirstOrderAt,
			LastOrderAt:   r.LastOrderAt,
		})
	}
	return out, nil
}

func (s *pgSpine) ChannelEngagementCounts(ctx context.Context, channelID uuid.UUID, since time.Time) ([]CustomerEngagement, error) {
	rows, err := s.read().ChannelEngagementCounts(ctx, signalsdb.ChannelEngagementCountsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: true},
		Since:     since,
	})
	if err != nil {
		return nil, err
	}
	out := make([]CustomerEngagement, 0, len(rows))
	for _, c := range rows {
		if !c.CustomerID.Valid {
			continue
		}
		out = append(out, CustomerEngagement{CustomerID: c.CustomerID.UUID, Count: c.Engagement})
	}
	return out, nil
}

func (s *pgSpine) ActiveCustomerIDs(ctx context.Context, channelID uuid.UUID, from, to time.Time) ([]uuid.UUID, error) {
	return s.read().ActiveSpineCustomerIDs(ctx, signalsdb.ActiveSpineCustomerIDsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: true},
		FromAt:    from,
		ToAt:      to,
	})
}

func (s *pgSpine) IngestedCustomerIDs(ctx context.Context, channelID uuid.UUID, from, to time.Time) ([]uuid.UUID, error) {
	return s.read().IngestedSpineCustomerIDs(ctx, signalsdb.IngestedSpineCustomerIDsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: true},
		FromAt:    from,
		ToAt:      to,
	})
}

func (s *pgSpine) ChannelEngagementCountsFor(ctx context.Context, channelID uuid.UUID, since time.Time, customerIDs []uuid.UUID) ([]CustomerEngagement, error) {
	rows, err := s.read().ChannelEngagementCountsFor(ctx, signalsdb.ChannelEngagementCountsForParams{
		ChannelID:   uuid.NullUUID{UUID: channelID, Valid: true},
		CustomerIds: customerIDs,
		Since:       since,
	})
	if err != nil {
		return nil, err
	}
	out := make([]CustomerEngagement, 0, len(rows))
	for _, c := range rows {
		if !c.CustomerID.Valid {
			continue
		}
		out = append(out, CustomerEngagement{CustomerID: c.CustomerID.UUID, Count: c.Engagement})
	}
	return out, nil
}

// --- Recorder delegates to the spine engine - the read bodies live on the driver, these keep the Recorder's public API stable for resolvers ---

func (r *Recorder) Funnel(ctx context.Context, channelID uuid.UUID, steps []string, since time.Time) ([]FunnelStep, error) {
	return r.spine.Funnel(ctx, channelID, steps, since)
}

func (r *Recorder) Insights(ctx context.Context, channelID uuid.UUID, event, breakdown, measure, unit string, since, now time.Time) (Insights, error) {
	return r.spine.Insights(ctx, channelID, event, breakdown, measure, unit, since, now)
}

func (r *Recorder) Stickiness(ctx context.Context, channelID uuid.UUID, event string, since, now time.Time, windowDays int) (Stickiness, error) {
	return r.spine.Stickiness(ctx, channelID, event, since, now, windowDays)
}

func (r *Recorder) Flow(ctx context.Context, channelID uuid.UUID, focus string, since time.Time, depth, topK int) (FlowResult, error) {
	return r.spine.Flow(ctx, channelID, focus, since, depth, topK)
}

func (r *Recorder) Retention(ctx context.Context, channelID uuid.UUID, startEvent, returnEvent string, since, now time.Time, periods int, unit time.Duration) (Retention, error) {
	return r.spine.Retention(ctx, channelID, startEvent, returnEvent, since, now, periods, unit)
}

func (r *Recorder) Engagement(ctx context.Context, channelID uuid.UUID, since time.Time) (Engagement, error) {
	return r.spine.Engagement(ctx, channelID, since)
}

func (r *Recorder) SynapseOverview(ctx context.Context, channelID uuid.UUID, since time.Time) (SynapseOverview, error) {
	return r.spine.SynapseOverview(ctx, channelID, since)
}

func (r *Recorder) CustomerProfile(ctx context.Context, customerID uuid.UUID, since time.Time) (Profile, error) {
	return r.spine.CustomerProfile(ctx, customerID, since)
}

func (r *Recorder) VisitorProfile(ctx context.Context, anonID string, since time.Time) (Profile, error) {
	return r.spine.VisitorProfile(ctx, anonID, since)
}

func (r *Recorder) CustomerExperiments(ctx context.Context, customerID uuid.UUID, since time.Time) ([]ExperimentArm, error) {
	return r.spine.CustomerExperiments(ctx, customerID, since)
}

func (r *Recorder) CustomerAffinities(ctx context.Context, customerID uuid.UUID, since time.Time, attrs []AffinityAttr) ([]AttributeAffinity, error) {
	return r.spine.CustomerAffinities(ctx, customerID, since, attrs)
}

func (r *Recorder) VisitorAffinities(ctx context.Context, anonID string, since time.Time, attrs []AffinityAttr) []AttributeAffinity {
	return r.spine.VisitorAffinities(ctx, anonID, since, attrs)
}

func (r *Recorder) AffinityActors(ctx context.Context, channelID uuid.UUID, attrSlug, value string, since time.Time) ([]uuid.UUID, error) {
	return r.spine.AffinityActors(ctx, channelID, attrSlug, value, since)
}

func (r *Recorder) AffinityValueCounts(ctx context.Context, channelID uuid.UUID, attrSlug string, since time.Time, limit int) ([]AffinityValueCount, error) {
	return r.spine.AffinityValueCounts(ctx, channelID, attrSlug, since, limit)
}

func (r *Recorder) CustomerTopAffinities(ctx context.Context, customerID uuid.UUID, since time.Time) (map[string]string, error) {
	return r.spine.CustomerTopAffinities(ctx, customerID, since)
}

func (r *Recorder) EventActors(ctx context.Context, channelID uuid.UUID, event string, minCount, days int, now time.Time) ([]uuid.UUID, error) {
	return r.spine.EventActors(ctx, channelID, event, minCount, days, now)
}

func (r *Recorder) SegmentActors(ctx context.Context, channelID uuid.UUID, conds []SegmentCondition, now time.Time) ([]uuid.UUID, error) {
	return r.spine.SegmentActors(ctx, channelID, conds, now)
}

func (r *Recorder) LensPageTotals(ctx context.Context, channelID uuid.UUID, pageKey string) (int64, int64, error) {
	return r.spine.LensPageTotals(ctx, channelID, pageKey)
}

func (r *Recorder) LensPageReferrers(ctx context.Context, channelID uuid.UUID, pageKey string) ([]ValueCount, error) {
	return r.spine.LensPageReferrers(ctx, channelID, pageKey)
}

func (r *Recorder) LensPageClients(ctx context.Context, channelID uuid.UUID, pageKey string) ([]ValueCount, error) {
	return r.spine.LensPageClients(ctx, channelID, pageKey)
}

func (r *Recorder) LensPageDeviceSplit(ctx context.Context, channelID uuid.UUID, pageKey string) ([]ValueCount, error) {
	return r.spine.LensPageDeviceSplit(ctx, channelID, pageKey)
}

func (r *Recorder) LensPageTimeline(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]LensDayCount, error) {
	return r.spine.LensPageTimeline(ctx, channelID, pageKey, since)
}

func (r *Recorder) LensListPages(ctx context.Context, channelID uuid.UUID) ([]ValueCount, error) {
	return r.spine.LensListPages(ctx, channelID)
}

func (r *Recorder) LensTopRageElements(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]LensRageElement, error) {
	return r.spine.LensTopRageElements(ctx, channelID, since, limit)
}

func (r *Recorder) LensTopErrors(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]LensErrorStat, error) {
	return r.spine.LensTopErrors(ctx, channelID, since, limit)
}

func (r *Recorder) LensVitalsByPage(ctx context.Context, channelID uuid.UUID, since time.Time) ([]LensVital, error) {
	return r.spine.LensVitalsByPage(ctx, channelID, since)
}

func (r *Recorder) LensVitalsOverall(ctx context.Context, channelID uuid.UUID, since time.Time) ([]LensVital, error) {
	return r.spine.LensVitalsOverall(ctx, channelID, since)
}

func (r *Recorder) LensSessionSignals(ctx context.Context, channelID uuid.UUID, sessionID string, from, to time.Time) ([]LensSessionSignal, error) {
	return r.spine.LensSessionSignals(ctx, channelID, sessionID, from, to)
}

func (r *Recorder) ExperimentGoalEvents(ctx context.Context, channelID uuid.UUID, since time.Time) ([]string, error) {
	return r.spine.ExperimentGoalEvents(ctx, channelID, since)
}

func (r *Recorder) ExperimentResults(ctx context.Context, channelID uuid.UUID, key, goalEvent string, since time.Time) ([]VariationResult, error) {
	return r.spine.ExperimentResults(ctx, channelID, key, goalEvent, since)
}

func (r *Recorder) EventsForForwarding(ctx context.Context, channelID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]ForwardEvent, error) {
	return r.spine.EventsForForwarding(ctx, channelID, afterCreatedAt, afterID, limit)
}

// GDPR erasure routes by engine: Postgres deletes inline (one indexed statement), ClickHouse enqueues for the daily sweep - each CH delete is a mutation, and erasure bursts would stack them
func (r *Recorder) DeleteCustomerIdentities(ctx context.Context, customerID uuid.UUID) error {
	if spineIsPostgres(r.spine) {
		return r.spine.DeleteCustomerIdentities(ctx, []uuid.UUID{customerID})
	}
	return r.qs.EnqueueIdentityErasure(ctx, customerID)
}

func spineIsPostgres(s SpineStore) bool { // sees through decorators (the ingest queue) so engine routing can't be fooled by a wrap
	for {
		if _, ok := s.(*pgSpine); ok {
			return true
		}
		u, ok := s.(interface{ Unwrap() SpineStore })
		if !ok {
			return false
		}
		s = u.Unwrap()
	}
}

// SpineStats delegates to the selected engine, and PendingErasures reports the GDPR backlog - always Postgres-side, since the queue table is the same on either engine and under Postgres it stays empty by design
func (r *Recorder) SpineStats(ctx context.Context) (SpineStats, error) { return r.spine.Stats(ctx) }

func (r *Recorder) PendingErasures(ctx context.Context) (int, error) {
	ids, err := r.qs.QueuedIdentityErasures(ctx, 1000)
	return len(ids), err
}

// Folds every queued erasure into one batched spine delete per page, with queue rows dying only after the batch lands so a failed sweep retries - then refreshes the engine's identity cache once for the whole sweep rather than once per page
func (r *Recorder) sweepIdentityErasures(ctx context.Context) error {
	swept := false
	for {
		ids, err := r.qs.QueuedIdentityErasures(ctx, 10000)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			break
		}
		if err := r.spine.DeleteCustomerIdentities(ctx, ids); err != nil {
			return err
		}
		if err := r.qs.DeleteIdentityErasures(ctx, ids); err != nil {
			return err
		}
		swept = true
		if len(ids) < 10000 {
			break
		}
	}
	if swept {
		refreshSpineIdentities(ctx, r.spine)
	}
	return nil
}

func refreshSpineIdentities(ctx context.Context, s SpineStore) { // walks decorators to the engine and reloads its identity cache when it has one (the ClickHouse dictionary)
	for {
		if rf, ok := s.(interface{ RefreshIdentities(context.Context) }); ok {
			rf.RefreshIdentities(ctx)
			return
		}
		u, ok := s.(interface{ Unwrap() SpineStore })
		if !ok {
			return
		}
		s = u.Unwrap()
	}
}

func (r *Recorder) AnonIDsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error) {
	return r.spine.AnonIDsForCustomer(ctx, customerID)
}

func (r *Recorder) PruneEventsBySource(ctx context.Context, source string, before time.Time) (int64, error) {
	return r.spine.PruneEventsBySource(ctx, source, before)
}

func (r *Recorder) CustomersForAnonIDs(ctx context.Context, anonIDs []string) (map[string]uuid.UUID, error) {
	return r.spine.CustomersForAnonIDs(ctx, anonIDs)
}

type LensCellCount struct {
	X, Y  int32
	Count int64
}

type LensElementCount struct {
	Selector, Text string
	Count          int64
}

type LensGridTotals struct {
	Clicks, ScrollWeight, ScrollTotal int64
}

func (r *Recorder) LensCellsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport, state, kind string) ([]LensCellCount, error) {
	return r.spine.LensCellsFromEvents(ctx, channelID, pageKey, viewport, state, kind)
}

func (r *Recorder) LensElementsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport string, limit int32) ([]LensElementCount, error) {
	return r.spine.LensElementsFromEvents(ctx, channelID, pageKey, viewport, limit)
}

func (r *Recorder) LensPageGridFromEvents(ctx context.Context, channelID uuid.UUID, pageKey string) (LensGridTotals, error) {
	return r.spine.LensPageGridFromEvents(ctx, channelID, pageKey)
}

// --- Lens spine reads (Postgres driver) ---

func (s *pgSpine) LensCellsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport, state, kind string) ([]LensCellCount, error) {
	rows, err := s.read().LensCellsFromEvents(ctx, signalsdb.LensCellsFromEventsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: true},
		PageKey:   pageKey, Viewport: viewport, State: state, Kind: kind,
	})
	if err != nil {
		return nil, err
	}
	out := make([]LensCellCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, LensCellCount{X: r.CellX, Y: r.CellY, Count: r.Count})
	}
	return out, nil
}

func (s *pgSpine) LensElementsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport string, limit int32) ([]LensElementCount, error) {
	rows, err := s.read().LensElementsFromEvents(ctx, signalsdb.LensElementsFromEventsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: true},
		PageKey:   pageKey, Viewport: viewport, Lim: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]LensElementCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, LensElementCount{Selector: r.Selector, Text: r.Text, Count: r.Count})
	}
	return out, nil
}

func (s *pgSpine) LensPageGridFromEvents(ctx context.Context, channelID uuid.UUID, pageKey string) (LensGridTotals, error) {
	g, err := s.read().LensPageGridFromEvents(ctx, signalsdb.LensPageGridFromEventsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: true},
		PageKey:   pageKey,
	})
	if err != nil {
		return LensGridTotals{}, err
	}
	return LensGridTotals{Clicks: g.Clicks, ScrollWeight: g.ScrollWeight, ScrollTotal: g.ScrollTotal}, nil
}

func (s *pgSpine) LensPageTotals(ctx context.Context, channelID uuid.UUID, pageKey string) (int64, int64, error) {
	r, err := s.read().LensPageTotals(ctx, signalsdb.LensPageTotalsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}, PageKey: pageKey,
	})
	if err != nil {
		return 0, 0, err
	}
	return r.Views, r.Visitors, nil
}

func (s *pgSpine) LensPageReferrers(ctx context.Context, channelID uuid.UUID, pageKey string) ([]ValueCount, error) {
	rows, err := s.read().LensPageReferrers(ctx, signalsdb.LensPageReferrersParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}, PageKey: pageKey,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ValueCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, ValueCount{Value: anyString(r.Referrer), Count: r.Views})
	}
	return out, nil
}

func (s *pgSpine) LensPageClients(ctx context.Context, channelID uuid.UUID, pageKey string) ([]ValueCount, error) {
	rows, err := s.read().LensPageClients(ctx, signalsdb.LensPageClientsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}, PageKey: pageKey,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ValueCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, ValueCount{Value: anyString(r.Client), Count: r.Views})
	}
	return out, nil
}

func (s *pgSpine) LensPageDeviceSplit(ctx context.Context, channelID uuid.UUID, pageKey string) ([]ValueCount, error) {
	rows, err := s.read().LensPageDeviceSplit(ctx, signalsdb.LensPageDeviceSplitParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}, PageKey: pageKey,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ValueCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, ValueCount{Value: anyString(r.Device), Count: r.Views})
	}
	return out, nil
}

func (s *pgSpine) LensPageTimeline(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]LensDayCount, error) {
	rows, err := s.read().LensPageTimeline(ctx, signalsdb.LensPageTimelineParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}, PageKey: pageKey, Since: since,
	})
	if err != nil {
		return nil, err
	}
	out := make([]LensDayCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, LensDayCount{Date: r.Date, Views: r.Views})
	}
	return out, nil
}

func (s *pgSpine) LensListPages(ctx context.Context, channelID uuid.UUID) ([]ValueCount, error) {
	rows, err := s.read().LensListPages(ctx, uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil})
	if err != nil {
		return nil, err
	}
	out := make([]ValueCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, ValueCount{Value: anyString(r.PageKey), Count: r.Views})
	}
	return out, nil
}

func (s *pgSpine) LensTopRageElements(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]LensRageElement, error) {
	rows, err := s.read().LensTopRageElements(ctx, signalsdb.LensTopRageElementsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}, Since: since, Lim: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]LensRageElement, 0, len(rows))
	for _, r := range rows {
		out = append(out, LensRageElement{Selector: anyString(r.Selector), Text: anyString(r.Text), Count: r.Count})
	}
	return out, nil
}

func (s *pgSpine) LensTopErrors(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]LensErrorStat, error) {
	rows, err := s.read().LensTopErrors(ctx, signalsdb.LensTopErrorsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}, Since: since, Lim: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]LensErrorStat, 0, len(rows))
	for _, r := range rows {
		e := LensErrorStat{Message: anyString(r.Message), Count: r.Count, Pages: r.Pages}
		if t, ok := r.LastSeen.(time.Time); ok {
			e.LastSeen = t
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *pgSpine) LensVitalsByPage(ctx context.Context, channelID uuid.UUID, since time.Time) ([]LensVital, error) {
	rows, err := s.read().LensVitalsByPage(ctx, signalsdb.LensVitalsByPageParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}, Since: since,
	})
	if err != nil {
		return nil, err
	}
	out := make([]LensVital, 0, len(rows))
	for _, r := range rows {
		out = append(out, LensVital{PageKey: anyString(r.PageKey), Metric: anyString(r.Metric), P75: r.P75, Samples: r.Samples})
	}
	return out, nil
}

func (s *pgSpine) LensVitalsOverall(ctx context.Context, channelID uuid.UUID, since time.Time) ([]LensVital, error) {
	rows, err := s.read().LensVitalsOverall(ctx, signalsdb.LensVitalsOverallParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil}, Since: since,
	})
	if err != nil {
		return nil, err
	}
	out := make([]LensVital, 0, len(rows))
	for _, r := range rows {
		out = append(out, LensVital{Metric: anyString(r.Metric), P75: r.P75, Samples: r.Samples})
	}
	return out, nil
}

func (s *pgSpine) LensSessionSignals(ctx context.Context, channelID uuid.UUID, sessionID string, from, to time.Time) ([]LensSessionSignal, error) {
	rows, err := s.read().LensSessionSignals(ctx, signalsdb.LensSessionSignalsParams{
		ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		SessionID: sessionID, FromTs: from, ToTs: to,
	})
	if err != nil {
		return nil, err
	}
	out := make([]LensSessionSignal, 0, len(rows))
	for _, r := range rows {
		out = append(out, LensSessionSignal{
			Name: anyString(r.Name), Message: anyString(r.Message),
			Metric: anyString(r.Metric), Value: anyString(r.Value), OccurredAt: r.OccurredAt,
		})
	}
	return out, nil
}

func anyString(v any) string { // sqlc types a COALESCE'd jsonb extract as any and the driver hands text back as either string or []byte depending on the path - both have to unwrap or every label silently renders empty
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	}
	return ""
}

// One string out of an event's stored properties, empty when it is absent or not a string
func propString(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}
