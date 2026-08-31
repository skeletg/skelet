package revalidate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/config"
)

type fakeStore struct {
	mu       sync.Mutex
	recorded []LogEntry
	recErr   error
	listErr  error
	listed   []LoggedPurge
}

func (s *fakeStore) Record(_ context.Context, e LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recErr != nil {
		return s.recErr
	}
	s.recorded = append(s.recorded, e)
	return nil
}

func (s *fakeStore) List(_ context.Context, _ int) ([]LoggedPurge, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.listed, nil
}

func (s *fakeStore) entries() []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]LogEntry, len(s.recorded))
	copy(out, s.recorded)
	return out
}

// A recording storefront stand-in - captures each request body so the wire contract can be asserted
type capturedRequest struct {
	Tags  []string `json:"tags"`
	Paths []string `json:"paths"`
	Warm  []string `json:"warm"`
}

func storefrontStub(t *testing.T, warmed int, status int) (*httptest.Server, *[]capturedRequest, *[]string) {
	t.Helper()
	var mu sync.Mutex
	reqs := &[]capturedRequest{}
	secrets := &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var got capturedRequest
		_ = json.Unmarshal(body, &got)
		mu.Lock()
		*reqs = append(*reqs, got)
		*secrets = append(*secrets, r.Header.Get("X-Revalidate-Secret"))
		mu.Unlock()
		if status >= 300 {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"warmed": warmed})
	}))
	t.Cleanup(srv.Close)
	return srv, reqs, secrets
}

// The whole layer is best-effort, and a nil client is the "no storefront wired" shape every call
// site is allowed to hold
func TestNilClientIsASafeNoOp(t *testing.T) {
	var c *Client
	ctx := context.Background()
	c.Purge(ctx, "cms", []string{"cms:all"}, nil, "test", nil)
	c.Rebuild(ctx, "cms", []string{"cms:all"}, nil, []string{"/"}, "test", nil)
	c.PurgeSurface(ctx, "cms", "test", nil)
	c.PurgeAll(ctx, "test", nil)
	if err := c.warmPath(ctx, nil, "/"); err != nil {
		t.Errorf("warmPath on a nil client = %v, want nil", err)
	}
	got, err := c.Log(ctx, 10)
	if got != nil || err != nil {
		t.Errorf("Log on a nil client = (%v, %v), want (nil, nil)", got, err)
	}
}

// Dev and CI run without a storefront, and the admin Cache page still has to show what was purged
func TestPurgeRecordsEvenWithNoStorefrontURL(t *testing.T) {
	store := &fakeStore{}
	c := New("", "secret", store)
	actor := uuid.New()

	c.Purge(context.Background(), "cms", []string{"cms:all"}, []string{"/about"}, "page edited", &actor)

	entries := store.entries()
	if len(entries) != 1 {
		t.Fatalf("%d entries recorded, want 1", len(entries))
	}
	e := entries[0]
	if e.Surface != "cms" || e.Reason != "page edited" {
		t.Errorf("entry = %+v, want the surface and reason carried through", e)
	}
	if len(e.Tags) != 1 || e.Tags[0] != "cms:all" || len(e.Paths) != 1 || e.Paths[0] != "/about" {
		t.Errorf("entry tags/paths = %v / %v, want them recorded verbatim", e.Tags, e.Paths)
	}
	if e.ActorID == nil || *e.ActorID != actor {
		t.Errorf("actor = %v, want %v so the log attributes the purge", e.ActorID, actor)
	}
}

// Nothing to invalidate means no request and no log row - an empty purge would otherwise churn the
// activity log every time an unrelated save runs
func TestPurgeWithNothingToDoDoesNothing(t *testing.T) {
	store := &fakeStore{}
	srv, reqs, _ := storefrontStub(t, 0, http.StatusOK)
	c := New(srv.URL, "secret", store)

	c.Purge(context.Background(), "cms", nil, nil, "no-op", nil)
	c.Rebuild(context.Background(), "cms", nil, nil, nil, "no-op", nil)

	if got := store.entries(); len(got) != 0 {
		t.Errorf("%d entries recorded for an empty purge, want 0", len(got))
	}
	if len(*reqs) != 0 {
		t.Errorf("%d requests sent for an empty purge, want 0", len(*reqs))
	}
}

