package wiring

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	cachepkg "github.com/skeletg/skelet/apps/backend/core/lib/cache"
)

func ctxWith(ch *channel.Channel, loc string) context.Context {
	ctx := context.Background()
	if ch != nil {
		ctx = channel.WithChannel(ctx, ch)
	}
	if loc != "" {
		ctx = locale.WithLocale(ctx, loc)
	}
	return ctx
}

func keyFor(ctx context.Context, op string, body string) string {
	req := httptest.NewRequest(http.MethodPost, "/api/front/graphql", strings.NewReader(body))
	return frontRespCacheKey(req.WithContext(ctx), op, "", []byte(body))
}

func TestFrontRespCacheKeyIsolation(t *testing.T) {
	us := &channel.Channel{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111")}
	eu := &channel.Channel{ID: uuid.MustParse("22222222-2222-2222-2222-222222222222")}
	body := `{"operationName":"Header","query":"query Header { locales { code } }"}`

	base := keyFor(ctxWith(us, "en"), "Header", body)

	// Cross-channel bleed is the prod incident this guards - host-resolved siblings carry no distinguishing header, only the resolved channel in context
	if same := keyFor(ctxWith(eu, "en"), "Header", body); same == base {
		t.Fatal("two channels share a cache key - cross-channel response bleed")
	}
	if same := keyFor(ctxWith(nil, "en"), "Header", body); same == base {
		t.Fatal("unresolved channel shares a cache key with a resolved one")
	}
	if same := keyFor(ctxWith(us, "de"), "Header", body); same == base {
		t.Fatal("two locales share a cache key")
	}
	otherBody := `{"operationName":"Header","query":"query Header { locales { name } }"}`
	if same := keyFor(ctxWith(us, "en"), "Header", otherBody); same == base {
		t.Fatal("different bodies share a cache key")
	}
	if again := keyFor(ctxWith(us, "en"), "Header", body); again != base {
		t.Fatal("identical request derived a different key - cache would never hit")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/front/graphql", strings.NewReader(body)).WithContext(ctxWith(us, "en"))
	if frontRespCacheKey(req, "Header", "nav@a.b", []byte(body)) == frontRespCacheKey(req, "Header", "nav@a.c", []byte(body)) {
		t.Fatal("two purge epochs share a cache key - a purge wouldn't orphan the entries built before it")
	}
}

// Nil-client cache is fail-open: lookups miss, stores no-op - which still exercises every gate in the middleware via the X-Cache marker (only cacheable paths set it)
func cacheProbe(t *testing.T, mutate func(*http.Request)) string {
	t.Helper()
	mw := frontResponseCache(cachepkg.New(nil, "frontresp"), frontRespEpochs{}, time.Minute)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	body := `{"operationName":"Header","query":"query Header { locales { code } }"}`
	req := httptest.NewRequest(http.MethodPost, "/api/front/graphql", strings.NewReader(body))
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("request failed with %d", rec.Code)
	}
	return rec.Header().Get("X-Cache")
}

func TestStorableFrontResponse(t *testing.T) {
	clean := []byte(`{"data":{"currentChannel":{"slug":"us"}}}`)
	if !storableFrontResponse(http.StatusOK, clean) {
		t.Fatal("clean 200 should be storable")
	}
	// The sticky-error incident: a transient 200-with-errors cached for the full TTL
	withErrors := []byte(`{"errors":[{"message":"internal error"}],"data":null}`)
	if storableFrontResponse(http.StatusOK, withErrors) {
		t.Fatal("a GraphQL error payload must never be cached")
	}
	if storableFrontResponse(http.StatusInternalServerError, clean) {
		t.Fatal("non-200 must never be cached")
	}
	// Escaped quotes inside data strings do NOT trip the marker - only a literal "errors" JSON key does, and that over-match fails safe by skipping the cache
	escaped := []byte(`{"data":{"page":{"body":"how we handle \"errors\" gracefully"}}}`)
	if !storableFrontResponse(http.StatusOK, escaped) {
		t.Fatal("escaped marker inside a data string should still be storable")
	}
	fieldNamedErrors := []byte(`{"data":{"validation":{"errors":[]}}}`)
	if storableFrontResponse(http.StatusOK, fieldNamedErrors) {
		t.Fatal("a data field literally named errors should fail safe and skip the cache")
	}
}

func TestFrontResponseCacheGates(t *testing.T) {
	if got := cacheProbe(t, nil); got != "MISS" {
		t.Fatalf("anonymous allowlisted op should enter the cache path, got X-Cache=%q", got)
	}
	if got := cacheProbe(t, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer token")
	}); got != "" {
		t.Fatalf("authenticated request must bypass the cache, got X-Cache=%q", got)
	}
	if got := cacheProbe(t, func(r *http.Request) {
		r.Header.Set("X-Review-Voter-Token", "voter")
	}); got != "" {
		t.Fatalf("a guest's review votes must not be cached for other guests, got X-Cache=%q", got)
	}
	if got := cacheProbe(t, func(r *http.Request) {
		r.Body = http.NoBody
		r.Method = http.MethodGet
	}); got != "" {
		t.Fatalf("non-POST must bypass the cache, got X-Cache=%q", got)
	}
	if got := cacheProbe(t, func(r *http.Request) {
		r.Body = io.NopCloser(strings.NewReader(`{"operationName":"Cart","query":"query Cart { cart { id } }"}`))
	}); got != "" {
		t.Fatalf("non-allowlisted op must bypass the cache, got X-Cache=%q", got)
	}
}

