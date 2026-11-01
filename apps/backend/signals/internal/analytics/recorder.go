// Package analytics is the event spine - a durable, append-only log of behavioural events, with email engagement as its first producer
// A leaf package: it depends only on the generated db layer and stdlib, never on campaign/flow/order, so those can depend on it without a cycle
package analytics

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"maps"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

const (
	EventEmailSent     = "email_sent"
	EventEmailOpened   = "email_opened"
	EventEmailClicked  = "email_clicked"
	EventConversion    = "conversion"
	EventPageView      = "page_view"      // web (Lens)
	EventCustom        = "custom"         // web (Lens) - arbitrary client event
	EventIdentify      = "identify"       // web (Lens) - anon visitor linked to a customer
	EventProductFilter = "product_filter" // web (Lens) - storefront facet/filter selection
	EventViewItem      = "view_item"      // the three below are storefront funnel events promoted out of the generic "custom" channel, so they're countable and selectable as funnel steps
	EventBeginCheckout = "begin_checkout"
	EventPurchase      = "purchase"
)

const (
	SourceWeb      = "web"      // storefront behaviour (Lens)
	SourceCommerce = "commerce" // server-side commerce actions (order/cart/return/review/...)
	SourceAccount  = "account"  // server-side account actions (auth/registration/profile/...)
	SourceLens     = "lens"     // heatmap grid increments under the ClickHouse engine - own source so behaviour reads never see them and Lens prunes them on its window
	// A visitor who hasn't allowed analytics, counted as nobody - in totals only: no actor, so every per-person read (profiles, funnels,
	// retention, flows, RFM, segments) leaves it out by its empty-actor guard, while event totals and revenue include it
	SourceAggregate = "aggregate"
)

// What an aggregate event may carry - amounts and items, never anything that could lead back to a person or an order
var aggregateProps = map[string]bool{
	"sku": true, "qty": true, "unit_price": true, "total_cents": true, "amount_cents": true, "currency": true,
	"path": true, "status": true, "bundle": true,
}

type Recorder struct {
	qs      *signalsdb.Queries // the signals database - segments, rosters, traits, the erasure queue
	spine   SpineStore         // the ClickHouse spine driver, behind the ingest queue when one is configured - ALL spine access goes through it, ingest and every analytical read alike
	archive ArchiveSink        // optional retention-archive destination (the BACKUP_* bucket) - nil makes retention_action=archive fail closed
}

// Layers a decorator (the ingest queue) over the engine
func (r *Recorder) WrapSpine(wrap func(SpineStore) SpineStore) { r.spine = wrap(r.spine) }

func (r *Recorder) SpineStore() SpineStore { return r.spine } // the queue consumer writes through the selected engine directly, not through its own producer wrap

// The Recorder reads and writes the signals database and the spine only - it holds no commerce handle
func NewRecorder(qs *signalsdb.Queries, spine SpineStore) *Recorder {
	return &Recorder{qs: qs, spine: spine}
}

type Hit struct {
	UA      string
	IP      string
	At      time.Time
	Machine bool // likely bot/proxy open (Apple MPP etc.); excluded from human counts
	Device  string
	Client  string
	OS      string
	Country string // best-effort GeoIP country name (empty when geo disabled)
	IsBot   bool   // UA matched a known/generic bot signature (DetectBot)
	BotName string // canonical bot name when IsBot (e.g. "Googlebot")
	BotCat  string // bot category when IsBot (ai|search|social|seo|monitoring|scraper|other)
}

// SendRef is the send an engagement belongs to, reduced to what the spine stores - the send log is core's, and this package never reads it The origin comes with the send rather than being resolved later, so the event stays true to it
type SendRef struct {
	ID         uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	// campaign | flow | transactional
	Source     string
	CampaignID *uuid.UUID
	FlowID     *uuid.UUID
}

// The origins attribution credits - a transactional send is not marketing and anchors nothing
const (
	SourceCampaign = "campaign"
	SourceFlow     = "flow"
)

func (r *Recorder) RecordSent(ctx context.Context, s SendRef) error {
	_, err := r.record(ctx, EventEmailSent, s, time.Now(), map[string]any{}, "sent:"+s.ID.String())
	return err
}

