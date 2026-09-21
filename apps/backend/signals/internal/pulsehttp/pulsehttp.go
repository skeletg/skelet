// Package pulsehttp is the public /e surface - the open pixel and the click redirect every marketing mail carries
// Core proxies it here the way it proxies /t, so a mail client reaches the storefront's host and the hit lands where the send log is
package pulsehttp

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/signals/internal/flow"
)

// Limiter bounds this unauthenticated path per IP - nil admits, rather than refusing every pixel
type Limiter interface {
	Allow(ctx context.Context, key string) bool
}

type Options struct {
	Campaigns *campaign.Service
	Flows     *flow.Service
	Limiter   Limiter
}

// New builds the /e handler. Both routes are best-effort: a pixel always serves and a click always
// redirects, whatever the recording did, since a mail client must never see an error
func New(o Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/e/o/", func(w http.ResponseWriter, r *http.Request) {
		// Over-limit still serves the pixel, just skips the recording - same stance as /t
		if o.Limiter == nil || o.Limiter.Allow(r.Context(), "eopen:"+auth.ClientIP(r)) {
			if ev := o.Campaigns.RecordOpen(r.Context(), strings.TrimPrefix(r.URL.Path, "/e/o/"), hit(r)); ev != nil {
				o.engage(r.Context(), signalsapi.TriggerEmailOpened, ev)
			}
		}
		w.Header().Set("Content-Type", "image/gif")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		_, _ = w.Write(campaign.TransparentGIF())
	})
	mux.HandleFunc("/e/c/", func(w http.ResponseWriter, r *http.Request) {
		if o.Limiter != nil && !o.Limiter.Allow(r.Context(), "eclick:"+auth.ClientIP(r)) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		// The destination is signed into the token, so a tampered link cannot become an open redirect
		dest, ev := o.Campaigns.RecordClick(r.Context(), strings.TrimPrefix(r.URL.Path, "/e/c/"), hit(r))
		if ev != nil {
			o.engage(r.Context(), signalsapi.TriggerEmailClicked, ev)
		}
		if dest == "" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, dest, http.StatusFound)
	})
	return mux
}

// A human open or click by a known customer starts the engagement-triggered flows
func (o Options) engage(ctx context.Context, trigger string, ev *campaign.Engaged) {
	if o.Flows == nil {
		return
	}
	o.Flows.Enroll(ctx, trigger, ev.ChannelID, ev.CustomerID, "", nil, "")
}

func hit(r *http.Request) analytics.Hit {
	return analytics.Hit{UA: r.UserAgent(), IP: auth.ClientIP(r), At: time.Now()}
}
