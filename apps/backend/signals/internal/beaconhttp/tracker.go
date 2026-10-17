package beaconhttp

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
)

const (
	// A full-page DOM snapshot with inlined stylesheets is the large payload - lower drops the snapshot flush, leaving pages with traffic but no heatmap backdrop
	collectBodyMax = 8 << 20
	replayBodyMax  = lens.MaxReplayChunkBytes + 1<<20
	crawlBodyMax   = 8 << 10
	// The tracker flushes on a timer, nowhere near this - without it one 8 MB body fans out into a hundred thousand spine writes
	maxCollectEvents = 2000
)

// Channels answers which channel a beacon belongs to. The suite keeps its own copy of core's active
// set, since every beacon resolves one and neither a call into core nor its tables are within reach
type Channels interface {
	Active(ctx context.Context, id uuid.UUID) bool
	Default(ctx context.Context) (uuid.UUID, bool)
}

// Geo turns a visitor's address into a country for the spine - unset, country attribution is simply absent
type Geo interface {
	Country(ip string) string
}

type (
	Lens interface {
		IngestConfig(ctx context.Context, channelID uuid.UUID) lens.ClientConfig
		Ingest(ctx context.Context, channelID uuid.UUID, anonID string, hit analytics.Hit, b lens.Batch, verifiedCustomer uuid.UUID)
		IngestReplay(ctx context.Context, channelID uuid.UUID, userAgent string, b lens.ReplayBatch, verifiedCustomer uuid.UUID)
	}
	Experiments interface {
		Active(ctx context.Context, channelID uuid.UUID) ([]experiment.Experiment, error)
	}
	Segments interface {
		SlugsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error)
	}
	Rollup interface {
		AddBot(channelID uuid.UUID, pageKey, botName, botCat string, at time.Time)
	}
)

type TrackerOptions struct {
	Channels    Channels
	Lens        Lens
	Experiments Experiments
	Segments    Segments
	Geo         Geo
	Rollup      Rollup
	Limiter     Limiter
	Customers   auth.CustomerVerifier
}

type tracker struct{ TrackerOptions }

// Tracker builds the /t handler - channel from X-Channel-Id, and beacons are best-effort: always 204, so a tracking hiccup never surfaces to a shopper
// Every route is per-IP rate-limited: over-limit POSTs drop with the usual 204 and GETs get an honest 429, which both SDKs read as no data
func Tracker(o TrackerOptions) http.Handler {
	t := tracker{o}
	mux := http.NewServeMux()
	t.route(mux, "GET /t/config", "config", t.config)
	t.route(mux, "GET /t/experiments", "exp", t.experiments)
	t.route(mux, "POST /t/collect", "collect", t.collect)
	t.route(mux, "POST /t/rec", "rec", t.rec)
	t.route(mux, "POST /t/crawl", "crawl", t.crawl)
	return mux
}

func (t tracker) route(mux *http.ServeMux, pattern, limitKey string, h func(http.ResponseWriter, *http.Request, uuid.UUID)) {
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := t.channel(r)
		if !ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !allow(t.Limiter, r, "ingest:"+limitKey) {
			if r.Method == http.MethodGet {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
		h(w, r, channelID)
	})
}

// The channel a beacon names - a header-less caller gets the default store, and an id naming no active channel resolves to
// nothing rather than falling back, since serving it would write spine rows nothing can scope
func (t tracker) channel(r *http.Request) (uuid.UUID, bool) {
	ctx := r.Context()
	if raw := strings.TrimSpace(r.Header.Get("X-Channel-Id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || !t.Channels.Active(ctx, id) {
			return uuid.Nil, false
		}
		return id, true
	}
	return t.Channels.Default(ctx)
}

func (t tracker) config(w http.ResponseWriter, r *http.Request, channelID uuid.UUID) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Vary", "X-Channel-Id")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = json.NewEncoder(w).Encode(t.Lens.IngestConfig(r.Context(), channelID))
}

type expVariation struct {
	Key    string `json:"key"`
	Weight int    `json:"weight"`
	Value  string `json:"value"`
}

type expOut struct {
	Key        string         `json:"key"`
	Coverage   int            `json:"coverage"`
	GoalEvent  string         `json:"goalEvent"`
	Variations []expVariation `json:"variations"`
}

