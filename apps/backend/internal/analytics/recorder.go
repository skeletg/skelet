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

	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
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
)

type Recorder struct {
	q       *db.Queries        // commerce db, primary read-write: traits, identity + orchestration - takes all writes
	qs      *signalsdb.Queries // signals write handle, backing the default pg spine driver
	qr      *db.Queries        // optional dedicated read pool (replica or bulkhead) the heavy read.go aggregations route to - never used for writes
	spine   SpineStore         // the pluggable engine driver, Postgres today and ClickHouse later - ALL spine access goes through it (ingest, identity, every analytical read) and the Recorder only orchestrates spine plus commerce
	archive ArchiveSink        // optional retention-archive destination (the BACKUP_* bucket) - nil makes retention_action=archive fail closed
	// The server-wide Signals switch - every spine write no-ops (web, activity, sends, identity). NewSend/GetSend are not gated by it: the flow engine delivers transactional mail either way
	disabled bool
}

func (r *Recorder) SetReadQueries(qr *db.Queries) { // optional - unset means reads use the primary pool
	r.qr = qr
}

// Gives the default Postgres spine driver its read pool, so spine aggregations don't run on the write pool - no-op for a non-pg driver
func (r *Recorder) SetSpineReadQueries(qsr *signalsdb.Queries) {
	if _, ok := r.spine.(*pgSpine); ok || r.spine == nil {
		r.spine = newPGSpine(r.qs, qsr)
	}
}

// The composition root's engine-selection hook - call at startup, after the pg read pool is wired
func (r *Recorder) SetSpine(s SpineStore) {
	if s != nil {
		r.spine = s
	}
}

// Layers a decorator (the ingest queue) over the selected engine - call LAST in the composition root, since SetSpine and SetSpineReadQueries both replace the store wholesale
func (r *Recorder) WrapSpine(wrap func(SpineStore) SpineStore) { r.spine = wrap(r.spine) }

func (r *Recorder) SpineStore() SpineStore { return r.spine } // the queue consumer writes through the selected engine directly, not through its own producer wrap

func (r *Recorder) SetEnabled(v bool) { r.disabled = !v }

func (r *Recorder) Enabled() bool { return !r.disabled }

func (r *Recorder) read() *db.Queries { // read-only aggregations - writes always use r.q directly
	if r.qr != nil {
		return r.qr
	}
	return r.q
}

func NewRecorder(q *db.Queries, qs *signalsdb.Queries) *Recorder { // defaults to the Postgres spine driver - SetSpine swaps in ClickHouse
	return &Recorder{q: q, qs: qs, spine: newPGSpine(qs, nil)}
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

// SendRef is the send an engagement belongs to, reduced to what the spine stores - the send log is
// core's, and this package never reads it
// The origin comes with the send rather than being resolved later, so the event stays true to it
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
	if r.disabled {
		return nil
	}
	_, err := r.record(ctx, EventEmailSent, s, time.Now(), map[string]any{}, "sent:"+s.ID.String())
	return err
}

// Repeat opens within the same minute from the same client collapse on the dedupe key - the send log's own first-open stamp is core's, written by the caller
func (r *Recorder) RecordOpen(ctx context.Context, s SendRef, h Hit) (bool, error) {
	if r.disabled {
		return false, nil
	}
	at := orNow(h.At)
	key := fmt.Sprintf("open:%s:%d:%s", s.ID, at.Unix()/60, uaHash(h.UA))
	ok, err := r.record(ctx, EventEmailOpened, s, at, hitProps(h, nil), key)
	if err != nil || !ok {
		return false, err
	}
	return true, nil
}

func (r *Recorder) RecordClick(ctx context.Context, s SendRef, url string, linkIndex int, h Hit) (bool, error) {
	if r.disabled {
		return false, nil
	}
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
	if r.disabled {
		return false, nil
	}
	at := orNow(cs.At)
	anchor, ok, err := r.mostRecentEngagement(ctx, cs.CustomerID, at.Add(-clickAttributionWindow))
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
func (r *Recorder) mostRecentEngagement(ctx context.Context, customerID uuid.UUID, since time.Time) (emailAnchor, bool, error) {
	cands, err := r.spine.RecentEngagementEvents(ctx, customerID, since)
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
func (r *Recorder) Maintain(ctx context.Context) {
	if err := r.sweepIdentityErasures(ctx); err != nil {
		log.Printf("[analytics] identity erasure sweep: %v", err)
	}
}

// Pre-creates the send log's upcoming month partitions - core owns the schedule, since the table is commerce-side and fills whether or not Signals runs
func (r *Recorder) EnsureEmailSendPartitions(ctx context.Context) error {
	return r.q.EnsureEmailSendPartitions(ctx)
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
	if r.disabled {
		return nil
	}
	at = orNow(at)
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
	if r.disabled {
		return false, nil
	}
	cid := w.ChannelID
	return r.spine.AppendEvent(ctx, SpineEvent{
		EventName:  w.EventName,
		OccurredAt: orNow(w.At),
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
	if r.disabled || (a.CustomerID == nil && a.AnonID == "") {
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
	if r.disabled || anonID == "" || customerID == uuid.Nil {
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

func orNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}

func nu(p *uuid.UUID) uuid.NullUUID {
	if p == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}

// The zero-uuid twin of nu for array params, where NULLIF in the SQL restores the null
func zu(p *uuid.UUID) uuid.UUID {
	if p == nil {
		return uuid.Nil
	}
	return *p
}

func pu(n uuid.NullUUID) *uuid.UUID {
	if !n.Valid {
		return nil
	}
	id := n.UUID
	return &id
}
