// Package lens is web behaviour analytics - the second product on the analytics spine after Pulse
// Page-views and custom events land on the append-only spine (source='web') while the high-frequency signals (clicks, cursor-moves, scroll) bin into bounded aggregate tables - Writes are best-effort and the package owns no transactional state
package lens

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Registered in register.go under pulse/lens
const (
	cfgEnabled            = "signals/lens/enabled"
	cfgSamplingRate       = "signals/lens/sampling_rate"
	cfgCaptureMoves       = "signals/lens/capture_moves"
	cfgReplayEnabled      = "signals/lens/replay_enabled"
	cfgReplaySamplingRate = "signals/lens/replay_sampling_rate"
	cfgReplayMaxMinutes   = "signals/lens/replay_max_minutes"
)

// Object storage for the large gzip blobs, replay chunks and DOM snapshots - the rows keep only the key - satisfied by *objstore.Storage
type BlobStore interface {
	PutBytes(ctx context.Context, key string, data []byte, contentType string) error
	GetBytes(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// Bounds one /t/rec flush's payload so a runaway recording can't blow up a chunk row - oversized flushes are dropped whole
const maxReplayChunkBytes = 5 << 20 // 5 MiB

// Heatmap grid resolution in CSS px at a viewport bucket's fixed render width - document coordinates floor into cells of this size
const cellSize = 10

// Reserved custom-event names the storefront emits for UX signals - rage and dead clicks, JS errors, web vitals - routed out of the generic interaction path
const (
	sigRageClick = "$rage_click"
	sigDeadClick = "$dead_click"
	sigJSError   = "$js_error"
	sigWebVitals = "$web_vitals"
	sigSearch    = "$search"
)

// q is the primary read-write pool taking ingest upserts, snapshots and prunes; qr is an optional replica or bulkheaded pool used only by read aggregations, never for writes
type Service struct {
	q      *signalsdb.Queries // signals db: lens_* + browse_seen/browse_notified (ingest + reads)
	qr     *signalsdb.Queries // optional signals read pool for the aggregations
	rec    *analytics.Recorder
	cfg    Settings
	rollup *analytics.TrafficRollup
	blobs  BlobStore // nil only on the CLI and test paths, where ingest drops blobs instead
}

// Settings is the slice of the store's settings Lens reads, declared here so the package depends on the shape rather than on whoever holds them
// A nil channel asks for the store-wide value, and a nil Settings falls back to permissive defaults
type Settings interface {
	Int(ctx context.Context, path string, channelID *uuid.UUID) (int64, error)
	Bool(ctx context.Context, path string, channelID *uuid.UUID) (bool, error)
	String(ctx context.Context, path string, channelID *uuid.UUID) (string, error)
}

func NewService(q *signalsdb.Queries, rec *analytics.Recorder, cfg Settings, rollup *analytics.TrafficRollup) *Service {
	return &Service{q: q, rec: rec, cfg: cfg, rollup: rollup}
}

// Optional - when unset, reads use the primary pool
func (s *Service) SetReadQueries(qr *signalsdb.Queries) { s.qr = qr }

// The commerce-db handle used only by the cross-db BrowseAbandoners scanner, which joins orders, carts and identities - unset on CLI and test paths

func (s *Service) SetBlobStore(b BlobStore) { s.blobs = b }

// Deterministic object keys derived from a row's identity, so writes are idempotent and the prune path can recompute a key
// Snapshot PK columns are hashed into one path-safe segment, since page_key contains slashes
func chunkBlobKey(sessionID uuid.UUID, seq int32) string {
	return fmt.Sprintf("lens/replay/%s/%d.gz", sessionID, seq)
}
func snapshotBlobKey(channelID uuid.UUID, pageKey, viewport, state, layoutHash string) string {
	sum := sha256.Sum256([]byte(pageKey + "|" + viewport + "|" + state + "|" + layoutHash))
	return fmt.Sprintf("lens/snapshot/%s/%s.gz", channelID, hex.EncodeToString(sum[:16]))
}

// Read-only aggregations only - ingest, snapshot and prune always use s.q directly
func (s *Service) read() *signalsdb.Queries {
	if s.qr != nil {
		return s.qr
	}
	return s.q
}

// One behavioural event in an ingest batch, the wire shape the storefront tracker posts to /t - fields are sparse, only those relevant to the event Type are set
type Event struct {
	ID         string         `json:"id"`   // client-generated; idempotency key
	Type       string         `json:"type"` // page_view | custom | click | move | scroll | snapshot
	PageKey    string         `json:"pageKey"`
	Title      string         `json:"title"`
	Referrer   string         `json:"referrer"`
	Viewport   string         `json:"viewport"` // desktop | tablet | mobile
	X          int            `json:"x"`
	Y          int            `json:"y"`
	Depth      int            `json:"depth"`
	Selector   string         `json:"selector"`
	Text       string         `json:"text"`
	State      string         `json:"state"` // UI state key (default | sidebar/modal name); empty → "default"
	LayoutHash string         `json:"layoutHash"`
	HTML       string         `json:"html"` // gzip+base64 serialized DOM (snapshot)
	Width      int            `json:"width"`
	Height     int            `json:"height"`
	TS         int64          `json:"ts"`         // epoch ms (optional)
	Name       string         `json:"name"`       // custom event name
	CustomerID string         `json:"customerId"` // identify: links this visitor to a customer
	Props      map[string]any `json:"props"`
}

type Batch struct {
	AnonID string  `json:"anonId"`
	Events []Event `json:"events"`
}

// One /t/rec flush - raw rrweb events for a session plus the metadata to attribute and index the recording - Events stays raw JSON: the server gzips it verbatim and reads only each event's timestamp, for the chunk's time range
type ReplayBatch struct {
	SessionID string          `json:"sessionId"`
	AnonID    string          `json:"anonId"`
	Seq       int32           `json:"seq"` // monotonic per-session (client sessionStorage)
	Page      string          `json:"page"`
	Viewport  string          `json:"viewport"`
	Events    json.RawMessage `json:"events"`
}

// Gzips the events into an append-only chunk and rolls the session metadata forward - best-effort, since the handler always 204s, so a malformed or oversized flush is dropped whole
// verifiedCustomer is the customer the request's session token proves, the only identity a recording is attached to - the body's customerId is forgeable
func (s *Service) IngestReplay(ctx context.Context, channelID uuid.UUID, userAgent string, b ReplayBatch, verifiedCustomer uuid.UUID) {
	if len(b.Events) == 0 || len(b.Events) > maxReplayChunkBytes {
		return
	}
	sid, err := uuid.Parse(b.SessionID)
	if err != nil || b.AnonID == "" {
		return
	}
	// Just the timestamps - rrweb events all carry one in epoch ms - for the chunk's time range and event count
	var stamped []struct {
		Timestamp int64 `json:"timestamp"`
	}
	if err := json.Unmarshal(b.Events, &stamped); err != nil || len(stamped) == 0 {
		return
	}
	firstTS, lastTS := stamped[0].Timestamp, stamped[0].Timestamp
	for _, e := range stamped {
		if e.Timestamp < firstTS {
			firstTS = e.Timestamp
		}
		if e.Timestamp > lastTS {
			lastTS = e.Timestamp
		}
	}

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b.Events); err != nil {
		return
	}
	if err := zw.Close(); err != nil {
		return
	}

	var cust uuid.NullUUID
	if verifiedCustomer != uuid.Nil {
		cust = uuid.NullUUID{UUID: verifiedCustomer, Valid: true}
	}
	if len(userAgent) > 400 {
		userAgent = userAgent[:400]
	}

	n, err := s.q.LensUpsertSession(ctx, signalsdb.LensUpsertSessionParams{
		ID:          sid,
		ChannelID:   channelID,
		AnonID:      b.AnonID,
		CustomerID:  cust,
		StartedAt:   time.UnixMilli(firstTS),
		LastEventAt: time.UnixMilli(lastTS),
		EventCount:  int64(len(stamped)),
		EntryPage:   b.Page,
		ExitPage:    b.Page,
		UserAgent:   userAgent,
		Viewport:    b.Viewport,
	})
	if err != nil || n == 0 {
		return
	}
	if s.blobs == nil {
		return
	}
	key := chunkBlobKey(sid, b.Seq)
	if err := s.blobs.PutBytes(ctx, key, buf.Bytes(), "application/octet-stream"); err != nil {
		return // best-effort: drop the chunk rather than insert a row pointing at nothing
	}
	_ = s.q.LensInsertChunk(ctx, signalsdb.LensInsertChunkParams{
		SessionID:  sid,
		Seq:        b.Seq,
		BlobKey:    key,
		EventCount: int32(len(stamped)),
		FirstTs:    firstTS,
		LastTs:     lastTS,
	})
}

type cellKey struct {
	page, viewport, state, kind string
	x, y                        int
}
type elemKey struct {
	page, viewport, selector string
}

// Routes a batch onto the spine and the aggregate tables, best-effort so one bad event never drops the rest page_view and custom land on the spine; click, move and scroll bin into the grid, pre-aggregated here so the upsert loop is bounded by distinct cells rather than raw points; snapshots upsert once per layout verifiedCustomer is the customer proven by the request's session token, and the identify path stitches ONLY to it - the batch body's customerId is client-supplied and forgeable, and trusting it let any request bind any anon id to any customer, poisoning that profile and triggering attacker-chosen marketing email through the browse-abandonment scanner
func (s *Service) Ingest(ctx context.Context, channelID uuid.UUID, anonID string, hit analytics.Hit, b Batch, verifiedCustomer uuid.UUID) {
	captureMoves := s.captureMoves(ctx, channelID)
	cells := map[cellKey]int64{}
	elems := map[elemKey]struct {
		text string
		n    int64
	}{}

	for _, e := range b.Events {
		state := stateOr(e.State)
		switch e.Type {
		case analytics.EventPageView:
			s.recordView(ctx, channelID, anonID, hit, e)
		case analytics.EventCustom:
			s.recordCustom(ctx, channelID, anonID, e)
		case analytics.EventIdentify:
			// Stitch only to the session-verified customer - no token, no stitch
			if verifiedCustomer != uuid.Nil && anonID != "" {
				_ = s.rec.Identify(ctx, anonID, verifiedCustomer, channelID)
			}
		case "click":
			// Binned per the click's UI state, resolved on the client at click time, so overlay clicks land on the open-state snapshot and default clicks on the default one
			cells[cellKey{e.PageKey, e.Viewport, state, "click", e.X / cellSize, e.Y / cellSize}]++
			if e.Selector != "" {
				k := elemKey{e.PageKey, e.Viewport, e.Selector}
				v := elems[k]
				v.n++
				v.text = e.Text
				elems[k] = v
			}
		case "move":
			if captureMoves {
				cells[cellKey{e.PageKey, e.Viewport, state, "move", e.X / cellSize, e.Y / cellSize}]++
			}
		case "scroll":
			cells[cellKey{e.PageKey, e.Viewport, state, "scroll", 0, clampDepth(e.Depth)}]++
		case "snapshot":
			s.storeSnapshot(ctx, channelID, state, e)
		}
	}

	// The grid goes to the spine as raw increments - a viral page would turn per-cell upserts into hot-row lock contention
	// Key and timestamp both come from the first event, so a sendBeacon retransmit reproduces the flush byte-for-byte and collapses on the same dedupe key
	batchKey, at := anonID, time.Time{}
	for _, e := range b.Events {
		if e.ID != "" {
			batchKey, at = e.ID, msToTime(e.TS)
			break
		}
	}
	cellsInc := make([]analytics.LensCellIncrement, 0, len(cells))
	for k, n := range cells {
		cellsInc = append(cellsInc, analytics.LensCellIncrement{PageKey: k.page, Viewport: k.viewport, State: k.state, Kind: k.kind, X: k.x, Y: k.y, N: n})
	}
	elemsInc := make([]analytics.LensElementIncrement, 0, len(elems))
	for k, v := range elems {
		elemsInc = append(elemsInc, analytics.LensElementIncrement{PageKey: k.page, Viewport: k.viewport, Selector: k.selector, Text: v.text, N: v.n})
	}
	_ = s.rec.RecordLensGrid(ctx, channelID, batchKey, at, cellsInc, elemsInc)
}

func (s *Service) storeSnapshot(ctx context.Context, channelID uuid.UUID, state string, e Event) {
	if e.HTML == "" || e.LayoutHash == "" {
		return
	}
	if s.blobs == nil {
		return
	}
	key := snapshotBlobKey(channelID, e.PageKey, e.Viewport, state, e.LayoutHash)
	if err := s.blobs.PutBytes(ctx, key, gzipBytes([]byte(e.HTML)), "application/octet-stream"); err != nil {
		return
	}
	_ = s.q.LensUpsertSnapshot(ctx, signalsdb.LensUpsertSnapshotParams{
		ChannelID: channelID, PageKey: e.PageKey, ViewportBucket: e.Viewport, StateKey: state,
		LayoutHash: e.LayoutHash, BlobKey: key, Width: int32(e.Width), Height: int32(e.Height),
	})
}

func stateOr(s string) string {
	if s == "" {
		return "default"
	}
	return s
}

// The cursor-move firehose kill-switch, defaulting true and defended server-side even though the client honors it too
func (s *Service) captureMoves(ctx context.Context, channelID uuid.UUID) bool {
	if s.cfg == nil {
		return true
	}
	if v, err := s.cfg.Bool(ctx, cfgCaptureMoves, &channelID); err == nil {
		return v
	}
	return true
}

func clampDepth(d int) int {
	if d < 0 {
		return 0
	}
	if d > 100 {
		return 100
	}
	return d
}

func (s *Service) recordView(ctx context.Context, channelID uuid.UUID, anonID string, hit analytics.Hit, e Event) {
	props := map[string]any{}
	// Structured page context the page annotated itself with (entity, entity_name, entity_image, entity_price, sku), powering rich timeline rows - merged first so the server-authoritative keys below always win
	maps.Copy(props, e.Props)
	props["page_key"] = e.PageKey
	props["viewport"] = e.Viewport
	if e.Title != "" {
		props["title"] = e.Title
	}
	if e.Referrer != "" {
		props["referrer"] = e.Referrer
	}
	if hit.Device != "" {
		props["device"] = hit.Device
	}
	if hit.Client != "" {
		props["client"] = hit.Client
	}
	if hit.OS != "" {
		props["os"] = hit.OS
	}
	if hit.Country != "" {
		props["country"] = hit.Country
	}
	// Truncated before storage, data minimisation on the long-retention spine - geo enrichment already ran on the full address
	if ip := analytics.AnonymizeIP(hit.IP); ip != "" {
		props["ip"] = ip
	}
	// A bot that renders JS fires the tracker too, so flag it and keep human Lens metrics honest - comprehensive crawl analytics come from the server-side beacon instead
	if hit.IsBot {
		props["is_bot"] = true
		if hit.BotName != "" {
			props["bot_name"] = hit.BotName
		}
		if hit.BotCat != "" {
			props["bot_category"] = hit.BotCat
		}
	} else if s.rollup != nil {
		// Feeds the bot dashboard's bot-vs-human counter, the Lens-hub view breakdowns and the visitor seen-table, so the hub reads bounded rollups rather than raw spine events - bots are excluded, their crawls counted via the beacon
		s.rollup.AddHumanView(channelID, e.PageKey, hit.Device, e.Referrer, anonID, hit.At)
		// Product views also feed the browse-abandonment scanner, anon-keyed and resolved to a customer at scan time via the identity table
		if anonID != "" && asString(props["entity"]) == "product" {
			s.rollup.AddBrowse(channelID, anonID, e.PageKey, asString(props["sku"]), asString(props["entity_name"]), asString(props["entity_image"]), hit.At)
		}
	}
	_, _ = s.rec.RecordWeb(ctx, analytics.WebSpec{
		EventName: analytics.EventPageView,
		ChannelID: channelID,
		AnonID:    anonID,
		EventID:   orEventID(e.ID),
		At:        msToTime(e.TS),
		Props:     props,
	})
}

func (s *Service) recordCustom(ctx context.Context, channelID uuid.UUID, anonID string, e Event) {
	props := map[string]any{"name": e.Name, "page_key": e.PageKey}
	maps.Copy(props, e.Props)
	// Promote known semantic browsing events, currently facet and filter selections, out of the generic "custom" channel to a first-class spine event so they're countable and labelable on the Synapse profile
	eventName := analytics.EventCustom
	switch e.Name {
	case analytics.EventProductFilter:
		eventName = analytics.EventProductFilter
	case analytics.EventViewItem, analytics.EventBeginCheckout, analytics.EventPurchase:
		// Commerce funnel events get a first-class spine name so the Synapse funnel can count checkout starts - buried as 'custom' there would be no step between cart_add and order_placed
		// Client-emitted, so consent-gated and sampled - the server-side cart_* and order events stay the unsampled source of truth, these just add browse-to-checkout visibility
		eventName = e.Name
	case sigRageClick, sigDeadClick, sigJSError, sigWebVitals:
		// Reserved UX signals roll into their own dim and bump the recording's counter, but do NOT feed the interactions list - still stored on the spine, whose raw props drive the errors list and vitals percentiles
		s.recordSignal(ctx, channelID, e)
	case sigSearch:
		// On-site search rolls the normalized term into dim='search', plus 'search_zero' when it returned nothing - kept out of the interactions list, with raw props on the spine for a future search-to-conversion funnel
		s.recordSearch(ctx, channelID, e)
	default:
		if s.rollup != nil {
			// Named UI interactions stay 'custom' and feed the hub's interactions list
			s.rollup.AddInteraction(channelID, e.Name, msToTime(e.TS))
		}
	}
	_, _ = s.rec.RecordWeb(ctx, analytics.WebSpec{
		EventName: eventName,
		ChannelID: channelID,
		AnonID:    anonID,
		EventID:   orEventID(e.ID),
		At:        msToTime(e.TS),
		Props:     props,
	})
}

// Rolls page-level counts into lens_traffic_daily under the signal's own dim and bumps the recording's per-session counter when the event carries a replay sessionId - web vitals get neither, their percentiles come from raw spine props
func (s *Service) recordSignal(ctx context.Context, channelID uuid.UUID, e Event) {
	var dim, kind string
	switch e.Name {
	case sigRageClick:
		dim, kind = "rage", "rage"
	case sigDeadClick:
		dim, kind = "dead", "dead"
	case sigJSError:
		dim, kind = "error", "error"
	}
	if dim != "" && s.rollup != nil {
		s.rollup.AddSignal(channelID, dim, e.PageKey, msToTime(e.TS))
	}
	if kind != "" {
		if sid, ok := e.Props["sessionId"].(string); ok && sid != "" {
			if su, err := uuid.Parse(sid); err == nil {
				_ = s.q.LensBumpSessionSignal(ctx, signalsdb.LensBumpSessionSignalParams{Kind: kind, ID: su, ChannelID: channelID})
			}
		}
	}
}

// dim='search' keyed by the normalized term, plus dim='search_zero' when the search returned nothing - the merchandising-gap signal
func (s *Service) recordSearch(ctx context.Context, channelID uuid.UUID, e Event) {
	if s.rollup == nil {
		return
	}
	term := normalizeQuery(propString(e.Props["query"]))
	if term == "" {
		return
	}
	at := msToTime(e.TS)
	s.rollup.AddSignal(channelID, "search", term, at)
	if resultsZero(e.Props["results"]) {
		s.rollup.AddSignal(channelID, "search_zero", term, at)
	}
}

// Lower-cases, trims, collapses internal whitespace and caps at 80 runes - bounds rollup key cardinality and groups "Red Shoes" with "red  shoes"
func normalizeQuery(q string) string {
	q = strings.ToLower(strings.Join(strings.Fields(q), " "))
	if r := []rune(q); len(r) > 80 {
		q = string(r[:80])
	}
	return q
}

func propString(v any) string {
	s, _ := v.(string)
	return s
}

// A missing or unknown value counts as non-zero, so we never falsely flag a zero-result search
func resultsZero(v any) bool {
	switch n := v.(type) {
	case float64:
		return n == 0
	case int:
		return n == 0
	case int64:
		return n == 0
	default:
		return false
	}
}

// The GET /t/config response - the knobs the storefront tracker honors - Defaults are permissive so a fresh install tracks out of the box
type ClientConfig struct {
	Enabled            bool `json:"enabled"`
	SamplingRate       int  `json:"samplingRate"`
	CaptureMoves       bool `json:"captureMoves"`
	ReplayEnabled      bool `json:"replayEnabled"`
	ReplaySamplingRate int  `json:"replaySamplingRate"` // anonymous-visitor sample %; logged-in always recorded
	ReplayMaxMinutes   int  `json:"replayMaxMinutes"`   // per-session recording cap; 0 = no cap
}

func (s *Service) IngestConfig(ctx context.Context, channelID uuid.UUID) ClientConfig {
	cfg := ClientConfig{Enabled: true, SamplingRate: 100, CaptureMoves: true, ReplayEnabled: true, ReplaySamplingRate: 10, ReplayMaxMinutes: 30}
	if s.cfg == nil {
		return cfg
	}
	if v, err := s.cfg.Bool(ctx, cfgEnabled, &channelID); err == nil {
		cfg.Enabled = v
	}
	if v, err := s.cfg.Int(ctx, cfgSamplingRate, &channelID); err == nil && v >= 0 {
		cfg.SamplingRate = int(v)
	}
	if v, err := s.cfg.Bool(ctx, cfgCaptureMoves, &channelID); err == nil {
		cfg.CaptureMoves = v
	}
	if v, err := s.cfg.Bool(ctx, cfgReplayEnabled, &channelID); err == nil {
		cfg.ReplayEnabled = v
	}
	if v, err := s.cfg.Int(ctx, cfgReplaySamplingRate, &channelID); err == nil && v >= 0 {
		cfg.ReplaySamplingRate = int(v)
	}
	if v, err := s.cfg.Int(ctx, cfgReplayMaxMinutes, &channelID); err == nil && v >= 0 {
		cfg.ReplayMaxMinutes = int(v)
	}
	return cfg
}

// --- read aggregates (dashboard) ---

// One row of the top-pages ranking - Views and Visitors are windowed while Clicks and AvgScroll are cumulative grid totals, since the grid has no per-hit timestamp
// Device is per-pageview, so it only shows in the channel-wide split, not per page where a single value would mislead
type PageStat struct {
	PageKey   string
	Views     int64
	Visitors  int64
	Clicks    int64
	AvgScroll int // 0..100
}

type LabelCount struct {
	Label string
	Views int64
}

type DayViews struct {
	Date  string
	Views int64
}

type Overview struct {
	Views        int64
	Visitors     int64
	Daily        []DayViews
	TopPages     []PageStat
	Devices      []LabelCount
	Referrers    []LabelCount
	Interactions []LabelCount
}

// Reads the daily rollup and visitor seen-table, never raw spine events - per-page unique visitors aren't in the rollup yet, so PageStat.Visitors is 0
func (s *Service) Overview(ctx context.Context, channelID uuid.UUID, since time.Time) (Overview, error) {
	views, err := s.read().LensTotalViews(ctx, signalsdb.LensTotalViewsParams{ChannelID: channelID, Since: since})
	if err != nil {
		return Overview{}, err
	}
	out := Overview{Views: views}
	if v, err := s.read().LensVisitorCount(ctx, signalsdb.LensVisitorCountParams{ChannelID: channelID, Since: since}); err == nil {
		out.Visitors = v
	}
	if rows, err := s.read().LensDailyViews(ctx, signalsdb.LensDailyViewsParams{ChannelID: channelID, Since: since}); err == nil {
		for _, r := range rows {
			out.Daily = append(out.Daily, DayViews{Date: r.Day, Views: r.Views})
		}
	}
	if pages, err := s.read().LensTopPages(ctx, signalsdb.LensTopPagesParams{ChannelID: channelID, Since: since, Lim: 50}); err == nil {
		// Per-page engagement - cumulative clicks and avg scroll from the grid
		type eng struct {
			clicks    int64
			avgScroll int
		}
		engByPage := map[string]eng{}
		if grids, err := s.rec.LensPagesGridFromEvents(ctx, channelID); err == nil {
			for page, g := range grids {
				avg := 0
				if g.ScrollTotal > 0 {
					avg = int(g.ScrollWeight / g.ScrollTotal)
				}
				engByPage[page] = eng{clicks: g.Clicks, avgScroll: avg}
			}
		}
		for _, p := range pages {
			e := engByPage[p.PageKey]
			out.TopPages = append(out.TopPages, PageStat{
				PageKey: p.PageKey, Views: p.Views,
				Clicks: e.clicks, AvgScroll: e.avgScroll,
			})
		}
	}
	if devs, err := s.read().LensDeviceSplit(ctx, signalsdb.LensDeviceSplitParams{ChannelID: channelID, Since: since}); err == nil {
		for _, d := range devs {
			out.Devices = append(out.Devices, LabelCount{Label: d.Device, Views: d.Views})
		}
	}
	if refs, err := s.read().LensTopReferrers(ctx, signalsdb.LensTopReferrersParams{ChannelID: channelID, Since: since, Lim: 10}); err == nil {
		for _, r := range refs {
			out.Referrers = append(out.Referrers, LabelCount{Label: r.Referrer, Views: r.Views})
		}
	}
	if ix, err := s.read().LensTopInteractions(ctx, signalsdb.LensTopInteractionsParams{ChannelID: channelID, Since: since, Lim: 15}); err == nil {
		for _, r := range ix {
			out.Interactions = append(out.Interactions, LabelCount{Label: r.Name, Views: r.Count})
		}
	}
	return out, nil
}

type BotAgent struct {
	Name     string
	Category string
	Hits     int64
	Pages    int64
	LastSeen time.Time
}

type BotPage struct {
	PageKey string
	Hits    int64
	Bots    int64
}

type BotDayPoint struct {
	Day        string
	BotHits    int64
	HumanViews int64
}

// The crawler-analytics rollup for a channel and window - bot-vs-human totals, the per-agent breakdown, the most-crawled pages (optionally for one bot), and a daily series
type BotTraffic struct {
	BotHits    int64
	HumanViews int64
	BotCount   int64
	PageCount  int64
	Bots       []BotAgent
	TopPages   []BotPage
	Daily      []BotDayPoint
}

// `bot` filters TopPages to one agent - empty means all bots
func (s *Service) BotTraffic(ctx context.Context, channelID uuid.UUID, since time.Time, bot string) (BotTraffic, error) {
	sum, err := s.read().BotTrafficSummary(ctx, signalsdb.BotTrafficSummaryParams{ChannelID: channelID, Since: since})
	if err != nil {
		return BotTraffic{}, err
	}
	out := BotTraffic{BotHits: sum.BotHits, HumanViews: sum.HumanViews, BotCount: sum.BotCount, PageCount: sum.PageCount}
	if rows, err := s.read().BotByAgent(ctx, signalsdb.BotByAgentParams{ChannelID: channelID, Since: since}); err == nil {
		for _, r := range rows {
			ls, _ := r.LastSeen.(time.Time)
			out.Bots = append(out.Bots, BotAgent{
				Name: r.BotName, Category: r.BotCategory,
				Hits: r.Hits, Pages: r.Pages, LastSeen: ls,
			})
		}
	}
	if rows, err := s.read().BotTopPages(ctx, signalsdb.BotTopPagesParams{ChannelID: channelID, Since: since, BotName: bot, Lim: 25}); err == nil {
		for _, r := range rows {
			out.TopPages = append(out.TopPages, BotPage{PageKey: r.PageKey, Hits: r.Hits, Bots: r.Bots})
		}
	}
	if rows, err := s.read().BotDailySeries(ctx, signalsdb.BotDailySeriesParams{ChannelID: channelID, Since: since}); err == nil {
		for _, r := range rows {
			out.Daily = append(out.Daily, BotDayPoint{Day: r.Day, BotHits: r.BotHits, HumanViews: r.HumanViews})
		}
	}
	return out, nil
}

// A customer who browsed a product and didn't cart or buy - the scanner's output, carrying the last browsed product for the recovery email's merge tags
type BrowseAbandoner struct {
	CustomerID   uuid.UUID
	ChannelID    uuid.UUID
	PageKey      string
	Sku          string
	ProductName  string
	ProductImage string
	LastBrowseAt time.Time
}

// Resolves idle product browses to the customers who made them, minus anyone already notified for that browse
// Bounded pages, newest-first: the browse rows are anon-keyed and the spine engine resolves each page to customers
func (s *Service) ListBrowseAbandoners(ctx context.Context, channelID uuid.UUID, threshold, maxAge time.Time, limit int32) ([]BrowseAbandoner, error) {
	const pageSize = 10000
	var cands []BrowseAbandoner
	seen := make(map[uuid.UUID]bool)
	// keyset cursor starts above any real row - browse times are event times, never a day in the future
	afterAt, afterAnon := time.Now().UTC().Add(24*time.Hour), ""
	for {
		rows, err := s.read().BrowseSeenSincePage(ctx, signalsdb.BrowseSeenSincePageParams{
			MaxAge: maxAge, AfterAt: afterAt, AfterAnon: afterAnon, Lim: pageSize,
			ChannelID: uuid.NullUUID{UUID: channelID, Valid: channelID != uuid.Nil},
		})
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		anonIDs := make([]string, len(rows))
		for i, r := range rows {
			anonIDs[i] = r.AnonID
		}
		resolved, err := s.rec.CustomersForAnonIDs(ctx, anonIDs)
		if err != nil {
			return nil, err
		}
		// newest-first across pages, so a customer's first resolved row is their latest browse - only an IDLE latest (before the threshold) is an abandonment, a fresher one means they're still browsing
		for _, r := range rows {
			cust, ok := resolved[r.AnonID]
			if !ok || seen[cust] {
				continue
			}
			seen[cust] = true
			if !r.LastBrowseAt.Before(threshold) {
				continue
			}
			cands = append(cands, BrowseAbandoner{
				CustomerID: cust, ChannelID: r.ChannelID, PageKey: r.PageKey,
				Sku: r.Sku, ProductName: r.ProductName, ProductImage: r.ProductImage, LastBrowseAt: r.LastBrowseAt,
			})
		}
		last := rows[len(rows)-1]
		afterAt, afterAnon = last.LastBrowseAt, last.AnonID
		if len(rows) < pageSize {
			break
		}
		// the ones left behind are the STALEST idlers - they surface on a later scan as notified customers
		// drop out of candidacy, while the seen-map and slice stay bounded on a busy window
		if limit > 0 && len(cands) >= int(limit) {
			break
		}
	}
	if len(cands) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(cands))
	ats := make([]time.Time, len(cands))
	for i, c := range cands {
		ids[i], ats[i] = c.CustomerID, c.LastBrowseAt
	}
	notifiedRows, err := s.read().BrowseNotifiedForCustomers(ctx, ids)
	if err != nil {
		return nil, err
	}
	type notifyKey struct{ channel, customer uuid.UUID }
	notified := make(map[notifyKey]time.Time, len(notifiedRows))
	for _, n := range notifiedRows {
		notified[notifyKey{n.ChannelID, n.CustomerID}] = n.LastBrowseAt
	}
	out := make([]BrowseAbandoner, 0, limit)
	for _, c := range cands {
		if at, ok := notified[notifyKey{c.ChannelID, c.CustomerID}]; ok && !at.Before(c.LastBrowseAt) {
			continue
		}
		out = append(out, c)
		if int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

// Records that a customer was enrolled for a browse at that time, so the episode fires at most once - a newer browse re-fires it
func (s *Service) MarkBrowseNotified(ctx context.Context, channelID, customerID uuid.UUID, lastBrowseAt time.Time) error {
	return s.q.MarkBrowseNotified(ctx, signalsdb.MarkBrowseNotifiedParams{ChannelID: channelID, CustomerID: customerID, LastBrowseAt: lastBrowseAt})
}

// --- heatmap reads (Stage B) ---

// One grid bucket - cell coordinates, multiply by cellSize for px, plus a hit count - For scroll, X is always 0 and Y is the depth percentage
type Cell struct {
	X, Y  int
	Count int64
}

type ElementClick struct {
	Selector string
	Text     string
	Count    int64
}

// A trafficked page for the viewer's page picker - same source as the overview's Top Pages, so the two lists stay consistent
type PageEntry struct {
	PageKey string
	Views   int64
}

// Everything the dashboard needs for one page and viewport - the snapshot backdrop as JSON for rrweb rebuild, its dimensions, the click/move/scroll density grids, and the clicked-element ranking
type Heatmap struct {
	SnapshotJSON string
	Width        int
	Height       int
	Click        []Cell
	Move         []Cell
	Scroll       []Cell
	Elements     []ElementClick
}

func (s *Service) ListPages(ctx context.Context, channelID uuid.UUID) ([]PageEntry, error) {
	rows, err := s.rec.LensListPages(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]PageEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, PageEntry{PageKey: r.Value, Views: r.Count})
	}
	return out, nil
}

