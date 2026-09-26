package analytics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type CustomerTraits struct { // the persisted per-customer summary Synapse computes and the rest of Signals reads, for profiles and trait-driven segments
	RecencyDays       int
	Frequency         int
	MonetaryCents     int64
	RFMRecency        int // 1..5 (5 best), 0 = never ordered
	RFMFrequency      int
	RFMMonetary       int
	Lifecycle         string // prospect | new | active | at_risk | lapsed
	EngagementScore   int    // 0..100
	ChurnRisk         int
	PredictedCLVCents int64
	NextOrderDays     int
	ComputedAt        time.Time
}

type TraitRule struct { // the trait analogue of SegmentCondition - a customer matches only by satisfying every set constraint, and an empty rule matches nobody
	Lifecycle         []string // match any of these stages
	MinRecencyScore   int      // rfm_r >=
	MinFrequencyScore int      // rfm_f >=
	MinMonetaryScore  int      // rfm_m >=
	MinEngagement     int      // engagement_score >=
}

func (t TraitRule) empty() bool {
	return len(t.Lifecycle) == 0 && t.MinRecencyScore <= 0 && t.MinFrequencyScore <= 0 &&
		t.MinMonetaryScore <= 0 && t.MinEngagement <= 0
}

// Day cutoffs mapping recency to a lifecycle stage, resolved from per-channel config by the trait-compute job so staging matches the catalog's repurchase cadence - a tire shop on a ~3-year cycle wants far larger windows than a consumables store
// Only these absolute bands need cadence tuning, since the RFM recency score is a relative quintile
type LifecycleWindows struct {
	NewDays    int // "new" is a first-and-only order inside this
	ActiveDays int // "active" inside this
	AtRiskDays int // "at_risk" inside this, "lapsed" beyond
}

// Guards against missing or misordered config - a non-positive value falls back to the fast-cadence defaults, and the bands are forced ascending so each stage owns a real range
func (lc LifecycleWindows) normalized() LifecycleWindows {
	if lc.NewDays <= 0 {
		lc.NewDays = 30
	}
	if lc.ActiveDays < lc.NewDays {
		lc.ActiveDays = lc.NewDays
	}
	if lc.AtRiskDays < lc.ActiveDays {
		lc.AtRiskDays = lc.ActiveDays
	}
	return lc
}

// The whole read-rank-upsert pass is ONE set-based statement on the primary, no per-customer round-trips, and it skips rows whose decision-relevant traits are unchanged so dormant customers don't churn - the return is the count of rows actually written
// RFM scores are quintile ranks among the channel's ordering customers (5 best, 0 never-ordered), lifecycle derives from recency and order count against LifecycleWindows, engagement is last-30d commerce + account + email spine activity capped at 100
// TraitCompute turns the roster and the spine into customer_traits and the projections the recompute reads
type TraitCompute struct {
	qs    *signalsdb.Queries
	spine SpineStore
}

func NewTraitCompute(qs *signalsdb.Queries, spine SpineStore) *TraitCompute {
	return &TraitCompute{qs: qs, spine: spine}
}

func (r *TraitCompute) ComputeTraits(ctx context.Context, channelID uuid.UUID, now time.Time, lc LifecycleWindows) (int, error) { // driven by the trait-compute scheduled job, never the request path
	lc = lc.normalized()
	// The engagement projection is refreshed first - the order inputs are already on the roster, pushed by core
	if err := r.refreshEngagement(ctx, channelID, now); err != nil {
		return 0, err
	}
	n, err := r.qs.RecomputeChannelTraits(ctx, signalsdb.RecomputeChannelTraitsParams{
		ChannelID:  channelID,
		NewDays:    int32(lc.NewDays),
		ActiveDays: int32(lc.ActiveDays),
		AtRiskDays: int32(lc.AtRiskDays),
		AsOf:       now,
	})
	return int(n), err
}

const (
	engagementWindow = 30 * 24 * time.Hour
	engagementChunk  = 5000 // bounds each id-set query and commerce statement
)

