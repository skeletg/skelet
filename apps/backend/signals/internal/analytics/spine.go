package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"

	"github.com/google/uuid"
)

// The engine behind the event spine, at the level of domain results rather than raw rows - ClickHouse is the one driver, wrapped by the queue decorator and test fakes
// It grows a slice at a time as the Recorder's methods move behind it - commerce-coupled reads (segment membership, email_sends attribution, trait projection) stay on the Recorder, which orchestrates the SpineStore plus the commerce db
type SpineStore interface {
	// Appends one event, idempotent on DedupeKey - returns whether a genuinely new row was written
	AppendEvent(ctx context.Context, e SpineEvent) (bool, error)
	PruneEvents(ctx context.Context, before time.Time) (int64, error)
	// Source-scoped retention - Lens prunes web events on a shorter window than the global sweep, and that window must hold on whichever engine stores them
	PruneEventsBySource(ctx context.Context, source string, before time.Time) (int64, error)
	// Archive retention: export each aged whole day through open (source '' = every source), deleting a day only after its upload commits - the global sweep becomes partition drops
	ArchiveEvents(ctx context.Context, source string, before time.Time, open ArchiveOpen) (int64, error)
	// Batch ingest - a buffer feed (ClickHouse) or an async publish window (queue) per batch, and every decorator must override this alongside AppendEvent or writes bypass it
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

	// Event-analytics reads - each returns a domain result, and people resolve across anonymous and identified activity

	Funnel(ctx context.Context, channelID uuid.UUID, steps []string, since time.Time) ([]FunnelStep, error)
	Insights(ctx context.Context, channelID uuid.UUID, event, breakdown, measure, unit string, since, now time.Time) (Insights, error)
	Stickiness(ctx context.Context, channelID uuid.UUID, event string, since, now time.Time, windowDays int) (Stickiness, error)
	Flow(ctx context.Context, channelID uuid.UUID, focus string, since time.Time, depth, topK int) (FlowResult, error)
	Retention(ctx context.Context, channelID uuid.UUID, startEvent, returnEvent string, since, now time.Time, periods int, unit time.Duration) (Retention, error)

	// Pulse engagement, the Synapse overview, per-profile and affinity reads, and segment actor lists - all pure-spine, since identity resolution lives there too - Commerce-coupled reads stay on the Recorder
	Engagement(ctx context.Context, channelID uuid.UUID, since time.Time) (Engagement, error)
	SynapseOverview(ctx context.Context, channelID uuid.UUID, since time.Time) (SynapseOverview, error)
	CustomerProfile(ctx context.Context, customerID uuid.UUID, since time.Time) (Profile, error)
	CustomerEmailEngagement(ctx context.Context, customerID uuid.UUID) (EmailEngagement, error)
	VisitorProfile(ctx context.Context, channelID uuid.UUID, anonID string, since time.Time) (Profile, error)
	CustomerExperiments(ctx context.Context, customerID uuid.UUID, since time.Time) ([]ExperimentArm, error)
	CustomerAffinities(ctx context.Context, customerID uuid.UUID, since time.Time, attrs []AffinityAttr) ([]AttributeAffinity, error)
	VisitorAffinities(ctx context.Context, channelID uuid.UUID, anonID string, since time.Time, attrs []AffinityAttr) []AttributeAffinity
	AffinityActors(ctx context.Context, channelID uuid.UUID, attrSlug, value string, since time.Time) ([]uuid.UUID, error)
	AffinityValueCounts(ctx context.Context, channelID uuid.UUID, attrSlug string, since time.Time, limit int) ([]AffinityValueCount, error)
	CustomerTopAffinities(ctx context.Context, customerID uuid.UUID, since time.Time) (map[string]string, error)
	EventActors(ctx context.Context, channelID uuid.UUID, event string, minCount, days int, now time.Time) ([]uuid.UUID, error)
	SegmentActors(ctx context.Context, channelID uuid.UUID, conds []SegmentCondition, now time.Time) ([]uuid.UUID, error)

	// Lens web-behaviour reads over the spine - Lens keeps its own heatmap/session tables on Postgres, only these raw-event reads are engine-pluggable
	LensPageTotals(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) (views, visitors int64, err error)
	LensPageReferrers(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]ValueCount, error)
	LensPageClients(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]ValueCount, error)
	LensPageDeviceSplit(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]ValueCount, error)
	LensPageTimeline(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]LensDayCount, error)
	LensListPages(ctx context.Context, channelID uuid.UUID, since time.Time) ([]ValueCount, error)
	LensTopRageElements(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]LensRageElement, error)
	LensTopErrors(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]LensErrorStat, error)
	LensVitalsByPage(ctx context.Context, channelID uuid.UUID, since time.Time) ([]LensVital, error)
	LensVitalsOverall(ctx context.Context, channelID uuid.UUID, since time.Time) ([]LensVital, error)
	LensSessionSignals(ctx context.Context, channelID uuid.UUID, sessionID string, from, to time.Time) ([]LensSessionSignal, error)
	// The ClickHouse heatmap path - grid increments live as raw lens_cell/lens_element events there (hot-row upserts don't scale), aggregated at read time
	LensCellsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport, state, kind string, since time.Time) ([]LensCellCount, error)
	LensElementsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport string, limit int32, since time.Time) ([]LensElementCount, error)
	LensPageGridFromEvents(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) (LensGridTotals, error)
	// Every page's grid totals in one scan, for the hub's page list
	LensPagesGridFromEvents(ctx context.Context, channelID uuid.UUID, since time.Time) (map[string]LensGridTotals, error)

	// Frontend A/B: pickable goal-event names, and per-variation exposed/converted/revenue for one experiment
	ExperimentGoalEvents(ctx context.Context, channelID uuid.UUID, since time.Time) ([]string, error)
	ExperimentResults(ctx context.Context, channelID uuid.UUID, key, goalEvent string, since time.Time) ([]VariationResult, error)

	// A keyset page of raw events for outbound forwarding, ordered by (created_at, id) - the destination service pages through this with its stored cursor
	EventsForForwarding(ctx context.Context, channelID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]ForwardEvent, error)

	// Spine primitives for commerce-orchestrated reads - the Recorder joins these with commerce data for segment membership, email_sends and trait projection
	SegmentAffinitiesForMembers(ctx context.Context, channelID uuid.UUID, memberIDs []uuid.UUID, since time.Time, limit int) ([]SegmentAffinityCount, error)
	RecentEngagementEvents(ctx context.Context, channelID, customerID uuid.UUID, since time.Time) ([]EngagementCandidate, error)
	ChannelEngagementCounts(ctx context.Context, channelID uuid.UUID, since time.Time) ([]CustomerEngagement, error)
	// The incremental-refresh trio: who changed in an occurred_at slice (aged-out edge), who ARRIVED in an ingest-time slice (fresh edge keys on arrival time, so queue-backlog drains and outbox redeliveries whose occurred_at predates the window still surface), then exact counts for just those ids
	ActiveCustomerIDs(ctx context.Context, channelID uuid.UUID, from, to time.Time) ([]uuid.UUID, error)
	IngestedCustomerIDs(ctx context.Context, channelID uuid.UUID, from, to time.Time) ([]uuid.UUID, error)
	ChannelEngagementCountsFor(ctx context.Context, channelID uuid.UUID, since time.Time, customerIDs []uuid.UUID) ([]CustomerEngagement, error)

	// Order aggregates per customer, netted against cancellations - the RFM inputs, so trait compute reads the spine rather than
	// joining the orders table. since narrows to orders placed from that instant on, and the zero time means lifetime; a
	// cancellation still voids its order whenever it happened, so a windowed count never includes an order cancelled later
	ChannelOrderAggregates(ctx context.Context, channelID uuid.UUID, since time.Time) ([]CustomerOrders, error)
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
	DedupeKey  string // stable across re-ingests, where ID is per insert
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