func (s *Service) Viewports(ctx context.Context, channelID uuid.UUID, pageKey string) ([]string, error) {
	return s.read().LensViewports(ctx, signalsdb.LensViewportsParams{ChannelID: channelID, PageKey: pageKey})
}

func (s *Service) PageDevices(ctx context.Context, channelID uuid.UUID, pageKey string) ([]LabelCount, error) {
	rows, err := s.rec.LensPageDeviceSplit(ctx, channelID, pageKey)
	if err != nil {
		return nil, err
	}
	out := make([]LabelCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, LabelCount{Label: r.Value, Views: r.Count})
	}
	return out, nil
}

type TimePoint struct {
	Date  string
	Views int64
}

// The rich per-page bundle for the heatmap detail view - view totals, daily timeline and referrer/client/device splits from the spine, plus cumulative click and scroll engagement from the grid
type PageStatsResult struct {
	Views     int64
	Visitors  int64
	Clicks    int64
	AvgScroll int
	Timeline  []TimePoint
	Referrers []LabelCount
	Clients   []LabelCount
	Devices   []LabelCount
}

func (s *Service) PageStats(ctx context.Context, channelID uuid.UUID, pageKey string) (PageStatsResult, error) {
	var res PageStatsResult
	if views, visitors, err := s.rec.LensPageTotals(ctx, channelID, pageKey); err == nil {
		res.Views, res.Visitors = views, visitors
	}
	if g, err := s.rec.LensPageGridFromEvents(ctx, channelID, pageKey); err == nil {
		res.Clicks = g.Clicks
		if g.ScrollTotal > 0 {
			res.AvgScroll = int(g.ScrollWeight / g.ScrollTotal)
		}
	}
	if rows, err := s.rec.LensPageTimeline(ctx, channelID, pageKey, time.Now().AddDate(0, 0, -30)); err == nil {
		for _, r := range rows {
			res.Timeline = append(res.Timeline, TimePoint{Date: r.Date, Views: r.Views})
		}
	}
	if rows, err := s.rec.LensPageReferrers(ctx, channelID, pageKey); err == nil {
		for _, r := range rows {
			res.Referrers = append(res.Referrers, LabelCount{Label: r.Value, Views: r.Count})
		}
	}
	if rows, err := s.rec.LensPageClients(ctx, channelID, pageKey); err == nil {
		for _, r := range rows {
			res.Clients = append(res.Clients, LabelCount{Label: r.Value, Views: r.Count})
		}
	}
	if devs, err := s.PageDevices(ctx, channelID, pageKey); err == nil {
		res.Devices = devs
	}
	return res, nil
}

