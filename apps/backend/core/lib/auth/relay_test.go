package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func downstream(t *testing.T) (*httptest.Server, func() (*http.Request, string)) {
	t.Helper()
	var got *http.Request
	var client string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, client = r.Clone(r.Context()), ClientIP(r)
	}))
	t.Cleanup(srv.Close)
	return srv, func() (*http.Request, string) { return got, client }
}

func relayThrough(t *testing.T, target string, in *http.Request) {
	t.Helper()
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	Relay(u).ServeHTTP(httptest.NewRecorder(), in)
}

func TestRelayCarriesTheResolvedClientNotItsOwnAddress(t *testing.T) {
	const key = "relay-key-0123456789abcdef0123456789ab"
	t.Cleanup(func() { _ = SetTrustedProxies(nil); SetTrustedProxyKeys("") })
	SetTrustedProxyKeys(key)
	srv, seen := downstream(t)

	// A storefront with no fixed address vouched for the shopper - the next hop must hear the shopper, by key
	in := httptest.NewRequest(http.MethodPost, "http://shop.example.com/t/collect", nil)
	in.RemoteAddr = "34.1.2.3:443"
	in.Header.Set(ClientIPHeader, "203.0.113.9")
	in.Header.Set(ProxyKeyHeader, key)
	in.Header.Set("X-Forwarded-Proto", "https")
	relayThrough(t, srv.URL, in)

	got, client := seen()
	if client != "203.0.113.9" {
		t.Errorf("downstream ClientIP = %q, want the shopper", client)
	}
	if xff := got.Header.Get("X-Forwarded-For"); xff != "203.0.113.9" {
		t.Errorf("X-Forwarded-For = %q, want the resolved client alone", xff)
	}
	if got.Host != "shop.example.com" {
		t.Errorf("Host = %q, want the shopper's host kept", got.Host)
	}
	if p := got.Header.Get("X-Forwarded-Proto"); p != "https" {
		t.Errorf("X-Forwarded-Proto = %q, want it passed on", p)
	}
}

func TestRelayWithoutAKeyStripsWhatTheCallerClaimed(t *testing.T) {
	t.Cleanup(func() { _ = SetTrustedProxies(nil); SetTrustedProxyKeys("") })
	SetTrustedProxyKeys("")
	srv, seen := downstream(t)

	in := httptest.NewRequest(http.MethodGet, "http://shop.example.com/e/o/x", nil)
	in.RemoteAddr = "198.51.100.7:4000"
	in.Header.Set(ClientIPHeader, "1.1.1.1")
	in.Header.Set(ProxyKeyHeader, "guess")
	in.Header.Set("X-Forwarded-For", "9.9.9.9")
	relayThrough(t, srv.URL, in)

	got, _ := seen()
	if v := got.Header.Get(ClientIPHeader) + got.Header.Get(ProxyKeyHeader); v != "" {
		t.Errorf("a caller's own claim went through: %q", v)
	}
	if xff := got.Header.Get("X-Forwarded-For"); xff != "198.51.100.7" {
		t.Errorf("X-Forwarded-For = %q, want the untrusted peer, not its spoofed chain", xff)
	}
}
