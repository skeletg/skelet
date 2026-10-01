package phone_test

import (
	"errors"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/phone"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		name, raw, region, want string
		wantErr                 bool
	}{
		{"blank is empty, not an error", "   ", "RS", "", false},
		{"serbian mobile typed nationally", "064 123 4567", "RS", "+381641234567", false},
		{"serbian mobile with punctuation", "(064) 123-45-67", "rs", "+381641234567", false},
		{"international prefix 00 under a region", "00381 64 123 4567", "RS", "+381641234567", false},
		{"us number without a +", "(201) 555-0123", "US", "+12015550123", false},
		{"international input ignores the region", "+49 1512 3456789", "RS", "+4915123456789", false},
		{"international input needs no region", "+381641234567", "", "+381641234567", false},
		{"national input needs a region", "064 123 4567", "", "", true},
		{"national number valid in one country only", "064 123 4567", "US", "", true},
		{"too short", "+381 64", "RS", "", true},
		{"garbage", "call me maybe", "RS", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := phone.Normalize(tc.raw, tc.region)
			if tc.wantErr {
				if !errors.Is(err, phone.ErrInvalid) {
					t.Fatalf("Normalize(%q, %q) err = %v, want ErrInvalid", tc.raw, tc.region, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("Normalize(%q, %q) = %q, %v, want %q", tc.raw, tc.region, got, err, tc.want)
			}
		})
	}
}

func TestNormalizePtr(t *testing.T) {
	if got, err := phone.NormalizePtr(nil, "RS"); got != nil || err != nil {
		t.Fatalf("nil = %v, %v, want nil, nil", got, err)
	}
	blank := ""
	if got, err := phone.NormalizePtr(&blank, "RS"); err != nil || got == nil || *got != "" {
		t.Fatalf("blank = %v, %v, want pointer to empty", got, err)
	}
	raw := "064 123 4567"
	if got, err := phone.NormalizePtr(&raw, "RS"); err != nil || got == nil || *got != "+381641234567" {
		t.Fatalf("national = %v, %v, want +381641234567", got, err)
	}
}

// The presenter maps the sentinel to a stable client-visible code - the storefront and dashboard branch on it
func TestErrInvalidIsAValidationError(t *testing.T) {
	if got := apperr.CodeOf(phone.ErrInvalid); got != "PHONE_INVALID" {
		t.Errorf("CodeOf = %q, want PHONE_INVALID", got)
	}
	if got := apperr.ClassOf(phone.ErrInvalid); got != apperr.ClassValidation {
		t.Errorf("ClassOf = %v, want ClassValidation", got)
	}
	if got := apperr.CodeOf(phone.ErrCountryNotAllowed); got != "PHONE_COUNTRY_NOT_ALLOWED" {
		t.Errorf("CodeOf = %q, want PHONE_COUNTRY_NOT_ALLOWED", got)
	}
}

func TestRulesReadANationalNumberInTheDefaultCountry(t *testing.T) {
	r := phone.Rules{Default: "RS"}
	if got, err := r.Normalize("064 123 4567", ""); err != nil || got != "+381641234567" {
		t.Fatalf("no region = %q, %v, want the default country's reading", got, err)
	}
	if got, err := r.Normalize("(201) 555-0123", "US"); err != nil || got != "+12015550123" {
		t.Fatalf("a region beats the default: %q, %v", got, err)
	}
}

func TestRulesKeepANumberToTheAllowedCountries(t *testing.T) {
	r := phone.Rules{Default: "US", Allowed: []string{"US", "DE"}}
	if _, err := r.Normalize("+49 1512 3456789", ""); err != nil {
		t.Errorf("an allowed country's number = %v", err)
	}
	if _, err := r.Normalize("+381 64 123 4567", ""); !errors.Is(err, phone.ErrCountryNotAllowed) {
		t.Errorf("a number from elsewhere = %v, want ErrCountryNotAllowed", err)
	}
	// +1 is shared - the country is the number's own, not its calling code's
	if _, err := r.Normalize("+1 604 555 0123", ""); !errors.Is(err, phone.ErrCountryNotAllowed) {
		t.Errorf("a Canadian number on a US+DE store = %v, want ErrCountryNotAllowed", err)
	}
	if got, err := r.Normalize("  ", ""); err != nil || got != "" {
		t.Errorf("blank = %q, %v, want it left optional", got, err)
	}
	if got, err := r.NormalizePtr(nil, ""); got != nil || err != nil {
		t.Errorf("nil = %v, %v", got, err)
	}
}

func TestFormatFor(t *testing.T) {
	f, ok := phone.FormatFor("rs")
	want := phone.CountryFormat{Country: "RS", CallingCode: "+381", Example: "+381 60 1234567", NationalExample: "060 1234567", Template: "xxx xxxxxxx"}
	if !ok || f != want {
		t.Fatalf("FormatFor(rs) = %+v, %v, want %+v", f, ok, want)
	}
	if f, _ := phone.FormatFor("US"); f.Template != "(xxx) xxx-xxxx" {
		t.Errorf("US template = %q", f.Template)
	}
	if _, ok := phone.FormatFor("XX"); ok {
		t.Error("an unknown country has a format")
	}
}

// A single-country store fixes the format - a national number is that country's even on an address elsewhere
func TestRulesWithOneCountryReadEveryNationalNumberInIt(t *testing.T) {
	r := phone.Rules{Default: "RS", Allowed: []string{"RS"}}
	if got, err := r.Normalize("064 123 4567", "DE"); err != nil || got != "+381641234567" {
		t.Fatalf("national number on a DE address = %q, %v, want +381641234567", got, err)
	}
	if _, err := r.Normalize("01512 3456789", "DE"); !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("a German national number = %v, want ErrInvalid - it isn't a Serbian number", err)
	}
}

func TestCallingCode(t *testing.T) {
	for country, want := range map[string]string{"RS": "+381", "us": "+1", "CA": "+1", "XK": "+383", "XX": "", "": ""} {
		if got := phone.CallingCode(country); got != want {
			t.Errorf("CallingCode(%q) = %q, want %q", country, got, want)
		}
	}
}
