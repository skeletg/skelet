package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/channel"
)

func resetRoutes(t *testing.T) *Host {
	t.Helper()
	clear := func() {
		routeRegistry.mu.Lock()
		routeRegistry.routes = nil
		routeRegistry.mu.Unlock()
	}
	clear()
	t.Cleanup(clear)
	return &Host{}
}

func callRoute(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	h := RouteHandler()
	if h == nil {
		t.Fatal("RouteHandler returned nil with a route registered")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Instrument recovers the panic, so without the wrapper writing a status the handler
// returns having touched nothing and net/http reports the crash to the caller as 200 OK
func TestPanickingRouteIs500(t *testing.T) {
	h := resetRoutes(t)
	_ = h.RegisterRoute("boom", "GET explode", func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	})

	rec := callRoute(t, httptest.NewRequest("GET", "/api/v1/plugins/boom/explode", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("panicking route answered %d, want 500 (a recovered panic must not read as success)", rec.Code)
	}
}

// A handler that already answered before dying keeps its response - stacking a second
// header on top would only produce a superfluous-WriteHeader log and a corrupt body
func TestPanicAfterResponseLeavesItAlone(t *testing.T) {
	h := resetRoutes(t)
	_ = h.RegisterRoute("boom", "GET half", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"partial":true}`))
		panic("died mid-response")
	})

	rec := callRoute(t, httptest.NewRequest("GET", "/api/v1/plugins/boom/half", nil))

	if rec.Code != http.StatusAccepted {
		t.Errorf("status %d overwrote the handler's own 202", rec.Code)
	}
	if rec.Body.String() != `{"partial":true}` {
		t.Errorf("body %q was not left as the handler wrote it", rec.Body.String())
	}
}

// Administratively off is 503, so a caller knows to stop rather than to retry harder
func TestDisabledPluginRouteIs503(t *testing.T) {
	h := resetRoutes(t)
	ran := false
	_ = h.RegisterRoute("off", "GET ping", func(http.ResponseWriter, *http.Request) { ran = true })
	disableAll(t)

	rec := callRoute(t, httptest.NewRequest("GET", "/api/v1/plugins/off/ping", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("disabled plugin route answered %d, want 503", rec.Code)
	}
	if ran {
		t.Error("disabled plugin's handler still ran")
	}
}

// Over the allowance is 429 - collapsing it into the disabled 503 leaves a client
// unable to tell "come back never" from "come back in a minute"
func TestRateLimitedRouteIs429(t *testing.T) {
	h := resetRoutes(t)
	ran := false
	_ = h.RegisterRoute("busy", "GET ping", func(http.ResponseWriter, *http.Request) { ran = true })

	orig := checkRateLimitFn
	t.Cleanup(func() { checkRateLimitFn = orig })
	checkRateLimitFn = func(context.Context, string) error { return ErrPluginRateLimited }

	rec := callRoute(t, httptest.NewRequest("GET", "/api/v1/plugins/busy/ping", nil))

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("rate-limited route answered %d, want 429", rec.Code)
	}
	if ran {
		t.Error("rate-limited plugin's handler still ran")
	}
}

// The channel middleware runs in front of these routes, so "disable this plugin on
// the EU channel" has to reach the route the way it reaches the payment hooks
func TestRouteGatesOnRequestChannel(t *testing.T) {
	h := resetRoutes(t)
	_ = h.RegisterRoute("scoped", "GET ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	want := uuid.New()
	var got uuid.UUID
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })
	isEnabledFromContext = func(_ context.Context, _ string, id uuid.UUID) bool {
		got = id
		return true
	}

	req := httptest.NewRequest("GET", "/api/v1/plugins/scoped/ping", nil)
	req = req.WithContext(channel.WithChannel(req.Context(), &channel.Channel{ID: want}))
	callRoute(t, req)

	if got != want {
		t.Errorf("kill switch consulted channel %v, want the request's %v", got, want)
	}
}

// No channel resolved (an unscoped caller) still has to answer something, and default
// scope is the same fallback the event handlers and health probes use
func TestRouteWithoutChannelUsesDefaultScope(t *testing.T) {
	h := resetRoutes(t)
	_ = h.RegisterRoute("scoped", "GET ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	got := uuid.New()
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })
	isEnabledFromContext = func(_ context.Context, _ string, id uuid.UUID) bool {
		got = id
		return true
	}

	callRoute(t, httptest.NewRequest("GET", "/api/v1/plugins/scoped/ping", nil))

	if got != uuid.Nil {
		t.Errorf("unscoped request gated on channel %v, want default scope", got)
	}
}

func TestHealthyRoutePassesThrough(t *testing.T) {
	h := resetRoutes(t)
	_ = h.RegisterRoute("ok", "GET ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})

	rec := callRoute(t, httptest.NewRequest("GET", "/api/v1/plugins/ok/ping", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "pong" {
		t.Errorf("healthy route answered %d %q", rec.Code, rec.Body.String())
	}
}

// nil lets main.go skip mounting the prefix entirely rather than serving 404s from it
func TestNoRoutesMeansNoHandler(t *testing.T) {
	resetRoutes(t)
	if RouteHandler() != nil {
		t.Error("RouteHandler returned a handler with nothing registered")
	}
}

func TestQualifyPattern(t *testing.T) {
	cases := []struct {
		slug, pattern, want string
	}{
		{"p", "GET client-config", "GET /api/v1/plugins/p/client-config"},
		{"p", "create-order", "/api/v1/plugins/p/create-order"},
		{"p", "POST /create-order", "POST /api/v1/plugins/p/create-order"},
		{"payment-paypal", "  POST   capture  ", "POST /api/v1/plugins/payment-paypal/capture"},
	}
	for _, c := range cases {
		if got := qualifyPattern(c.slug, c.pattern); got != c.want {
			t.Errorf("qualifyPattern(%q, %q) = %q, want %q", c.slug, c.pattern, got, c.want)
		}
	}
}
