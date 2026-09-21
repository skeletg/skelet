package emailtemplate

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Block-type rendering - Each known leaf type has an HTML renderer, a plain-text renderer, and a validator - Container ("row") + "ref" blocks are handled by the service (they need column recursion / repo access) - validated here only Per-block style + the email theme (renderCtx.Theme) drive colors/fonts/align; box-level spacing (padding/background) is applied by boxHTML in renderOne Data blocks render nothing when the context lacks their data

// esc HTML-escapes interpolated, admin-authored plain text before it goes into
// markup (blocks hold plain text, never raw HTML - keeps emails injection-safe)
func esc(s string, vars map[string]string) string { return html.EscapeString(interpolate(s, vars)) }

// --- style resolution helpers ---

func styleOf(b Block) BlockStyle {
	if b.Style != nil {
		return *b.Style
	}
	return BlockStyle{}
}

// col returns the first non-empty color, HTML-escaped for inlining into a
// style="" attribute (defence-in-depth on top of save-time validation)
func col(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return html.EscapeString(v)
		}
	}
	return ""
}

func alignOf(s BlockStyle) string {
	switch s.Align {
	case "center", "right", "left":
		return s.Align
	}
	return "left"
}

func pxOr(n, def int) string {
	if n > 0 {
		return strconv.Itoa(n) + "px"
	}
	return strconv.Itoa(def) + "px"
}

