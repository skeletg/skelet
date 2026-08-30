package mediahttp

import (
	"net/http/httptest"
	"testing"
)

func TestAcceptsWebP(t *testing.T) {
	cases := map[string]bool{
		"image/avif,image/webp,image/apng,*/*;q=0.8": true, // a browser's image Accept
		"image/webp": true,
		"*/*":        false, // wildcard isn't an advertisement - Outlook's fetcher sends this and can't render webp
		"":           false,
	}
	for accept, want := range cases {
		r := httptest.NewRequest("GET", "/media/t/w640/p.jpg", nil)
		if accept != "" {
			r.Header.Set("Accept", accept)
		}
		if got := acceptsWebP(r); got != want {
			t.Errorf("acceptsWebP(%q) = %v, want %v", accept, got, want)
		}
	}
}

func TestParseTransformPath(t *testing.T) {
	const prefix = "/media/t/"

	ok := []struct {
		path    string
		w, h    int
		wantKey string
	}{
		{"/media/t/w640/p/img.jpg", 640, 0, "p/img.jpg"},
		{"/media/t/w640h480/p/img.jpg", 640, 480, "p/img.jpg"},
		{"/media/t/w1/a.png", 1, 0, "a.png"},
		// Nested keys keep their slashes - the key is everything after the spec
		{"/media/t/w320/2026/07/deep/nested/img.webp", 320, 0, "2026/07/deep/nested/img.webp"},
	}
	for _, c := range ok {
		spec, key, got := parseTransformPath(prefix, c.path)
		if !got {
			t.Errorf("%s rejected, want accepted", c.path)
			continue
		}
		if spec.w != c.w || spec.h != c.h || key != c.wantKey {
			t.Errorf("%s = w%d h%d %q, want w%d h%d %q", c.path, spec.w, spec.h, key, c.w, c.h, c.wantKey)
		}
	}

	bad := []struct {
		path, why string
	}{
		{"/other/w640/img.jpg", "wrong prefix"},
		{"/media/t/w640", "no key"},
		{"/media/t/w640/", "empty key"},
		{"/media/t/640/img.jpg", "spec missing the w"},
		{"/media/t/w0/img.jpg", "zero width"},
		{"/media/t/w-5/img.jpg", "negative width"},
		{"/media/t/wabc/img.jpg", "non-numeric width"},
		{"/media/t/w640h0/img.jpg", "zero height"},
		{"/media/t/w640habc/img.jpg", "non-numeric height"},
		// Both of these matter: no derivatives-of-derivatives (unbounded
		// nesting), and no traversal out of the key namespace
		{"/media/t/w640/derived/w320/img.jpg", "derived source"},
		{"/media/t/w640/../../etc/passwd", "traversal"},
		{"/media/t/w640/a/../../b.jpg", "traversal mid-key"},
	}
	for _, c := range bad {
		if _, _, got := parseTransformPath(prefix, c.path); got {
			t.Errorf("%s accepted, want rejected (%s)", c.path, c.why)
		}
	}
}

func TestParseTransformPathHonorsCustomPrefix(t *testing.T) {
	spec, key, ok := parseTransformPath("/img/", "/img/w960/a.jpg")
	if !ok || spec.w != 960 || key != "a.jpg" {
		t.Fatalf("got w%d %q ok=%v", spec.w, key, ok)
	}
	if _, _, ok := parseTransformPath("/img/", "/media/t/w960/a.jpg"); ok {
		t.Error("default prefix accepted under a custom mount")
	}
}
