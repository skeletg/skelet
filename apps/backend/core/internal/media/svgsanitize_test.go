package media_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/media"
)

// Each case is a way an SVG has been used to run script or phone home in the wild. The assertion is on the OUTPUT, not on an error: a sanitizer that quietly keeps the payload while returning nil is the failure mode worth guarding, so every case checks the dangerous token is gone
func TestSanitizeSVG_StripsAttacks(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		banned  []string // must not survive, matched case-insensitively
		wantErr error
	}{
		{
			name:   "script element",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script><rect width="1" height="1"/></svg>`,
			banned: []string{"script", "alert"},
		},
		{
			name:   "script child text is dropped with its parent",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><script><![CDATA[fetch("//evil")]]></script></svg>`,
			banned: []string{"fetch", "evil"},
		},
		{
			name:   "onload handler",
			src:    `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><rect width="1" height="1"/></svg>`,
			banned: []string{"onload", "alert"},
		},
		{
			name:   "event handler on a child",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1" onmouseover="alert(1)"/></svg>`,
			banned: []string{"onmouseover", "alert"},
		},
		{
			name:   "foreignObject smuggling html",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><body xmlns="http://www.w3.org/1999/xhtml"><img src="x" onerror="alert(1)"/></body></foreignObject></svg>`,
			banned: []string{"foreignObject", "onerror", "alert", "img"},
		},
		{
			// Catches a dropped element leaking its ALLOWLISTED children - the payload here is a
			// perfectly ordinary <rect> that must still vanish because its parent did
			name:   "a dropped element takes its whole subtree",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><rect width="1" height="1"/></foreignObject></svg>`,
			banned: []string{"<rect", "foreignObject"},
		},
		{
			name:   "anchor with a javascript url",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><a href="javascript:alert(1)"><rect width="1" height="1"/></a></svg>`,
			banned: []string{"javascript:", "alert", "<a"},
		},
		{
			name:   "use pulling an external document",
			src:    `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="http://evil.example/x.svg#a"/></svg>`,
			banned: []string{"evil.example", "xlink"},
		},
		{
			name:   "use with a data url",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><use href="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="/></svg>`,
			banned: []string{"data:", "base64"},
		},
		{
			name:   "external image reference",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><image href="http://evil.example/pixel.png"/></svg>`,
			banned: []string{"evil.example", "image"},
		},
		{
			name:   "style element with an import",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><style>@import url("http://evil.example/x.css");</style><rect width="1" height="1"/></svg>`,
			banned: []string{"@import", "evil.example", "<style"},
		},
		{
			name:   "style attribute fetching a remote url",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1" style="fill:red;background:url(http://evil.example/x.png)"/></svg>`,
			banned: []string{"evil.example"},
		},
		{
			name:   "paint attribute fetching a remote url",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1" fill="url(http://evil.example/x)"/></svg>`,
			banned: []string{"evil.example"},
		},
		{
			name:   "animate rewriting an attribute",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"><animate attributeName="href" values="javascript:alert(1)"/></rect></svg>`,
			banned: []string{"animate", "javascript:", "alert"},
		},
		{
			name:   "set element",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><set attributeName="onload" to="alert(1)"/><rect width="1" height="1"/></svg>`,
			banned: []string{"<set", "onload", "alert"},
		},
		{
			name:   "handler element",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><handler type="text/javascript">alert(1)</handler><rect width="1" height="1"/></svg>`,
			banned: []string{"handler", "alert"},
		},
		{
			name:   "comment hiding markup",
			src:    `<svg xmlns="http://www.w3.org/2000/svg"><!-- <script>alert(1)</script> --><rect width="1" height="1"/></svg>`,
			banned: []string{"script", "alert", "<!--"},
		},
		{
			// The classic XXE read. encoding/xml resolves no external entity, so this must fail loudly
			// rather than silently emit an empty document
			name:    "external entity",
			src:     `<!DOCTYPE svg [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><svg xmlns="http://www.w3.org/2000/svg"><text>&xxe;</text></svg>`,
			wantErr: media.ErrSVGUnsanitized,
		},
		{
			name:    "billion laughs",
			src:     `<!DOCTYPE svg [<!ENTITY a "aa"><!ENTITY b "&a;&a;"><!ENTITY c "&b;&b;">]><svg xmlns="http://www.w3.org/2000/svg"><text>&c;</text></svg>`,
			wantErr: media.ErrSVGUnsanitized,
		},
		{
			name:    "html that is not svg at all",
			src:     `<html><body><script>alert(1)</script></body></html>`,
			wantErr: media.ErrNotSVG,
		},
		{
			name:    "truncated document",
			src:     `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1">`,
			wantErr: media.ErrSVGUnsanitized,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := media.SanitizeSVG([]byte(tc.src))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got err %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := strings.ToLower(string(out))
			for _, bad := range tc.banned {
				if strings.Contains(got, strings.ToLower(bad)) {
					t.Errorf("sanitized output still contains %q:\n%s", bad, out)
				}
			}
		})
	}
}

