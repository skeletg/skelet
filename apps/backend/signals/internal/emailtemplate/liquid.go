package emailtemplate

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/osteele/liquid"

	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

// Liquid has no file, network or code access, which is why it can run operator-authored markup - but nothing bounds what a
// template costs, so these do: output and string growth are capped, literal loop ranges are refused past a size, and a
// scripted block must render its sample within a budget before it can be saved
const (
	maxScriptedOutput   = 512 << 10
	maxScriptedString   = 64 << 10
	maxScriptedRange    = 1000
	scriptedTrialBudget = 200 * time.Millisecond
)

// The engine is stateless per render and safe to share
var liquidEngine = newLiquidEngine()

func newLiquidEngine() *liquid.Engine {
	e := liquid.NewEngine()
	e.RegisterFilter("append", func(s, suffix string) string { return capString(s + suffix) })
	e.RegisterFilter("prepend", func(s, prefix string) string { return capString(prefix + s) })
	e.RegisterFilter("replace", func(s, old, repl string) string { return cappedReplace(s, old, repl, -1) })
	e.RegisterFilter("replace_first", func(s, old, repl string) string { return cappedReplace(s, old, repl, 1) })
	return e
}

func capString(s string) string {
	if len(s) > maxScriptedString {
		return s[:maxScriptedString]
	}
	return s
}

// Sizes the result before building it, since a short needle and a long replacement can multiply a string past any cap
func cappedReplace(s, old, repl string, n int) string {
	if old == "" {
		return s
	}
	count := strings.Count(s, old)
	if n >= 0 {
		count = min(count, n)
	}
	if len(s)+count*(len(repl)-len(old)) > maxScriptedString {
		return capString(s)
	}
	return strings.Replace(s, old, repl, n)
}

var (
	errScriptedOutput = errors.New("scripted block output is over its cap")
	errNoCapture      = errors.New("capture isn't available in email blocks - assign a value instead")
)

type cappedWriter struct {
	strings.Builder
	over bool
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if w.Len()+len(p) > maxScriptedOutput {
		w.over = true
		return 0, errScriptedOutput
	}
	return w.Builder.Write(p)
}

// A capture can feed itself, doubling a string every loop pass - Liquid can't drop its standard tags, so the source is refused instead
var reLiquidCapture = regexp.MustCompile(`\{%-?\s*capture\b`)

func renderLiquid(src string, bindings map[string]any) (string, bool, error) {
	if reLiquidCapture.MatchString(src) {
		return "", false, errNoCapture
	}
	var w cappedWriter
	err := liquidEngine.ParseAndFRender(&w, []byte(src), bindings)
	return w.String(), w.over, err
}

var reLiquidRange = regexp.MustCompile(`\(\s*(-?\d+)\s*\.\.\s*(-?\d+)\s*\)`)

// checkScripted is the save-time gate on a scripted block: no literal loop past the range cap, and a sample render that
// finishes inside the budget and under the output cap - a template that fails it is refused rather than met by a send
func checkScripted(sn Snippet) error {
	for _, src := range []string{sn.HTMLTemplate, sn.TextTemplate} {
		for _, m := range reLiquidRange.FindAllStringSubmatch(src, -1) {
			lo, _ := strconv.Atoi(m[1])
			hi, _ := strconv.Atoi(m[2])
			if hi-lo >= maxScriptedRange {
				return fmt.Errorf("scripted block loops over %s, the most one loop may run is %d", m[0], maxScriptedRange)
			}
		}
	}
	vars := map[string]string{"store.name": "Store", "customer.firstName": "Alex", "storefront.url": "https://shop.example"}
	for _, f := range sn.Fields {
		vars[f.Key] = f.Default
	}
	bindings := liquidBindings(vars, sampleOrderCtx())
	done := make(chan error, 1)
	go func() {
		for _, src := range []string{sn.HTMLTemplate, sn.TextTemplate} {
			if _, over, err := renderLiquid(src, bindings); over {
				done <- fmt.Errorf("scripted block renders more than %d KB", maxScriptedOutput>>10)
				return
			} else if err != nil {
				done <- fmt.Errorf("scripted block: %w", err)
				return
			}
		}
		done <- nil
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(scriptedTrialBudget):
		return fmt.Errorf("scripted block takes longer than %s to render", scriptedTrialBudget)
	}
}

// Renders a scripted block's HTML body through Liquid over a nested data model built from the email's merge vars and structured order data, then sanitizes the output - so template LOGIC is author-controlled while injected data still can't smuggle in active content
func (s *Service) renderScriptedHTML(snip Snippet, vars map[string]string, rc renderCtx) (htmlOut, textOut string) {
	bindings := liquidBindings(vars, rc)
	out, _, err := renderLiquid(snip.HTMLTemplate, bindings)
	if err != nil {
		// A template error must never break the email - it falls back to the plainly interpolated body, and the reason goes to the
		// log, never into the customer's mail
		log.Printf("[emailtemplate] scripted block %s: %v", snip.Slug, err)
		out = interpolateEscaped(snip.HTMLTemplate, vars)
	}
	htmlOut = sanitizeHTML(out)
	if strings.TrimSpace(snip.TextTemplate) != "" {
		if t, _, terr := renderLiquid(snip.TextTemplate, bindings); terr == nil {
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

// A three-line order, so a scripted block's loops over order.items run during the save-time trial
func sampleOrderCtx() renderCtx {
	lines := make([]email.OrderEmailLine, 3)
	for i := range lines {
		lines[i] = email.OrderEmailLine{Name: "Sample product", SKU: "SKU-" + strconv.Itoa(i+1), Quantity: 1, UnitPrice: 1999, LineTotal: 1999}
	}
	return renderCtx{Currency: "USD", Lines: lines, ShipTo: []string{"Alex Sample", "1 Sample Street"}}
}
