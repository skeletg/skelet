package emailtemplate

import (
	"strings"
	"testing"
)

// These exercise the pure HTML-safety helpers in component.go directly
// (sanitizeHTML / interpolateEscaped / renderCustomHTML / stripTags) - The
// service-level path is covered by render_test.go's TestRenderCustomTypeAndSanitize;
// here we pin the primitives against a focused XSS matrix

func TestSanitizeHTML(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantAbsent  []string // substrings that must NOT survive
		wantPresent []string // substrings that must survive (trusted chrome)
	}{
		{
			name:       "strips script tag",
			in:         `before<script>alert(1)</script>after`,
			wantAbsent: []string{"<script", "alert(1)"},
		},
		{
			name:       "strips style tag",
			in:         `<style>body{display:none}</style>hi`,
			wantAbsent: []string{"<style", "display:none"},
		},
		{
			name:        "script strip is case-insensitive",
			in:          `<ScRiPt>x</ScRiPt>keep`,
			wantAbsent:  []string{"ScRiPt", "<script"},
			wantPresent: []string{"keep"},
		},
		{
			name:       "script strip spans newlines",
			in:         "<script>\nvar a = 1;\nalert(a);\n</script>tail",
			wantAbsent: []string{"<script", "alert"},
		},
		{
			name:        "removes double-quoted on* handler",
			in:          `<img src="x" onerror="steal()">`,
			wantAbsent:  []string{"onerror", "steal()"},
			wantPresent: []string{`src="x"`},
		},
		{
			name:       "removes single-quoted on* handler",
			in:         `<div onclick='hack()'>x</div>`,
			wantAbsent: []string{"onclick", "hack()"},
		},
		{
			name:       "removes unquoted on* handler",
			in:         `<body onload=boom>`,
			wantAbsent: []string{"onload", "boom"},
		},
		{
			// The allowlist sanitizer drops the disallowed-scheme href entirely
			// (keeping the link text), rather than rewriting it to "unsafe:"
			name:        "neutralizes javascript scheme",
			in:          `<a href="javascript:evil()">go</a>`,
			wantAbsent:  []string{"javascript:"},
			wantPresent: []string{"go"},
		},
		{
			name:        "neutralizes vbscript scheme",
			in:          `<a href="vbscript:Msgbox">go</a>`,
			wantAbsent:  []string{"vbscript:"},
			wantPresent: []string{"go"},
		},
		{
			name:        "neutralizes scheme with spaces and mixed case",
			in:          `<a href="JavaScript : evil">go</a>`,
			wantAbsent:  []string{"JavaScript :", "javascript:"},
			wantPresent: []string{"go"},
		},
		{
			name:        "leaves benign chrome untouched",
			in:          `<table class="hero"><tr><td>Hello</td></tr></table>`,
			wantPresent: []string{`class="hero"`, "Hello", "<table"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeHTML(tt.in)
			for _, sub := range tt.wantAbsent {
				if strings.Contains(got, sub) {
					t.Errorf("sanitizeHTML(%q) = %q, must not contain %q", tt.in, got, sub)
				}
			}
			for _, sub := range tt.wantPresent {
				if !strings.Contains(got, sub) {
					t.Errorf("sanitizeHTML(%q) = %q, want to contain %q", tt.in, got, sub)
				}
			}
		})
	}
}

func TestInterpolateEscaped(t *testing.T) {
	vars := map[string]string{
		"name":    `<b>"Bob" & co</b>`,
		"plain":   "hello",
		"padded":  "padval",
		"dotted":  "deep",
		"injects": `<img src=x onerror=1>`,
	}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain substitution", "hi {{plain}}", "hi hello"},
		{
			"escapes angle brackets ampersand and quotes",
			"x={{name}}",
			`x=&lt;b&gt;&#34;Bob&#34; &amp; co&lt;/b&gt;`,
		},
		{"whitespace-padded token", "v={{ padded }}", "v=padval"},
		{"dotted key", "{{dotted}}", "deep"},
		{"missing token becomes empty", "[{{nope}}]", "[]"},
		{"escapes an injection payload", "{{injects}}", `&lt;img src=x onerror=1&gt;`},
		{"multiple tokens", "{{plain}}-{{plain}}", "hello-hello"},
		{"no tokens passthrough", "just literal text", "just literal text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := interpolateEscaped(tt.in, vars); got != tt.want {
				t.Errorf("interpolateEscaped(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStripTags(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"removes simple tags", "<p>Hello <b>world</b></p>", "Hello world"},
		{"trims surrounding whitespace", "  <div>  text </div>  ", "text"},
		{"tags spanning newlines", "<a\nhref='x'>link</a>", "link"},
		{"no tags untouched", "plain text", "plain text"},
		{"only tags collapses to empty", "<br><hr>", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripTags(tt.in); got != tt.want {
				t.Errorf("stripTags(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRenderCustomHTML(t *testing.T) {
	vars := map[string]string{"label": "<Sale>", "who": "Sam"}

	t.Run("escapes fields and sanitizes html, with explicit text template", func(t *testing.T) {
		htmlOut, textOut := renderCustomHTML(
			`<div class="cd">{{label}} for {{who}}<script>alert(1)</script><a href="javascript:x">go</a></div>`,
			"Plain {{label}} for {{who}}",
			vars,
		)
		if !strings.Contains(htmlOut, `class="cd"`) {
			t.Errorf("trusted chrome dropped: %q", htmlOut)
		}
		if !strings.Contains(htmlOut, "&lt;Sale&gt;") {
			t.Errorf("field value not HTML-escaped: %q", htmlOut)
		}
		if !strings.Contains(htmlOut, "for Sam") {
			t.Errorf("merge tag not interpolated: %q", htmlOut)
		}
		if strings.Contains(htmlOut, "<script") {
			t.Errorf("script not sanitized: %q", htmlOut)
		}
		if strings.Contains(htmlOut, "javascript:") {
			t.Errorf("javascript URL not neutralized: %q", htmlOut)
		}
		// Text template uses raw (unescaped) interpolate and a trailing newline
		wantText := "Plain <Sale> for Sam\n"
		if textOut != wantText {
			t.Errorf("textOut = %q, want %q", textOut, wantText)
		}
	})

	t.Run("empty text template falls back to stripped html", func(t *testing.T) {
		plainVars := map[string]string{"label": "Big Sale", "who": "Sam"}
		htmlOut, textOut := renderCustomHTML(`<p>{{label}} for {{who}}</p>`, "   ", plainVars)
		if !strings.Contains(htmlOut, "Big Sale for Sam") {
			t.Errorf("html out missing interpolated content: %q", htmlOut)
		}
		// Fallback strips tags from the interpolated (raw) html template
		wantText := "Big Sale for Sam\n"
		if textOut != wantText {
			t.Errorf("fallback textOut = %q, want %q", textOut, wantText)
		}
	})

	t.Run("text fallback strips substituted values that look like tags", func(t *testing.T) {
		// interpolate is the RAW (non-escaping) variant, so a field value of "<Sale>" is interpolated literally and then
		// stripTags treats it as a tag and removes it - documenting the plain-text fallback's behaviour
		_, textOut := renderCustomHTML(`<p>{{label}} for {{who}}</p>`, "", vars)
		if textOut != "for Sam\n" {
			t.Errorf("textOut = %q, want %q", textOut, "for Sam\n")
		}
	})
}