// The other half of the job: a real logo has to come out the far side still drawable
func TestSanitizeSVG_KeepsDrawableContent(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 40" width="100" height="40">
  <title>Acme</title>
  <defs>
    <linearGradient id="g" x1="0" y1="0" x2="1" y2="0">
      <stop offset="0" stop-color="#ff0000"/>
      <stop offset="1" stop-color="#0000ff"/>
    </linearGradient>
  </defs>
  <rect x="0" y="0" width="100" height="40" rx="4" fill="url(#g)"/>
  <path d="M10 10 L30 30 Z" stroke="#fff" stroke-width="2" fill="none"/>
  <circle cx="60" cy="20" r="8" style="fill:#00ff00;opacity:0.8"/>
  <text x="12" y="26" font-family="Inter" font-size="12" fill="#111">Acme</text>
</svg>`
	out, err := media.SanitizeSVG([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		`xmlns="http://www.w3.org/2000/svg"`,
		`viewBox="0 0 100 40"`,
		`<linearGradient`, `stop-color="#ff0000"`,
		`fill="url(#g)"`, // an internal reference is the whole point of gradients - it must survive
		`d="M10 10 L30 30 Z"`,
		`<circle`, `style="fill:#00ff00;opacity:0.8"`,
		`<text`, `Acme`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("sanitized output lost %q:\n%s", want, got)
		}
	}
}

// Sanitizing an already-sanitized document must not keep changing it, or a re-upload would drift
func TestSanitizeSVG_Idempotent(t *testing.T) {
	src := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="#abc"/></svg>`
	once, err := media.SanitizeSVG([]byte(src))
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	twice, err := media.SanitizeSVG(once)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if string(once) != string(twice) {
		t.Errorf("not idempotent:\n first: %s\nsecond: %s", once, twice)
	}
}

func TestSanitizeSVG_RejectsRunawayNesting(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg">`)
	for range 200 {
		b.WriteString("<g>")
	}
	for range 200 {
		b.WriteString("</g>")
	}
	b.WriteString(`</svg>`)
	if _, err := media.SanitizeSVG([]byte(b.String())); !errors.Is(err, media.ErrSVGUnsanitized) {
		t.Fatalf("got %v, want ErrSVGUnsanitized", err)
	}
}

// The presign path hands the browser a key it writes directly, with the server never seeing the bytes - which is exactly why SVG must not be on that allowlist. Adding it there would look like "enabling SVG uploads" and would quietly restore the hole UploadSVG exists to close
func TestSVGStaysOffThePresignAllowlist(t *testing.T) {
	if media.IsAllowedImage("image/svg+xml") {
		t.Error("image/svg+xml is on AllowedImageMimeTypes: presigned uploads would store it unsanitized - it belongs on the UploadSVG route instead")
	}
}