// Repeat opens within the same minute from the same client collapse on the dedupe key - the send log's own first-open stamp is core's, written by the caller
func (r *Recorder) RecordOpen(ctx context.Context, s SendRef, h Hit) (bool, error) {
	at := orNow(h.At)
	key := fmt.Sprintf("open:%s:%d:%s", s.ID, at.Unix()/60, uaHash(h.UA))
	ok, err := r.record(ctx, EventEmailOpened, s, at, hitProps(h, nil), key)
	if err != nil || !ok {
		return false, err
	}
	return true, nil
}

func (r *Recorder) RecordClick(ctx context.Context, s SendRef, url string, linkIndex int, h Hit) (bool, error) {
	at := orNow(h.At)
	key := fmt.Sprintf("click:%s:%d:%d", s.ID, linkIndex, at.Unix()/60)
	ok, err := r.record(ctx, EventEmailClicked, s, at, hitProps(h, map[string]any{"url": url, "link_index": linkIndex}), key)
	if err != nil || !ok {
		return false, err
	}
	return true, nil
}

const ( // last-touch attribution windows - a click counts for longer than a bare open
	clickAttributionWindow = 7 * 24 * time.Hour
	openAttributionWindow  = 24 * time.Hour
)

type ConversionSpec struct {
	CustomerID   uuid.UUID
	ChannelID    uuid.UUID
	OrderID      uuid.UUID
	RevenueCents int64
	Currency     string
	At           time.Time
}

// Last-touch: a click within 7d, else an open within 1d, logging one conversion event per order - marketing-only, and it never feeds order or accounting totals
func (r *Recorder) RecordConversion(ctx context.Context, cs ConversionSpec) (bool, error) {
	at := orNow(cs.At)
	anchor, ok, err := r.mostRecentEngagement(ctx, cs.ChannelID, cs.CustomerID, at.Add(-clickAttributionWindow))
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	if anchor.eventName == EventEmailOpened && anchor.occurredAt.Before(at.Add(-openAttributionWindow)) { // a bare open only attributes within the shorter window
		return false, nil
	}
	props := map[string]any{
		"order_id":           cs.OrderID.String(),
		"revenue_cents":      cs.RevenueCents,
		"currency":           cs.Currency,
		"via":                anchor.eventName,
		"attributed_send_id": anchor.sendID.String(),
		"attributed_source":  anchor.source,
	}
	if anchor.campaignID != "" {
		props["campaign_id"] = anchor.campaignID
	}
	if anchor.flowID != "" {
		props["flow_id"] = anchor.flowID
	}
	cust := cs.CustomerID
	s := SendRef{ID: anchor.sendID, ChannelID: cs.ChannelID, CustomerID: &cust, Source: anchor.source}
	ok, err = r.record(ctx, EventConversion, s, at, props, "conversion:"+cs.OrderID.String())
	return ok, err
}

type emailAnchor struct { // the last-touch open or click a conversion attributes to, read off the spine event alone
	eventName  string
	occurredAt time.Time
	sendID     uuid.UUID
	source     string
	campaignID string
	flowID     string
}

// A click outranks an open, then most-recent wins, keeping only sends that came from a campaign or flow
// One query: the send's origin was written onto the event, so there is nothing to resolve afterwards
func (r *Recorder) mostRecentEngagement(ctx context.Context, channelID, customerID uuid.UUID, since time.Time) (emailAnchor, bool, error) {
	cands, err := r.spine.RecentEngagementEvents(ctx, channelID, customerID, since)
	if err != nil {
		return emailAnchor{}, false, err
	}
	for _, c := range cands {
		if c.Source != SourceCampaign && c.Source != SourceFlow {
			continue
		}
		return emailAnchor{
			eventName: c.EventName, occurredAt: c.OccurredAt, sendID: c.SendID,
			source: c.Source, campaignID: c.CampaignID, flowID: c.FlowID,
		}, true, nil
	}
	return emailAnchor{}, false, nil
}

