package wiring

import (
	"net/http"
	"net/http/httptest"
	"testing"

	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func TestTrackingProxiesForwardTheClientIP(t *testing.T) {
	var got string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Forwarded-For")
	}))
	defer upstream.Close()
	t.Setenv("SIGNALS_INGEST_URL", upstream.URL)
	t.Setenv("SIGNALS_URL", upstream.URL)
	t.Cleanup(func() { _ = authpkg.SetTrustedProxies(nil) })

	for name, h := range map[string]http.Handler{"beacon": beaconProxy(true), "pulse": pulseProxy(true)} {
		for _, c := range []struct {
			name, peer, trusted, want string
		}{
			{"untrusted caller", "203.0.113.7:51000", "", "203.0.113.7"},
			{"behind a trusted proxy", "10.0.0.2:51000", "10.0.0.0/8", "198.51.100.9"},
		} {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				if err := authpkg.SetTrustedProxies([]string{c.trusted}); err != nil {
					t.Fatal(err)
				}
				got = ""
				req := httptest.NewRequest(http.MethodGet, "/t/p", nil)
				req.RemoteAddr = c.peer
				req.Header.Set("X-Forwarded-For", "198.51.100.9")
				h.ServeHTTP(httptest.NewRecorder(), req)
				if got != c.want {
					t.Fatalf("X-Forwarded-For upstream = %q, want %q", got, c.want)
				}
			})
		}
	}
}
