package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
)

// tRequest drives one /t call through the real handler - channel middleware, rate limiter and all - extra headers come in pairs
func tRequest(t *testing.T, ts *httptest.Server, method, path, channelID, body string, hdr ...string) (int, http.Header, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL+path, rdr)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if channelID != "" {
		req.Header.Set("X-Channel-Id", channelID)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, resp.Header, string(raw)
}

// The /t surface is public and best-effort - a wrong or missing channel answers 204, never an error a probe could learn the channel inventory from
func TestIngestWithoutAValidChannelFailsSoft(t *testing.T) {
	h := newHarness(t)
	ts := beaconServer(t, h)

	for name, chID := range map[string]string{
		"unknown channel": uuid.NewString(),
		"garbage header":  "not-a-uuid",
	} {
		status, _, body := tRequest(t, ts, http.MethodGet, "/t/config", chID, "")
		if status != http.StatusNoContent {
			t.Errorf("%s: GET /t/config = %d %q, want 204", name, status, body)
		}
		status, _, _ = tRequest(t, ts, http.MethodPost, "/t/collect", chID, `{"anonId":"a","events":[]}`)
		if status != http.StatusNoContent {
			t.Errorf("%s: POST /t/collect = %d, want 204", name, status)
		}
	}
}

// The channel a beacon names is checked against the suite's own copy of core's active channels, not
// against core's table - a live channel core has not pushed across is one this surface will not serve
func TestIngestServesOnlyTheChannelsCorePushed(t *testing.T) {
	h := newHarness(t)
	ch := h.channel(t)
	ts := beaconServer(t, h)

	if status, _, body := tRequest(t, ts, http.MethodGet, "/t/config", ch.String(), ""); status != http.StatusOK {
		t.Fatalf("GET /t/config for a pushed channel = %d %q, want 200", status, body)
	}
	if _, _, err := h.d.Channels.SyncChannels(h.ctx, nil); err != nil {
		t.Fatalf("empty the roster: %v", err)
	}
	if status, _, body := tRequest(t, ts, http.MethodGet, "/t/config", ch.String(), ""); status != http.StatusNoContent {
		t.Errorf("GET /t/config for a channel no longer pushed = %d %q, want 204", status, body)
	}
}

func TestIngestConfigServesTheChannelKnobs(t *testing.T) {
	h := newHarness(t)
	ch := h.channel(t)
	ts := beaconServer(t, h)

	status, hdrs, body := tRequest(t, ts, http.MethodGet, "/t/config", ch.String(), "")
	if status != http.StatusOK {
		t.Fatalf("GET /t/config = %d %q, want 200", status, body)
	}
	if cc := hdrs.Get("Cache-Control"); cc != "public, max-age=300" {
		t.Errorf("Cache-Control = %q, want the 5-minute public cache the tracker relies on", cc)
	}
	var cfg struct {
		Enabled            bool `json:"enabled"`
		SamplingRate       int  `json:"samplingRate"`
		CaptureMoves       bool `json:"captureMoves"`
		ReplayEnabled      bool `json:"replayEnabled"`
		ReplaySamplingRate int  `json:"replaySamplingRate"`
		ReplayMaxMinutes   int  `json:"replayMaxMinutes"`
	}
	if err := json.Unmarshal([]byte(body), &cfg); err != nil {
		t.Fatalf("response is not the ClientConfig JSON: %v\n%s", err, body)
	}
	if !cfg.Enabled || cfg.SamplingRate != 100 || !cfg.CaptureMoves || cfg.ReplaySamplingRate != 10 || cfg.ReplayMaxMinutes != 30 {
		t.Errorf("defaults = %+v, want tracking on out of the box", cfg)
	}

	// a per-channel override must reach the tracker - this is the operator's sampling dial, saved by
	// core over the contract into the store this process reads
	chScope := config.Scope{Kind: config.ScopeChannel, ChannelID: &ch}
	if _, err := h.d.Settings.Set(h.ctx, "signals/lens/sampling_rate", "25", chScope); err != nil {
		t.Fatalf("set sampling rate: %v", err)
	}
	_, _, body = tRequest(t, ts, http.MethodGet, "/t/config", ch.String(), "")
	if err := json.Unmarshal([]byte(body), &cfg); err != nil {
		t.Fatalf("second read: %v\n%s", err, body)
	}
	if cfg.SamplingRate != 25 {
		t.Errorf("samplingRate = %d after the channel override, want 25", cfg.SamplingRate)
	}
}

