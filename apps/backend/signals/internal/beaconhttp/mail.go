package beaconhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/campaign"
)

type (
	Campaigns interface {
		RecordOpen(ctx context.Context, token string, h analytics.Hit) *campaign.Engaged
		RecordClick(ctx context.Context, token string, h analytics.Hit) (string, *campaign.Engaged)
		ClickTarget(token string) string
	}
	Flows interface {
		Enroll(ctx context.Context, trigger string, channelID, customerID uuid.UUID, email string, payload json.RawMessage, key string) int
	}
)

type MailOptions struct {
	Campaigns Campaigns
	Flows     Flows // nil runs no flows
	Limiter   Limiter
}

type mail struct{ MailOptions }

// Mail builds the /e handler. Both routes are best-effort: a pixel always serves and a valid click always redirects, whatever
// the recording did or the limiter said, since a mail client must never see an error
func Mail(o MailOptions) http.Handler {
	m := mail{o}
	mux := http.NewServeMux()
	mux.HandleFunc("/e/o/", m.open)
	mux.HandleFunc("/e/c/", m.click)
	return mux
}

func (m mail) open(w http.ResponseWriter, r *http.Request) {
	if allow(m.Limiter, r, "eopen") {
		if ev := m.Campaigns.RecordOpen(r.Context(), strings.TrimPrefix(r.URL.Path, "/e/o/"), hit(r)); ev != nil {
			m.engage(r.Context(), signalsapi.TriggerEmailOpened, ev)
		}
	}
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_, _ = w.Write(campaign.TransparentGIF())
}

// The destination is signed into the token, so a tampered link cannot become an open redirect
func (m mail) click(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/e/c/")
	var dest string
	if allow(m.Limiter, r, "eclick") {
		var ev *campaign.Engaged
		if dest, ev = m.Campaigns.RecordClick(r.Context(), token, hit(r)); ev != nil {
			m.engage(r.Context(), signalsapi.TriggerEmailClicked, ev)
		}
	} else {
		dest = m.Campaigns.ClickTarget(token)
	}
	if dest == "" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// A human open or click by a known customer starts the engagement-triggered flows
func (m mail) engage(ctx context.Context, trigger string, ev *campaign.Engaged) {
	if m.Flows == nil {
		return
	}
	m.Flows.Enroll(ctx, trigger, ev.ChannelID, ev.CustomerID, "", nil, "")
}

func hit(r *http.Request) analytics.Hit {
	return analytics.Hit{UA: r.UserAgent(), IP: auth.ClientIP(r), At: time.Now()}
}