// Incremental by default - only customers whose events entered or left the window since the last run get recomputed, with the first run and a weekly self-heal doing the full swap and the fresh slice starting 1h early to absorb late arrivals
func (r *TraitCompute) refreshEngagement(ctx context.Context, channelID uuid.UUID, now time.Time) error {
	since := now.Add(-engagementWindow)
	last, werr := r.qs.GetEngagementWatermark(ctx, channelID)
	if werr != nil && !errors.Is(werr, sql.ErrNoRows) {
		return fmt.Errorf("engagement watermark: %w", werr)
	}
	if werr != nil || now.Sub(last) > 7*24*time.Hour {
		if err := r.refreshEngagementFull(ctx, channelID, since); err != nil {
			return err
		}
		return r.qs.SetEngagementWatermark(ctx, signalsdb.SetEngagementWatermarkParams{ChannelID: channelID, RanAt: now})
	}
	aged, err := r.spine.ActiveCustomerIDs(ctx, channelID, last.Add(-engagementWindow), since)
	if err != nil {
		return fmt.Errorf("aged-out customers: %w", err)
	}
	// The fresh slice keys on ARRIVAL time, not occurred_at - a queue-backlog drain or an outbox redelivery inserts events whose occurred_at predates any reasonable overlap, and those customers would otherwise stay stale until the weekly full swap
	fresh, err := r.spine.IngestedCustomerIDs(ctx, channelID, last.Add(-time.Hour), now)
	if err != nil {
		return fmt.Errorf("fresh customers: %w", err)
	}
	changed := make([]uuid.UUID, 0, len(aged)+len(fresh))
	seen := make(map[uuid.UUID]bool, len(aged)+len(fresh))
	for _, id := range append(aged, fresh...) {
		if !seen[id] {
			seen[id] = true
			changed = append(changed, id)
		}
	}
	for start := 0; start < len(changed); start += engagementChunk {
		chunk := changed[start:min(start+engagementChunk, len(changed))]
		counts, err := r.spine.ChannelEngagementCountsFor(ctx, channelID, since, chunk)
		if err != nil {
			return fmt.Errorf("engagement counts for: %w", err)
		}
		ids := make([]uuid.UUID, 0, len(counts))
		vals := make([]int32, 0, len(counts))
		present := make(map[uuid.UUID]bool, len(counts))
		for _, c := range counts {
			ids, vals = append(ids, c.CustomerID), append(vals, c.Count)
			present[c.CustomerID] = true
		}
		if len(ids) > 0 {
			if err := r.qs.UpsertChannelEngagementFor(ctx, signalsdb.UpsertChannelEngagementForParams{
				CustomerIds: ids, ChannelID: channelID, Engagements: vals,
			}); err != nil {
				return fmt.Errorf("upsert engagement: %w", err)
			}
		}
		gone := make([]uuid.UUID, 0, len(chunk)-len(ids))
		for _, id := range chunk {
			if !present[id] {
				gone = append(gone, id)
			}
		}
		if len(gone) > 0 {
			if err := r.qs.DeleteChannelEngagementFor(ctx, gone); err != nil {
				return fmt.Errorf("delete engagement: %w", err)
			}
		}
	}
	return r.qs.SetEngagementWatermark(ctx, signalsdb.SetEngagementWatermarkParams{ChannelID: channelID, RanAt: now}) // only after every chunk landed - a partial run redoes its slices next time and recomputes to the same values
}

// The full atomic swap - one statement, so a crash can't leave the projection half-cleared for the recompute that reads it next
func (r *TraitCompute) refreshEngagementFull(ctx context.Context, channelID uuid.UUID, since time.Time) error {
	counts, err := r.spine.ChannelEngagementCounts(ctx, channelID, since)
	if err != nil {
		return fmt.Errorf("engagement counts: %w", err)
	}
	if len(counts) == 0 {
		if err := r.qs.ClearChannelEngagement(ctx, channelID); err != nil {
			return fmt.Errorf("clear engagement: %w", err)
		}
		return nil
	}
	ids := make([]uuid.UUID, 0, len(counts))
	vals := make([]int32, 0, len(counts))
	for _, c := range counts {
		ids = append(ids, c.CustomerID)
		vals = append(vals, c.Count)
	}
	if err := r.qs.ReplaceChannelEngagement(ctx, signalsdb.ReplaceChannelEngagementParams{
		CustomerIds: ids, ChannelID: channelID, Engagements: vals,
	}); err != nil {
		return fmt.Errorf("replace engagement: %w", err)
	}
	return nil
}

func (r *Recorder) CustomerTraits(ctx context.Context, customerID uuid.UUID) (CustomerTraits, bool, error) { // ok=false when they haven't been computed yet, e.g. a brand-new customer before the next run
	t, err := r.qs.GetCustomerTrait(ctx, customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return CustomerTraits{}, false, nil
	}
	if err != nil {
		return CustomerTraits{}, false, err
	}
	return CustomerTraits{
		RecencyDays: int(t.RecencyDays), Frequency: int(t.Frequency), MonetaryCents: t.MonetaryCents,
		RFMRecency: int(t.RfmR), RFMFrequency: int(t.RfmF), RFMMonetary: int(t.RfmM),
		Lifecycle: t.LifecycleStage, EngagementScore: int(t.EngagementScore),
		ChurnRisk: int(t.ChurnRisk), PredictedCLVCents: t.PredictedClvCents, NextOrderDays: int(t.NextOrderDays),
		ComputedAt: t.ComputedAt,
	}, true, nil
}

// The trait analogue of SegmentActors, used to materialise trait segments - an empty rule returns nothing, since a segment needs at least one constraint
func (r *Recorder) TraitActors(ctx context.Context, channelID uuid.UUID, rule TraitRule) ([]uuid.UUID, error) {
	if rule.empty() {
		return nil, nil
	}
	rows, err := r.qs.ListCustomerTraitsByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	lifecycleSet := make(map[string]bool, len(rule.Lifecycle))
	for _, l := range rule.Lifecycle {
		lifecycleSet[l] = true
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, t := range rows {
		if len(lifecycleSet) > 0 && !lifecycleSet[t.LifecycleStage] {
			continue
		}
		if int(t.RfmR) < rule.MinRecencyScore || int(t.RfmF) < rule.MinFrequencyScore || int(t.RfmM) < rule.MinMonetaryScore {
			continue
		}
		if int(t.EngagementScore) < rule.MinEngagement {
			continue
		}
		out = append(out, t.CustomerID)
	}
	return out, nil
}
