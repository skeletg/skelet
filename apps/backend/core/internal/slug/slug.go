// Package slug holds the one set of slug rules the domains share, so parallel copies can't drift
package slug

import "fmt"

const MaxLength = 200

// Enforces URL-slug rules - non-empty, bounded, only lowercase ASCII letters, digits and the separators '-', '_' and '.', and never the "." or ".." path segments
func Validate(s string, invalid error) error {
	return validate(s, invalid, true)
}

// Validate without '.', for slugs that name a record rather than a URL path segment
func ValidateIdentifier(s string, invalid error) error {
	return validate(s, invalid, false)
}

func validate(s string, invalid error, allowDot bool) error {
	if s == "" {
		return fmt.Errorf("%w: empty", invalid)
	}
	if len(s) > MaxLength {
		return fmt.Errorf("%w: longer than %d characters", invalid, MaxLength)
	}
	if s == "." || s == ".." {
		return fmt.Errorf("%w: %q is a relative path segment", invalid, s)
	}
	for _, r := range s {
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		isSep := r == '-' || r == '_' || (allowDot && r == '.')
		if !(isLower || isDigit || isSep) {
			return fmt.Errorf("%w: %q contains %q", invalid, s, r)
		}
	}
	return nil
}
