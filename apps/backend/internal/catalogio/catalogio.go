// Package catalogio is the products entry in the data registry - pure orchestration owning no storage, reading and writing through catalog.Service and channel.Service
// One flat schema round-trips both directions: one product per row, with configurable variant children as their own rows linked by parent_sku - import is upsert-by-SKU
package catalogio

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/catalog"
)

// The same algorithm the catalog service uses for SKU fallbacks, so generated slugs pass its validateSlug
func slugify(name string) string {
	var b strings.Builder
	prev := '-'
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prev = r
		case prev != '-':
			b.WriteRune('-')
			prev = '-'
		}
	}
	return strings.Trim(b.String(), "-")
}

func splitMulti(s string) []string {
	parts := strings.Split(s, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// The catalog service wants a CONCRETE typed value for a scalar attribute - a *string is rejected outright and the wrong type is silently dropped
func coerceScalar(it catalog.InputType, cell string) (any, error) {
	switch it {
	case catalog.InputTypeNumeric:
		f, err := strconv.ParseFloat(cell, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q", cell)
		}
		return f, nil
	case catalog.InputTypeBoolean:
		switch strings.ToLower(strings.TrimSpace(cell)) {
		case "1", "true", "yes", "y", "t":
			return true, nil
		case "0", "false", "no", "n", "f":
			return false, nil
		}
		return nil, fmt.Errorf("invalid boolean %q", cell)
	default: // plain_text, date, date_time - the service validates date formats
		return cell, nil
	}
}

// Order-independent, so a re-import skips a no-op SetConfigurableOptions the service would reject once children are linked
func sameOptionSet(current []catalog.Attribute, desired []uuid.UUID) bool {
	if len(current) != len(desired) {
		return false
	}
	have := make(map[uuid.UUID]bool, len(current))
	for _, a := range current {
		have[a.ID] = true
	}
	for _, id := range desired {
		if !have[id] {
			return false
		}
	}
	return true
}

func stringifyScalar(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}
