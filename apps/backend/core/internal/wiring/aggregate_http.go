package wiring

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
)

const countPath = "/api/front/count"

type activityWriter interface {
	RecordActivity(ctx context.Context, a signalsclient.ActivitySpec) error
}

// A page view from a visitor who hasn't allowed analytics, sent by the storefront's proxy rather than the browser - no
// cookie is read or set for it. It reaches Signals as nobody: the path alone, with no visitor, IP or user agent, so it
// counts in totals and never as a person. The IP is used for the rate limit and dropped
func countHandler(rec activityWriter, cfg *config.Service, channels *channel.Service, limiter *ratelimit.RedisLimiter) http.Handler {
	return channel.Middleware(channels)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer w.WriteHeader(http.StatusNoContent)
		ch, ok := channel.FromContext(r.Context())
		if r.Method != http.MethodPost || !ok || ch == nil || rec == nil {
			return
		}
		if limiter != nil && !limiter.Allow(r.Context(), "ingest:count:"+auth.ClientIP(r)) {
			return
		}
		path, ok := countedPath(w, r)
		if !ok {
			return
		}
		if on, err := cfg.GetBool(r.Context(), "general/privacy/aggregate_counts", config.Scope{Kind: config.ScopeChannel, ChannelID: &ch.ID}); err == nil && !on {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		_ = rec.RecordActivity(ctx, signalsclient.ActivitySpec{
			EventName: "page_view", Source: "aggregate", ChannelID: ch.ID, At: time.Now().UTC(),
			Props: map[string]any{"path": path}, DedupeKey: "agg:" + uuid.NewString(), // every view is its own
		})
	}))
}

var idSegment = regexp.MustCompile(`^(?:[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}|\d{6,}|[A-Za-z0-9_]{20,})$`)

// countedPath reads the path, without a query, with any segment that looks like an id (an order's uuid, a token) made
// ":id" - a path like /orders/<uuid> would otherwise name the order it shows
func countedPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	var in struct {
		Path string `json:"path"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in) != nil || !strings.HasPrefix(in.Path, "/") || len(in.Path) > 512 {
		return "", false
	}
	path, _, _ := strings.Cut(in.Path, "?")
	path, _, _ = strings.Cut(path, "#")
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if idSegment.MatchString(s) {
			segs[i] = ":id"
		}
	}
	return strings.Join(segs, "/"), true
}
