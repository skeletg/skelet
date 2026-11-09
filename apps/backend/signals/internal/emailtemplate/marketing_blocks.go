package emailtemplate

import (
	"fmt"
	"html"
	"strings"
)

// A marketing email's building blocks past words and pictures: discount code, footer, menu links, image beside text

type MenuLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

const maxMenuLinks = 12

// The button block's link, for the blocks that end on one - empty without both a label and an address
func ctaLink(b Block, rc renderCtx) string {
	if strings.TrimSpace(b.Label) == "" || strings.TrimSpace(b.URL) == "" {
		return ""
	}
	s := styleOf(b)
	return `<a href="` + esc(b.URL, rc.Vars) + `" style="display:inline-block;background:` + col(s.ButtonColor, rc.Theme.LinkColor, "#222") +
		`;color:` + col(s.ButtonTextColor, "#fff") + `;text-decoration:none;padding:10px 18px;border-radius:` + pxOr(s.Radius, 4) + `;">` +
		esc(b.Label, rc.Vars) + `</a>`
}

func ctaText(b Block, rc renderCtx) string {
	if strings.TrimSpace(b.Label) == "" || strings.TrimSpace(b.URL) == "" {
		return ""
	}
	return interpolate(b.Label, rc.Vars) + ": " + interpolate(b.URL, rc.Vars) + "\n"
}

func centeredOr(s BlockStyle) string {
	if s.Align == "" {
		return "center"
	}
	return alignOf(s)
}