// Captured UI states for a page and viewport, 'default' first
func (s *Service) States(ctx context.Context, channelID uuid.UUID, pageKey, viewport string) ([]string, error) {
	return s.read().LensStates(ctx, signalsdb.LensStatesParams{ChannelID: channelID, PageKey: pageKey, ViewportBucket: viewport})
}

// Returns ok=false when that state has no snapshot yet
func (s *Service) Heatmap(ctx context.Context, channelID uuid.UUID, pageKey, viewport, state string) (Heatmap, bool, error) {
	state = stateOr(state)
	snap, err := s.read().LensLatestSnapshot(ctx, signalsdb.LensLatestSnapshotParams{ChannelID: channelID, PageKey: pageKey, ViewportBucket: viewport, StateKey: state})
	if err != nil {
		return Heatmap{}, false, nil // no snapshot yet - not an error to the caller
	}
	if s.blobs == nil {
		return Heatmap{}, false, nil
	}
	htmlGz, err := s.blobs.GetBytes(ctx, snap.BlobKey)
	if err != nil {
		return Heatmap{}, false, nil // the object is gone, so neither is the heatmap
	}
	hm := Heatmap{SnapshotJSON: string(gunzipBytes(htmlGz)), Width: int(snap.Width), Height: int(snap.Height)}
	hm.Click = s.cells(ctx, channelID, pageKey, viewport, state, "click")
	hm.Move = s.cells(ctx, channelID, pageKey, viewport, state, "move")
	hm.Scroll = s.cells(ctx, channelID, pageKey, viewport, state, "scroll")
	if els, err := s.rec.LensElementsFromEvents(ctx, channelID, pageKey, viewport, 25); err == nil {
		for _, e := range els {
			hm.Elements = append(hm.Elements, ElementClick{Selector: e.Selector, Text: e.Text, Count: e.Count})
		}
	}
	return hm, true, nil
}