// One event to append in engine-neutral form - the driver owns how it's stored
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

type SpineStats struct { // arrival-time bounds of the spine - deliberately no row count, see the query's note
	Oldest *time.Time
	Newest *time.Time
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

func (r *Recorder) CustomerEmailEngagement(ctx context.Context, customerID uuid.UUID) (EmailEngagement, error) {
	return r.spine.CustomerEmailEngagement(ctx, customerID)
}

func (r *Recorder) VisitorProfile(ctx context.Context, channelID uuid.UUID, anonID string, since time.Time) (Profile, error) {
	return r.spine.VisitorProfile(ctx, channelID, anonID, since)
}

func (r *Recorder) CustomerExperiments(ctx context.Context, customerID uuid.UUID, since time.Time) ([]ExperimentArm, error) {
	return r.spine.CustomerExperiments(ctx, customerID, since)
}

func (r *Recorder) CustomerAffinities(ctx context.Context, customerID uuid.UUID, since time.Time, attrs []AffinityAttr) ([]AttributeAffinity, error) {
	return r.spine.CustomerAffinities(ctx, customerID, since, attrs)
}

func (r *Recorder) VisitorAffinities(ctx context.Context, channelID uuid.UUID, anonID string, since time.Time, attrs []AffinityAttr) []AttributeAffinity {
	return r.spine.VisitorAffinities(ctx, channelID, anonID, since, attrs)
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

func (r *Recorder) LensPageTotals(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) (int64, int64, error) {
	return r.spine.LensPageTotals(ctx, channelID, pageKey, since)
}

func (r *Recorder) LensPageReferrers(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]ValueCount, error) {
	return r.spine.LensPageReferrers(ctx, channelID, pageKey, since)
}

func (r *Recorder) LensPageClients(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]ValueCount, error) {
	return r.spine.LensPageClients(ctx, channelID, pageKey, since)
}

