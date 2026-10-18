package channel_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
)

func TestStorefrontHostNormalizesThePublicURL(t *testing.T) {
	cases := map[string]string{
		"":                                 "",
		"http://localhost:5174":            "localhost:5174",
		"http://localhost:3000/":           "localhost:3000",
		"HTTPS://Shop.Example.COM:443/x/y": "shop.example.com",
		"http://shop.example.com:80":       "shop.example.com",
		"https://shop.example.com:80":      "shop.example.com:80",
		"http://shop.example.com:443":      "shop.example.com:443",
		"https://user:pw@shop.example.com": "shop.example.com",
		"http://[::1]:8080/store":          "[::1]:8080",
		"http://[::1]:80":                  "[::1]",
		"shop.example.com/store":           "shop.example.com",
		"localhost:3000":                   "localhost:3000",
		"  https://Shop.Example.com  ":     "shop.example.com",
	}
	for in, want := range cases {
		if got := channel.StorefrontHost(in); got != want {
			t.Errorf("StorefrontHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeHostMatchesWhatTheHostHeaderCarries(t *testing.T) {
	cases := []struct{ scheme, in, want string }{
		{"", "Shop.Example.COM", "shop.example.com"},
		{"", "shop.example.com:443", "shop.example.com"},
		{"", "shop.example.com:80", "shop.example.com"},
		{"", "localhost:5174", "localhost:5174"},
		{"", "[::1]:8080", "[::1]:8080"},
		{"", "[::1]", "[::1]"},
		{"https", "x:443", "x"},
		{"http", "x:443", "x:443"},
		{"", "", ""},
		{"", strings.Repeat("a", 300), ""},
	}
	for _, tc := range cases {
		if got := channel.NormalizeHost(tc.scheme, tc.in); got != tc.want {
			t.Errorf("NormalizeHost(%q, %q) = %q, want %q", tc.scheme, tc.in, got, tc.want)
		}
	}
}

func TestPublicURLValidation(t *testing.T) {
	ok := []string{"", "  ", "https://shop.example.com", "http://localhost:3000", "https://shop.example.com/eu/", "http://[::1]:8080"}
	for _, in := range ok {
		if _, err := channel.NormalizePublicURL(in); err != nil {
			t.Errorf("%q rejected: %v", in, err)
		}
	}
	bad := []string{
		"shop.example.com",
		"ftp://shop.example.com",
		"https://",
		"https://user:pw@shop.example.com",
		"https://shop.example.com/?ref=x",
		"https://shop.example.com/#top",
		"javascript:alert(1)",
		"https://" + strings.Repeat("a", 254),
	}
	for _, in := range bad {
		if _, err := channel.NormalizePublicURL(in); !errors.Is(err, channel.ErrInvalidPublicURL) {
			t.Errorf("%q = %v, want ErrInvalidPublicURL", in, err)
		}
	}
}