func (s *Service) cells(ctx context.Context, channelID uuid.UUID, pageKey, viewport, state, kind string) []Cell {
	rows, err := s.rec.LensCellsFromEvents(ctx, channelID, pageKey, viewport, state, kind)
	if err != nil {
		return nil
	}
	out := make([]Cell, 0, len(rows))
	for _, r := range rows {
		out = append(out, Cell{X: int(r.X), Y: int(r.Y), Count: r.Count})
	}
	return out
}

// --- UX signals: frustration / errors / web vitals (read aggregates) ---

type SignalPage struct {
	PageKey string
	Count   int64
}
type RageElement struct {
	Selector, Text string
	Count          int64
}
type Frustration struct {
	RagePages    []SignalPage
	DeadPages    []SignalPage
	RageElements []RageElement
}
type ErrorStat struct {
	Message      string
	Count, Pages int64
	LastSeen     time.Time
}
type PageVitals struct {
	PageKey             string
	LCP, CLS, INP, TTFB *float64 // p75; nil when the page has no sample for that metric
	Samples             int64
}

// Top pages by rage and dead clicks from the rollup, plus the most rage-clicked elements from the raw spine
func (s *Service) Frustration(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) (Frustration, error) {
	var f Frustration
	for _, d := range []struct {
		dim string
		dst *[]SignalPage
	}{{"rage", &f.RagePages}, {"dead", &f.DeadPages}} {
		rows, err := s.read().LensTopSignalPages(ctx, signalsdb.LensTopSignalPagesParams{ChannelID: channelID, Since: since, Dim: d.dim, Lim: limit})
		if err != nil {
			return f, err
		}
		for _, r := range rows {
			*d.dst = append(*d.dst, SignalPage{PageKey: r.PageKey, Count: r.Count})
		}
	}
	if els, err := s.rec.LensTopRageElements(ctx, channelID, since, limit); err == nil {
		for _, r := range els {
			f.RageElements = append(f.RageElements, RageElement{Selector: r.Selector, Text: r.Text, Count: r.Count})
		}
	}
	return f, nil
}

