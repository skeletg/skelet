package commands

import "testing"

// The scaffolder names the plugin's Go package after its slug - a change here breaks every scaffolded plugin's package clause
func TestPluginPackageName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ slug, want string }{
		{"payment-stripe", "paymentstripe"},
		{"klarna", "klarna"},
		{"my_plugin", "myplugin"},
		{"a-b-c-d", "abcd"},
	} {
		if got := pluginPackageName(tc.slug); got != tc.want {
			t.Errorf("pluginPackageName(%q): got %q, want %q", tc.slug, got, tc.want)
		}
	}
}
