package customer

import (
	"testing"
	"time"
)

func TestNormalizeEmail(t *testing.T) { // signup and login both run through this - the two disagreeing is what locks a real account out
	for in, want := range map[string]string{
		"  Alice@Example.com ": "alice@example.com",
		"ALICE@EXAMPLE.COM":    "alice@example.com",
		"alice@example.com":    "alice@example.com",
		"\tbob@x.io\n":         "bob@x.io",
		"":                     "",
	} {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

// Slugs are stable identifiers in GraphQL inputs and promotion rules, so they can't drift on case or spacing
func TestValidateGroupSlug(t *testing.T) {
	for _, bad := range []string{"", "VIP", "Wholesale", "two words", "tab\there", "line\nbreak"} {
		if err := validateGroupSlug(bad); err == nil {
			t.Errorf("validateGroupSlug(%q) = nil, want an error", bad)
		}
	}
	for _, ok := range []string{"vip", "wholesale-eu", "tier_2"} {
		if err := validateGroupSlug(ok); err != nil {
			t.Errorf("validateGroupSlug(%q) = %v", ok, err)
		}
	}
}

// Reset emails quote this back to the customer, so the sub-hour and plural boundaries both matter
func TestHumanDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		30 * time.Second: "0 minutes",
		time.Minute:      "1 minute",
		30 * time.Minute: "30 minutes",
		90 * time.Minute: "1 hour",
		time.Hour:        "1 hour",
		4 * time.Hour:    "4 hours",
	} {
		if got := humanDuration(d); got != want {
			t.Errorf("humanDuration(%v) = %q, want %q", d, got, want)
		}
	}
}
