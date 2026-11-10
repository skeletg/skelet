package admin

import "testing"

func TestStorefrontMCPURLNeedsAPublicURL(t *testing.T) {
	for in, want := range map[string]string{"https://shop.example": "https://shop.example/api/mcp", " https://shop.example/ ": "https://shop.example/api/mcp"} {
		if got := storefrontMCPURL(in); got == nil || *got != want {
			t.Errorf("%q -> %v, want %s", in, got, want)
		}
	}
	if got := storefrontMCPURL("  "); got != nil {
		t.Errorf("a channel with no public URL got %s", *got)
	}
}
