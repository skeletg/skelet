package analytics

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
)

const (
	rollupKindBot    = "bot"
	rollupKindHuman  = "human"
	rollupFlushEvery = 5 * time.Second
)

// The bot/human-total grain (analytics_traffic_rollup) - human rows leave botName, botCat and pageKey empty, so humans collapse to one counter per (channel, day)
type rollupKey struct {
	channelID uuid.UUID
	day       string // YYYY-MM-DD (UTC)
	kind      string
	botName   string
	botCat    string
	pageKey   string
}

type lensKey struct { // the Lens-hub daily-views grain (lens_traffic_daily) - summable view counts by dimension
	channelID uuid.UUID
	day       string
	dim       string
	key       string
}

type seenKey struct {
	channelID uuid.UUID
	anonID    string
}

type browseKey struct { // browseKey/browseVal hold an anonymous visitor's latest product browse, for the browse-abandonment scanner
	channelID uuid.UUID
	anonID    string
}
type browseVal struct {
	at      time.Time
	pageKey string
	sku     string
	name    string
	image   string
}

// Coalesces crawl and human page-view counts in memory and flushes them as additive upserts, so millions of hits become a trickle of aggregate writes and dashboards read a bounded rollup instead of scanning raw events - per-process, every replica flushing its own counts that the additive ON CONFLICT sums, and a crash loses at most one flush window
type TrafficRollup struct {
	q         *signalsdb.Queries
	mu        sync.Mutex
	counts    map[rollupKey]int64 // bot + human-total (analytics_traffic_rollup)
	lastSeen  map[rollupKey]time.Time
	lensViews map[lensKey]int64       // Lens-hub daily views by dimension (lens_traffic_daily)
	seen      map[seenKey]time.Time   // visitor last-seen (lens_visitor_seen)
	browse    map[browseKey]browseVal // latest product browse per visitor (browse_seen)
}

func NewTrafficRollup(q *signalsdb.Queries) *TrafficRollup {
	return &TrafficRollup{
		q:         q,
		counts:    map[rollupKey]int64{},
		lastSeen:  map[rollupKey]time.Time{},
		lensViews: map[lensKey]int64{},
		seen:      map[seenKey]time.Time{},
		browse:    map[browseKey]browseVal{},
	}
}

func dayKey(at time.Time) string { return at.UTC().Format("2006-01-02") }

func (t *TrafficRollup) AddBot(channelID uuid.UUID, pageKey, botName, botCat string, at time.Time) {
	if channelID == uuid.Nil {
		return
	}
	t.add(rollupKey{channelID: channelID, day: dayKey(at), kind: rollupKindBot, botName: botName, botCat: botCat, pageKey: pageKey}, at)
}

// Feeds three things at once: the per-(channel, day) counter behind the bot-vs-human split, the Lens-hub view breakdowns, and the visitor last-seen for exact uniques - empty device and referrer map to 'unknown' and '(direct)' so they match the backfill
func (t *TrafficRollup) AddHumanView(channelID uuid.UUID, pageKey, device, referrer, anonID string, at time.Time) {
	if channelID == uuid.Nil {
		return
	}
	day := dayKey(at)
	if device == "" {
		device = "unknown"
	}
	if referrer == "" {
		referrer = "(direct)"
	}
	t.mu.Lock()
	t.counts[rollupKey{channelID: channelID, day: day, kind: rollupKindHuman}]++
	t.lensViews[lensKey{channelID, day, "total", ""}]++
	t.lensViews[lensKey{channelID, day, "page", pageKey}]++
	t.lensViews[lensKey{channelID, day, "device", device}]++
	t.lensViews[lensKey{channelID, day, "referrer", referrer}]++
	if anonID != "" {
		k := seenKey{channelID, anonID}
		if at.After(t.seen[k]) {
			t.seen[k] = at
		}
	}
	t.mu.Unlock()
}

func (t *TrafficRollup) AddBrowse(channelID uuid.UUID, anonID, pageKey, sku, name, image string, at time.Time) { // only the most recent browse per visitor survives the flush window
	if channelID == uuid.Nil || anonID == "" {
		return
	}
	k := browseKey{channelID, anonID}
	t.mu.Lock()
	if cur, ok := t.browse[k]; !ok || at.After(cur.at) {
		t.browse[k] = browseVal{at: at, pageKey: pageKey, sku: sku, name: name, image: image}
	}
	t.mu.Unlock()
}

