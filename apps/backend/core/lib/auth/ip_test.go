package auth

import (
	"net/http"
	"strings"
	"testing"
)

func req(remoteAddr, xff, xrealip string) *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = remoteAddr
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	if xrealip != "" {
		r.Header.Set("X-Real-IP", xrealip)
	}
	return r
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name    string
		trusted []string
		r       *http.Request
		want    string
	}{
		{"no proxies: headers ignored", nil,
			req("198.51.100.7:4123", "1.2.3.4", ""), "198.51.100.7"},
		{"no proxies: spoofed XFF ignored", nil,
			req("198.51.100.7:4123", "9.9.9.9, 8.8.8.8", ""), "198.51.100.7"},
		{"trusted proxy: client from XFF", []string{"10.0.0.0/8"},
			req("10.0.0.2:333", "203.0.113.9", ""), "203.0.113.9"},
		{"trusted proxy: rightmost untrusted wins", []string{"10.0.0.0/8"},
			req("10.0.0.2:333", "9.9.9.9, 203.0.113.9, 10.0.0.3", ""), "203.0.113.9"},
		{"untrusted peer sends XFF: ignored", []string{"10.0.0.0/8"},
			req("198.51.100.7:4123", "9.9.9.9", ""), "198.51.100.7"},
		{"malformed XFF entry: fall back to peer", []string{"10.0.0.0/8"},
			req("10.0.0.2:333", "9.9.9.9, not-an-ip", ""), "10.0.0.2"},
		{"all hops trusted: leftmost", []string{"10.0.0.0/8"},
			req("10.0.0.2:333", "10.0.0.9, 10.0.0.3", ""), "10.0.0.9"},
		{"x-real-ip honored from trusted proxy", []string{"10.0.0.0/8"},
			req("10.0.0.2:333", "", "203.0.113.9"), "203.0.113.9"},
		{"x-real-ip ignored from untrusted peer", []string{"10.0.0.0/8"},
			req("198.51.100.7:4123", "", "203.0.113.9"), "198.51.100.7"},
		{"bare ip spec", []string{"172.16.0.5"},
			req("172.16.0.5:9000", "203.0.113.9", ""), "203.0.113.9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := SetTrustedProxies(c.trusted); err != nil {
				t.Fatalf("SetTrustedProxies: %v", err)
			}
			defer func() { _ = SetTrustedProxies(nil) }()
			if got := ClientIP(c.r); got != c.want {
				t.Errorf("ClientIP() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSetTrustedProxiesRejectsGarbage(t *testing.T) {
	if err := SetTrustedProxies([]string{"not-a-cidr"}); err == nil {
		t.Fatal("expected error for invalid spec")
	}
	_ = SetTrustedProxies(nil)
}

// The gate for every client-supplied visitor id - anything that isn't a short token becomes anonymous
func TestSanitizeAnonID(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"uuid passes", "8b8f4a2e-1c3d-4f5a-9b6c-7d8e9f0a1b2c", "8b8f4a2e-1c3d-4f5a-9b6c-7d8e9f0a1b2c"},
		{"short custom token passes", "visitor_42", "visitor_42"},
		{"empty stays empty", "", ""},
		{"over 64 chars rejected", strings.Repeat("a", 65), ""},
		{"exactly 64 chars passes", strings.Repeat("a", 64), strings.Repeat("a", 64)},
		{"whitespace rejected", "abc def", ""},
		{"sql-ish payload rejected", "x'; drop table--", ""},
		{"newline rejected", "abc\ndef", ""},
		{"non-ascii rejected", "визитор", ""},
	}
	for _, tc := range cases {
		if got := SanitizeAnonID(tc.in); got != tc.want {
			t.Errorf("%s: SanitizeAnonID(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}
