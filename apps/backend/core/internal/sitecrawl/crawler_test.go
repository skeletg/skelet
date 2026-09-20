package sitecrawl

import (
	"strings"
	"testing"
)

func newTestCrawler(t *testing.T, base string) *Crawler {
	t.Helper()
	c, err := New(Options{BaseURL: base})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNormalizeStaysOnOrigin(t *testing.T) {
	c := newTestCrawler(t, "https://shop.example.com")
	cases := []struct {
		href string
		want string
		ok   bool
	}{
		{"/products/tee", "/products/tee", true},
		{"products/tee", "/products/tee", true},
		{"https://shop.example.com/a", "/a", true},
		{"/a?page=2", "/a?page=2", true},
		{"/a#reviews", "/a", true},
		{"//evil.example.com/a", "", false},
		{"https://evil.example.com/a", "", false},
		{"http://shop.example.com/a", "", false},
		{"https://shop.example.com:8443/a", "", false},
		{"mailto:hi@example.com", "", false},
		{"javascript:alert(1)", "", false},
		{"tel:+123", "", false},
		{"/logo.png", "", false},
		{"/feed.xml", "", false},
		{"", "", false},
		{"#top", "", false},
	}
	for _, tc := range cases {
		got, ok := c.normalize(tc.href, "/")
		if ok != tc.ok {
			t.Errorf("normalize(%q) ok = %v, want %v", tc.href, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("normalize(%q) = %q, want %q", tc.href, got, tc.want)
		}
	}
}

func TestNormalizeResolvesRelativeToTheCurrentPage(t *testing.T) {
	c := newTestCrawler(t, "https://shop.example.com")
	got, ok := c.normalize("../hats", "/shop/shirts/tee")
	if !ok || got != "/shop/hats" {
		t.Fatalf("normalize = %q, %v; want /shop/hats, true", got, ok)
	}
}

// A base url with a path is a subsite boundary - links above it belong to someone else
func TestNormalizeHonoursABasePath(t *testing.T) {
	c := newTestCrawler(t, "https://example.com/store")
	if got, ok := c.normalize("/store/tee", "/"); !ok || got != "/tee" {
		t.Fatalf("in-scope link = %q, %v", got, ok)
	}
	if _, ok := c.normalize("/blog/post", "/"); ok {
		t.Fatal("link outside the base path should be rejected")
	}
}

func TestRobotsLongestMatchWins(t *testing.T) {
	r := parseRobots(`
User-agent: *
Disallow: /account/
Disallow: /search
Allow: /account/public

User-agent: GPTBot
Disallow: /
`)
	if r == nil {
		t.Fatal("expected rules")
	}
	cases := map[string]bool{
		"/":                     true,
		"/products/tee":         true,
		"/account/orders":       false,
		"/account/public/about": true,
		"/search?q=hat":         false,
	}
	for path, want := range cases {
		if got := r.allows(path); got != want {
			t.Errorf("allows(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestRobotsWildcardAndAnchor(t *testing.T) {
	r := parseRobots("User-agent: *\nDisallow: /*.pdf$\nDisallow: /a/*/b\n")
	if r == nil {
		t.Fatal("expected rules")
	}
	for path, want := range map[string]bool{
		"/docs/manual.pdf":     false,
		"/docs/manual.pdf?v=2": true,
		"/a/anything/b":        false,
		"/a/b":                 true,
	} {
		if got := r.allows(path); got != want {
			t.Errorf("allows(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestLookupPathStripsLocaleAndQuery(t *testing.T) {
	locales := []string{"en", "de", "en-GB"}
	for in, want := range map[string]string{
		"/":                    "",
		"/de":                  "",
		"/de/products/tee":     "products/tee",
		"/en-GB/products/tee":  "products/tee",
		"/products/tee":        "products/tee",
		"/products/tee?page=2": "products/tee",
		"/deals/summer":        "deals/summer",
	} {
		if got := LookupPath(in, locales); got != want {
			t.Errorf("lookupPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCrawlerRefusesLinkLocalAddresses(t *testing.T) {
	c := newTestCrawler(t, "http://169.254.169.254")
	resp, err := c.client.Get("http://169.254.169.254/latest/meta-data/")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected the dial guard to refuse a link-local address")
	}
	if !strings.Contains(err.Error(), "link-local") {
		t.Fatalf("expected a link-local refusal, got %v", err)
	}
}