// Segment-targeted experiments are the authz-shaped part of /t: identity comes ONLY from a verified session token, so an anonymous caller (or one waving somebody's id in the body) never receives them
func TestIngestExperimentsGateOnTheVerifiedSession(t *testing.T) {
	h := newHarness(t)
	ch := h.channel(t)
	ts := beaconServer(t, h)

	open := experiment.Input{
		ChannelID: ch, Key: "e2e-open-" + uuid.NewString()[:8], Name: "Open", Status: experiment.StatusRunning,
		Coverage: 100, GoalEvent: "add_to_cart",
		Variations: []experiment.Variation{{Key: "control", Weight: 50}, {Key: "b", Weight: 50, Value: "blue"}},
	}
	if _, err := h.d.Experiment.Create(h.ctx, open); err != nil {
		t.Fatalf("create open experiment: %v", err)
	}
	targeted := open
	targeted.Key = "e2e-vip-" + uuid.NewString()[:8]
	targeted.SegmentSlug = "e2e-vip-segment-that-nobody-is-in"
	if _, err := h.d.Experiment.Create(h.ctx, targeted); err != nil {
		t.Fatalf("create targeted experiment: %v", err)
	}

	keysOf := func(body string) map[string]bool {
		var out []struct {
			Key        string `json:"key"`
			Variations []any  `json:"variations"`
		}
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatalf("experiments response: %v\n%s", err, body)
		}
		keys := map[string]bool{}
		for _, e := range out {
			keys[e.Key] = true
			if len(e.Variations) != 2 {
				t.Errorf("experiment %s has %d variations in the payload, want 2", e.Key, len(e.Variations))
			}
		}
		return keys
	}

	status, hdrs, body := tRequest(t, ts, http.MethodGet, "/t/experiments", ch.String(), "")
	if status != http.StatusOK {
		t.Fatalf("GET /t/experiments = %d %q", status, body)
	}
	if cc := hdrs.Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("anonymous Cache-Control = %q, want the public minute cache", cc)
	}
	anon := keysOf(body)
	if !anon[open.Key] {
		t.Errorf("the open experiment is missing for an anonymous visitor: %v", anon)
	}
	if anon[targeted.Key] {
		t.Error("a segment-targeted experiment leaked to an anonymous visitor")
	}

	// a verified customer who is NOT in the segment still doesn't get it, and the response goes uncacheable
	// The customer is core's; here they are a token, which is all this surface ever sees of one
	custID := uuid.New()
	tok := h.customerToken(t, custID, ch, "exp-"+uuid.NewString()[:8]+"@e2e.test")
	_, hdrs, body = tRequest(t, ts, http.MethodGet, "/t/experiments", ch.String(), "", "Authorization", "Bearer "+tok)
	if cc := hdrs.Get("Cache-Control"); cc != "private, no-store" {
		t.Errorf("authenticated Cache-Control = %q, want private no-store - a shared cache must never serve one customer's targeting to another", cc)
	}
	authed := keysOf(body)
	if authed[targeted.Key] {
		t.Error("a segment-targeted experiment reached a customer outside the segment")
	}
	if !strings.Contains(hdrs.Get("Vary"), "Authorization") {
		t.Errorf("Vary = %q, want Authorization in it", hdrs.Get("Vary"))
	}
}

func TestIngestBeaconsAlwaysAnswer204(t *testing.T) { // beacons are fire-and-forget from a shopper's browser - no payload shape may turn into an error the page sees
	h := newHarness(t)
	ch := h.channel(t)
	ts := beaconServer(t, h)

	for name, call := range map[string]struct{ path, body string }{
		"collect ok":        {"/t/collect", `{"anonId":"e2e-anon","events":[{"type":"page_view","page":"/"}]}`},
		"collect malformed": {"/t/collect", `{"events":[`},
		"collect empty":     {"/t/collect", ``},
		"rec malformed":     {"/t/rec", `not json`},
		"crawl bot":         {"/t/crawl", `{"path":"/p/tire","ua":"Mozilla/5.0 (compatible; Googlebot/2.1)"}`},
		"crawl human ua":    {"/t/crawl", `{"path":"/p/tire","ua":"Mozilla/5.0 (Windows NT 10.0) Chrome/126.0"}`},
	} {
		status, _, body := tRequest(t, ts, http.MethodPost, call.path, ch.String(), call.body)
		if status != http.StatusNoContent {
			t.Errorf("%s: POST %s = %d %q, want 204", name, call.path, status, body)
		}
	}
	// Only a route is a beacon - anything else is refused before it can reach the limiter or the spine
	if status, _, _ := tRequest(t, ts, http.MethodPost, "/t/whatever", ch.String(), `{}`); status != http.StatusNotFound {
		t.Errorf("POST /t/whatever = %d, want 404", status)
	}
	if status, _, _ := tRequest(t, ts, http.MethodPost, "/t/config", ch.String(), `{}`); status != http.StatusMethodNotAllowed {
		t.Errorf("POST /t/config = %d, want 405", status)
	}
}

// Over-limit GETs get an honest 429 through the wired limiter - the silent 204 for an over-limit POST is pinned in beaconhttp, since
// 300 POSTs here would answer the same either way. This runs last among the /t tests, so the drained bucket starves nothing after it
func TestIngestRateLimitThrottlesGets(t *testing.T) {
	h := newHarness(t)
	ch := h.channel(t)
	ts := beaconServer(t, h)

	var got429 bool
	for i := range 320 {
		status, _, _ := tRequest(t, ts, http.MethodGet, "/t/experiments", ch.String(), "")
		if status == http.StatusTooManyRequests {
			got429 = true
			break
		}
		if status != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 until the limit", i, status)
		}
	}
	if !got429 {
		t.Fatal("320 GETs never hit the rate limit, want a 429 within the 300/min budget")
	}
}
