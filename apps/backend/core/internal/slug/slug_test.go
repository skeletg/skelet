package slug

import (
	"errors"
	"strings"
	"testing"
)

func check(t *testing.T, name string, fn func(string, error) error, ok, bad []string) {
	t.Helper()
	sentinel := errors.New("invalid slug")
	for _, s := range ok {
		if err := fn(s, sentinel); err != nil {
			t.Errorf("%s(%q) = %v, want nil", name, s, err)
		}
	}
	for _, s := range bad {
		err := fn(s, sentinel)
		if err == nil {
			t.Errorf("%s(%q) = nil, want error", name, s)
			continue
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("%s(%q) error does not wrap the caller's sentinel", name, s)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := []string{"hello", "hello-world", "a_b.c", "p123", "2024-summer-sale", "...", strings.Repeat("a", MaxLength)}
	bad := []string{"", "Hello", "white space", "café", "a/b", "UPPER", ".", "..", strings.Repeat("a", MaxLength+1)}
	check(t, "Validate", Validate, ok, bad)
}

func TestValidateIdentifier(t *testing.T) {
	ok := []string{"hello", "hello-world", "a_b", "p123", "-", strings.Repeat("a", MaxLength)}
	bad := []string{"", "Hello", " main ", "café", "a/b", "a.b", ".", "..", "...", "v1.2", strings.Repeat("a", MaxLength+1)}
	check(t, "ValidateIdentifier", ValidateIdentifier, ok, bad)
}
