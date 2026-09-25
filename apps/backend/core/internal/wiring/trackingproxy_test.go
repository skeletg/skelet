package wiring

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrackingProxiesForwardTheClientIP(t *testing.T) {
	var got string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Forwarded-For")
	}))
	defer upstream.Close()
	t.Setenv("SIGNALS_INGEST_URL", upstream.URL)
	t.Setenv("SIGNALS_URL", upstream.URL)

	for name, h := range map[string]http.Handler{"beacon": beaconProxy(true), "pulse": pulseProxy(true)} {
		t.Run(name, func(t *testing.T) {
			got = ""
			req := httptest.NewRequest(http.MethodGet, "/t/p", nil)
			req.RemoteAddr = "203.0.113.7:51000"
			req.Header.Set("X-Forwarded-For", "198.51.100.9")
			h.ServeHTTP(httptest.NewRecorder(), req)
			if got != "198.51.100.9, 203.0.113.7" {
				t.Fatalf("X-Forwarded-For upstream = %q, want the chain ending in the caller", got)
			}
		})
	}
}
