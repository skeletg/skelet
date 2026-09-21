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
	}{
		{"too short", "abc123", false},
		{"exactly min", "abcd1234", true},
		{"comfortable", "correct horse battery staple", true},
		{"empty", "", false},
		{"seven chars", "1234567", false},
		{"too long (>72 bytes)", strings.Repeat("a", 73), false},
		{"72 bytes ok", strings.Repeat("a", 72), true},
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
				} else if !errors.Is(err, ErrWeakPassword) {
					t.Errorf("want ErrWeakPassword, got %v", err)
				}
			}
		})
	}
}
