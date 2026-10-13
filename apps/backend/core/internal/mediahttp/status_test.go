package mediahttp

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/media"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

type store struct {
	mu      sync.Mutex
	objects map[string][]byte
	getErr  error
	gate    chan struct{}
	public  bool
}

func newStore() *store { return &store{objects: map[string][]byte{}} }

func (s *store) GetBytes(ctx context.Context, key string) ([]byte, error) {
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.getErr != nil {
		return nil, s.getErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.objects[key]
	if !ok {
		return nil, objstore.ErrObjectNotFound
	}
	return b, nil
}
func (s *store) GetBytesTyped(_ context.Context, key string) ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.objects[key]; ok && len(key) > 8 && key[:8] == "derived/" {
		return b, "image/png", nil
	}
	return nil, "", objstore.ErrObjectNotFound
}
func (s *store) PutBytes(_ context.Context, key string, data []byte, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = data
	return nil
}
func (s *store) Head(_ context.Context, key string) (int64, string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.objects[key]
	return 0, "image/png", ok, nil
}
func (s *store) HasPublicBase() bool       { return s.public }
func (s *store) PublicURL(k string) string { return "https://cdn.test/" + k }

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serve(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestAColdRenderIsServedAndCached(t *testing.T) {
	st := newStore()
	st.objects["p/a.png"] = pngOf(t, 800, 600)
	h := NewTransformHandler(media.NewTransformer(st), Options{})
	rec := serve(h, http.MethodGet, "/media/t/w320/p/a.png")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("status %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=2592000, immutable" {
		t.Errorf("Cache-Control %q", cc)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("no nosniff on a derivative")
	}
	if _, ok := st.objects[media.DerivedKey("p/a.png", 320, 0, false)]; !ok {
		t.Error("the derivative was not cached back")
	}
}

// A warm derivative goes straight to object storage, so no image bytes cross this process
func TestAWarmDerivativeRedirects(t *testing.T) {
	st := newStore()
	st.public = true
	st.objects[media.DerivedKey("p/a.png", 320, 0, false)] = []byte("x")
	rec := serve(NewTransformHandler(media.NewTransformer(st), Options{}), http.MethodGet, "/media/t/w300/p/a.png")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://cdn.test/derived/w320/p/a.png" {
		t.Fatalf("status %d location %q, want a redirect to the snapped derivative", rec.Code, rec.Header().Get("Location"))
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("redirect Cache-Control %q", cc)
	}
}

func TestFailuresAnswerWithTheRightStatus(t *testing.T) {
	big := newStore()
	big.objects["p/huge.png"] = pngOf(t, 400, 400)
	tooBig := media.NewTransformer(big)
	tooBig.SetMaxSourcePixels(1000)

	down := newStore()
	down.getErr = errors.New("storage unreachable")

	for _, tc := range []struct {
		name   string
		h      http.Handler
		method string
		path   string
		want   int
	}{
		{"post", NewTransformHandler(media.NewTransformer(newStore()), Options{}), http.MethodPost, "/media/t/w320/p/a.png", http.StatusMethodNotAllowed},
		{"rate limited", NewTransformHandler(media.NewTransformer(newStore()), Options{AllowRateLimit: func(*http.Request) bool { return false }}), http.MethodGet, "/media/t/w320/p/a.png", http.StatusTooManyRequests},
		{"malformed spec", NewTransformHandler(media.NewTransformer(newStore()), Options{}), http.MethodGet, "/media/t/x320/p/a.png", http.StatusNotFound},
		{"missing original", NewTransformHandler(media.NewTransformer(newStore()), Options{}), http.MethodGet, "/media/t/w320/p/none.png", http.StatusNotFound},
		{"over the pixel cap", NewTransformHandler(tooBig, Options{}), http.MethodGet, "/media/t/w320/p/huge.png", http.StatusUnprocessableEntity},
		{"storage down", NewTransformHandler(media.NewTransformer(down), Options{}), http.MethodGet, "/media/t/w320/p/a.png", http.StatusBadGateway},
	} {
		rec := serve(tc.h, tc.method, tc.path)
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, rec.Code, tc.want)
		}
		if tc.want == http.StatusBadGateway && rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: a storage outage is cacheable (%q), so the CDN pins the missing image", tc.name, rec.Header().Get("Cache-Control"))
		}
		if tc.want == http.StatusMethodNotAllowed && rec.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("%s: Allow %q", tc.name, rec.Header().Get("Allow"))
		}
	}
}

// Every render slot taken past the wait - a burst queues, then sheds with a retry hint
func TestASaturatedRendererAsksToRetry(t *testing.T) {
	st := newStore()
	st.objects["p/a.png"] = pngOf(t, 800, 600)
	st.objects["p/b.png"] = pngOf(t, 800, 600)
	st.gate = make(chan struct{})
	tr := media.NewTransformer(st)
	tr.SetRenderConcurrency(1, 20*time.Millisecond)
	h := NewTransformHandler(tr, Options{})

	held := make(chan *httptest.ResponseRecorder, 1)
	go func() { held <- serve(h, http.MethodGet, "/media/t/w320/p/a.png") }()
	time.Sleep(50 * time.Millisecond) // the first render holds the only slot
	rec := serve(h, http.MethodGet, "/media/t/w320/p/b.png")
	close(st.gate)
	<-held
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("status %d Retry-After %q, want 503 with a retry hint", rec.Code, rec.Header().Get("Retry-After"))
	}
}

// A HEIC original is there - this build just can't draw it, so it is not a storage fault to 502 on every request
func TestAnUndrawableOriginalAnswers415(t *testing.T) {
	st := newStore()
	st.objects["p/photo.heic"] = []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic")
	rec := serve(NewTransformHandler(media.NewTransformer(st), Options{}), http.MethodGet, "/media/t/w320/p/photo.heic")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status %d, want 415", rec.Code)
	}
}
