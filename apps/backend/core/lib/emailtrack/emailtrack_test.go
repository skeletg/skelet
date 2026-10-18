package emailtrack

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestTokenRoundTrip(t *testing.T) {
	s := NewSigner([]byte("test-secret"))
	sid := uuid.New()

	tok := s.Sign(Payload{S: sid, U: "https://x.test/p", L: 3})
	p, err := s.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if p.S != sid || p.U != "https://x.test/p" || p.L != 3 {
		t.Fatalf("round-trip mismatch: %+v", p)
	}

	for _, bad := range []string{tok + "x", "garbage", "a.b", "", tok[:len(tok)-1]} {
		if _, err := s.Verify(bad); err == nil {
			t.Errorf("Verify(%q) accepted a forged token", bad)
		}
	}
	if _, err := NewSigner([]byte("other")).Verify(tok); err == nil {
		t.Error("token verified under the wrong key")
	}
}

func TestInject(t *testing.T) {
	s := NewSigner([]byte("test-secret"))
	sid := uuid.New()
	html := `<html><body>` +
		`<a href="https://store.example/a">A</a> ` +
		`<a href="https://store.example/b">B</a> ` +
		`<a href="https://shop.example/unsubscribe/abc">unsub</a>` +
		`</body></html>`

	out := s.Inject(html, "https://shop.example", sid)

	if !strings.Contains(out, `<img src="https://shop.example/e/o/`) {
		t.Errorf("open pixel not injected:\n%s", out)
	}
	if strings.Count(out, `href="https://shop.example/e/c/`) != 2 {
		t.Errorf("both outbound links should be rewritten:\n%s", out)
	}
	if strings.Contains(out, `href="https://store.example/a"`) {
		t.Errorf("original link should be rewritten:\n%s", out)
	}
	if !strings.Contains(out, "/unsubscribe/abc") {
		t.Errorf("unsubscribe link must be left intact:\n%s", out)
	}
	if pixel, body := strings.Index(out, "/e/o/"), strings.Index(strings.ToLower(out), "</body>"); pixel < 0 || pixel > body {
		t.Errorf("pixel should sit before </body>")
	}
	if got := s.Inject(html, "", sid); got != html {
		t.Errorf("with no base URL, html should be unchanged")
	}
}

func TestInjectPixelEmptyBaseNoOp(t *testing.T) {
	s := NewSigner([]byte("test-secret"))
	html := `<html><body>hi</body></html>`
	if got := s.InjectPixel(html, "", uuid.New()); got != html {
		t.Errorf("InjectPixel with empty base should leave html unchanged, got:\n%s", got)
	}
	// Whitespace/slash-only base trims to empty → also a no-op
	if got := s.InjectPixel(html, "/", uuid.New()); got != html {
		t.Errorf("InjectPixel with '/' base trims to empty and should be a no-op, got:\n%s", got)
	}
}

func TestInjectPixelPlacement(t *testing.T) {
	s := NewSigner([]byte("test-secret"))
	sid := uuid.New()

	// With a </body>, the pixel sits just before it
	withBody := `<html><body>hello</body></html>`
	out := s.InjectPixel(withBody, "https://shop.example", sid)
	pixel := strings.Index(out, "/e/o/")
	body := strings.Index(strings.ToLower(out), "</body>")
	if pixel < 0 {
		t.Fatalf("pixel not injected:\n%s", out)
	}
	if pixel > body {
		t.Errorf("pixel should precede </body>:\n%s", out)
	}

	// Without a </body>, the pixel is appended at the end
	noBody := `<div>hello</div>`
	out = s.InjectPixel(noBody, "https://shop.example", sid)
	if !strings.HasPrefix(out, noBody) {
		t.Errorf("original content should be preserved at the start:\n%s", out)
	}
	if !strings.Contains(out, "/e/o/") {
		t.Errorf("pixel should be appended when there's no </body>:\n%s", out)
	}
}

func TestInjectDoesNotDoubleRewriteTrackedLinks(t *testing.T) {
	s := NewSigner([]byte("test-secret"))
	sid := uuid.New()
	base := "https://shop.example"

	html := `<html><body><a href="https://store.example/a">A</a></body></html>`
	once := s.Inject(html, base, sid)
	twice := s.Inject(once, base, sid)

	// The click route already in `once` must be skipped on the second pass (the /e/c/ guard short-circuits
	// already-rewritten hrefs), so the click link count stays at 1 - the redirect is not chained through itself
	if c := strings.Count(twice, base+"/e/c/"); c != 1 {
		t.Errorf("expected exactly 1 click link after re-inject (no double rewrite), got %d:\n%s", c, twice)
	}
	// Note: InjectPixel appends unconditionally, so the open pixel is NOT idempotent - re-injecting yields a second pixel
	// - Assert the documented behavior rather than an imagined dedup
	if c := strings.Count(twice, base+"/e/o/"); c != 2 {
		t.Errorf("InjectPixel appends a pixel each call; expected 2 after re-inject, got %d:\n%s", c, twice)
	}
}

func TestUnsubscribeTokenRoundTrip(t *testing.T) {
	s := NewSigner([]byte("test-secret"))
	id := uuid.New()
	tok := s.SignUnsubscribe(id)
	got, err := s.VerifyUnsubscribe(tok)
	if err != nil || got != id {
		t.Fatalf("VerifyUnsubscribe = %s, %v; want %s", got, err, id)
	}
	for _, bad := range []string{tok + "x", "garbage", "a.b", ""} {
		if _, err := s.VerifyUnsubscribe(bad); err == nil {
			t.Errorf("VerifyUnsubscribe(%q) accepted a forged token", bad)
		}
	}
	if _, err := NewSigner([]byte("other")).VerifyUnsubscribe(tok); err == nil {
		t.Error("token verified under the wrong key")
	}
}
