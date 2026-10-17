package sitecrawl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func testSite(t *testing.T) *httptest.Server {
	t.Helper()
	pages := map[string]string{
		"/": `<html><head><title>Home</title><meta name="description" content="Front door"></head><body>
			<h1>Welcome</h1><h3>Skipped a level</h3>
			<a href="/a">A</a><a href="/a">A again</a><a href="/old">Old</a><a href="/secret/x">Hidden</a>
			<a href="/ignored" rel="nofollow">Nofollow</a><a href="https://elsewhere.test/">Out</a>
			<a href="/` + "š" + `">` + strings.Repeat("š", 100) + `</a></body></html>`,
		"/a":        `<html><head><title>A</title><meta name="robots" content="nofollow"></head><body><a href="/from-a">x</a> one two three</body></html>`,
		"/new":      `<html><head><title>New</title></head><body>moved here</body></html>`,
		"/" + "š":   `<html><body>serbian</body></html>`,
		"/from-a":   `<html><body>never reached</body></html>`,
		"/ignored":  `<html><body>never reached</body></html>`,
		"/secret/x": `<html><body>never reached</body></html>`,
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /secret\n"))
			return
		case "/old":
			http.Redirect(w, r, "/new", http.StatusMovedPermanently)
			return
		}
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
}

func crawlAll(t *testing.T, opts Options) (map[string]Result, Run) {
	t.Helper()
	c, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Result{}
	run, err := c.Crawl(context.Background(), func(r Result) error { got[r.Page.Path] = r; return nil })
	if err != nil {
		t.Fatal(err)
	}
	return got, run
}

func TestCrawlWalksTheSiteLikeASearchEngine(t *testing.T) {
	srv := testSite(t)
	defer srv.Close()
	got, run := crawlAll(t, Options{BaseURL: srv.URL, RespectRobots: true})

	home := got["/"]
	if home.Page.Title != "Home" || home.Page.MetaDesc != "Front door" || len(home.Page.Headings) != 2 || home.Page.Headings[1].Level != 3 {
		t.Errorf("home page = %+v", home.Page)
	}
	if home.Page.ExtLinks != 1 {
		t.Errorf("external links = %d, want 1", home.Page.ExtLinks)
	}
	for _, l := range home.Links {
		if l.ToPath == "/a" && l.Occurrences != 2 {
			t.Errorf("/a occurrences = %d, want 2", l.Occurrences)
		}
		if !utf8.ValidString(l.Anchor) {
			t.Errorf("anchor %q is not valid UTF-8", l.Anchor)
		}
	}
	if old := got["/old"].Page; old.StatusCode != http.StatusMovedPermanently || old.RedirectTo != "/new" {
		t.Errorf("/old = %+v, want a recorded 301 to /new", old)
	}
	if _, ok := got["/new"]; !ok {
		t.Error("the redirect target was not crawled")
	}
	for _, never := range []string{"/from-a", "/ignored", "/secret/x"} {
		if _, ok := got[never]; ok {
			t.Errorf("%s was crawled - nofollow or robots ignored", never)
		}
	}
	if run.HitBudget {
		t.Error("budget reported hit on a small site")
	}

	_, run = crawlAll(t, Options{BaseURL: srv.URL, MaxPages: 2})
	if run.Pages != 2 || !run.HitBudget {
		t.Errorf("MaxPages 2: run = %+v, want 2 pages and the budget hit", run)
	}
}

func TestTruncateKeepsWholeCharacters(t *testing.T) {
	s := strings.Repeat("š", 100)
	if got := truncate(s, 121); !utf8.ValidString(got) || len(got) != 120 {
		t.Errorf("truncate = %d bytes, valid %v - want 120 valid bytes", len(got), utf8.ValidString(got))
	}
	if got := truncate("short", 120); got != "short" {
		t.Errorf("truncate(short) = %q", got)
	}
}

func TestACrawlStartsOnlyAtAVouchedOrigin(t *testing.T) {
	t.Setenv("STOREFRONT_URL", "http://localhost:13000")
	t.Setenv("SEO_CRAWL_ALLOWED_ORIGINS", " http://vetrina:3000 ,")
	for _, ok := range []string{"https://shop.example.com/en", "HTTPS://Shop.Example.com", "http://localhost:13000", "http://vetrina:3000/"} {
		if err := checkOrigin(ok, "https://shop.example.com"); err != nil {
			t.Errorf("checkOrigin(%q) = %v, want allowed", ok, err)
		}
	}
	for _, bad := range []string{"https://attacker.test", "http://shop.example.com", "http://jaeger:16686", "http://vetrina:3001"} {
		if err := checkOrigin(bad, "https://shop.example.com"); !errors.Is(err, errBaseURLRefused) {
			t.Errorf("checkOrigin(%q) = %v, want %v", bad, err, errBaseURLRefused)
		}
	}
	if err := checkOrigin("/relative", "https://shop.example.com"); !errors.Is(err, errBaseURLMalformed) {
		t.Errorf("a relative base = %v, want %v", err, errBaseURLMalformed)
	}
}
