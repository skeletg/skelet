package emailtemplate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// FormatRich marks a text block holding the editor's HTML subset - one without it is the older plain kind
const FormatRich = "rich"

const maxRichContent = 64 << 10

// Stored markup is never trusted - every render cuts it back to this
var richPolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "strong", "b", "em", "i", "u", "s", "ul", "ol", "li")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireParseableURLs(true)
	return p
}()

var blankLines = regexp.MustCompile(`\n{3,}`)

// Email clients ignore stylesheets, so the subset's spacing and the link colour go inline
func richHTML(content string, vars map[string]string, linkColor string) string {
	clean := richPolicy.Sanitize(interpolateEscaped(content, vars))
	link := "<a "
	if linkColor != "" {
		link = `<a style="color:` + linkColor + `;" `
	}
	return strings.NewReplacer(
		"<p>", `<p style="margin:0 0 12px;">`,
		"<ul>", `<ul style="margin:0 0 12px;padding-left:20px;">`,
		"<ol>", `<ol style="margin:0 0 12px;padding-left:20px;">`,
		"<a ", link,
	).Replace(clean)
}

// Merge values are filled in as the text is read, so a value can't become markup
func richPlain(content string, vars map[string]string) string {
	var b strings.Builder
	var hrefs []string
	z := html.NewTokenizer(strings.NewReader(content))
	for {
		switch z.Next() {
		case html.ErrorToken:
			out := blankLines.ReplaceAllString(b.String(), "\n\n")
			return strings.TrimSpace(out) + "\n"
		case html.TextToken:
			b.WriteString(interpolate(string(z.Text()), vars))
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "br":
				b.WriteString("\n")
			case "li":
				b.WriteString("\n- ")
			case "a":
				href := ""
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					if string(k) == "href" {
						href = interpolate(string(v), vars)
					}
				}
				hrefs = append(hrefs, href)
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "p", "ul", "ol":
				b.WriteString("\n\n")
			case "a":
				if n := len(hrefs); n > 0 {
					if href := hrefs[n-1]; href != "" {
						fmt.Fprintf(&b, " (%s)", href)
					}
					hrefs = hrefs[:n-1]
				}
			}
		}
	}
}

func validateRichText(b Block) error {
	switch b.Format {
	case "":
		return nil
	case FormatRich:
		if b.Type != "text" {
			return fmt.Errorf("only a text block can hold rich text")
		}
		if len(b.Content) > maxRichContent {
			return fmt.Errorf("a text block holds up to %d KB", maxRichContent>>10)
		}
		return nil
	}
	return fmt.Errorf("unknown text format %q", b.Format)
}
