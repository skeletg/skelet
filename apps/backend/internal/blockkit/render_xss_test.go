package blockkit

import (
	"regexp"
	"strings"
	"testing"
)

var reHandlerAttr = regexp.MustCompile(`(?is)<[^>]*\son[a-z]+\s*=`) // a real handler attribute, not a literal "onerror" the parser folded into a quoted attribute value (src="x/onerror=..."), which is inert

func TestSanitizeHTML_BlocksBypassPayloads(t *testing.T) { // a regex blocklist lets every one of these through - the allowlist parser must neutralize them all
	cases := []string{
		`<img src=x/onerror=alert(document.cookie)>`,
		`<svg/onload=alert(document.domain)>`,
		`<iframe srcdoc="&lt;script&gt;alert(1)&lt;/script&gt;"></iframe>`,
		`<img src=x onerror=alert(1)>`,
		`<a href="javascript:alert(1)">x</a>`,
		`<details ontoggle=alert(1) open>x</details>`,
		`<a href="vbscript:msgbox(1)">x</a>`,
		`<body onload=alert(1)>`,
	}
	for _, in := range cases {
		out := SanitizeHTML(in)
		low := strings.ToLower(out)
		if reHandlerAttr.MatchString(out) {
			t.Errorf("event-handler attribute survived:\n in : %s\n out: %s", in, out)
		}
		for _, bad := range []string{"<script", "<iframe", "srcdoc", "javascript:", "vbscript:"} {
			if strings.Contains(low, bad) {
				t.Errorf("dangerous token %q survived:\n in : %s\n out: %s", bad, in, out)
			}
		}
	}
}

func TestSanitizeHTML_KeepsSafeMarkup(t *testing.T) { // benign formatting must survive, or the sanitizer guts legitimate authored content
	in := `<p class="lead">Hello <strong>world</strong> <a href="https://example.com">link</a></p>`
	out := SanitizeHTML(in)
	for _, want := range []string{"<p", "class=", "<strong>", `href="https://example.com"`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q preserved, got: %s", want, out)
		}
	}
}