func (s *Service) TopErrors(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]ErrorStat, error) {
	rows, err := s.rec.LensTopErrors(ctx, channelID, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ErrorStat, 0, len(rows))
	for _, r := range rows {
		out = append(out, ErrorStat{Message: r.Message, Count: r.Count, Pages: r.Pages, LastSeen: r.LastSeen})
	}
	return out, nil
}

// Pivots the per-(page, metric) p75 rows into one row per page with LCP/CLS/INP/TTFB columns
func (s *Service) Performance(ctx context.Context, channelID uuid.UUID, since time.Time) ([]PageVitals, error) {
	rows, err := s.rec.LensVitalsByPage(ctx, channelID, since)
	if err != nil {
		return nil, err
	}
	byPage := map[string]*PageVitals{}
	order := make([]string, 0)
	for _, r := range rows {
		pk := r.PageKey
		pv := byPage[pk]
		if pv == nil {
			pv = &PageVitals{PageKey: pk}
			byPage[pk] = pv
			order = append(order, pk)
		}
		v := r.P75
		switch asString(r.Metric) {
		case "LCP":
			pv.LCP = &v
		case "CLS":
			pv.CLS = &v
		case "INP":
			pv.INP = &v
		case "TTFB":
			pv.TTFB = &v
		}
		pv.Samples += r.Samples
	}
	out := make([]PageVitals, 0, len(order))
	for _, pk := range order {
		out = append(out, *byPage[pk])
	}
	return out, nil
}

