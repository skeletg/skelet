package shoptools

import "testing"

func TestThePageKeepsNoTokenOrQueryButTheSearchedWords(t *testing.T) {
	for raw, want := range map[string]string{
		"/account/reset/9f2c1d0e7a":                              "/account",
		"/de/account/reset/9f2c1d0e7a":                           "/de/account",
		"/wishlist/Zx81kQ_share":                                 "/wishlist",
		"/handoff/abc123":                                        "/handoff",
		"/verify-email?token=abc":                                "/verify-email",
		"/cart/offer?token=abc":                                  "/cart",
		"/search?q=warm+socks&utm_source=email":                  "/search?q=warm+socks",
		"/shop/t-shirts/tee?ref=newsletter":                      "/shop/t-shirts/tee",
		"/de/shop/t-shirts/f/color/v/navy":                       "/de/shop/t-shirts/f/color/v/navy",
		"/mein-konto/bestellung/a8F3kQ9zL2mN7pR4tV6wX1yB":        "/mein-konto",
		"/members/7b0c9d1e-2f3a-4b5c-8d9e-0f1a2b3c4d5e":          "/members",
		"/de/7b0c9d1e-2f3a-4b5c-8d9e-0f1a2b3c4d5e":               "/de/",
		"/newsletter/confirm/jane@example.com":                   "/newsletter",
		"/shop/nike-air-max-90-essential-white-trainers-for-men": "/shop/nike-air-max-90-essential-white-trainers-for-men",
		"//evil.example/path":                                    "",
		"https://evil.example/path":                              "",
	} {
		if got := pagePath(raw); got != want {
			t.Errorf("pagePath(%q) = %q, want %q", raw, got, want)
		}
	}
}
