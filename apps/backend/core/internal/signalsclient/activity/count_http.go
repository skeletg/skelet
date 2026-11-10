package activity

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
)

const CountPath = "/api/front/count"

// CountHandler takes a page view from a visitor who hasn't allowed analytics, sent by the storefront's proxy rather than
// the browser - no cookie is read or set for it. It reaches Signals as nobody, through CountView: the path alone, with no
// visitor, IP or user agent. The IP is used for the rate limit and dropped. Mount it behind channel.Middleware
func (t *Tracker) CountHandler(limiter *ratelimit.RedisLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer w.WriteHeader(http.StatusNoContent)
		ch, ok := channel.FromContext(r.Context())
		if r.Method != http.MethodPost || !ok || ch == nil {
			return
		}
		if limiter != nil && !limiter.Allow(r.Context(), "ingest:count:"+auth.ClientIP(r)) {
			return
		}
		if path, ok := countedPath(w, r); ok {
			_ = t.CountView(r.Context(), ch.ID, path)
		}
	}
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
