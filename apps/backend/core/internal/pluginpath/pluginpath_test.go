package pluginpath

import "testing"

// TestPackageName covers the slug→identifier translation rule used by both the bridge generator and the CLI scaffolder
// - A change here breaks plugin builds, so the table catches accidental regressions
func TestPackageName(t *testing.T) {
	t.Parallel()

	cases := []struct{ slug, want string }{
		{"payment-stripe", "paymentstripe"},
		{"klarna", "klarna"},
		{"my_plugin", "myplugin"},
		{"a-b-c-d", "abcd"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.slug, func(t *testing.T) {
			t.Parallel()
			got := PackageName(tc.slug)
			if got != tc.want {
				t.Errorf("PackageName(%q): got %q, want %q", tc.slug, got, tc.want)
			}
		})
	}
}
