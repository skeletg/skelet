// Package ingesthttp is the public /t surface - storefront tracker beacons (collect/rec/crawl) and SDK reads (config/experiments/forms)
// Shared by cmd/server and the horizontally-scalable cmd/ingest tier, so both mount the identical handler and the proxy contract never changes
package ingesthttp

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/backinstock"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/internal/form"
	"github.com/skeletg/skelet/apps/backend/internal/geo"
	"github.com/skeletg/skelet/apps/backend/internal/lens"
	"github.com/skeletg/skelet/apps/backend/internal/ratelimit"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
)

// Options carries the /t dependencies - BackInStock and Forms may be nil (those paths publish bus events and enroll flows, so they need the app's subscriber graph and cmd/ingest doesn't wire them)
// Their requests go to Fallback instead, a reverse proxy to the app in cmd/ingest, or 404 when that's nil too
type Options struct {
	Channels       *channel.Service
	Lens           *lens.Service
	Experiments    *experiment.Service
	Segments       *segment.Service
	Config         *config.Service
	Geo            *geo.Resolver
	Rollup         *analytics.TrafficRollup
	Limiter        *ratelimit.RedisLimiter
	JWTSecret      string
	SignalsEnabled bool
	BackInStock    *backinstock.Service
	Forms          *form.Service
	Fallback       http.Handler
}