func liveValkey(t *testing.T) *redis.Client {
	t.Helper()
	dsn := os.Getenv("VALKEY_URL")
	if dsn == "" {
		t.Skip("set VALKEY_URL to exercise the purge epochs")
	}
	opt, err := redis.ParseURL(dsn)
	if err != nil {
		t.Skipf("parse VALKEY_URL: %v", err)
	}
	c := redis.NewClient(opt)
	if err := c.Ping(context.Background()).Err(); err != nil {
		_ = c.Close()
		t.Skipf("valkey unreachable: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestAPurgeOrphansTheResponsesItsSurfaceCached(t *testing.T) {
	rdb := liveValkey(t)
	epochs := frontRespEpochs{rdb}
	surface := "test" + strings.Map(func(r rune) rune { return 'a' + (r % 26) }, uuid.NewString()[:8])
	t.Cleanup(func() { rdb.Del(context.Background(), frontRespEpochKey(surface)) })
	ch := &channel.Channel{ID: uuid.New()}
	served := 0
	const servedBody = `{"data":{"page":{"body":"<p>Fish & chips</p>","slug":"us"}}}`
	var duringRequest func()
	mw := frontResponseCache(cachepkg.New(rdb, "frontresp"), epochs, time.Minute)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served++
		if duringRequest != nil {
			duringRequest()
		}
		_, _ = io.WriteString(w, servedBody)
	}))
	var lastBody string
	call := func(withSurface bool) string {
		body := `{"operationName":"Header","query":"query Header { currentChannel { slug } }"}`
		req := httptest.NewRequest(http.MethodPost, "/api/front/graphql", strings.NewReader(body)).WithContext(ctxWith(ch, "en"))
		if withSurface {
			req.Header.Set(frontRespSurfaceHeader, surface)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		lastBody = rec.Body.String()
		return rec.Header().Get("X-Cache")
	}

	if a, b := call(true), call(true); a != "MISS" || b != "HIT" {
		t.Fatalf("got %s then %s, want MISS then HIT", a, b)
	}
	if lastBody != servedBody {
		t.Fatalf("a hit served %q, want the origin's bytes %q unchanged", lastBody, servedBody)
	}
	if a, b := call(false), call(false); a != "MISS" || b != "HIT" {
		t.Fatalf("headerless got %s then %s, want MISS then HIT", a, b)
	}
	epochs.bump(context.Background(), surface)
	if got := call(true); got != "MISS" {
		t.Fatalf("after a purge of its surface got %s, want MISS", got)
	}
	if got := call(false); got != "HIT" {
		t.Fatalf("a headerless entry got %s after a surface purge, want it untouched", got)
	}

	epochs.bump(context.Background(), surface)
	duringRequest = func() { epochs.bump(context.Background(), surface) }
	if got := call(true); got != "MISS" {
		t.Fatalf("got %s, want MISS", got)
	}
	duringRequest = nil
	before := served
	if got := call(true); got != "MISS" || served != before+1 {
		t.Fatalf("got %s after a purge landed mid-request, want MISS - the in-flight answer must not be served", got)
	}
}

func TestADeadEpochStoreBypassesTheCache(t *testing.T) {
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = dead.Close() })
	mw := frontResponseCache(cachepkg.New(dead, "frontresp"), frontRespEpochs{dead}, time.Minute)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodPost, "/api/front/graphql", strings.NewReader(`{"operationName":"Header","query":"query Header { locales { code } }"}`))
	req.Header.Set(frontRespSurfaceHeader, "nav")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("X-Cache") != "" {
		t.Fatalf("got %d X-Cache=%q, want the request served uncached", rec.Code, rec.Header().Get("X-Cache"))
	}
}
