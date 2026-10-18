package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePasswordStrength(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		ok   bool
		want error
	}{
		{"too short", "abc123", false, ErrWeakPassword},
		{"exactly min", "abcd1234", true, nil},
		{"comfortable", "correct horse battery staple", true, nil},
		{"empty", "", false, ErrWeakPassword},
		{"seven chars", "1234567", false, ErrWeakPassword},
		{"too long (>72 bytes)", strings.Repeat("a", 73), false, ErrPasswordTooLong},
		{"72 bytes ok", strings.Repeat("a", 72), true, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidatePasswordStrength(c.pw)
			if c.ok && err != nil {
				t.Errorf("want ok, got %v", err)
			}
			if !c.ok {
				if err == nil {
					t.Errorf("want error, got nil")
				} else if !errors.Is(err, c.want) {
					t.Errorf("want %v, got %v", c.want, err)
				}
			}
		})
	}
}