// New builds the /t handler, channel middleware included - public, channel from X-Channel-Id (the storefront proxies /t via a rewrite, so the backend never sees the storefront Host), and beacons are best-effort: always 204, a tracking hiccup never surfaces to a shopper
// Every endpoint is per-IP, per-endpoint rate-limited - POSTs are an unauthenticated write path into the spine/blob store and the GETs do real DB reads per request
// The limit absorbs many shoppers behind one corporate NAT while still bounding abuse; over-limit POSTs drop with the usual 204 (a scraper learns nothing) and GETs get an honest 429 (both SDKs treat non-OK as "no data" and fall back gracefully)
func New(o Options) http.Handler {
	fallback := o.Fallback
	if fallback == nil {
		fallback = http.NotFoundHandler()
	}
	// The ONLY customer identity /t endpoints may trust (headers/body ids are client-supplied and forgeable here) - Bearer token, else the storefront's httpOnly session cookie riding the same-origin /t proxy, uuid.Nil when absent or invalid
	verifiedCustomer := func(r *http.Request) uuid.UUID {
		tokenStr := ""
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			tokenStr = strings.TrimPrefix(h, "Bearer ")
		} else if c, cerr := r.Cookie("storefront_access_token"); cerr == nil {
			tokenStr = c.Value
		}
		if tokenStr == "" {
			return uuid.Nil
		}
		cc, err := auth.ValidateCustomerToken(tokenStr, o.JWTSecret)
		if err != nil {
			return uuid.Nil
		}
		return cc.CustomerID
	}
	return channel.Middleware(o.Channels)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Whole Signals suite off: beacons get a silent 204, SDK GETs a 404 - every client treats both as "nothing to do" (fail-soft by design)
		if !o.SignalsEnabled {
			if r.Method == http.MethodGet {
				http.NotFound(w, r)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
		ch, ok := channel.FromContext(r.Context())
		if !ok || ch == nil {
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
		case "/t/back-in-stock":
			limitKey = "bis"
		case "/t/forms":
			limitKey = "forms"
		case "/t/forms/submit":
			limitKey = "formsubmit"
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
			_ = json.NewEncoder(w).Encode(o.Lens.IngestConfig(r.Context(), ch.ID))
		case r.Method == http.MethodGet && r.URL.Path == "/t/experiments":
			// Active experiments for the storefront SDK - it assigns the visitor from key+weights+coverage and renders the variation value
			// Segment-targeted ones are filtered to the SESSION-VERIFIED customer's segments; anonymous or unverified callers never get one, a client-claimed id could pull any customer's variations
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Vary", "Authorization, Cookie")
			var custSegments map[string]bool
			if custID := verifiedCustomer(r); custID != uuid.Nil {
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
			exps, _ := o.Experiments.Active(r.Context(), ch.ID)
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
				o.Lens.Ingest(r.Context(), ch.ID, b.AnonID, hit, b, verifiedCustomer(r))
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/t/rec":
			// Session-replay flush (rrweb stream) - 6 MB cap, just above the service's 5 MB chunk guard
			var rb lens.ReplayBatch
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6<<20)).Decode(&rb); err == nil {
				rb.AnonID = auth.SanitizeAnonID(rb.AnonID) // IngestReplay drops an id-less batch, so a garbage id drops the flush whole
				o.Lens.IngestReplay(r.Context(), ch.ID, r.UserAgent(), rb)
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
					o.Rollup.AddBot(ch.ID, c.Path, bot.Name, bot.Category, time.Now())
				}
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/t/back-in-stock":
			if o.BackInStock == nil {
				fallback.ServeHTTP(w, r)
				return
			}
			// "Notify me when it's back": public submit from the PDP, customer from the session cookie (else guest by email), best-effort 204
			var b struct {
				ProductID string `json:"productId"`
				SKU       string `json:"sku"`
				Email     string `json:"email"`
				Name      string `json:"name"`
				Image     string `json:"image"`
				URL       string `json:"url"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&b); err == nil {
				if pid, perr := uuid.Parse(b.ProductID); perr == nil && strings.TrimSpace(b.Email) != "" {
					req := backinstock.Request{
						ChannelID: ch.ID, ProductID: pid, SKU: b.SKU, Email: b.Email,
						AnonID: auth.SanitizeAnonID(r.Header.Get(auth.AnonHeader)), Name: b.Name, Image: b.Image, URL: b.URL,
					}
					if cid := verifiedCustomer(r); cid != uuid.Nil {
						req.CustomerID = &cid
					}
					_ = o.BackInStock.Record(r.Context(), req)
				}
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/t/forms":
			if o.Forms == nil {
				fallback.ServeHTTP(w, r)
				return
			}
			// Active signup forms for the storefront widget - public-safe projection only (no target segment / consent internals)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "public, max-age=60")
			if on, _ := o.Config.GetBool(r.Context(), "pulse/forms/enabled", config.DefaultScope); !on {
				_ = json.NewEncoder(w).Encode([]any{}) // master switch off
				return
			}
			forms, _ := o.Forms.Active(r.Context(), ch.ID)
			type pubForm struct {
				ID             string             `json:"id"`
				Slug           string             `json:"slug"`
				Type           string             `json:"type"`
				Heading        string             `json:"heading"`
				Body           string             `json:"body"`
				ButtonLabel    string             `json:"buttonLabel"`
				SuccessMessage string             `json:"successMessage"`
				CollectName    bool               `json:"collectName"`
				Display        form.DisplayConfig `json:"display"`
			}
			out := make([]pubForm, 0, len(forms))
			for _, f := range forms {
				out = append(out, pubForm{
					ID: f.ID.String(), Slug: f.Slug, Type: f.Type, Heading: f.Heading, Body: f.Body,
					ButtonLabel: f.ButtonLabel, SuccessMessage: f.SuccessMessage, CollectName: f.CollectName, Display: f.Display,
				})
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodPost && r.URL.Path == "/t/forms/submit":
			if o.Forms == nil {
				fallback.ServeHTTP(w, r)
				return
			}
			// Records the lead, ensures a marketing contact, opts them in, adds to the segment, fires the welcome flow - returns the success message
			var b struct {
				Form  string `json:"form"` // slug or id
				Email string `json:"email"`
				Name  string `json:"name"`
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&b); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad request"})
				return
			}
			msg, ferr := o.Forms.Submit(r.Context(), ch.ID, b.Form, b.Email, b.Name)
			if ferr != nil {
				// Public endpoint - only the two shopper-facing sentinels get echoed; anything else is a
				// server-side failure whose text would leak internals, and answering 400 for it also told
				// the storefront the shopper's input was at fault
				switch {
				case errors.Is(ferr, form.ErrEmailRequired), errors.Is(ferr, form.ErrFormNotFound):
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": ferr.Error()})
				default:
					slog.Error("signup form submit", "channel", ch.ID, "form", b.Form, "err", ferr)
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "could not submit right now"})
				}
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
}