// Active experiments for the storefront SDK - it assigns the visitor from key+weights+coverage and renders the variation value
// Segment-targeted ones are filtered to the SESSION-VERIFIED customer's segments; a client-claimed id could pull any customer's variations
func (t tracker) experiments(w http.ResponseWriter, r *http.Request, channelID uuid.UUID) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Vary", "Authorization, Cookie, X-Channel-Id")
	var custSegments map[string]bool
	if custID := auth.VerifiedStorefrontCustomer(r, t.Customers, channelID); custID != uuid.Nil {
		if slugs, err := t.Segments.SlugsForCustomer(r.Context(), custID); err == nil {
			custSegments = make(map[string]bool, len(slugs))
			for _, s := range slugs {
				custSegments[s] = true
			}
		}
		w.Header().Set("Cache-Control", "private, no-store")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=60")
	}
	exps, err := t.Experiments.Active(r.Context(), channelID)
	if err != nil {
		log.Printf("[beacon] experiments for %s: %v", channelID, err)
		w.Header().Set("Cache-Control", "no-store") // a blip must not stick at the CDN as "no experiments"
	}
	out := make([]expOut, 0, len(exps))
	for _, e := range exps {
		if e.SegmentSlug != "" && !custSegments[e.SegmentSlug] {
			continue
		}
		vs := make([]expVariation, 0, len(e.Variations))
		for _, v := range e.Variations {
			vs = append(vs, expVariation{Key: v.Key, Weight: v.Weight, Value: v.Value})
		}
		out = append(out, expOut{Key: e.Key, Coverage: e.Coverage, GoalEvent: e.GoalEvent, Variations: vs})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (t tracker) collect(w http.ResponseWriter, r *http.Request, channelID uuid.UUID) {
	defer w.WriteHeader(http.StatusNoContent)
	var b lens.Batch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, collectBodyMax)).Decode(&b); err != nil {
		return
	}
	b.AnonID = auth.SanitizeAnonID(b.AnonID) // body ids are client-supplied - garbage becomes anonymous, not a spine row
	if len(b.Events) > maxCollectEvents {
		b.Events = b.Events[:maxCollectEvents]
	}
	// sinceSend is meaningless for web hits - a long span keeps the "<3s = prefetch" rule from firing, machine reflects UA markers
	enr := analytics.Classify(r.UserAgent(), time.Hour)
	// ClientIP honors the trusted-proxy X-Forwarded-For chain, so the spine records the visitor, not the proxy
	hit := analytics.Hit{UA: r.UserAgent(), IP: auth.ClientIP(r), At: time.Now(), Machine: enr.Machine, Device: enr.Device, Client: enr.Client, OS: enr.OS}
	hit.Country = t.Geo.Country(hit.IP)
	// A bot that renders JS fires the tracker too - name it so human Lens metrics can exclude it (/t/crawl is the authoritative crawl record)
	if bot := analytics.DetectBot(r.UserAgent()); bot.IsBot {
		hit.IsBot, hit.BotName, hit.BotCat = true, bot.Name, bot.Category
	}
	// Identity stitching trusts ONLY the session-verified customer - the batch body's customerId is attacker-controlled here
	t.Lens.Ingest(r.Context(), channelID, b.AnonID, hit, b, auth.VerifiedStorefrontCustomer(r, t.Customers, channelID))
}

// Session-replay flush (rrweb stream), capped just above the service's chunk guard
func (t tracker) rec(w http.ResponseWriter, r *http.Request, channelID uuid.UUID) {
	defer w.WriteHeader(http.StatusNoContent)
	var rb lens.ReplayBatch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, replayBodyMax)).Decode(&rb); err != nil {
		return
	}
	rb.AnonID = auth.SanitizeAnonID(rb.AnonID) // IngestReplay drops an id-less batch, so a garbage id drops the flush whole
	t.Lens.IngestReplay(r.Context(), channelID, r.UserAgent(), rb, auth.VerifiedStorefrontCustomer(r, t.Customers, channelID))
}

// Server-side crawler beacon from the storefront middleware - catches bots that never run the tracker JS (the middleware
// pre-filters on a broad UA regex, DetectBot is authoritative, and the rollup refuses a page key that isn't a bounded path)
func (t tracker) crawl(w http.ResponseWriter, r *http.Request, channelID uuid.UUID) {
	defer w.WriteHeader(http.StatusNoContent)
	var c struct {
		Path string `json:"path"`
		UA   string `json:"ua"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, crawlBodyMax)).Decode(&c); err != nil {
		return
	}
	if bot := analytics.DetectBot(c.UA); bot.IsBot {
		t.Rollup.AddBot(channelID, c.Path, bot.Name, bot.Category, time.Now())
	}
}
