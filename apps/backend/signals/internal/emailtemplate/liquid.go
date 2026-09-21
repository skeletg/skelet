package emailtemplate

import (
	"regexp"
	"strings"

	"github.com/osteele/liquid"
)

// Renders "scripted" reusable blocks - Liquid is a sandboxed template language with no file, network or code access, which is why it can run operator-authored markup
// The engine is stateless per render and safe to share
var liquidEngine = liquid.NewEngine()

// Renders a scripted block's HTML body through Liquid over a nested data model built from the email's merge vars and structured order data, then sanitizes the output - so template LOGIC is author-controlled while injected data still can't smuggle in active content
func (s *Service) renderScriptedHTML(snip Snippet, vars map[string]string, rc renderCtx) (htmlOut, textOut string) {
	bindings := liquidBindings(vars, rc)
	out, err := liquidEngine.ParseAndRenderString(snip.HTMLTemplate, bindings)
	if err != nil {
		// A template error must never break the email - emit a comment and fall back to the raw interpolated body, so something still renders
		out = "<!-- block " + snip.Slug + ": template error: " + err.Error() + " -->" +
			interpolateEscaped(snip.HTMLTemplate, vars)
	}
	htmlOut = sanitizeHTML(out)
	if strings.TrimSpace(snip.TextTemplate) != "" {
		if t, terr := liquidEngine.ParseAndRenderString(snip.TextTemplate, bindings); terr == nil {
			textOut = t + "\n"
		}
	}
	if textOut == "" {
		textOut = stripTags(htmlOut) + "\n"
	}
	return htmlOut, textOut
}

// Turns the flat dotted merge vars into a nested map Liquid can dot-walk, and layers on the structured data the flat vars can't carry - order.items for loops, and order.ship_to
func liquidBindings(vars map[string]string, rc renderCtx) map[string]any {
	root := map[string]any{}
	for k, v := range vars {
		parts := strings.Split(k, ".")
		cur := root
		for i, p := range parts {
			if i == len(parts)-1 {
				cur[p] = v
				break
			}
			next, ok := cur[p].(map[string]any)
			if !ok {
				next = map[string]any{}
				cur[p] = next
			}
			cur = next
		}
	}

	order, ok := root["order"].(map[string]any)
	if !ok {
		order = map[string]any{}
		root["order"] = order
	}
	items := make([]map[string]any, len(rc.Lines))
	for i, l := range rc.Lines {
		items[i] = map[string]any{
			"name":       l.Name,
			"sku":        l.SKU,
			"quantity":   l.Quantity,
			"unit_price": money(l.UnitPrice, rc.Currency),
			"line_total": money(l.LineTotal, rc.Currency),
		}
	}
	order["items"] = items
	order["ship_to"] = rc.ShipTo
	return root
}

var (
	reCSSAngle    = regexp.MustCompile(`[<>]`)
	reCSSImport   = regexp.MustCompile(`(?i)@import`)
	reCSSExpr     = regexp.MustCompile(`(?i)expression\s*\(`)
	reCSSJSScheme = regexp.MustCompile(`(?i)(javascript|vbscript)\s*:`)
	reCSSBind     = regexp.MustCompile(`(?i)-moz-binding`)
)

// Neutralizes the CSS-vector attacks before the operator's stylesheet is inlined into a <style> - tag breakout, external fetch through @import, legacy IE JS like expression() and -moz-binding, and script URL schemes
// The CSS is admin-authored, so this is defence-in-depth, matching sanitizeHTML's posture
func sanitizeCSS(css string) string {
	css = strings.TrimSpace(css)
	if css == "" {
		return ""
	}
	css = reCSSAngle.ReplaceAllString(css, "")
	css = reCSSImport.ReplaceAllString(css, "/*import*/")
	css = reCSSExpr.ReplaceAllString(css, "/*expr*/(")
	css = reCSSBind.ReplaceAllString(css, "/*bind*/")
	css = reCSSJSScheme.ReplaceAllString(css, "unsafe:")
	return css
}
