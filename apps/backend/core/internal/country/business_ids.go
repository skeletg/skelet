package country

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// BusinessIDField is one company identifier a business purchase from the country asks for - Serbia asks for a PIB and
// a Matični broj, most of the EU for a VAT number. Edited per country in the dashboard; checkout validates against it
type BusinessIDField struct {
	Key      string `json:"key"`   // stable slug the values are stored under ("pib")
	Label    string `json:"label"` // what the shopper and the invoice read ("PIB")
	Required bool   `json:"required"`
	Pattern  string `json:"pattern,omitempty"` // an RE2 regular expression the whole value must match, empty for any
	Hint     string `json:"hint,omitempty"`    // shown under the field ("9 digits")
}

var (
	ErrBusinessIDFieldInvalid = errors.New("invalid company ID field")

	businessIDKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
)

// NormalizeBusinessIDFields trims each field, derives a missing key from the label, and refuses a list checkout
// couldn't validate against - duplicate keys, a field with no label, or a pattern that doesn't compile
func NormalizeBusinessIDFields(fields []BusinessIDField) ([]BusinessIDField, error) {
	out := make([]BusinessIDField, 0, len(fields))
	seen := map[string]bool{}
	for i, f := range fields {
		f.Label = strings.TrimSpace(f.Label)
		f.Pattern = strings.TrimSpace(f.Pattern)
		f.Hint = strings.TrimSpace(f.Hint)
		f.Key = strings.TrimSpace(f.Key)
		if f.Label == "" {
			return nil, fmt.Errorf("%w: field %d needs a label", ErrBusinessIDFieldInvalid, i+1)
		}
		if f.Key == "" {
			f.Key = slugKey(f.Label)
		}
		if !businessIDKeyRe.MatchString(f.Key) {
			return nil, fmt.Errorf("%w: key %q must be lowercase letters, digits and underscores", ErrBusinessIDFieldInvalid, f.Key)
		}
		if seen[f.Key] {
			return nil, fmt.Errorf("%w: key %q is used twice", ErrBusinessIDFieldInvalid, f.Key)
		}
		seen[f.Key] = true
		if f.Pattern != "" {
			if _, err := regexp.Compile(anchored(f.Pattern)); err != nil {
				return nil, fmt.Errorf("%w: pattern for %q doesn't compile: %w", ErrBusinessIDFieldInvalid, f.Label, err)
			}
		}
		out = append(out, f)
	}
	return out, nil
}

// Matches reports whether a value satisfies the field's pattern - the whole value, so "^\d{9}$" and "\d{9}" mean the same
func (f BusinessIDField) Matches(value string) bool {
	if f.Pattern == "" {
		return true
	}
	re, err := regexp.Compile(anchored(f.Pattern))
	return err == nil && re.MatchString(value)
}

func anchored(p string) string {
	return "^(?:" + strings.TrimSuffix(strings.TrimPrefix(p, "^"), "$") + ")$"
}

// "Matični broj" -> "maticni_broj"
func slugKey(label string) string {
	fold := strings.NewReplacer("č", "c", "ć", "c", "š", "s", "ž", "z", "đ", "dj", "ä", "a", "ö", "o", "ü", "u", "ß", "ss", "é", "e", "è", "e", "á", "a", "í", "i", "ó", "o", "ú", "u", "ñ", "n")
	s := fold.Replace(strings.ToLower(label))
	var b strings.Builder
	under := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			under = false
		case !under && b.Len() > 0:
			b.WriteByte('_')
			under = true
		}
	}
	key := strings.TrimSuffix(b.String(), "_")
	if key == "" || key[0] < 'a' || key[0] > 'z' {
		key = "id_" + key
	}
	if len(key) > 40 {
		key = key[:40]
	}
	return key
}
