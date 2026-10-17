package beaconhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
)

var (
	activeCh   = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	defaultCh  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	inactiveCh = uuid.MustParse("33333333-3333-3333-3333-333333333333")
)

type channels struct{}

func (channels) Active(_ context.Context, id uuid.UUID) bool {
	return id == activeCh || id == defaultCh
}
func (channels) Default(context.Context) (uuid.UUID, bool) { return defaultCh, true }

type limiter struct {
	mu    sync.Mutex
	deny  bool
	asked []string
}

func (l *limiter) Allow(_ context.Context, key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.asked = append(l.asked, key)
	return !l.deny
}

type fakeLens struct {
	channel  uuid.UUID
	events   int
	customer uuid.UUID
	calls    int
}

func (f *fakeLens) IngestConfig(context.Context, uuid.UUID) lens.ClientConfig {
	return lens.ClientConfig{Enabled: true}
}

func (f *fakeLens) Ingest(_ context.Context, ch uuid.UUID, _ string, _ analytics.Hit, b lens.Batch, verified uuid.UUID) {
	f.calls++
	f.channel, f.events, f.customer = ch, len(b.Events), verified
}

func (f *fakeLens) IngestReplay(context.Context, uuid.UUID, string, lens.ReplayBatch, uuid.UUID) {
	f.calls++
}

type experiments struct{ list []experiment.Experiment }

func (e experiments) Active(context.Context, uuid.UUID) ([]experiment.Experiment, error) {
	return e.list, nil
}

type segments map[uuid.UUID][]string

func (s segments) SlugsForCustomer(_ context.Context, id uuid.UUID) ([]string, error) {
	return s[id], nil
}

type noGeo struct{}

func (noGeo) Country(string) string { return "" }

type bots struct{ pages []string }

func (b *bots) AddBot(_ uuid.UUID, page, _, _ string, _ time.Time) { b.pages = append(b.pages, page) }