// The per-send projection is unaffected, but the Pulse engagement rollups read the raw spine over a lookback window - so keep the retention cutoff well beyond any report window
func (r *Recorder) Prune(ctx context.Context, before time.Time) (int64, error) {
	// identity mappings age out on the same window - identify bumps last_seen on every tracker mount while logged in, so a mapping this stale resolves nothing the spine still holds (and one ancient login shouldn't attribute browsing forever)
	if n, err := r.spine.PruneIdentities(ctx, before); err != nil {
		log.Printf("[analytics] prune identities: %v", err)
	} else if n > 0 {
		refreshSpineIdentities(ctx, r.spine)
		log.Printf("[analytics] pruned %d stale identity mappings", n)
	}
	return r.spine.PruneEvents(ctx, before)
}

// Spine hygiene that must NOT hang off the retention decision - queued ClickHouse erasures still have to run under keep-forever retention, or erasure requests sit unswept
func (r *Recorder) Maintain(ctx context.Context) error {
	if err := r.sweepIdentityErasures(ctx); err != nil {
		return fmt.Errorf("identity erasure sweep: %w", err)
	}
	return nil
}

type LensCellIncrement struct { // a binned heatmap increment from one tracker flush - ClickHouse appends these as raw events where Postgres upserts hot counter rows
	PageKey, Viewport, State, Kind string
	X, Y                           int
	N                              int64
}

type LensElementIncrement struct {
	PageKey, Viewport, Selector, Text string
	N                                 int64
}

// ONE batch per flush, so retransmits collapse per cell on the batch key - `at` has to be as stable as that key (the flush's client timestamp), because the ClickHouse dedupe window is the daily partition and a retransmit stamped with a fresh server time could cross midnight into a partition where it never collapses
func (r *Recorder) RecordLensGrid(ctx context.Context, channelID uuid.UUID, batchKey string, at time.Time, cells []LensCellIncrement, elems []LensElementIncrement) error {
	at = ClientTime(at, time.Now())
	events := make([]SpineEvent, 0, len(cells)+len(elems))
	for _, c := range cells {
		events = append(events, SpineEvent{
			EventName: "lens_cell", OccurredAt: at, ChannelID: &channelID, Source: SourceLens,
			Props:     map[string]any{"page_key": c.PageKey, "viewport": c.Viewport, "state": c.State, "kind": c.Kind, "x": c.X, "y": c.Y, "n": c.N},
			DedupeKey: fmt.Sprintf("lenscell:%s:%s|%s|%s|%s|%d|%d", batchKey, c.PageKey, c.Viewport, c.State, c.Kind, c.X, c.Y),
		})
	}
	for _, e := range elems {
		events = append(events, SpineEvent{
			EventName: "lens_element", OccurredAt: at, ChannelID: &channelID, Source: SourceLens,
			Props:     map[string]any{"page_key": e.PageKey, "viewport": e.Viewport, "selector": e.Selector, "text": e.Text, "n": e.N},
			DedupeKey: fmt.Sprintf("lenselem:%s:%s|%s|%s", batchKey, e.PageKey, e.Viewport, e.Selector),
		})
	}
	return r.spine.AppendEvents(ctx, events)
}

type WebSpec struct { // a behavioural event from the storefront tracker
	EventName  string
	ChannelID  uuid.UUID
	AnonID     string     // the first-party visitor cookie
	CustomerID *uuid.UUID // usually nil
	EventID    string     // client-generated, so ingest is idempotent against sendBeacon retransmits via dedupe_key "web:<EventID>"
	At         time.Time
	Props      map[string]any
}

func (r *Recorder) RecordWeb(ctx context.Context, w WebSpec) (bool, error) { // reports whether a genuinely new row was logged
	cid := w.ChannelID
	return r.spine.AppendEvent(ctx, SpineEvent{
		EventName:  w.EventName,
		OccurredAt: ClientTime(w.At, time.Now()),
		ChannelID:  &cid,
		CustomerID: w.CustomerID,
		AnonID:     w.AnonID,
		Source:     SourceWeb,
		Props:      w.Props,
		DedupeKey:  "web:" + w.EventID,
	})
}

type ActivitySpec struct { // a server-side action, from authenticated flows rather than the anonymous tracker
	EventName  string
	Source     string // SourceCommerce | SourceAccount
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	AnonID     string // for guest commerce: attribute to the anon visitor when no customer
	At         time.Time
	Props      map[string]any
	DedupeKey  string // keeps ingest idempotent against the at-least-once event bus - outbox replays, detached-goroutine retries
}

