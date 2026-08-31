package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/config"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/internal/form"
)

// tRequest drives one /t call through the real root handler - channel middleware, rate limiter and all - extra headers come in pairs
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
	ts := httptest.NewServer(h.d.Root)
	defer ts.Close()

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

func TestIngestConfigServesTheChannelKnobs(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 1000)
	ts := httptest.NewServer(h.d.Root)
	defer ts.Close()

	status, hdrs, body := tRequest(t, ts, http.MethodGet, "/t/config", f.Channel.ID.String(), "")
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

	// a per-channel override must reach the tracker - this is the operator's sampling dial
	chScope := config.Scope{Kind: config.ScopeChannel, ChannelID: &f.Channel.ID}
	if _, err := h.d.Config.Set(h.ctx, "pulse/lens/sampling_rate", "25", chScope); err != nil {
		t.Fatalf("set sampling rate: %v", err)
	}
	_, _, body = tRequest(t, ts, http.MethodGet, "/t/config", f.Channel.ID.String(), "")
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
	f := h.newStore(t, 5, 1000)
	ts := httptest.NewServer(h.d.Root)
	defer ts.Close()

	open := experiment.Input{
		ChannelID: f.Channel.ID, Key: "e2e-open-" + uuid.NewString()[:8], Name: "Open", Status: experiment.StatusRunning,
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

	status, hdrs, body := tRequest(t, ts, http.MethodGet, "/t/experiments", f.Channel.ID.String(), "")
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
	cust, err := h.d.Customer.Register(h.ctx, f.Channel.ID, "exp-"+uuid.NewString()[:8]+"@e2e.test", "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	tok := h.customerToken(t, cust.ID, f.Channel.ID, cust.Email)
	_, hdrs, body = tRequest(t, ts, http.MethodGet, "/t/experiments", f.Channel.ID.String(), "", "Authorization", "Bearer "+tok)
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

func TestIngestBackInStockRecordsTheRequest(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 0, 1000)
	ts := httptest.NewServer(h.d.Root)
	defer ts.Close()

	pending := func() []db.ListPendingBackInStockRow {
		rows, err := h.d.Queries.ListPendingBackInStock(h.ctx, db.ListPendingBackInStockParams{
			ProductID: f.Product.ID, ChannelID: f.Channel.ID,
		})
		if err != nil {
			t.Fatalf("list pending: %v", err)
		}
		return rows
	}

	// a guest subscribes from the PDP - the email is normalized, no customer attached
	body := fmt.Sprintf(`{"productId":%q,"sku":%q,"email":"  Guest@E2E.test ","name":"Tire","url":"https://shop/p"}`, f.Product.ID, f.Product.SKU)
	status, _, _ := tRequest(t, ts, http.MethodPost, "/t/back-in-stock", f.Channel.ID.String(), body)
	if status != http.StatusNoContent {
		t.Fatalf("POST /t/back-in-stock = %d, want 204", status)
	}
	rows := pending()
	if len(rows) != 1 {
		t.Fatalf("%d pending requests, want 1", len(rows))
	}
	if rows[0].Email != "guest@e2e.test" {
		t.Errorf("email = %q, want it lowered and trimmed", rows[0].Email)
	}
	if rows[0].CustomerID.Valid {
		t.Error("a guest request carries a customer id")
	}

	// a logged-in shopper's identity comes from the session cookie, never the body
	cust, err := h.d.Customer.Register(h.ctx, f.Channel.ID, "bis-"+uuid.NewString()[:8]+"@e2e.test", "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	tok := h.customerToken(t, cust.ID, f.Channel.ID, cust.Email)
	body = fmt.Sprintf(`{"productId":%q,"email":%q}`, f.Product.ID, cust.Email)
	status, _, _ = tRequest(t, ts, http.MethodPost, "/t/back-in-stock", f.Channel.ID.String(), body, "Cookie", "storefront_access_token="+tok)
	if status != http.StatusNoContent {
		t.Fatalf("authenticated POST = %d, want 204", status)
	}
	rows = pending()
	if len(rows) != 2 {
		t.Fatalf("%d pending requests, want 2", len(rows))
	}
	var authed *db.ListPendingBackInStockRow
	for i := range rows {
		if rows[i].Email == cust.Email {
			authed = &rows[i]
		}
	}
	if authed == nil {
		t.Fatal("the authenticated request is not in the waiting list")
	}
	if !authed.CustomerID.Valid || authed.CustomerID.UUID != cust.ID {
		t.Errorf("customer = %+v, want the session cookie's customer resolved", authed.CustomerID)
	}

	// junk stays out but still answers 204, so a prober learns nothing
	for name, junk := range map[string]string{
		"no email":       fmt.Sprintf(`{"productId":%q}`, f.Product.ID),
		"bad product id": `{"productId":"nope","email":"a@b.test"}`,
		"not json":       `<xml/>`,
	} {
		status, _, _ := tRequest(t, ts, http.MethodPost, "/t/back-in-stock", f.Channel.ID.String(), junk)
		if status != http.StatusNoContent {
			t.Errorf("%s: status = %d, want the silent 204", name, status)
		}
	}
	if got := pending(); len(got) != 2 {
		t.Errorf("%d pending after the junk posts, want still 2", len(got))
	}
}

func TestIngestFormsServeAndSubmit(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 1000)
	ts := httptest.NewServer(h.d.Root)
	defer ts.Close()

	created, err := h.d.Form.Create(h.ctx, form.SaveInput{
		ChannelID: f.Channel.ID, Slug: "e2e-news-" + uuid.NewString()[:8], Name: "Newsletter",
		Status: "active", Type: "popup", Heading: "Join us", ButtonLabel: "Sign up",
		SuccessMessage: "Welcome aboard!", CollectName: true,
		Display: form.DisplayConfig{Trigger: "immediate"},
	})
	if err != nil {
		t.Fatalf("create form: %v", err)
	}

	status, _, body := tRequest(t, ts, http.MethodGet, "/t/forms", f.Channel.ID.String(), "")
	if status != http.StatusOK {
		t.Fatalf("GET /t/forms = %d %q", status, body)
	}
	var forms []map[string]any
	if err := json.Unmarshal([]byte(body), &forms); err != nil {
		t.Fatalf("forms response: %v\n%s", err, body)
	}
	var mine map[string]any
	for _, fm := range forms {
		if fm["slug"] == created.Slug {
			mine = fm
		}
	}
	if mine == nil {
		t.Fatalf("the active form is not served: %s", body)
	}
	if mine["heading"] != "Join us" || mine["successMessage"] != "Welcome aboard!" {
		t.Errorf("form projection = %v, want the storefront fields", mine)
	}
	// the projection is public-safe - segment and consent internals stay server-side
	for key := range mine {
		if strings.Contains(strings.ToLower(key), "segment") || strings.Contains(strings.ToLower(key), "consent") {
			t.Errorf("public form payload leaks %q", key)
		}
	}

	status, _, body = tRequest(t, ts, http.MethodPost, "/t/forms/submit",
		f.Channel.ID.String(), fmt.Sprintf(`{"form":%q,"email":"lead@e2e.test","name":"Ana"}`, created.Slug))
	if status != http.StatusOK {
		t.Fatalf("submit = %d %q", status, body)
	}
	var out map[string]string
	_ = json.Unmarshal([]byte(body), &out)
	if out["message"] != "Welcome aboard!" {
		t.Errorf("message = %q, want the form's success message", out["message"])
	}
	n, err := h.d.Form.SubmissionCount(h.ctx, created.ID)
	if err != nil || n != 1 {
		t.Errorf("submissions = %d (%v), want the lead recorded once", n, err)
	}

	// a broken body is the caller's fault and says so - unlike the beacons, the widget shows this error
	status, _, _ = tRequest(t, ts, http.MethodPost, "/t/forms/submit", f.Channel.ID.String(), `{"form":`)
	if status != http.StatusBadRequest {
		t.Errorf("malformed submit = %d, want 400", status)
	}
	status, _, body = tRequest(t, ts, http.MethodPost, "/t/forms/submit", f.Channel.ID.String(), `{"form":"no-such-form","email":"a@b.test"}`)
	if status != http.StatusBadRequest {
		t.Errorf("unknown form = %d %q, want 400", status, body)
	}

	// the master switch takes every form off the storefront without touching them
	if _, err := h.d.Config.Set(h.ctx, "pulse/forms/enabled", "false", config.DefaultScope); err != nil {
		t.Fatalf("flip master switch: %v", err)
	}
	defer func() { _, _ = h.d.Config.Set(h.ctx, "pulse/forms/enabled", "true", config.DefaultScope) }()
	_, _, body = tRequest(t, ts, http.MethodGet, "/t/forms", f.Channel.ID.String(), "")
	if strings.TrimSpace(body) != "[]" {
		t.Errorf("forms with the switch off = %q, want an empty list", body)
	}
}

func TestIngestBeaconsAlwaysAnswer204(t *testing.T) { // beacons are fire-and-forget from a shopper's browser - no payload shape may turn into an error the page sees
	h := newHarness(t)
	f := h.newStore(t, 5, 1000)
	ts := httptest.NewServer(h.d.Root)
	defer ts.Close()

	for name, call := range map[string]struct{ path, body string }{
		"collect ok":        {"/t/collect", `{"anonId":"e2e-anon","events":[{"type":"page_view","page":"/"}]}`},
		"collect malformed": {"/t/collect", `{"events":[`},
		"collect empty":     {"/t/collect", ``},
		"rec malformed":     {"/t/rec", `not json`},
		"crawl bot":         {"/t/crawl", `{"path":"/p/tire","ua":"Mozilla/5.0 (compatible; Googlebot/2.1)"}`},
		"crawl human ua":    {"/t/crawl", `{"path":"/p/tire","ua":"Mozilla/5.0 (Windows NT 10.0) Chrome/126.0"}`},
		"unknown path":      {"/t/whatever", `{}`},
	} {
		status, _, body := tRequest(t, ts, http.MethodPost, call.path, f.Channel.ID.String(), call.body)
		if status != http.StatusNoContent {
			t.Errorf("%s: POST %s = %d %q, want 204", name, call.path, status, body)
		}
	}
}

// Over-limit GETs get an honest 429 while POST beacons keep answering 204, so a scraper can't tell when it's being throttled - the "other" bucket is used so this test can't starve the real endpoints
func TestIngestRateLimitThrottlesGetsSilencesPosts(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 1000)
	ts := httptest.NewServer(h.d.Root)
	defer ts.Close()

	var got429 bool
	for i := range 320 {
		status, _, _ := tRequest(t, ts, http.MethodGet, "/t/unknown-probe", f.Channel.ID.String(), "")
		if status == http.StatusTooManyRequests {
			got429 = true
			break
		}
		if status != http.StatusNoContent {
			t.Fatalf("request %d: status = %d, want 204 until the limit", i, status)
		}
	}
	if !got429 {
		t.Fatal("320 GETs never hit the rate limit, want a 429 within the 300/min budget")
	}
	// same bucket, but a POST - the refusal must stay indistinguishable from success
	status, _, _ := tRequest(t, ts, http.MethodPost, "/t/unknown-probe", f.Channel.ID.String(), `{}`)
	if status != http.StatusNoContent {
		t.Errorf("over-limit POST = %d, want the silent 204", status)
	}
}