func couponHTML(b Block, rc renderCtx) string {
	code := strings.TrimSpace(interpolate(b.Code, rc.Vars))
	if code == "" {
		return ""
	}
	s := styleOf(b)
	var sb strings.Builder
	fmt.Fprintf(&sb, `<div style="margin:16px 0;padding:20px;border:2px dashed %s;border-radius:8px;text-align:%s;color:%s;">`,
		col(s.ButtonColor, rc.Theme.LinkColor, "#222"), centeredOr(s), col(s.Color, rc.Theme.TextColor, "#222"))
	if t := strings.TrimSpace(b.Text); t != "" {
		sb.WriteString(`<div style="font-size:16px;font-weight:700;margin:0 0 10px;">` + esc(t, rc.Vars) + `</div>`)
	}
	sb.WriteString(`<div style="display:inline-block;font-family:'Courier New',Courier,monospace;font-size:24px;font-weight:700;letter-spacing:3px;padding:8px 16px;background:#f4f4f5;border-radius:4px;">` +
		html.EscapeString(code) + `</div>`)
	if n := strings.TrimSpace(b.Content); n != "" {
		sb.WriteString(`<div style="font-size:13px;color:#666;margin-top:10px;">` + esc(n, rc.Vars) + `</div>`)
	}
	if a := ctaLink(b, rc); a != "" {
		sb.WriteString(`<div style="margin-top:14px;">` + a + `</div>`)
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

func couponText(b Block, rc renderCtx) string {
	code := strings.TrimSpace(interpolate(b.Code, rc.Vars))
	if code == "" {
		return ""
	}
	var sb strings.Builder
	if t := strings.TrimSpace(b.Text); t != "" {
		sb.WriteString(interpolate(t, rc.Vars) + "\n")
	}
	sb.WriteString("Code: " + code + "\n")
	if n := strings.TrimSpace(b.Content); n != "" {
		sb.WriteString(interpolate(n, rc.Vars) + "\n")
	}
	sb.WriteString(ctaText(b, rc))
	return sb.String()
}

// The unsubscribe link comes from the send, so a footer never forgets it - a send without one leaves it out
func footerHTML(b Block, rc renderCtx) string {
	s := styleOf(b)
	c := col(s.Color, "#888")
	var sb strings.Builder
	fmt.Fprintf(&sb, `<div style="margin:24px 0 0;padding-top:16px;border-top:1px solid #eee;text-align:%s;font-size:12px;line-height:1.6;color:%s;">`, centeredOr(s), c)
	if t := strings.TrimSpace(b.Content); t != "" {
		sb.WriteString(strings.ReplaceAll(esc(t, rc.Vars), "\n", "<br>"))
	}
	if u := strings.TrimSpace(rc.Vars["unsubscribe.link"]); u != "" {
		fmt.Fprintf(&sb, `<div style="margin-top:8px;"><a href="%s" style="color:%s;text-decoration:underline;">%s</a></div>`,
			html.EscapeString(u), c, esc(footerUnsubscribe(b), rc.Vars))
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

func footerText(b Block, rc renderCtx) string {
	var sb strings.Builder
	if t := strings.TrimSpace(b.Content); t != "" {
		sb.WriteString(interpolate(t, rc.Vars) + "\n")
	}
	if u := strings.TrimSpace(rc.Vars["unsubscribe.link"]); u != "" {
		sb.WriteString(interpolate(footerUnsubscribe(b), rc.Vars) + ": " + u + "\n")
	}
	return sb.String()
}

func footerUnsubscribe(b Block) string {
	if l := strings.TrimSpace(b.Label); l != "" {
		return l
	}
	return "Unsubscribe"
}

func menuHTML(b Block, rc renderCtx) string {
	s := styleOf(b)
	c := col(s.Color, rc.Theme.LinkColor, "#222")
	var links []string
	for _, l := range b.Links {
		u := strings.TrimSpace(interpolate(l.URL, rc.Vars))
		if u == "" || strings.TrimSpace(l.Label) == "" {
			continue
		}
		links = append(links, `<a href="`+html.EscapeString(u)+`" style="color:`+c+`;text-decoration:none;font-weight:600;font-size:14px;">`+esc(l.Label, rc.Vars)+`</a>`)
	}
	if len(links) == 0 {
		return ""
	}
	return `<div style="margin:12px 0;text-align:` + centeredOr(s) + `;">` + strings.Join(links, `<span style="color:#ccc;padding:0 10px;">|</span>`) + `</div>`
}

func menuText(b Block, rc renderCtx) string {
	var parts []string
	for _, l := range b.Links {
		u := strings.TrimSpace(interpolate(l.URL, rc.Vars))
		if u != "" && strings.TrimSpace(l.Label) != "" {
			parts = append(parts, interpolate(l.Label, rc.Vars)+": "+u)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n") + "\n"
}

// A two-cell table every client lays side by side - the skl-stack rule in the head stacks the cells on a phone
func imageTextHTML(b Block, rc renderCtx) string {
	s := styleOf(b)
	src := strings.TrimSpace(interpolate(b.Image, rc.Vars))
	right := b.Side == "right"
	pad := "padding:0 16px 0 0;"
	if right {
		pad = "padding:0 0 0 16px;"
	}
	img := `<td class="skl-stack" width="45%" valign="top" style="` + pad + `">`
	if src != "" {
		img += `<img src="` + html.EscapeString(src) + `" alt="` + esc(b.Alt, rc.Vars) + `" width="100%" style="display:block;width:100%;max-width:100%;height:auto;border-radius:4px;">`
	}
	img += `</td>`
	var inner strings.Builder
	if t := strings.TrimSpace(b.Text); t != "" {
		inner.WriteString(`<h3 style="margin:0 0 8px;font-size:18px;">` + esc(t, rc.Vars) + `</h3>`)
	}
	if c := strings.TrimSpace(b.Content); c != "" {
		inner.WriteString(`<p style="margin:0 0 12px;line-height:1.5;">` + strings.ReplaceAll(esc(c, rc.Vars), "\n", "<br>") + `</p>`)
	}
	inner.WriteString(ctaLink(b, rc))
	look := `text-align:` + alignOf(s) + `;color:` + col(s.Color, rc.Theme.TextColor, "#333") + `;`
	if src == "" {
		return `<div style="margin:12px 0;` + look + `">` + inner.String() + `</div>`
	}
	words := `<td class="skl-stack" valign="middle" style="` + look + `">` + inner.String() + `</td>`
	cells := img + words
	if right {
		cells = words + img
	}
	return `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:12px 0;"><tr>` + cells + `</tr></table>`
}

func imageTextText(b Block, rc renderCtx) string {
	var sb strings.Builder
	if t := strings.TrimSpace(b.Text); t != "" {
		sb.WriteString(interpolate(t, rc.Vars) + "\n")
	}
	if c := strings.TrimSpace(b.Content); c != "" {
		sb.WriteString(interpolate(c, rc.Vars) + "\n")
	}
	sb.WriteString(ctaText(b, rc))
	return sb.String()
}

func validateMarketingBlock(b Block) error {
	switch b.Type {
	case "coupon":
		if strings.TrimSpace(b.Code) == "" {
			return fmt.Errorf("discount code block: add the code")
		}
		return validateLink(b.URL)
	case "store_footer":
		return nil
	case "menu":
		if len(b.Links) == 0 || len(b.Links) > maxMenuLinks {
			return fmt.Errorf("menu block: between 1 and %d links", maxMenuLinks)
		}
		for _, l := range b.Links {
			if strings.TrimSpace(l.Label) == "" || strings.TrimSpace(l.URL) == "" {
				return fmt.Errorf("menu block: every link needs a label and an address")
			}
			if err := validateLink(l.URL); err != nil {
				return fmt.Errorf("menu block: %w", err)
			}
		}
		return nil
	case "image_text":
		if b.Side != "" && b.Side != "left" && b.Side != "right" {
			return fmt.Errorf("image with text block: the image goes left or right")
		}
		if err := validateLink(b.Image); err != nil {
			return err
		}
		return validateLink(b.URL)
	}
	return nil
}
