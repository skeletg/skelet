package remoteprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/circuit"
)

type fakeRepo struct {
	providers map[uuid.UUID]Provider
	secrets   map[uuid.UUID]string
}

func newFakeRepo(providers ...Provider) *fakeRepo {
	r := &fakeRepo{providers: map[uuid.UUID]Provider{}, secrets: map[uuid.UUID]string{}}
	for _, p := range providers {
		r.providers[p.ID] = p
		r.secrets[p.ID] = "rpsec_test"
	}
	return r
}

func (r *fakeRepo) Create(_ context.Context, input CreateInput, secret string) (Provider, error) {
	p := Provider{ID: uuid.New(), Kind: input.Kind, Slug: input.Slug, Name: input.Name,
		TargetURL: input.TargetURL, Config: input.Config, TimeoutMs: input.TimeoutMs,
		CacheTTLs: input.CacheTTLs, FailurePolicy: input.FailurePolicy, ChannelID: input.ChannelID, IsActive: true}
	r.providers[p.ID] = p
	r.secrets[p.ID] = secret
	return p, nil
}
func (r *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (Provider, error) {
	p, ok := r.providers[id]
	if !ok {
		return Provider{}, ErrNotFound
	}
	return p, nil
}
func (r *fakeRepo) GetBySlug(_ context.Context, kind Kind, slug string, _ *uuid.UUID) (Provider, error) {
	for _, p := range r.providers {
		if p.Kind == kind && p.Slug == slug {
			return p, nil
		}
	}
	return Provider{}, ErrNotFound
}
func (r *fakeRepo) List(context.Context) ([]Provider, error) { return nil, nil }
func (r *fakeRepo) ListActiveByKind(_ context.Context, kind Kind, _ *uuid.UUID) ([]Provider, error) {
	var out []Provider
	for _, p := range r.providers {
		if p.Kind == kind && p.IsActive {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *fakeRepo) ListActiveForChannel(ctx context.Context, ch *uuid.UUID) ([]Provider, error) {
	var out []Provider
	for _, k := range AllKinds {
		ps, _ := r.ListActiveByKind(ctx, k, ch)
		out = append(out, ps...)
	}
	return out, nil
}
func (r *fakeRepo) Update(context.Context, UpdateInput) (Provider, error) {
	return Provider{}, ErrNotFound
}
func (r *fakeRepo) SetActive(context.Context, uuid.UUID, bool) (Provider, error) {
	return Provider{}, ErrNotFound
}
func (r *fakeRepo) UpdateSecret(context.Context, uuid.UUID, string) (Provider, error) {
	return Provider{}, ErrNotFound
}
func (r *fakeRepo) GetSecret(_ context.Context, id uuid.UUID) (string, error) {
	return r.secrets[id], nil
}
func (r *fakeRepo) Delete(context.Context, uuid.UUID) error { return nil }

func resetBreaker() {
	breaker = circuit.New(breakerTripThreshold, breakerCooldown)
	invocationCache = &responseCache{m: map[string]cacheEntry{}}
}

func testProvider(url string, cacheTTLs int32) Provider {
	return Provider{
		ID: uuid.New(), Kind: KindTaxProvider, Slug: "avatax-ish", Name: "Test",
		TargetURL: url, Config: []byte(`{"apiKey":"k"}`), CacheTTLs: cacheTTLs,
		FailurePolicy: FailOpen, IsActive: true,
	}
}

func TestInvokeSignsAndDecodes(t *testing.T) {
	resetBreaker()
	var gotSig, gotKind string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Provider-Signature-256")
		gotKind = r.Header.Get("X-Provider-Kind")
		gotBody = make([]byte, r.ContentLength)
		_, _ = r.Body.Read(gotBody)
		_, _ = w.Write([]byte(`{"taxCents": 825}`))
	}))
	defer srv.Close()

	p := testProvider(srv.URL, 0)
	inv := NewInvoker(NewService(newFakeRepo(p)), nil)

	var out struct {
		TaxCents int64 `json:"taxCents"`
	}
	if err := inv.Invoke(context.Background(), p, map[string]any{"subtotal": 10000}, &out); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if out.TaxCents != 825 {
		t.Fatalf("response not decoded: %+v", out)
	}
	if gotSig == "" || gotKind != "tax_provider" {
		t.Fatalf("signing headers missing: sig=%q kind=%q", gotSig, gotKind)
	}
	for _, want := range []string{`"apiKey":"k"`, `"subtotal":10000`, `"provider":"avatax-ish"`} {
		if !strings.Contains(string(gotBody), want) {
			t.Fatalf("envelope missing %s: %s", want, gotBody)
		}
	}
}

func TestInvokeCachesByPayload(t *testing.T) {
	resetBreaker()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer srv.Close()

	p := testProvider(srv.URL, 60)
	inv := NewInvoker(NewService(newFakeRepo(p)), nil)
	var out map[string]any
	for range 3 {
		if err := inv.Invoke(context.Background(), p, map[string]any{"q": "same"}, &out); err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 upstream hit with caching, got %d", hits.Load())
	}
	// different payload = different cache key
	if err := inv.Invoke(context.Background(), p, map[string]any{"q": "other"}, &out); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 upstream hits, got %d", hits.Load())
	}
}

func TestInvokeBreakerTrips(t *testing.T) {
	resetBreaker()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	p := testProvider(srv.URL, 0)
	inv := NewInvoker(NewService(newFakeRepo(p)), nil)
	for i := range breakerTripThreshold {
		if err := inv.Invoke(context.Background(), p, map[string]any{"i": i}, nil); err == nil {
			t.Fatal("expected error from 502")
		}
	}
	err := inv.Invoke(context.Background(), p, map[string]any{"i": 99}, nil)
	if err == nil || !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected circuit-open error, got %v", err)
	}
	if hits.Load() != breakerTripThreshold {
		t.Fatalf("open circuit still hit upstream: %d", hits.Load())
	}
}

func TestInvokeTimeout(t *testing.T) {
	resetBreaker()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	p := testProvider(srv.URL, 0)
	p.TimeoutMs = 50
	inv := NewInvoker(NewService(newFakeRepo(p)), nil)
	start := time.Now()
	if err := inv.Invoke(context.Background(), p, map[string]any{}, nil); err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 400*time.Millisecond {
		t.Fatal("timeout_ms not honored")
	}
}