func (t *TrafficRollup) AddInteraction(channelID uuid.UUID, name string, at time.Time) { // feeds the Lens hub's interactions list
	if channelID == uuid.Nil {
		return
	}
	if name == "" {
		name = "(unnamed)"
	}
	t.mu.Lock()
	t.lensViews[lensKey{channelID, dayKey(at), "interaction", name}]++
	t.mu.Unlock()
}

// Records one Lens UX signal - a rage or dead click, a JS error - into the daily rollup under its own dimension and key, so the dashboards aggregate frustration and errors without scanning raw spine events
func (t *TrafficRollup) AddSignal(channelID uuid.UUID, dim, key string, at time.Time) {
	if channelID == uuid.Nil || dim == "" {
		return
	}
	if key == "" {
		key = "(none)"
	}
	t.mu.Lock()
	t.lensViews[lensKey{channelID, dayKey(at), dim, key}]++
	t.mu.Unlock()
}

func (t *TrafficRollup) add(k rollupKey, at time.Time) {
	t.mu.Lock()
	t.counts[k]++
	if at.After(t.lastSeen[k]) {
		t.lastSeen[k] = at
	}
	t.mu.Unlock()
}

// Drains the buffer to the rollup table - safe to call concurrently with Add, and best-effort, so a failed upsert drops that bucket's counts rather than retrying forever
func (t *TrafficRollup) Flush(ctx context.Context) error {
	t.mu.Lock()
	if len(t.counts) == 0 && len(t.lensViews) == 0 && len(t.seen) == 0 && len(t.browse) == 0 {
		t.mu.Unlock()
		return nil
	}
	counts, lastSeen := t.counts, t.lastSeen
	lensViews, seen, browse := t.lensViews, t.seen, t.browse
	t.counts = map[rollupKey]int64{}
	t.lastSeen = map[rollupKey]time.Time{}
	t.lensViews = map[lensKey]int64{}
	t.seen = map[seenKey]time.Time{}
	t.browse = map[browseKey]browseVal{}
	t.mu.Unlock()

	var firstErr error
	setErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for k, n := range counts {
		day, err := time.Parse("2006-01-02", k.day)
		if err != nil {
			continue
		}
		ls := lastSeen[k]
		if ls.IsZero() {
			ls = day
		}
		setErr(t.q.UpsertTrafficRollup(ctx, signalsdb.UpsertTrafficRollupParams{
			ChannelID: k.channelID, Day: day, Kind: k.kind,
			BotName: k.botName, BotCategory: k.botCat, PageKey: k.pageKey,
			Hits: n, LastSeen: ls,
		}))
	}
	for k, n := range lensViews {
		day, err := time.Parse("2006-01-02", k.day)
		if err != nil {
			continue
		}
		setErr(t.q.UpsertLensDaily(ctx, signalsdb.UpsertLensDailyParams{
			ChannelID: k.channelID, Day: day, Dim: k.dim, Key: k.key, Views: n,
		}))
	}
	for k, ts := range seen {
		setErr(t.q.UpsertLensVisitor(ctx, signalsdb.UpsertLensVisitorParams{
			ChannelID: k.channelID, AnonID: k.anonID, LastSeen: ts,
		}))
	}
	for k, v := range browse {
		setErr(t.q.UpsertBrowseSeen(ctx, signalsdb.UpsertBrowseSeenParams{
			ChannelID: k.channelID, AnonID: k.anonID, LastBrowseAt: v.at,
			PageKey: v.pageKey, Sku: v.sku, ProductName: v.name, ProductImage: v.image,
		}))
	}
	return firstErr
}

// Flushes on a ticker until ctx is cancelled, then does one final flush so the last window isn't lost on graceful shutdown
func (t *TrafficRollup) Run(ctx context.Context) {
	tk := time.NewTicker(rollupFlushEvery)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = t.Flush(fctx)
			cancel()
			return
		case <-tk.C:
			_ = t.Flush(ctx)
		}
	}
}
