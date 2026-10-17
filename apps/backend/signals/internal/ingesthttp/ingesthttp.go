// Package ingesthttp is the public /t surface - storefront tracker beacons (collect/rec/crawl) and SDK reads (config/experiments)
// Shared by cmd/server and the horizontally-scalable cmd/ingest tier, so both mount the identical handler and the proxy contract never changes
package ingesthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
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

// Limiter bounds this unauthenticated write path per IP and endpoint - nil admits, rather than refusing every beacon
type Limiter interface {
	Allow(ctx context.Context, key string) bool
}

// Options carries the /t dependencies
type Options struct {
	Channels    Channels
	Lens        *lens.Service
	Experiments *experiment.Service
	Segments    *segment.Service
	Geo         Geo
	Rollup      *analytics.TrafficRollup
	Limiter     Limiter
	Customers   auth.CustomerVerifier
}

// New builds the /t handler - public, channel from X-Channel-Id, and beacons are best-effort: always 204, so a tracking hiccup never surfaces to a shopper
// Every endpoint is per-IP rate-limited: over-limit POSTs drop with the usual 204 and GETs get an honest 429, which both SDKs read as no data
func New(o Options) http.Handler {
	// The channel a beacon names, sent by the storefront tracker on every request - a header-less caller gets the default store
	// An id naming no active channel resolves to nothing rather than falling back, since serving it would write spine rows nothing can scope
	resolveChannel := func(r *http.Request) (uuid.UUID, bool) {
		ctx := r.Context()
		if raw := strings.TrimSpace(r.Header.Get("X-Channel-Id")); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil || !o.Channels.Active(ctx, id) {
				return uuid.Nil, false
			}
			return id, true
		}
		return o.Channels.Default(ctx)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := resolveChannel(r)
		if !ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Normalized limiter key: unknown paths share one bucket so an attacker can't mint unbounded Valkey keys by walking random /t/* paths
		var limitKey string
		switch r.URL.Path {
		case "/t/collect":
			limitKey = "collect"
		case "/t/rec":
			limitKey = "rec"
		case "/t/crawl":
			limitKey = "crawl"
		case "/t/config":
			limitKey = "config"
		case "/t/experiments":
			limitKey = "exp"
		default:
			limitKey = "other"
		}
		if o.Limiter != nil && !o.Limiter.Allow(r.Context(), "ingest:"+limitKey+":"+auth.ClientIP(r)) { // an unconfigured limiter admits rather than panicking on the first beacon
			if r.Method == http.MethodGet {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/t/config":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "public, max-age=300")
			_ = json.NewEncoder(w).Encode(o.Lens.IngestConfig(r.Context(), channelID))
		case r.Method == http.MethodGet && r.URL.Path == "/t/experiments":
			// Active experiments for the storefront SDK - it assigns the visitor from key+weights+coverage and renders the variation value
			// Segment-targeted ones are filtered to the SESSION-VERIFIED customer's segments; anonymous or unverified callers never get one, a client-claimed id could pull any customer's variations
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Vary", "Authorization, Cookie")
			var custSegments map[string]bool
			if custID := auth.VerifiedStorefrontCustomer(r, o.Customers, channelID); custID != uuid.Nil {
				if slugs, e := o.Segments.SlugsForCustomer(r.Context(), custID); e == nil {
					custSegments = make(map[string]bool, len(slugs))
					for _, s := range slugs {
						custSegments[s] = true
					}
				}
				w.Header().Set("Cache-Control", "private, no-store")
			} else {
				w.Header().Set("Cache-Control", "public, max-age=60")
			}
			exps, _ := o.Experiments.Active(r.Context(), channelID)
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
			out := make([]expOut, 0, len(exps))
			for _, e := range exps {
				if e.SegmentSlug != "" && !custSegments[e.SegmentSlug] {
					continue // segment-targeted; visitor not in the segment (or anonymous)
				}
				vs := make([]expVariation, 0, len(e.Variations))
				for _, v := range e.Variations {
					vs = append(vs, expVariation{Key: v.Key, Weight: v.Weight, Value: v.Value})
				}
				out = append(out, expOut{Key: e.Key, Coverage: e.Coverage, GoalEvent: e.GoalEvent, Variations: vs})
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodPost && r.URL.Path == "/t/collect":
			var b lens.Batch
			// 8 MB cap: a full-page DOM snapshot (inlined stylesheets) is the large payload - too low a cap silently drops the snapshot flush, leaving pages with traffic but no heatmap backdrop
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&b); err == nil {
				b.AnonID = auth.SanitizeAnonID(b.AnonID) // body ids are client-supplied - garbage becomes anonymous, not a spine row
				// sinceSend is meaningless for web hits - a long span keeps the "<3s = prefetch" rule from firing, machine reflects UA markers
				enr := analytics.Classify(r.UserAgent(), time.Hour)
				// ClientIP honors the trusted-proxy X-Forwarded-For chain, so the spine records the visitor, not the proxy
				hit := analytics.Hit{UA: r.UserAgent(), IP: auth.ClientIP(r), At: time.Now(), Machine: enr.Machine, Device: enr.Device, Client: enr.Client, OS: enr.OS}
				hit.Country = o.Geo.Country(hit.IP)
				// A bot that renders JS fires the tracker too - name it so human Lens metrics can exclude it (the /t/crawl beacon is the authoritative crawl record)
				if bot := analytics.DetectBot(r.UserAgent()); bot.IsBot {
					hit.IsBot, hit.BotName, hit.BotCat = true, bot.Name, bot.Category
				}
				// Identity stitching trusts ONLY the session-verified customer - the batch body's customerId is attacker-controlled here
				o.Lens.Ingest(r.Context(), channelID, b.AnonID, hit, b, auth.VerifiedStorefrontCustomer(r, o.Customers, channelID))
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/t/rec":
			// Session-replay flush (rrweb stream) - 6 MB cap, just above the service's 5 MB chunk guard
			var rb lens.ReplayBatch
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6<<20)).Decode(&rb); err == nil {
				rb.AnonID = auth.SanitizeAnonID(rb.AnonID) // IngestReplay drops an id-less batch, so a garbage id drops the flush whole
				o.Lens.IngestReplay(r.Context(), channelID, r.UserAgent(), rb, auth.VerifiedStorefrontCustomer(r, o.Customers, channelID))
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/t/crawl":
			// Server-side crawler beacon from the storefront middleware - catches bots that never run the tracker JS (the middleware pre-filters on a broad UA regex, DetectBot is authoritative)
			var c struct {
				Path string `json:"path"`
				UA   string `json:"ua"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&c); err == nil {
				if bot := analytics.DetectBot(c.UA); bot.IsBot && c.Path != "" {
					o.Rollup.AddBot(channelID, c.Path, bot.Name, bot.Category, time.Now())
				}
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
}