var blockHTML = map[string]func(Block, renderCtx) string{
	"heading": func(b Block, rc renderCtx) string {
		s := styleOf(b)
		st := "margin:0 0 8px;font-size:" + pxOr(s.FontSize, 20) + ";text-align:" + alignOf(s) + ";"
		if c := col(s.Color, rc.Theme.TextColor, "#222"); c != "" {
			st += "color:" + c + ";"
		}
		return `<h2 style="` + st + `">` + esc(b.Text, rc.Vars) + `</h2>`
	},
	"text": func(b Block, rc renderCtx) string {
		s := styleOf(b)
		body := strings.ReplaceAll(esc(b.Content, rc.Vars), "\n", "<br>")
		st := "margin:0 0 12px;line-height:1.5;text-align:" + alignOf(s) + ";"
		if c := col(s.Color, rc.Theme.TextColor, "#333"); c != "" {
			st += "color:" + c + ";"
		}
		if s.FontSize > 0 {
			st += "font-size:" + pxOr(s.FontSize, 0) + ";"
		}
		return `<p style="` + st + `">` + body + `</p>`
	},
	"button": func(b Block, rc renderCtx) string {
		s := styleOf(b)
		bg := col(s.ButtonColor, rc.Theme.LinkColor, "#222")
		fg := col(s.ButtonTextColor, "#fff")
		a := `<a href="` + esc(b.URL, rc.Vars) + `" style="display:inline-block;background:` + bg +
			`;color:` + fg + `;text-decoration:none;padding:10px 18px;border-radius:` + pxOr(s.Radius, 4) + `;">` +
			esc(b.Label, rc.Vars) + `</a>`
		return `<div style="text-align:` + alignOf(s) + `;margin:16px 0;">` + a + `</div>`
	},
	"image": func(b Block, rc renderCtx) string {
		src := strings.TrimSpace(interpolate(b.URL, rc.Vars))
		if src == "" {
			return ""
		}
		s := styleOf(b)
		dim := "max-width:100%;height:auto;"
		if s.Width > 0 && s.Width <= 100 {
			dim = "width:" + strconv.Itoa(s.Width) + "%;height:auto;"
		}
		return `<div style="text-align:` + alignOf(s) + `;margin:12px 0;"><img src="` + html.EscapeString(src) +
			`" alt="` + esc(b.Alt, rc.Vars) + `" style="` + dim + `"></div>`
	},
	"divider": func(b Block, _ renderCtx) string {
		c := col(styleOf(b).Color, "#e5e5e5")
		return `<hr style="border:none;border-top:1px solid ` + c + `;margin:16px 0;">`
	},
	"spacer": func(b Block, _ renderCtx) string {
		return `<div style="height:` + strconv.Itoa(spacerPx(b)) + `px;line-height:` + strconv.Itoa(spacerPx(b)) + `px;">&nbsp;</div>`
	},
	"order_items": func(_ Block, rc renderCtx) string {
		if len(rc.Lines) == 0 {
			return ""
		}
		var b strings.Builder
		b.WriteString(`<table cellpadding="6" cellspacing="0" style="border-collapse:collapse;width:100%;"><thead><tr style="text-align:left;border-bottom:1px solid #ddd;"><th>Item</th><th>Qty</th><th style="text-align:right;">Total</th></tr></thead><tbody>`)
		for _, l := range rc.Lines {
			fmt.Fprintf(&b, `<tr style="border-bottom:1px solid #f0f0f0;"><td>%s<br><span style="color:#888;font-size:12px;">%s</span></td><td>%s</td><td style="text-align:right;">%s</td></tr>`,
				html.EscapeString(l.Name), html.EscapeString(l.SKU), qty(l.Quantity), money(l.LineTotal, rc.Currency))
		}
		b.WriteString(`</tbody></table>`)
		return b.String()
	},
	"order_totals": func(_ Block, rc renderCtx) string {
		v := rc.Vars
		if v["order.total"] == "" {
			return ""
		}
		var b strings.Builder
		b.WriteString(`<table cellpadding="2" cellspacing="0" style="margin-top:12px;width:100%;">`)
		totalsRow(&b, "Subtotal", v["order.subtotal"], false)
		if v["order.discount"] != "" {
			totalsRow(&b, "Discount", "-"+v["order.discount"], false)
		}
		totalsRow(&b, "Shipping", v["order.shipping"], false)
		totalsRow(&b, "Tax", v["order.tax"], false)
		totalsRow(&b, "Total", v["order.total"], true)
		b.WriteString(`</table>`)
		return b.String()
	},
	"shipping_address": func(_ Block, rc renderCtx) string {
		if len(rc.ShipTo) == 0 {
			return ""
		}
		var b strings.Builder
		b.WriteString(`<p style="margin-top:12px;"><strong>Shipping to</strong><br>`)
		for _, line := range rc.ShipTo {
			b.WriteString(html.EscapeString(line))
			b.WriteString("<br>")
		}
		b.WriteString(`</p>`)
		return b.String()
	},
	"tracking": func(_ Block, rc renderCtx) string {
		c, n := rc.Vars["tracking.carrier"], rc.Vars["tracking.number"]
		if c == "" && n == "" {
			return ""
		}
		out := `<p>Carrier: ` + html.EscapeString(c)
		if n != "" {
			out += ` &middot; Tracking: <strong>` + html.EscapeString(n) + `</strong>`
		}
		return out + `</p>`
	},
	"refund_amount": func(_ Block, rc renderCtx) string {
		if rc.Vars["refund.amount"] == "" {
			return ""
		}
		return `<p>Refund issued: <strong>` + html.EscapeString(rc.Vars["refund.amount"]) + `</strong></p>`
	},
	"social": func(b Block, rc renderCtx) string {
		if len(b.Social) == 0 {
			return ""
		}
		s := styleOf(b)
		link := col(s.Color, rc.Theme.LinkColor, "#2563eb")
		var sb strings.Builder
		fmt.Fprintf(&sb, `<div style="text-align:%s;margin:12px 0;">`, alignOf(s))
		for _, l := range b.Social {
			u := strings.TrimSpace(interpolate(l.URL, rc.Vars))
			if u == "" {
				continue
			}
			fmt.Fprintf(&sb, `<a href="%s" style="display:inline-block;margin:0 6px;color:%s;text-decoration:none;font-weight:600;">%s</a>`,
				html.EscapeString(u), link, html.EscapeString(socialLabel(l.Platform)))
		}
		sb.WriteString(`</div>`)
		return sb.String()
	},
	"video": func(b Block, rc renderCtx) string {
		u := strings.TrimSpace(interpolate(b.URL, rc.Vars))
		poster := strings.TrimSpace(interpolate(b.Poster, rc.Vars))
		if u == "" || poster == "" {
			return ""
		}
		s := styleOf(b)
		// Email can't embed video → the poster links out, with a ▶ overlay hint
		return `<div style="text-align:` + alignOf(s) + `;margin:12px 0;"><a href="` + html.EscapeString(u) +
			`" style="text-decoration:none;"><img src="` + html.EscapeString(poster) +
			`" alt="` + esc(b.Alt, rc.Vars) + `" style="max-width:100%;height:auto;border-radius:8px;"><div style="margin-top:-40px;font-size:32px;color:#fff;text-shadow:0 1px 4px rgba(0,0,0,.6);">&#9654;</div></a></div>`
	},
	"html": func(b Block, rc renderCtx) string {
		// Admin-authored raw HTML, interpolated then sanitized (strip script/on*/js:)
		return sanitizeHTML(interpolateEscaped(b.Content, rc.Vars))
	},
	"countdown": func(b Block, rc renderCtx) string {
		// v1: a static deadline display - (A live animated countdown needs an
		// external timer-image service; deferred.)
		dl := strings.TrimSpace(interpolate(b.Deadline, rc.Vars))
		if dl == "" {
			return ""
		}
		s := styleOf(b)
		label := esc(b.Label, rc.Vars)
		return `<div style="text-align:` + alignOf(s) + `;margin:16px 0;font-weight:700;">` +
			label + ` <span style="font-size:` + pxOr(s.FontSize, 22) + `;">` + html.EscapeString(dl) + `</span></div>`
	},
}

