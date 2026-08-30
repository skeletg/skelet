package emailtemplate

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// Helpers for parameterized and HTML-body blocks, the capability a standalone "block type" would otherwise carry - field-schema validation and safe rendering of an admin-authored HTML body

var fieldKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func validFieldKind(k string) bool {
	switch k {
	case "text", "textarea", "url", "color", "number", "date", "image", "select", "boolean":
		return true
	}
	return false
}

// Whether a name belongs to a built-in block, so a block's slug can't shadow one when referenced through the {type:<slug>} alias
func isBuiltinType(t string) bool {
	if t == "row" || t == "ref" {
		return true
	}
	_, ok := blockHTML[t]
	return ok
}

func validateFields(fields []FieldDef) error {
	seen := map[string]bool{}
	for _, f := range fields {
		if !fieldKeyRe.MatchString(f.Key) {
			return fmt.Errorf("field key %q must be lowercase letters/digits/underscores (start with a letter)", f.Key)
		}
		if seen[f.Key] {
			return fmt.Errorf("duplicate field key %q", f.Key)
		}
		seen[f.Key] = true
		if !validFieldKind(f.Kind) {
			return fmt.Errorf("field %q has invalid kind %q", f.Key, f.Kind)
		}
	}
	return nil
}

var tokenRe = regexp.MustCompile(`\{\{\s*([\w.]+)\s*\}\}`)

// Substitutes {{token}} with the HTML-escaped var value, leaving the admin-trusted surrounding template HTML intact
func interpolateEscaped(s string, vars map[string]string) string {
	return tokenRe.ReplaceAllStringFunc(s, func(m string) string {
		key := strings.TrimSpace(tokenRe.FindStringSubmatch(m)[1])
		return html.EscapeString(vars[key])
	})
}

var reTags = regexp.MustCompile(`(?s)<[^>]*>`)

// Allowlist-sanitizes merge and field values before they go into email HTML - same rationale as blockkit.SanitizeHTML, since a regex blocklist is bypassable and a real parser isn't - Built once, goroutine-safe
var htmlPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class").Globally()
	return p
}()

// Allowlist-sanitizes HTML - drops <script>, <style>, all event handlers, unsafe URL schemes, and <iframe>, <object> and <embed>
func sanitizeHTML(s string) string {
	return htmlPolicy.Sanitize(s)
}

func stripTags(s string) string {
	return strings.TrimSpace(reTags.ReplaceAllString(s, ""))
}

// Renders a block's HTML body and plain text - field and merge values are escaped then sanitized, while the template's literal HTML is the admin's trusted chrome
func renderCustomHTML(htmlTemplate, textTemplate string, vars map[string]string) (htmlOut, textOut string) {
	htmlOut = sanitizeHTML(interpolateEscaped(htmlTemplate, vars))
	if strings.TrimSpace(textTemplate) != "" {
		textOut = interpolate(textTemplate, vars) + "\n"
	} else {
		textOut = stripTags(interpolate(htmlTemplate, vars)) + "\n"
	}
	return htmlOut, textOut
}