// A site-wide p75 for one Core Web Vital, powering the Lighthouse-style gauges on the Lens home
type SiteVital struct {
	Metric  string
	P75     float64
	Samples int64
}

func (s *Service) VitalsOverall(ctx context.Context, channelID uuid.UUID, since time.Time) ([]SiteVital, error) {
	rows, err := s.rec.LensVitalsOverall(ctx, channelID, since)
	if err != nil {
		return nil, err
	}
	out := make([]SiteVital, 0, len(rows))
	for _, r := range rows {
		out = append(out, SiteVital{Metric: r.Metric, P75: r.P75, Samples: r.Samples})
	}
	return out, nil
}

type TermCount struct {
	Term  string
	Count int64
}

type SearchDay struct {
	Date        string
	Searches    int64
	ZeroResults int64
}

// The on-site-search report - headline totals, top terms, zero-result terms as merchandising gaps, and a daily trend
type SearchAnalytics struct {
	TotalSearches      int64
	DistinctTerms      int64
	ZeroResultSearches int64
	Top                []TermCount
	ZeroResult         []TermCount
	Daily              []SearchDay
}

// All rollup-backed, from dim='search' and 'search_zero'
func (s *Service) SearchAnalytics(ctx context.Context, channelID uuid.UUID, since time.Time) (SearchAnalytics, error) {
	var out SearchAnalytics
	totals, err := s.read().LensSearchTotals(ctx, signalsdb.LensSearchTotalsParams{ChannelID: channelID, Since: since})
	if err != nil {
		return out, err
	}
	out.TotalSearches = totals.TotalSearches
	out.DistinctTerms = totals.DistinctTerms
	out.ZeroResultSearches = totals.ZeroResultSearches
	if rows, err := s.read().LensTopSearches(ctx, signalsdb.LensTopSearchesParams{ChannelID: channelID, Since: since, Lim: 50}); err == nil {
		for _, r := range rows {
			out.Top = append(out.Top, TermCount{Term: r.Term, Count: r.Count})
		}
	}
	if rows, err := s.read().LensZeroResultSearches(ctx, signalsdb.LensZeroResultSearchesParams{ChannelID: channelID, Since: since, Lim: 50}); err == nil {
		for _, r := range rows {
			out.ZeroResult = append(out.ZeroResult, TermCount{Term: r.Term, Count: r.Count})
		}
	}
	if rows, err := s.read().LensSearchDaily(ctx, signalsdb.LensSearchDailyParams{ChannelID: channelID, Since: since}); err == nil {
		for _, r := range rows {
			out.Daily = append(out.Daily, SearchDay{Date: r.Day, Searches: r.Searches, ZeroResults: r.ZeroResults})
		}
	}
	return out, nil
}

