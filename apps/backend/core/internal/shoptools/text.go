package shoptools

import (
	"encoding/json"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

const quoteChars = 600

func price(minor int64, code string) string { return money.FormatCode(minor, code) }

// The front API takes prices as a 32-bit Int, so anything past it means "no bound"
func toMinor(major float64, code string) int64 {
	return min(int64(math.Round(major*math.Pow10(money.ISODecimals(code)))), math.MaxInt32)
}

// Shopper-written text goes back quoted, so an agent reads it as data
func quote(s string) string { return strconv.Quote(clip(s, quoteChars)) }

func clip(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

func clamp(n, def, max int) int {
	if n <= 0 {
		return def
	}
	return min(n, max)
}

// Keys whose string values are prose in rich-text and block JSON - everything else is ids, styles and settings
var proseKeys = map[string]bool{"text": true, "title": true, "heading": true, "body": true, "content": true}

// The readable text of a rich-text or block JSON value, in a stable order - its prose leaves, or the value itself when it's a plain string
func plainText(raw json.RawMessage, max int) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return clip(s, max)
	}
	var parts []string
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for _, k := range slices.Sorted(maps.Keys(t)) {
				if s, ok := t[k].(string); ok {
					if proseKeys[k] {
						parts = append(parts, s)
					}
					continue
				}
				walk(t[k])
			}
		}
	}
	walk(v)
	return clip(strings.Join(parts, " "), max)
}
