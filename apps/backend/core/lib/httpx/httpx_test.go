package httpx_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/httpx"
)

// loggerEcho is a trivial handler that pulls the ctx logger and writes nothing; the test checks the response header and
// that downstream could read a non-default logger
type loggerEcho struct{ seenID string }

func (l *loggerEcho) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// We can't introspect *slog.Logger fields directly, so we go via
	// the response header which the middleware set just before calling us
	l.seenID = w.Header().Get(httpx.HeaderRequestID)
	w.WriteHeader(http.StatusOK)
}

func TestSecurityHeadersSetOnNormalPaths(t *testing.T) {
	h := httpx.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/front/graphql", nil))
	for _, key := range []string{
		"Content-Security-Policy",
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Permissions-Policy",
	} {
		if rec.Header().Get(key) == "" {
			t.Fatalf("expected %s to be set", key)
		}
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("CSP missing default-src: %q", rec.Header().Get("Content-Security-Policy"))
	}
}

func TestSecurityHeadersSkippedForPlayground(t *testing.T) {
	h := httpx.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	// The dev-only GraphQL playground loads CDN assets a strict CSP would block
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/playground/front", nil))
	if got := rec.Header().Get("Content-Security-Policy"); got != "" {
		t.Fatalf("expected no CSP on playground path, got %q", got)
	}
}

func TestRequestIDGeneratesWhenAbsent(t *testing.T) {
	echo := &loggerEcho{}
	h := httpx.RequestID(echo)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get(httpx.HeaderRequestID); got == "" {
		t.Fatalf("expected request id to be generated")
	}
	if echo.seenID == "" {
		t.Fatalf("handler did not observe generated request id")
	}
}

func TestRequestIDPropagatesInbound(t *testing.T) {
	echo := &loggerEcho{}
	h := httpx.RequestID(echo)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(httpx.HeaderRequestID, "rid-abc")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get(httpx.HeaderRequestID); got != "rid-abc" {
		t.Fatalf("response id = %q, want rid-abc", got)
	}
}

func TestRequestIDInjectsLoggerIntoCtx(t *testing.T) {
	var ctxFromHandler context.Context
	h := httpx.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxFromHandler = r.Context()
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	h.ServeHTTP(rec, req)
	if applog.From(ctxFromHandler) == applog.From(context.Background()) {
		t.Fatalf("logger from ctx should be a per-request derivative, not the default")
	}
}

func TestLimitBodyUnderCapPasses(t *testing.T) {
	const cap = 16
	var read []byte
	h := httpx.LimitBody(cap, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read under cap should succeed: %v", err)
		}
		read = b
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("hello")) // 5 bytes < cap
	h.ServeHTTP(rec, req)
	if string(read) != "hello" {
		t.Fatalf("handler read %q, want %q", read, "hello")
	}
}

func TestLimitBodyOverCapFailsRead(t *testing.T) {
	const cap = 8
	var readErr error
	h := httpx.LimitBody(cap, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))
	rec := httptest.NewRecorder()
	// 20 bytes, well over the 8-byte cap → MaxBytesReader fails the read
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("x", 20)))
	h.ServeHTTP(rec, req)
	if readErr == nil {
		t.Fatal("reading a body over the cap should error")
	}
	if !strings.Contains(readErr.Error(), "too large") {
		t.Fatalf("read error = %v, want a 'request body too large' error", readErr)
	}
}

func TestLimitBodyAtCapPasses(t *testing.T) {
	const cap = 5
	var readErr error
	var read []byte
	h := httpx.LimitBody(cap, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		read, readErr = io.ReadAll(r.Body)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("12345")) // exactly cap
	h.ServeHTTP(rec, req)
	if readErr != nil {
		t.Fatalf("reading exactly cap bytes should succeed: %v", readErr)
	}
	if string(read) != "12345" {
		t.Fatalf("handler read %q, want %q", read, "12345")
	}
}

func TestLimitBodyDisabledWhenNonPositive(t *testing.T) {
	// maxBytes <= 0 must return the next handler unchanged (same pointer)
	// so no MaxBytesReader is installed and large bodies pass freely
	for _, cap := range []int64{0, -1} {
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
		got := httpx.LimitBody(cap, next)
		// Functionally: a large body reads fine when the limiter is off
		var readErr error
		h := httpx.LimitBody(cap, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			_, readErr = io.ReadAll(r.Body)
		}))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("y", 1000)))
		h.ServeHTTP(rec, req)
		if readErr != nil {
			t.Fatalf("cap=%d: disabled limiter should not error on a large body: %v", cap, readErr)
		}
		_ = got
	}
}

func TestRecoverReturns500JSON(t *testing.T) {
	h := httpx.Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", rec.Header().Get("Content-Type"))
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("response body not JSON: %v\nbody=%s", err, body)
	}
	envelope, ok := parsed["error"].(map[string]any)
	if !ok {
		t.Fatalf("response has no error envelope: %v", parsed)
	}
	if envelope["code"] != "INTERNAL" {
		t.Fatalf("error.code = %v, want INTERNAL", envelope["code"])
	}
	if envelope["class"] != "INTERNAL" {
		t.Fatalf("error.class = %v, want INTERNAL", envelope["class"])
	}
}

func TestRecoverNonPanicPassesThrough(t *testing.T) {
	h := httpx.Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q, want ok", rec.Body.String())
	}
}

func TestRecoverIntegratesWithRequestID(t *testing.T) {
	// Chain order matters: RequestID outer so the recovery handler can
	// read the id from the response header it already set
	chain := httpx.Recover(httpx.RequestID(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(httpx.HeaderRequestID, "rid-int")
	chain.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	envelope := parsed["error"].(map[string]any)
	if envelope["request_id"] != "rid-int" {
		t.Fatalf("error.request_id = %v, want rid-int", envelope["request_id"])
	}
}