func newSigner(t *testing.T) auth.CustomerSigner {
	t.Helper()
	seed, _, err := auth.GenerateCustomerKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := auth.NewCustomerSigner(seed)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type trackerKit struct {
	h       http.Handler
	lens    *fakeLens
	limiter *limiter
	bots    *bots
	signer  auth.CustomerSigner
}

func newTracker(t *testing.T, exps []experiment.Experiment, segs segments) trackerKit {
	k := trackerKit{lens: &fakeLens{}, limiter: &limiter{}, bots: &bots{}, signer: newSigner(t)}
	k.h = Tracker(TrackerOptions{
		Channels: channels{}, Lens: k.lens, Experiments: experiments{exps}, Segments: segs,
		Geo: noGeo{}, Rollup: k.bots, Limiter: k.limiter, Customers: k.signer.Verifier(),
	})
	return k
}

func do(h http.Handler, method, path, channel, body string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if channel != "" {
		r.Header.Set("X-Channel-Id", channel)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAChannelNamingNoActiveStoreIsDropped(t *testing.T) {
	k := newTracker(t, nil, nil)
	for _, ch := range []string{inactiveCh.String(), "not-a-uuid"} {
		if w := do(k.h, http.MethodPost, "/t/collect", ch, `{"events":[{"type":"page_view"}]}`); w.Code != http.StatusNoContent {
			t.Errorf("%s: status %d, want 204", ch, w.Code)
		}
	}
	if k.lens.calls != 0 || len(k.limiter.asked) != 0 {
		t.Fatalf("an unresolved channel reached the limiter (%d) or the spine (%d)", len(k.limiter.asked), k.lens.calls)
	}
	do(k.h, http.MethodPost, "/t/collect", "", `{"events":[{"type":"page_view"}]}`)
	if k.lens.channel != defaultCh {
		t.Errorf("a header-less beacon landed on %s, want the default store", k.lens.channel)
	}
}

func TestOverTheLimitGetsRefuseHonestlyAndPostsDropQuietly(t *testing.T) {
	k := newTracker(t, nil, nil)
	k.limiter.deny = true
	if w := do(k.h, http.MethodGet, "/t/config", activeCh.String(), ""); w.Code != http.StatusTooManyRequests {
		t.Errorf("over-limit GET = %d, want 429", w.Code)
	}
	if w := do(k.h, http.MethodPost, "/t/collect", activeCh.String(), `{"events":[{"type":"page_view"}]}`); w.Code != http.StatusNoContent {
		t.Errorf("over-limit POST = %d, want 204", w.Code)
	}
	if k.lens.calls != 0 {
		t.Error("an over-limit beacon reached the spine")
	}
	if got := k.limiter.asked; len(got) != 2 || !strings.HasPrefix(got[0], "ingest:config:") || !strings.HasPrefix(got[1], "ingest:collect:") {
		t.Errorf("limiter keys = %v, want one bucket per route", got)
	}
}

func TestUnknownRoutesAndMethodsAreRefused(t *testing.T) {
	k := newTracker(t, nil, nil)
	if w := do(k.h, http.MethodPost, "/t/config", activeCh.String(), ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /t/config = %d, want 405", w.Code)
	}
	if w := do(k.h, http.MethodGet, "/t/anything", activeCh.String(), ""); w.Code != http.StatusNotFound {
		t.Errorf("GET /t/anything = %d, want 404", w.Code)
	}
	if len(k.limiter.asked) != 0 {
		t.Errorf("an unrouted request minted limiter keys: %v", k.limiter.asked)
	}
}

// Both reads answer per store, so a shared cache keyed on the URL alone would hand one store's answer to another
func TestPerChannelReadsVaryOnTheChannel(t *testing.T) {
	k := newTracker(t, nil, nil)
	for _, path := range []string{"/t/config", "/t/experiments"} {
		w := do(k.h, http.MethodGet, path, activeCh.String(), "")
		if !strings.Contains(w.Header().Get("Vary"), "X-Channel-Id") {
			t.Errorf("%s Vary = %q, want X-Channel-Id", path, w.Header().Get("Vary"))
		}
	}
}

func TestSegmentExperimentsNeedAVerifiedSession(t *testing.T) {
	cust := uuid.New()
	exps := []experiment.Experiment{{Key: "open"}, {Key: "vip-only", SegmentSlug: "vip"}}
	k := newTracker(t, exps, segments{cust: {"vip"}})
	keys := func(w *httptest.ResponseRecorder) []string {
		var out []struct{ Key string }
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		var ks []string
		for _, e := range out {
			ks = append(ks, e.Key)
		}
		return ks
	}

	anon := do(k.h, http.MethodGet, "/t/experiments", activeCh.String(), "")
	if got := keys(anon); len(got) != 1 || got[0] != "open" || anon.Header().Get("Cache-Control") != "public, max-age=60" {
		t.Errorf("anonymous got %v (%s), want only the open experiment, publicly cacheable", got, anon.Header().Get("Cache-Control"))
	}
	tok, err := auth.GenerateCustomerToken(cust, activeCh, "c@example.test", k.signer, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	signed := do(k.h, http.MethodGet, "/t/experiments", activeCh.String(), "", "Authorization", "Bearer "+tok)
	if got := keys(signed); len(got) != 2 || signed.Header().Get("Cache-Control") != "private, no-store" {
		t.Errorf("verified member got %v (%s), want both, never shared-cached", got, signed.Header().Get("Cache-Control"))
	}
	otherStore := do(k.h, http.MethodGet, "/t/experiments", defaultCh.String(), "", "Authorization", "Bearer "+tok)
	if got := keys(otherStore); len(got) != 1 {
		t.Errorf("a session from another store pulled %v", got)
	}
}

func TestACollectBatchIsCapped(t *testing.T) {
	k := newTracker(t, nil, nil)
	cust := uuid.New()
	body := `{"anonId":"a","customerId":"` + cust.String() + `","events":[` + strings.TrimSuffix(strings.Repeat(`{"type":"page_view"},`, maxCollectEvents+50), ",") + `]}`
	do(k.h, http.MethodPost, "/t/collect", activeCh.String(), body)
	if k.lens.events != maxCollectEvents {
		t.Errorf("spine got %d events from one batch, want the %d cap", k.lens.events, maxCollectEvents)
	}
	if k.lens.customer != uuid.Nil {
		t.Error("a body-claimed customer id was stitched without a session")
	}
}

func TestCrawlBeaconsCountOnlyBots(t *testing.T) {
	k := newTracker(t, nil, nil)
	do(k.h, http.MethodPost, "/t/crawl", activeCh.String(), `{"path":"/p/1","ua":"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"}`)
	do(k.h, http.MethodPost, "/t/crawl", activeCh.String(), `{"path":"/p/2","ua":"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0"}`)
	if len(k.bots.pages) != 1 || k.bots.pages[0] != "/p/1" {
		t.Errorf("crawl rollup got %v, want only the bot's page", k.bots.pages)
	}
}

type campaigns struct {
	targets  map[string]string
	recorded int
}

func (c *campaigns) RecordOpen(context.Context, string, analytics.Hit) *campaign.Engaged {
	c.recorded++
	return nil
}

func (c *campaigns) RecordClick(_ context.Context, token string, _ analytics.Hit) (string, *campaign.Engaged) {
	c.recorded++
	return c.targets[token], nil
}

func (c *campaigns) ClickTarget(token string) string { return c.targets[token] }

func TestAClickAlwaysRedirectsAndAForgedOneNever(t *testing.T) {
	c := &campaigns{targets: map[string]string{"good": "https://shop.example/sale"}}
	l := &limiter{}
	h := Mail(MailOptions{Campaigns: c, Limiter: l})

	if w := do(h, http.MethodGet, "/e/c/good", "", ""); w.Code != http.StatusFound || w.Header().Get("Location") != "https://shop.example/sale" {
		t.Fatalf("click = %d → %q", w.Code, w.Header().Get("Location"))
	}
	l.deny = true
	if w := do(h, http.MethodGet, "/e/c/good", "", ""); w.Code != http.StatusFound || w.Header().Get("Location") != "https://shop.example/sale" {
		t.Errorf("over-limit click = %d → %q, want the redirect anyway", w.Code, w.Header().Get("Location"))
	}
	if c.recorded != 1 {
		t.Errorf("recorded %d clicks, want the over-limit one skipped", c.recorded)
	}
	if w := do(h, http.MethodGet, "/e/c/forged", "", ""); w.Code != http.StatusNotFound {
		t.Errorf("forged click = %d, want 404", w.Code)
	}
	if w := do(h, http.MethodGet, "/e/o/anything", "", ""); w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/gif" {
		t.Errorf("over-limit open = %d %q, want the pixel", w.Code, w.Header().Get("Content-Type"))
	}
	if c.recorded != 1 {
		t.Error("an over-limit open was recorded")
	}
}