// SessionError / SessionVital / SessionSignals are this recording's JS errors and web vitals, for the replay page
type SessionError struct {
	Message string
	At      time.Time
}
type SessionVital struct {
	Metric string
	Value  float64
}
type SessionSignals struct {
	Errors []SessionError
	Vitals []SessionVital
}

// Matched by sessionId and bounded to the session's channel and time window - [from, to] should be the session's started_at and last_event_at
func (s *Service) SessionSignals(ctx context.Context, channelID uuid.UUID, sessionID string, from, to time.Time) (SessionSignals, error) {
	rows, err := s.rec.LensSessionSignals(ctx, channelID, sessionID, from.Add(-2*time.Second), to.Add(2*time.Second))
	if err != nil {
		return SessionSignals{}, err
	}
	var sig SessionSignals
	vital := map[string]float64{} // last value wins per metric
	order := make([]string, 0, 4)
	for _, r := range rows {
		switch r.Name {
		case sigJSError:
			sig.Errors = append(sig.Errors, SessionError{Message: r.Message, At: r.OccurredAt})
		case sigWebVitals:
			m := asString(r.Metric)
			if m == "" {
				continue
			}
			if _, ok := vital[m]; !ok {
				order = append(order, m)
			}
			vital[m], _ = strconv.ParseFloat(asString(r.Value), 64)
		}
	}
	for _, m := range order {
		sig.Vitals = append(sig.Vitals, SessionVital{Metric: m, Value: vital[m]})
	}
	return sig, nil
}

