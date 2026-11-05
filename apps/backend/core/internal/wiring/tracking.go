package wiring

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
)

func newTrackingService(q *db.Queries, cipher *secrets.Cipher, cfg *config.Service) *tracking.Service {
	var sealer tracking.Sealer // a nil *Cipher in the interface would read as a key that panics on use
	if cipher != nil {
		sealer = cipher
	}
	return tracking.NewService(q, sealer, cfg)
}

// The same SSRF-safe transport webhooks use - a destination's endpoint (a server-side GTM container, say) is typed by an admin
func newTrackingWorker(svc *tracking.Service, allow AllowPrivateTargets) *tracking.Worker {
	return tracking.NewWorker(svc, &http.Client{Transport: otelx.HTTPTransport(webhook.SafeTransport(bool(allow))), Timeout: 30 * time.Second})
}

const trackPath = "/api/front/track"

// What the storefront's /t/e route forwards: one browser event, with the shopper's consent, click ids and visitor in the
// headers it adds server-side. Only the browser's own events pass - cart changes, purchases and sign-ups are reported by
// the server itself. Public like the beacons, so per-IP rate limited, and always a 204
func trackHandler(svc *tracking.Service, channels *channel.Service, limiter *ratelimit.RedisLimiter, customers auth.CustomerVerifier) http.Handler {
	return channel.Middleware(channels)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer w.WriteHeader(http.StatusNoContent)
		ch, ok := channel.FromContext(r.Context())
		if r.Method != http.MethodPost || !ok || ch == nil {
			return
		}
		if limiter != nil && !limiter.Allow(r.Context(), "ingest:track:"+auth.ClientIP(r)) {
			return
		}
		ev, ok := browserEvent(w, r, ch.ID, customers)
		if !ok {
			return
		}
		_ = svc.Track(auth.WithRequest(r.Context(), r), ev)
	}))
}

// browserEvent reads one forwarded browser event - false for anything that isn't one the browser may report
func browserEvent(w http.ResponseWriter, r *http.Request, channelID uuid.UUID, customers auth.CustomerVerifier) (tracking.Event, bool) {
	var in struct {
		EventID   string              `json:"event_id"`
		Name      string              `json:"name"`
		PageURL   string              `json:"page_url"`
		Referrer  string              `json:"referrer"`
		Locale    string              `json:"locale"`
		Ecommerce *tracking.Ecommerce `json:"ecommerce"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in) != nil || !tracking.BrowserEvent(in.Name) {
		return tracking.Event{}, false
	}
	if auth.SanitizeAnonID(in.EventID) == "" || len(in.PageURL) > 2048 || len(in.Referrer) > 2048 || len(in.Locale) > 16 {
		return tracking.Event{}, false
	}
	if in.Ecommerce != nil && len(in.Ecommerce.Items) > 200 {
		in.Ecommerce.Items = in.Ecommerce.Items[:200]
	}
	ev := tracking.Event{
		ID: in.EventID, Name: in.Name, OccurredAt: time.Now().UTC(), ChannelID: channelID, Source: "browser",
		Ecommerce: in.Ecommerce,
		Context:   tracking.Context{PageURL: in.PageURL, Referrer: in.Referrer, Locale: in.Locale},
	}
	ev.WithShopper(auth.ShopperFromContext(auth.WithRequest(r.Context(), r)))
	if ev.Consent != nil && ev.Consent.Analytics { // the account is a profile link, so it needs what the anon id needs
		if cid := auth.VerifiedStorefrontCustomer(r, customers, channelID); cid != uuid.Nil {
			ev.User.CustomerID = &cid
		}
	}
	return ev, true
}
