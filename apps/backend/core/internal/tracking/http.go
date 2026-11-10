package tracking

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
)

const TrackPath = "/api/front/track"

// What the storefront's /t/e route forwards: one browser event, with the shopper's consent, click ids and visitor in the
// headers it adds server-side. Only the browser's own events pass - cart changes, purchases and sign-ups are reported by
// the server itself. Public like the beacons, so per-IP rate limited, and always a 204. Mount it behind channel.Middleware
func (s *Service) BrowserHandler(limiter *ratelimit.RedisLimiter, customers auth.CustomerVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		_ = s.Track(auth.WithRequest(r.Context(), r), ev)
	}
}

// browserEvent reads one forwarded browser event - false for anything that isn't one the browser may report
func browserEvent(w http.ResponseWriter, r *http.Request, channelID uuid.UUID, customers auth.CustomerVerifier) (Event, bool) {
	var in struct {
		EventID    string     `json:"event_id"`
		Name       string     `json:"name"`
		PageURL    string     `json:"page_url"`
		Referrer   string     `json:"referrer"`
		Locale     string     `json:"locale"`
		PageTitle  string     `json:"page_title"`
		SearchTerm string     `json:"search_term"`
		Ecommerce  *Ecommerce `json:"ecommerce"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in) != nil || !BrowserEvent(in.Name) {
		return Event{}, false
	}
	if auth.SanitizeAnonID(in.EventID) == "" || len(in.PageURL) > 2048 || len(in.Referrer) > 2048 || len(in.Locale) > 16 || len(in.PageTitle) > 300 || len(in.SearchTerm) > 200 {
		return Event{}, false
	}
	if in.Ecommerce != nil && len(in.Ecommerce.Items) > 200 {
		in.Ecommerce.Items = in.Ecommerce.Items[:200]
	}
	ev := Event{
		ID: in.EventID, Name: in.Name, OccurredAt: time.Now().UTC(), ChannelID: channelID, Source: "browser",
		Ecommerce: in.Ecommerce, SearchTerm: in.SearchTerm,
		Context: Context{PageURL: in.PageURL, Referrer: in.Referrer, Locale: in.Locale, PageTitle: in.PageTitle},
	}
	WithShopper(&ev, auth.ShopperFromContext(auth.WithRequest(r.Context(), r)))
	if ev.Consent != nil && ev.Consent.Analytics { // the account is a profile link, so it needs what the anon id needs
		if cid := auth.VerifiedStorefrontCustomer(r, customers, channelID); cid != uuid.Nil {
			ev.User.CustomerID = &cid
		}
	}
	return ev, true
}