// Drops web spine events, heatmap aggregates and snapshots past the cutoff - used by the lens-prune scheduler job
// archive routes the raw spine events through the retention archive before they delete; the heatmap/snapshot aggregates always just die (screenshots and binned counters, not re-ingestable events)
func (s *Service) Prune(ctx context.Context, before time.Time, archive bool) (int64, error) {
	// Delete externalized snapshot objects before their rows go, best-effort
	if s.blobs != nil {
		if keys, err := s.q.LensSnapshotBlobKeysBefore(ctx, before); err == nil {
			s.deleteObjects(ctx, keys)
		}
	}
	// web spine events age out on the engine - under ClickHouse a Postgres DELETE would silently stop enforcing this window - and so do the event-derived heatmap increments (source=lens)
	spineRetire := func(source string) func(context.Context, time.Time) (int64, error) {
		return func(ctx context.Context, before time.Time) (int64, error) {
			if archive {
				return s.rec.ArchiveEventsBySource(ctx, source, before)
			}
			return s.rec.PruneEventsBySource(ctx, source, before)
		}
	}
	var total int64
	for _, del := range []func(context.Context, time.Time) (int64, error){
		spineRetire(analytics.SourceWeb),
		spineRetire(analytics.SourceLens),
		s.q.LensPruneStaleSnapshots,
		s.q.PruneLensVisitors,
		s.q.PruneBrowseSeen,
		s.q.PruneBrowseNotified,
	} {
		n, err := del(ctx, before)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// PruneRollups ages out the per-day aggregates on their own (much longer) window - they're a few rows per channel per day and what makes year-over-year trends possible after the raw events are gone, so they deliberately left Prune's shared cutoff
func (s *Service) PruneRollups(ctx context.Context, before time.Time) (int64, error) {
	var total int64
	for _, del := range []func(context.Context, time.Time) (int64, error){
		s.q.PruneTrafficRollup,
		s.q.PruneLensDaily,
	} {
		n, err := del(ctx, before)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// Separate from Prune because replay has its own shorter retention window - it's heavier and more PII-sensitive
func (s *Service) PruneReplay(ctx context.Context, before time.Time) (int64, error) {
	// Delete externalized chunk objects before the rows cascade away
	if s.blobs != nil {
		if keys, err := s.q.LensSessionBlobKeysBefore(ctx, before); err == nil {
			s.deleteObjects(ctx, keys)
		}
	}
	return s.q.LensPruneSessions(ctx, before)
}

// Best-effort - a failed delete just leaves an orphan for a later sweep or lifecycle rule to reap
func (s *Service) deleteObjects(ctx context.Context, keys []string) {
	for _, k := range keys {
		_ = s.blobs.Delete(ctx, k)
	}
}

// --- session replay reads (dashboard) ---

// One row in a recordings list - enough to label and open a replay
type SessionMeta struct {
	ID         uuid.UUID
	AnonID     string
	CustomerID *uuid.UUID
	StartedAt  time.Time
	EndedAt    time.Time
	DurationMs int64
	EventCount int64
	EntryPage  string
	ExitPage   string
	Viewport   string
	UserAgent  string
	RageCount  int64 // frustration signals tied to this recording (for the badge/filter)
	ErrorCount int64
}

// SessionMeta plus the owning channel, for the player's permission check before streaming events
type SessionInfo struct {
	SessionMeta
	ChannelID uuid.UUID
}

func metaFrom(id, anon string, cust uuid.NullUUID, started, ended time.Time, events int64, entry, exit, viewport, ua string) SessionMeta {
	m := SessionMeta{
		StartedAt:  started,
		EndedAt:    ended,
		DurationMs: ended.Sub(started).Milliseconds(),
		EventCount: events,
		EntryPage:  entry,
		ExitPage:   exit,
		Viewport:   viewport,
		UserAgent:  ua,
	}
	m.ID, _ = uuid.Parse(id)
	m.AnonID = anon
	if cust.Valid {
		c := cust.UUID
		m.CustomerID = &c
	}
	return m
}

// Resolved by customer id OR any anon id they later identified with, so pre-login sessions show up too
func (s *Service) SessionsForCustomer(ctx context.Context, customerID uuid.UUID, channels []uuid.UUID, limit int32) ([]SessionMeta, error) {
	// the identity map lives on the spine engine - a direct signalsdb read misses every stitch made after a ClickHouse flip, and an error surfaces rather than silently dropping pre-login sessions
	anonIDs, err := s.rec.AnonIDsForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	rows, err := s.read().LensSessionsForCustomer(ctx, signalsdb.LensSessionsForCustomerParams{
		CustomerID: uuid.NullUUID{UUID: customerID, Valid: true}, AnonIds: anonIDs,
		AllChannels: channels == nil, ChannelIds: emptyIDs(channels), Lim: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]SessionMeta, len(rows))
	for i, r := range rows {
		m := metaFrom(r.ID.String(), r.AnonID, r.CustomerID, r.StartedAt, r.LastEventAt, r.EventCount, r.EntryPage, r.ExitPage, r.Viewport, r.UserAgent)
		m.RageCount, m.ErrorCount = int64(r.RageCount), int64(r.ErrorCount)
		out[i] = m
	}
	return out, nil
}

func (s *Service) SessionsForAnon(ctx context.Context, anonID string, channels []uuid.UUID, limit int32) ([]SessionMeta, error) {
	rows, err := s.read().LensSessionsForAnon(ctx, signalsdb.LensSessionsForAnonParams{
		AnonID: anonID, AllChannels: channels == nil, ChannelIds: emptyIDs(channels), Lim: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]SessionMeta, len(rows))
	for i, r := range rows {
		m := metaFrom(r.ID.String(), r.AnonID, r.CustomerID, r.StartedAt, r.LastEventAt, r.EventCount, r.EntryPage, r.ExitPage, r.Viewport, r.UserAgent)
		m.RageCount, m.ErrorCount = int64(r.RageCount), int64(r.ErrorCount)
		out[i] = m
	}
	return out, nil
}

func emptyIDs(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

func (s *Service) SessionsForChannel(ctx context.Context, channelID uuid.UUID, limit int32) ([]SessionMeta, error) {
	rows, err := s.read().LensSessionsForChannel(ctx, signalsdb.LensSessionsForChannelParams{ChannelID: channelID, Lim: limit})
	if err != nil {
		return nil, err
	}
	out := make([]SessionMeta, len(rows))
	for i, r := range rows {
		m := metaFrom(r.ID.String(), r.AnonID, r.CustomerID, r.StartedAt, r.LastEventAt, r.EventCount, r.EntryPage, r.ExitPage, r.Viewport, r.UserAgent)
		m.RageCount, m.ErrorCount = int64(r.RageCount), int64(r.ErrorCount)
		out[i] = m
	}
	return out, nil
}

func (s *Service) GetSession(ctx context.Context, id uuid.UUID) (SessionInfo, error) {
	r, err := s.read().LensGetSession(ctx, id)
	if err != nil {
		return SessionInfo{}, err
	}
	return SessionInfo{
		SessionMeta: metaFrom(r.ID.String(), r.AnonID, r.CustomerID, r.StartedAt, r.LastEventAt, r.EventCount, r.EntryPage, r.ExitPage, r.Viewport, r.UserAgent),
		ChannelID:   r.ChannelID,
	}, nil
}

// Reassembles a recording into one ordered JSON array of rrweb events - every chunk gunzipped and concatenated in seq order, ready for the dashboard's player
func (s *Service) SessionEvents(ctx context.Context, id uuid.UUID) ([]byte, error) {
	chunks, err := s.read().LensSessionChunks(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.blobs == nil {
		return nil, errors.New("lens: no object store, which is where replay chunks live")
	}
	all := []json.RawMessage{}
	for _, key := range chunks {
		gz, err := s.blobs.GetBytes(ctx, key)
		if err != nil {
			continue
		}
		zr, err := gzip.NewReader(bytes.NewReader(gz))
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(zr)
		_ = zr.Close()
		if err != nil {
			continue
		}
		var evs []json.RawMessage
		if err := json.Unmarshal(raw, &evs); err != nil {
			continue
		}
		all = append(all, evs...)
	}
	return json.Marshal(all)
}

func gzipBytes(b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Bytes()
}

func gunzipBytes(b []byte) []byte {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return b // not gzipped (defensive) - return as-is
	}
	defer func() { _ = r.Close() }()
	out, err := io.ReadAll(r)
	if err != nil {
		return b
	}
	return out
}

func orEventID(id string) string {
	if id == "" {
		return uuid.NewString()
	}
	return id
}

func msToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{} // RecordWeb's orNow() handles the zero value
	}
	return time.UnixMilli(ms)
}

// Unwraps the interface{} columns sqlc emits for COALESCE and jsonb-to-text projections, which lib/pq returns as string or []byte
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