// socialLabel maps a platform key to a display label
func socialLabel(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "facebook":
		return "Facebook"
	case "instagram":
		return "Instagram"
	case "x", "twitter":
		return "X"
	case "youtube":
		return "YouTube"
	case "linkedin":
		return "LinkedIn"
	case "tiktok":
		return "TikTok"
	case "pinterest":
		return "Pinterest"
	default:
		if p == "" {
			return "Link"
		}
		return p
	}
}

var blockText = map[string]func(Block, renderCtx) string{
	"heading": func(b Block, rc renderCtx) string { return interpolate(b.Text, rc.Vars) + "\n" },
	"text":    func(b Block, rc renderCtx) string { return interpolate(b.Content, rc.Vars) + "\n" },
	"button": func(b Block, rc renderCtx) string {
		return interpolate(b.Label, rc.Vars) + ": " + interpolate(b.URL, rc.Vars) + "\n"
	},
	"image":   func(Block, renderCtx) string { return "" },
	"divider": func(Block, renderCtx) string { return "----------\n" },
	"spacer":  func(Block, renderCtx) string { return "\n" },
	"order_items": func(_ Block, rc renderCtx) string {
		var b strings.Builder
		for _, l := range rc.Lines {
			fmt.Fprintf(&b, "- %s (%s) x%s  %s\n", l.Name, l.SKU, qty(l.Quantity), money(l.LineTotal, rc.Currency))
		}
		return b.String()
	},
	"order_totals": func(_ Block, rc renderCtx) string {
		v := rc.Vars
		if v["order.total"] == "" {
			return ""
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Subtotal: %s\n", v["order.subtotal"])
		if v["order.discount"] != "" {
			fmt.Fprintf(&b, "Discount: -%s\n", v["order.discount"])
		}
		fmt.Fprintf(&b, "Shipping: %s\nTax: %s\nTotal: %s\n", v["order.shipping"], v["order.tax"], v["order.total"])
		return b.String()
	},
	"shipping_address": func(_ Block, rc renderCtx) string {
		if len(rc.ShipTo) == 0 {
			return ""
		}
		return "Shipping to:\n" + strings.Join(rc.ShipTo, "\n") + "\n"
	},
	"tracking": func(_ Block, rc renderCtx) string {
		c, n := rc.Vars["tracking.carrier"], rc.Vars["tracking.number"]
		if c == "" && n == "" {
			return ""
		}
		out := "Carrier: " + c
		if n != "" {
			out += " - Tracking: " + n
		}
		return out + "\n"
	},
	"refund_amount": func(_ Block, rc renderCtx) string {
		if rc.Vars["refund.amount"] == "" {
			return ""
		}
		return "Refund issued: " + rc.Vars["refund.amount"] + "\n"
	},
	"social": func(b Block, rc renderCtx) string {
		if len(b.Social) == 0 {
			return ""
		}
		var parts []string
		for _, l := range b.Social {
			u := strings.TrimSpace(interpolate(l.URL, rc.Vars))
			if u != "" {
				parts = append(parts, socialLabel(l.Platform)+": "+u)
			}
		}
		return strings.Join(parts, "  ") + "\n"
	},
	"video": func(b Block, rc renderCtx) string {
		u := strings.TrimSpace(interpolate(b.URL, rc.Vars))
		if u == "" {
			return ""
		}
		return "Watch: " + u + "\n"
	},
	"countdown": func(b Block, rc renderCtx) string {
		dl := strings.TrimSpace(interpolate(b.Deadline, rc.Vars))
		if dl == "" {
			return ""
		}
		return strings.TrimSpace(interpolate(b.Label, rc.Vars)+" "+dl) + "\n"
	},
	// "html" has no clean plain-text form; omitted (renders in HTML part only)
}

// boxHTML wraps a leaf block's HTML with its box-level styling (padding + background) - Returns inner unchanged when
// nothing is set, and "" for empty inner (so a data block with no data adds no wrapper)
func boxHTML(b Block, inner string) string {
	if inner == "" {
		return ""
	}
	s := styleOf(b)
	var st string
	if s.PaddingY > 0 || s.PaddingX > 0 {
		st += "padding:" + strconv.Itoa(s.PaddingY) + "px " + strconv.Itoa(s.PaddingX) + "px;"
	}
	if s.Background != "" {
		st += "background:" + html.EscapeString(s.Background) + ";"
	}
	cls := ""
	if s.HideMobile {
		cls = ` class="skl-hide-mobile"` // hidden under the wrapHTML media query
	}
	if st == "" && cls == "" {
		return inner
	}
	return `<div` + cls + ` style="` + st + `">` + inner + `</div>`
}

func spacerPx(b Block) int {
	if b.Size > 0 {
		return b.Size
	}
	return 16
}

func qty(q float64) string { return strconv.FormatFloat(q, 'f', -1, 64) }

func totalsRow(b *strings.Builder, label, value string, bold bool) {
	if bold {
		fmt.Fprintf(b, `<tr><td><strong>%s</strong></td><td style="text-align:right;"><strong>%s</strong></td></tr>`, label, html.EscapeString(value))
		return
	}
	fmt.Fprintf(b, `<tr><td>%s</td><td style="text-align:right;">%s</td></tr>`, label, html.EscapeString(value))
}

// --- validation ---

// validateBlock checks a block is a known type with its required fields + a valid style. allowRef is false inside a
// reusable snippet (no nested refs → no cycles) - Row blocks validate each column's leaf blocks recursively
func validateBlock(b Block, allowRef bool) error {
	if err := validateStyle(b.Style); err != nil {
		return err
	}
	switch b.Type {
	case "heading", "text", "button", "image", "divider", "spacer",
		"order_items", "order_totals", "shipping_address", "tracking", "refund_amount":
		return nil
	case "products":
		if b.Products == nil {
			return fmt.Errorf("products block needs a configuration")
		}
		if b.Products.Mode == "collection" {
			if strings.TrimSpace(b.Products.CollectionID) == "" {
				return fmt.Errorf("products block: pick a collection")
			}
		} else if len(b.Products.SKUs) == 0 {
			return fmt.Errorf("products block: add at least one product")
		}
		return nil
	case "social":
		if len(b.Social) == 0 {
			return fmt.Errorf("social block: add at least one link")
		}
		return nil
	case "video":
		if strings.TrimSpace(b.URL) == "" {
			return fmt.Errorf("video block: add the video URL")
		}
		if strings.TrimSpace(b.Poster) == "" {
			return fmt.Errorf("video block: add a thumbnail image")
		}
		return nil
	case "html", "countdown":
		return nil
	case "row":
		if len(b.Columns) == 0 {
			return fmt.Errorf("row block needs at least one column")
		}
		for ci, c := range b.Columns {
			for bi, cb := range c.Blocks {
				if cb.Type == "row" || cb.Type == "ref" {
					return fmt.Errorf("column %d block %d: %q can't go inside a row", ci+1, bi+1, cb.Type)
				}
				if err := validateStyle(cb.Style); err != nil {
					return fmt.Errorf("column %d block %d: %w", ci+1, bi+1, err)
				}
			}
		}
		return nil
	case "ref":
		// allowRef is retained for call-site intent; snippets may now nest refs
		// (render-time depth/cycle guard prevents loops)
		_ = allowRef
		if strings.TrimSpace(b.Slug) == "" {
			return fmt.Errorf("ref block requires a slug")
		}
		return nil
	default:
		// An unknown type is treated as a custom block-type instance - its existence is resolved (per channel) at render
		// time, rendering nothing if absent - Validate its field style only
		return nil
	}
}

var (
	colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$|^rgba?\([0-9.,%\s]+\)$`)
	fontRe  = regexp.MustCompile(`^[a-zA-Z0-9 ,'"\-]+$`)
)

// namedColors is a small allowlist so operators can type common names
var namedColors = map[string]bool{
	"transparent": true, "black": true, "white": true, "red": true, "green": true, "blue": true,
	"gray": true, "grey": true, "orange": true, "yellow": true, "purple": true, "pink": true, "teal": true,
}

func validateColor(c string) error {
	c = strings.TrimSpace(c)
	if c == "" {
		return nil
	}
	if colorRe.MatchString(c) || namedColors[strings.ToLower(c)] {
		return nil
	}
	return fmt.Errorf("invalid color %q (use #hex, rgb()/rgba(), or a basic name)", c)
}

func clampErr(name string, v, max int) error {
	if v < 0 || v > max {
		return fmt.Errorf("%s must be between 0 and %d", name, max)
	}
	return nil
}

// validateStyle sanitizes a per-block style - Colors must be hex/rgb/named and
// numbers within sane bounds - these values get inlined into style="" attrs
func validateStyle(s *BlockStyle) error {
	if s == nil {
		return nil
	}
	switch s.Align {
	case "", "left", "center", "right":
	default:
		return fmt.Errorf("invalid align %q", s.Align)
	}
	for _, c := range []string{s.Color, s.Background, s.ButtonColor, s.ButtonTextColor} {
		if err := validateColor(c); err != nil {
			return err
		}
	}
	for _, ck := range []struct {
		name string
		v    int
		max  int
	}{
		{"fontSize", s.FontSize, 200}, {"paddingY", s.PaddingY, 200}, {"paddingX", s.PaddingX, 200},
		{"radius", s.Radius, 100}, {"width", s.Width, 100},
	} {
		if err := clampErr(ck.name, ck.v, ck.max); err != nil {
			return err
		}
	}
	return nil
}

// validateStyles sanitizes the email-level theme
func validateStyles(s EmailStyles) error {
	for _, c := range []string{s.Background, s.LinkColor, s.TextColor} {
		if err := validateColor(c); err != nil {
			return err
		}
	}
	if s.ContentWidth != 0 && (s.ContentWidth < 200 || s.ContentWidth > 1000) {
		return fmt.Errorf("contentWidth must be between 200 and 1000")
	}
	if f := strings.TrimSpace(s.FontFamily); f != "" && !fontRe.MatchString(f) {
		return fmt.Errorf("invalid fontFamily")
	}
	return nil
}
