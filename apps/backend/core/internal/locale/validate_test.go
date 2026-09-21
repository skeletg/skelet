package locale

import (
	"errors"
	"testing"
)

func TestNormalizeCode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"en", "en"},
		{"EN", "en"},
		{"en-US", "en-us"},     // lowercases the whole code (no canonical casing)
		{"EN-us", "en-us"},     // mixed case → lowered
		{"  en-GB  ", "en-gb"}, // trims surrounding whitespace
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := normalizeCode(tt.in); got != tt.want {
				t.Errorf("normalizeCode(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateCode(t *testing.T) {
	valid := []string{
		"en",
		"de",
		"eng",        // 3-letter language
		"en-us",      // language + region (must be pre-normalized to lowercase)
		"zh-hans",    // language + script
		"sr-latn-rs", // language + script + region
		"en-001",     // numeric region subtag
	}
	for _, code := range valid {
		t.Run("valid/"+code, func(t *testing.T) {
			if err := validateCode(code); err != nil {
				t.Errorf("validateCode(%q) rejected a valid code: %v", code, err)
			}
		})
	}

	invalid := []string{
		"",                 // empty
		"e",                // 1-letter language
		"engl",             // 4-letter language
		"en_US",            // underscore separator
		"en-US",            // uppercase not normalized (validateCode is called post-normalize)
		"123",              // leading digits not allowed for the language subtag
		"en-",              // trailing hyphen / empty subtag
		"en-toolongsubtag", // subtag > 8 chars
	}
	for _, code := range invalid {
		t.Run("invalid/"+code, func(t *testing.T) {
			err := validateCode(code)
			if err == nil {
				t.Errorf("validateCode(%q) accepted an invalid code", code)
				return
			}
			if !errors.Is(err, ErrInvalidCode) {
				t.Errorf("validateCode(%q) error = %v, want wrapped ErrInvalidCode", code, err)
			}
		})
	}
}

func TestIsTwoLetterCountry(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"US", true},
		{"GB", true},
		{"RS", true},
		{"us", false},  // lowercase rejected (caller upcases first)
		{"Us", false},  // mixed case
		{"USA", false}, // 3 letters
		{"U", false},   // 1 letter
		{"", false},
		{"U1", false}, // digit
		{"U-", false}, // punctuation
		{"  ", false}, // spaces
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := isTwoLetterCountry(tt.in); got != tt.want {
				t.Errorf("isTwoLetterCountry(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateOptionalFields(t *testing.T) {
	tests := []struct {
		name        string
		firstDay    int16
		countryCode string
		hreflang    string
		wantErr     bool
	}{
		{"all empty/zero ok", 0, "", "", false},
		{"valid combo", 6, "US", "en-us", false},
		{"firstDay negative", -1, "", "", true},
		{"firstDay too high", 7, "", "", true},
		{"firstDay boundary 6 ok", 6, "", "", false},
		{"country lowercased input ok (upcased internally)", 1, "us", "", false},
		{"country bad length", 1, "USA", "", true},
		{"hreflang bad shape", 1, "", "en_US", true},
		{"hreflang uppercase ok (lowered internally)", 1, "", "EN-US", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOptionalFields(tt.firstDay, tt.countryCode, tt.hreflang)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOptionalFields(%d, %q, %q) err = %v, wantErr %v",
					tt.firstDay, tt.countryCode, tt.hreflang, err, tt.wantErr)
			}
		})
	}
}
