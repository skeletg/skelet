package lens

import (
	"context"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

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
	HTML       string         `json:"html"` // rrweb-serialized DOM as JSON (snapshot)
	Width      int            `json:"width"`
	Height     int            `json:"height"`
	TS         int64          `json:"ts"`   // epoch ms (optional)
	Name       string         `json:"name"` // custom event name
	Props      map[string]any `json:"props"`
}

type Batch struct {
	AnonID string  `json:"anonId"`
	Events []Event `json:"events"`
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
	kept := b.Events[:0]
	for i := range b.Events {
		if clean(&b.Events[i]) {
			kept = append(kept, b.Events[i])
		}
	}
	b.Events = kept
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

// The row goes first and the upload only follows a new one, so a later post for a known layout can't replace its backdrop
func (s *Service) storeSnapshot(ctx context.Context, channelID uuid.UUID, state string, e Event) {
	if e.HTML == "" || e.LayoutHash == "" || s.blobs == nil {
		return
	}
	key := snapshotBlobKey(channelID, e.PageKey, e.Viewport, state, e.LayoutHash)
	n, err := s.q.LensInsertSnapshot(ctx, signalsdb.LensInsertSnapshotParams{
		ChannelID: channelID, PageKey: e.PageKey, ViewportBucket: e.Viewport, StateKey: state,
		LayoutHash: e.LayoutHash, BlobKey: key, Width: int32(e.Width), Height: int32(e.Height), MaxLayouts: maxSnapshotLayouts,
	})
	if err != nil {
		return
	}
	pk := signalsdb.LensTouchSnapshotParams{ChannelID: channelID, PageKey: e.PageKey, ViewportBucket: e.Viewport, StateKey: state, LayoutHash: e.LayoutHash}
	if n == 0 {
		_ = s.q.LensTouchSnapshot(ctx, pk)
		return
	}
	if err := s.blobs.PutBytes(ctx, key, gzipBytes([]byte(e.HTML)), "application/octet-stream"); err != nil {
		_ = s.q.LensDeleteSnapshot(ctx, signalsdb.LensDeleteSnapshotParams(pk))
	}
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
	cfg := ClientConfig{Enabled: true, SamplingRate: 100, CaptureMoves: true, ReplayEnabled: true, ReplaySamplingRate: 10, ReplayMaxMinutes: defaultReplayMaxMinutes}
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