// The shared secret is what stops anyone on the internet forcing a rebuild storm
func TestPostSendsTheSecretAndTheFullBody(t *testing.T) {
	srv, reqs, secrets := storefrontStub(t, 1, http.StatusOK)
	c := New(srv.URL, "s3cr3t", nil)

	c.Rebuild(context.Background(), "cms", []string{"cms:all"}, []string{"/about"}, []string{"/about"}, "edit", nil)

	if len(*reqs) != 1 {
		t.Fatalf("%d requests sent, want 1", len(*reqs))
	}
	got := (*reqs)[0]
	if len(got.Tags) != 1 || got.Tags[0] != "cms:all" {
		t.Errorf("tags = %v, want [cms:all]", got.Tags)
	}
	if len(got.Paths) != 1 || got.Paths[0] != "/about" {
		t.Errorf("paths = %v, want [/about]", got.Paths)
	}
	if len(got.Warm) != 1 || got.Warm[0] != "/about" {
		t.Errorf("warm = %v, want [/about] - Rebuild is what separates a warm from a plain purge", got.Warm)
	}
	if (*secrets)[0] != "s3cr3t" {
		t.Errorf("secret header = %q, want the configured secret", (*secrets)[0])
	}
}

// Purge is fire-and-forget, so a storefront that's down or erroring must not fail the admin mutation
// that triggered it
func TestPurgeSwallowsStorefrontFailures(t *testing.T) {
	store := &fakeStore{}
	srv, _, _ := storefrontStub(t, 0, http.StatusInternalServerError)
	c := New(srv.URL, "secret", store)

	c.Purge(context.Background(), "cms", []string{"cms:all"}, nil, "edit", nil) // must not panic

	if got := store.entries(); len(got) != 1 {
		t.Errorf("%d entries recorded, want the purge logged even though the HTTP call failed", len(got))
	}
}

// A store that can't write must not stop the actual invalidation - the cache being stale is worse
// than the log being incomplete
func TestPurgeStillSendsWhenTheStoreFails(t *testing.T) {
	store := &fakeStore{recErr: errors.New("db down")}
	srv, reqs, _ := storefrontStub(t, 0, http.StatusOK)
	c := New(srv.URL, "secret", store)

	c.Purge(context.Background(), "cms", []string{"cms:all"}, nil, "edit", nil)

	if len(*reqs) != 1 {
		t.Errorf("%d requests sent, want the invalidation to go out despite the log failure", len(*reqs))
	}
}

// The queue worker retries on error, so "the storefront answered but rebuilt nothing" has to read as
// a failure rather than success
func TestWarmPathRequiresTheStorefrontToConfirmARebuild(t *testing.T) {
	srv, _, _ := storefrontStub(t, 1, http.StatusOK)
	if err := New(srv.URL, "secret", nil).warmPath(context.Background(), []string{"cms:all"}, "/about"); err != nil {
		t.Errorf("warmPath with warmed=1 = %v, want nil", err)
	}

	cold, _, _ := storefrontStub(t, 0, http.StatusOK)
	if err := New(cold.URL, "secret", nil).warmPath(context.Background(), nil, "/about"); err == nil {
		t.Error("warmPath with warmed=0 = nil, want an error so the queue retries")
	}

	failing, _, _ := storefrontStub(t, 0, http.StatusBadGateway)
	if err := New(failing.URL, "secret", nil).warmPath(context.Background(), nil, "/about"); err == nil {
		t.Error("warmPath against a 502 = nil, want an error so the queue retries")
	}

	// no storefront configured is a no-op success, not a permanently failing task
	if err := New("", "secret", nil).warmPath(context.Background(), nil, "/about"); err != nil {
		t.Errorf("warmPath with no URL = %v, want nil", err)
	}
}