func (r *Recorder) RecordActivity(ctx context.Context, a ActivitySpec) (bool, error) { // no-op with neither id - an action we can't attribute to anyone has no place on a profile
	if a.Source == SourceAggregate {
		a.CustomerID, a.AnonID = nil, "" // nobody, whatever the caller sent
		for k := range a.Props {
			if !aggregateProps[k] {
				delete(a.Props, k)
			}
		}
	} else if a.CustomerID == nil && a.AnonID == "" {
		return false, nil
	}
	var ch *uuid.UUID
	if a.ChannelID != uuid.Nil {
		cid := a.ChannelID
		ch = &cid
	}
	return r.spine.AppendEvent(ctx, SpineEvent{
		EventName:  a.EventName,
		OccurredAt: orNow(a.At),
		ChannelID:  ch,
		CustomerID: a.CustomerID,
		AnonID:     a.AnonID,
		Source:     a.Source,
		Props:      a.Props,
		DedupeKey:  a.DedupeKey,
	})
}

// Stitches an anonymous visitor to a known customer, upserting last-write-wins on anon_id plus a one-off identify event as a timeline marker - re-firing is idempotent
// No past event is rewritten: profile reads resolve web activity through the mapping, so anonymous activity attributes retroactively
func (r *Recorder) Identify(ctx context.Context, anonID string, customerID, channelID uuid.UUID) error {
	if anonID == "" || customerID == uuid.Nil {
		return nil
	}
	if err := r.spine.UpsertIdentity(ctx, anonID, customerID, channelID); err != nil { // analytics_identities lives on the spine engine, next to the events it stitches, so this goes through the driver
		return err
	}
	cid := customerID
	// The dedupe key below is day-scoped and stamped from this same instant - ClickHouse dedupes within the daily partition, so a forever-stable key with a fresh timestamp would duplicate on every re-login day there while Postgres kept one row
	now := time.Now().UTC()
	_, _ = r.RecordWeb(ctx, WebSpec{
		EventName:  EventIdentify,
		ChannelID:  channelID,
		AnonID:     anonID,
		CustomerID: &cid,
		At:         now,
		EventID:    "identify:" + anonID + ":" + customerID.String() + ":" + now.Format("2006-01-02"),
	})
	return nil
}

func (r *Recorder) record(ctx context.Context, name string, s SendRef, at time.Time, props map[string]any, dedupeKey string) (bool, error) { // false on dedupe
	if props == nil {
		props = map[string]any{}
	}
	if s.CampaignID != nil {
		props["campaign_id"] = s.CampaignID.String()
	}
	if s.FlowID != nil {
		props["flow_id"] = s.FlowID.String()
	}
	return r.spine.AppendEvent(ctx, SpineEvent{
		EventName:  name,
		OccurredAt: at,
		ChannelID:  &s.ChannelID,
		CustomerID: s.CustomerID,
		Source:     s.Source,
		SendID:     &s.ID,
		Props:      props,
		DedupeKey:  dedupeKey,
	})
}

func hitProps(h Hit, extra map[string]any) map[string]any {
	p := map[string]any{"ua": h.UA, "machine": h.Machine}
	if h.Device != "" {
		p["device"] = h.Device
	}
	if h.Client != "" {
		p["client"] = h.Client
	}
	maps.Copy(p, extra)
	return p
}

func uaHash(ua string) string {
	sum := sha1.Sum([]byte(ua))
	return hex.EncodeToString(sum[:])[:8]
}

// The spine partitions by event day, so browser-stamped times are held to a window around the server clock - a forged spread of
// timestamps would otherwise fan one shared insert across more partitions than ClickHouse accepts, failing everyone's events with it
const (
	clientClockBehind = 24 * time.Hour
	clientClockAhead  = 5 * time.Minute
)

// ClientTime is a browser-supplied instant, or now when it is zero or outside the window
func ClientTime(t, now time.Time) time.Time {
	if t.IsZero() || t.Before(now.Add(-clientClockBehind)) || t.After(now.Add(clientClockAhead)) {
		return now
	}
	return t
}

func orNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}
