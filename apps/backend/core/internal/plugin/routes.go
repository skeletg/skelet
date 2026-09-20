package plugin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Namespaced per plugin so two can't collide and an access log says whose traffic it is
// What replaced the GraphQL bridge - a plugin needing a browser-reachable endpoint gets one here rather than having gqlgen's output rewritten around it

const RoutePrefix = "/api/v1/plugins/"

type pluginRoute struct {
	slug    string
	pattern string
	handler sdk.RouteHandler
}

var routeRegistry struct {
	mu     sync.Mutex
	routes []pluginRoute
}

func (h *Host) RegisterRoute(slug, pattern string, handler sdk.RouteHandler) error {
	routeRegistry.mu.Lock()
	defer routeRegistry.mu.Unlock()
	routeRegistry.routes = append(routeRegistry.routes, pluginRoute{slug: slug, pattern: pattern, handler: handler})
	return nil
}

// nil when no plugin registered a route, so main.go can skip mounting entirely
// Instrument gives these the same kill switch, rate limit, timeout and panic isolation the payment hooks get
func RouteHandler() http.Handler {
	routeRegistry.mu.Lock()
	routes := append([]pluginRoute(nil), routeRegistry.routes...)
	routeRegistry.mu.Unlock()
	if len(routes) == 0 {
		return nil
	}

	mux := http.NewServeMux()
	for _, rt := range routes {
		mux.HandleFunc(qualifyPattern(rt.slug, rt.pattern), func(w http.ResponseWriter, r *http.Request) {
			rw := &routeWriter{ResponseWriter: w}
			var err error
			// registered BEFORE finish so it runs after it - finish is what turns a recovered panic into err, and a handler that dies without this returns net/http's empty 200
			defer func() { writeRouteError(rw, err) }()
			ctx, finish, gate := Instrument(r.Context(), rt.slug, "route "+rt.pattern, routeChannelID(r.Context()))
			defer finish(&err)
			if gate != nil {
				err = gate
				return
			}
			rt.handler(rw, r.WithContext(ctx))
		})
	}
	return mux
}

// The two gate reasons need distinct codes - a client backing off has to tell "administratively off" from "slow down" - and anything else reaching here is a recovered panic, which is a 500 rather than the success net/http would otherwise report
func writeRouteError(rw *routeWriter, err error) {
	if err == nil || rw.wrote {
		return
	}
	switch {
	case errors.Is(err, ErrPluginDisabled):
		http.Error(rw, "plugin unavailable", http.StatusServiceUnavailable)
	case errors.Is(err, ErrPluginRateLimited):
		http.Error(rw, "too many requests", http.StatusTooManyRequests)
	default:
		http.Error(rw, "internal error", http.StatusInternalServerError)
	}
}

// Records whether the handler already answered, so a panic midway through a response doesn't stack a second header on top of it
type routeWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *routeWriter) WriteHeader(code int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *routeWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// Keeps http.ResponseController working through the wrapper, so a plugin streaming a response can still flush
func (w *routeWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// The channel middleware runs in front of these routes, so a per-channel kill switch has to bite here the way it bites the payment hooks - default scope only when nothing resolved
func routeChannelID(ctx context.Context) uuid.UUID {
	if ch, ok := channel.FromContext(ctx); ok {
		return ch.ID
	}
	return uuid.Nil
}

// "POST create-order" on plugin "p" becomes "POST /api/v1/plugins/p/create-order" - method stays in front, where net/http's pattern syntax wants it
func qualifyPattern(slug, pattern string) string {
	method, path := "", strings.TrimSpace(pattern)
	if i := strings.IndexByte(path, ' '); i >= 0 {
		method, path = path[:i+1], strings.TrimSpace(path[i+1:])
	}
	return method + RoutePrefix + slug + "/" + strings.TrimPrefix(path, "/")
}