// warmPath invalidates AND warms the same path, so both lists have to carry it
func TestWarmPathInvalidatesAndWarmsTheSamePath(t *testing.T) {
	srv, reqs, _ := storefrontStub(t, 1, http.StatusOK)
	if err := New(srv.URL, "secret", nil).warmPath(context.Background(), []string{"cms:all"}, "/about"); err != nil {
		t.Fatalf("warmPath: %v", err)
	}
	got := (*reqs)[0]
	if len(got.Paths) != 1 || got.Paths[0] != "/about" {
		t.Errorf("paths = %v, want the path invalidated too", got.Paths)
	}
	if len(got.Warm) != 1 || got.Warm[0] != "/about" {
		t.Errorf("warm = %v, want the path warmed", got.Warm)
	}
}

func TestLogReadsThroughToTheStore(t *testing.T) {
	want := []LoggedPurge{{ID: uuid.New(), Surface: "cms", Reason: "edit"}}
	c := New("", "secret", &fakeStore{listed: want})
	got, err := c.Log(context.Background(), 10)
	if err != nil || len(got) != 1 || got[0].ID != want[0].ID {
		t.Errorf("Log = (%v, %v), want the store's rows", got, err)
	}

	failing := New("", "secret", &fakeStore{listErr: errors.New("db down")})
	if _, err := failing.Log(context.Background(), 10); err == nil {
		t.Error("Log with a failing store = nil, want the error surfaced to the admin page")
	}

	// a client with no store at all reads as an empty log, not an error
	if got, err := New("", "secret", nil).Log(context.Background(), 10); got != nil || err != nil {
		t.Errorf("Log with no store = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestSurfaceConfigPaths(t *testing.T) {
	enabled, ttl := SurfaceConfigPaths("cms")
	if enabled != "cache/cms/enabled" {
		t.Errorf("enabled path = %q, want %q", enabled, "cache/cms/enabled")
	}
	if ttl != "cache/cms/ttl_seconds" {
		t.Errorf("ttl path = %q, want %q", ttl, "cache/cms/ttl_seconds")
	}
	if _, _, _, ok := config.ParsePath(enabled); !ok {
		t.Errorf("%q is not a well-formed config path, so Validate would reject it at boot", enabled)
	}
}

// Registering a surface has to wire its two config keys, or the admin Cache page renders a surface
// with no toggle and the storefront reads a default that was never declared
func TestRegisterSurfaceWiresItsConfigKeys(t *testing.T) {
	RegisterSurface(Surface{Key: "testsurface", Label: "Test surface", AllTag: "testsurface:all", DefaultEnabled: true})

	got, ok := Lookup("testsurface")
	if !ok {
		t.Fatal("the surface is missing from the registry")
	}
	if got.AllTag != "testsurface:all" {
		t.Errorf("AllTag = %q, want it stored", got.AllTag)
	}

	enabled, ttl := SurfaceConfigPaths("testsurface")
	def, ok := config.Find(enabled)
	if !ok {
		t.Fatalf("%q was not registered as a config key", enabled)
	}
	if def.Default != "true" {
		t.Errorf("enabled default = %q, want %q for a DefaultEnabled surface", def.Default, "true")
	}
	if !def.Public {
		t.Error("the enabled key is not public, so the storefront can't read whether it caches")
	}
	if def.Type != config.TypeBool {
		t.Errorf("enabled type = %q, want bool", def.Type)
	}
	if ttlDef, ok := config.Find(ttl); !ok || ttlDef.Type != config.TypeInt || ttlDef.Default != "3600" {
		t.Errorf("ttl key = %+v (found=%v), want an int defaulting to 3600", ttlDef, ok)
	}

	// registering a second, distinct surface must not disturb the first
	RegisterSurface(Surface{Key: "othersurface", Label: "Other", DefaultEnabled: false})
	if _, ok := Lookup("testsurface"); !ok {
		t.Error("the first surface vanished after a second registered")
	}
	otherEnabled, _ := SurfaceConfigPaths("othersurface")
	if d, ok := config.Find(otherEnabled); !ok || d.Default != "false" {
		t.Errorf("othersurface enabled default = %+v, want false for a surface that isn't wired yet", d)
	}
}

// Surfaces come back in registration order, which is the order the admin Cache page renders
func TestSurfacesPreserveRegistrationOrder(t *testing.T) {
	before := len(Surfaces())
	RegisterSurface(Surface{Key: "ordera", Label: "A", AllTag: "a:all"})
	RegisterSurface(Surface{Key: "orderb", Label: "B"})

	all := Surfaces()
	if len(all) != before+2 {
		t.Fatalf("%d surfaces, want %d", len(all), before+2)
	}
	if all[len(all)-2].Key != "ordera" || all[len(all)-1].Key != "orderb" {
		t.Errorf("tail = %q, %q, want ordera then orderb", all[len(all)-2].Key, all[len(all)-1].Key)
	}
	if _, ok := Lookup("never-registered"); ok {
		t.Error("Lookup on an unregistered key = true, want false")
	}
}

// PurgeSurface is the "bust this whole area" button, and a surface without an AllTag is per-item
// only by design - firing a purge with no tags would be a silent no-op that still logs
func TestPurgeSurfaceOnlyFiresForSurfacesWithAnAllTag(t *testing.T) {
	RegisterSurface(Surface{Key: "coarse", Label: "Coarse", AllTag: "coarse:all"})
	RegisterSurface(Surface{Key: "fine", Label: "Fine"}) // per-item only

	store := &fakeStore{}
	c := New("", "secret", store)
	ctx := context.Background()

	c.PurgeSurface(ctx, "coarse", "admin button", nil)
	c.PurgeSurface(ctx, "fine", "admin button", nil)
	c.PurgeSurface(ctx, "not-a-surface", "admin button", nil)

	entries := store.entries()
	if len(entries) != 1 {
		t.Fatalf("%d purges recorded, want only the surface that has an AllTag", len(entries))
	}
	if entries[0].Surface != "coarse" || len(entries[0].Tags) != 1 || entries[0].Tags[0] != "coarse:all" {
		t.Errorf("entry = %+v, want the coarse surface purged by its AllTag", entries[0])
	}
}

// "Invalidate everything" deliberately skips per-item surfaces so it can't stampede a large catalog
func TestPurgeAllCollectsEveryAllTagAndTheHomepage(t *testing.T) {
	RegisterSurface(Surface{Key: "bulk1", Label: "Bulk one", AllTag: "bulk1:all"})
	RegisterSurface(Surface{Key: "bulk2", Label: "Bulk two", AllTag: "bulk2:all"})
	RegisterSurface(Surface{Key: "peritem", Label: "Per item"})

	store := &fakeStore{}
	New("", "secret", store).PurgeAll(context.Background(), "full flush", nil)

	entries := store.entries()
	if len(entries) != 1 {
		t.Fatalf("%d purges recorded, want 1", len(entries))
	}
	tags := entries[0].Tags
	for _, want := range []string{"bulk1:all", "bulk2:all"} {
		if !slices.Contains(tags, want) {
			t.Errorf("tags = %v, want %q included", tags, want)
		}
	}
	for _, unwanted := range tags {
		if unwanted == "" {
			t.Error("an empty tag was included, which would come from a surface with no AllTag")
		}
	}
	if len(entries[0].Paths) != 1 || entries[0].Paths[0] != "/" {
		t.Errorf("paths = %v, want the homepage included", entries[0].Paths)
	}
}

// The log columns are NOT NULL arrays, so a nil slice has to become an empty one on the way in
func TestNonNil(t *testing.T) {
	if got := nonNil(nil); got == nil || len(got) != 0 {
		t.Errorf("nonNil(nil) = %v, want an empty non-nil slice", got)
	}
	in := []string{"a"}
	if got := nonNil(in); len(got) != 1 || got[0] != "a" {
		t.Errorf("nonNil(%v) = %v, want it passed through", in, got)
	}
}

// The queue dedupes pending tasks on this key, so two surfaces sharing a path must not collide
func TestDedupKeyDistinguishesSurfaceAndPath(t *testing.T) {
	seen := map[string]string{}
	for name, key := range map[string]string{
		"cms /about":    dedupKey("cms", "/about"),
		"blog /about":   dedupKey("blog", "/about"),
		"cms /contact":  dedupKey("cms", "/contact"),
		"empty surface": dedupKey("", "/about"),
		"empty path":    dedupKey("cms", ""),
	} {
		if prev, dup := seen[key]; dup {
			t.Errorf("%s and %s both produce key %q", name, prev, key)
		}
		seen[key] = name
	}
}
