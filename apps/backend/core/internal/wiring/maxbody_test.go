package wiring

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxBodyCapsRequestBody(t *testing.T) { // maxBody must make oversized request bodies fail at read time - the GraphQL complexity limit bounds query shape, not raw payload bytes
	var readErr error
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	})
	h := maxBody(1024)(inner)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 4096)))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if readErr == nil {
		t.Fatal("4KB body passed a 1KB cap")
	}

	readErr = nil
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 512)))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if readErr != nil {
		t.Fatalf("512B body rejected by a 1KB cap: %v", readErr)
	}
}
