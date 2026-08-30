package locale

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Content rows store lowercase codes and every lookup matches them exactly, so a tag that reaches
// a service with its original casing misses every translation and silently falls back to the store
// default - the admin surface has no channel clamp to canonicalise it, so this is the only guard

func TestWithLocaleCanonicalisesCase(t *testing.T) {
	for _, tag := range []string{"de", "DE", "dE", "  De  "} {
		if got := FromContext(WithLocale(context.Background(), tag)); got != "de" {
			t.Errorf("WithLocale(%q) = %q, want %q - a case variant returns the wrong language", tag, got, "de")
		}
	}
}

func TestWithLocaleCanonicalisesRegionSubtag(t *testing.T) {
	// The conventional BCP 47 form uppercases the region, so this is what a correct client sends
	if got := FromContext(WithLocale(context.Background(), "de-DE")); got != "de-de" {
		t.Errorf("WithLocale(\"de-DE\") = %q, want %q", got, "de-de")
	}
}

func TestWithLocaleIgnoresEmpty(t *testing.T) {
	for _, tag := range []string{"", "   "} {
		if got := FromContext(WithLocale(context.Background(), tag)); got != "" {
			t.Errorf("WithLocale(%q) stored %q, want no value", tag, got)
		}
	}
}

func TestLooksLikeCode(t *testing.T) {
	for _, ok := range []string{"en", "de", "de-DE", "sr-Latn-RS", "EN"} {
		if !LooksLikeCode(ok) {
			t.Errorf("LooksLikeCode(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "e", "en_US", "../../etc", "en;drop", "toolongsubtag123456"} {
		if LooksLikeCode(bad) {
			t.Errorf("LooksLikeCode(%q) = true, want false", bad)
		}
	}
}

func TestMiddlewareDropsMalformedTags(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"de", "de"},
		{"DE", "de"},
		{"de-DE", "de-de"},
		{"en_US", ""},   // underscore is not BCP 47 - carrying it only poisons cache keys
		{"../../x", ""}, // path-ish junk must never reach a term filter
		{"", ""},
	}
	for _, c := range cases {
		var got string
		h := Middleware()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got = FromContext(r.Context())
		}))
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if c.header != "" {
			req.Header.Set(HeaderName, c.header)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if got != c.want {
			t.Errorf("header %q put %q in context, want %q", c.header, got, c.want)
		}
	}
}
