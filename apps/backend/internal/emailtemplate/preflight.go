package emailtemplate

import (
	"fmt"
	"regexp"
	"strings"
)

// One pre-send checklist finding - Severity is "warn" for should-fix or "info" for nice-to-fix
type PreflightWarning struct {
	Severity string
	Message  string
}

var spamWordRe = regexp.MustCompile(`(?i)\b(free|act now|limited time|buy now|click here|100% free|risk[- ]?free|guarantee|winner|congratulations|cash|cheap|order now|urgent)\b`)

// The pre-send checklist over an email's content - the deliverability and quality problems worth catching before a send: a missing subject or preheader, images without alt text, buttons with no link, spam-trigger words, and a heavy image-to-text ratio
// Pure and fast, so it's callable on every editor keystroke, and it recurses into rows
func Preflight(subject, preheader string, blocks []Block) []PreflightWarning {
	var w []PreflightWarning
	if strings.TrimSpace(subject) == "" {
		w = append(w, PreflightWarning{"warn", "No subject line."})
	}
	if strings.TrimSpace(preheader) == "" {
		w = append(w, PreflightWarning{"info", "No preheader - add the inbox-preview text that shows after the subject."})
	}
	if spamWordRe.MatchString(subject) {
		w = append(w, PreflightWarning{"warn", "Subject contains spam-trigger words that can hurt deliverability."})
	}

	var images, textish int
	var walk func(bs []Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			switch b.Type {
			case "image":
				images++
				if strings.TrimSpace(b.Alt) == "" {
					w = append(w, PreflightWarning{"warn", "An image is missing alt text (bad for accessibility + spam scoring)."})
				}
				if strings.TrimSpace(b.URL) == "" {
					w = append(w, PreflightWarning{"warn", "An image block has no source URL."})
				}
			case "button":
				if strings.TrimSpace(b.URL) == "" {
					w = append(w, PreflightWarning{"warn", fmt.Sprintf("The %q button has no link.", firstNonEmpty(b.Label, "call-to-action"))})
				}
			case "heading", "text":
				if strings.TrimSpace(b.Text)+strings.TrimSpace(b.Content) != "" {
					textish++
				}
			case "row":
				for _, c := range b.Columns {
					walk(c.Blocks)
				}
			}
		}
	}
	walk(blocks)

	if images > 0 && textish == 0 {
		w = append(w, PreflightWarning{"warn", "This email is all images and no text - image-only emails often land in spam and break with images off."})
	}
	return w
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
