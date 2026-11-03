package shoptools

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

func price(minor int64, code string) string { return money.FormatCode(minor, code) }

func toMinor(major float64, code string) int64 {
	return int64(math.Round(major * math.Pow10(money.ISODecimals(code))))
}

// Shopper-written text goes back quoted, so an agent reads it as data
func quote(s string) string { return strconv.Quote(clip(s, 600)) }

func clip(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

func clamp(n, def, max int) int {
	if n <= 0 {
		return def
	}
	return min(n, max)
}

// The readable text of a rich-text or block JSON value - its "text" leaves, or the value itself when it's a plain string
func plainText(raw json.RawMessage, max int) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	var parts []string
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			parts = append(parts, t)
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for k, e := range t {
				if s, ok := e.(string); ok {
					if k == "text" || k == "title" || k == "heading" || k == "body" || k == "content" {
						parts = append(parts, s)
					}
					continue
				}
				walk(e)
			}
		}
	}
	if s, ok := v.(string); ok {
		return clip(s, max)
	}
	walk(v)
	return clip(strings.Join(parts, " "), max)
}