func (r *Recorder) LensPageDeviceSplit(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]ValueCount, error) {
	return r.spine.LensPageDeviceSplit(ctx, channelID, pageKey, since)
}

func (r *Recorder) LensPageTimeline(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]LensDayCount, error) {
	return r.spine.LensPageTimeline(ctx, channelID, pageKey, since)
}

func (r *Recorder) LensListPages(ctx context.Context, channelID uuid.UUID, since time.Time) ([]ValueCount, error) {
	return r.spine.LensListPages(ctx, channelID, since)
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

// ErrErasureInFlight is an engine still applying an earlier detach, which a second one would only duplicate
var ErrErasureInFlight = errors.New("an earlier identity erasure is still being applied")

// GDPR erasure is queued for the daily sweep rather than run inline - each ClickHouse delete is a mutation, and erasure bursts would stack them
func (r *Recorder) DeleteCustomerIdentities(ctx context.Context, customerID uuid.UUID) error {
	return r.qs.EnqueueIdentityErasure(ctx, customerID)
}

// SpineStats delegates to the selected engine, and PendingErasures reports the GDPR backlog - read from the signals Postgres database, where the erasure queue lives
func (r *Recorder) SpineStats(ctx context.Context) (SpineStats, error) { return r.spine.Stats(ctx) }

func (r *Recorder) PendingErasures(ctx context.Context) (int, error) {
	ids, err := r.qs.QueuedIdentityErasures(ctx, signalsdb.QueuedIdentityErasuresParams{OlderThan: time.Now(), Lim: 1000})
	return len(ids), err
}

// A request waits this long, so an identify buffered in any process before it has flushed (or aged out) by the time the sweep erases
const erasureGrace = 5 * time.Minute

// Folds every queued erasure into one batched spine delete per page, with queue rows dying only after the batch lands so a failed sweep retries - then refreshes the engine's identity cache once for the whole sweep rather than once per page
func (r *Recorder) sweepIdentityErasures(ctx context.Context) error {
	swept := false
	for {
		ids, err := r.qs.QueuedIdentityErasures(ctx, signalsdb.QueuedIdentityErasuresParams{OlderThan: time.Now().Add(-erasureGrace), Lim: 10000})
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			break
		}
		if err := r.spine.DeleteCustomerIdentities(ctx, ids); errors.Is(err, ErrErasureInFlight) {
			log.Printf("[analytics] identity erasure: the last sweep's detach is still running, %d stay queued for the next", len(ids))
			break
		} else if err != nil {
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

func (r *Recorder) LensCellsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport, state, kind string, since time.Time) ([]LensCellCount, error) {
	return r.spine.LensCellsFromEvents(ctx, channelID, pageKey, viewport, state, kind, since)
}

func (r *Recorder) LensElementsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport string, limit int32, since time.Time) ([]LensElementCount, error) {
	return r.spine.LensElementsFromEvents(ctx, channelID, pageKey, viewport, limit, since)
}

func (r *Recorder) LensPageGridFromEvents(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) (LensGridTotals, error) {
	return r.spine.LensPageGridFromEvents(ctx, channelID, pageKey, since)
}

func (r *Recorder) LensPagesGridFromEvents(ctx context.Context, channelID uuid.UUID, since time.Time) (map[string]LensGridTotals, error) {
	return r.spine.LensPagesGridFromEvents(ctx, channelID, since)
}

// --- Lens spine reads ---

// One customer's lifetime email tallies, as the admin profile shows them - unbounded rather than
// windowed, since "has this person ever opened anything we sent" is what the panel answers
type EmailEngagement struct {
	Sent    int
	Opened  int // machine opens excluded - an image proxy is not the customer reading anything
	Clicked int
	// Nil when the customer has never been sent to, or never opened
	LastSentAt *time.Time
	LastOpenAt *time.Time
}
