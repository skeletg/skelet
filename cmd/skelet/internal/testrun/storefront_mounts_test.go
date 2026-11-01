package testrun

import (
	"slices"
	"testing"
)

func TestMountFlags(t *testing.T) {
	got, err := mountFlags([]mount{
		{Type: "bind", Source: "/home/me/my,shop", Destination: "/var/www/skelet", RW: true},
		{Type: "bind", Source: "/dev/null", Destination: "/var/www/skelet/.env"},
		{Type: "volume", Name: "skelet_shop-node-modules", Destination: "/var/www/skelet/node_modules", RW: true},
		{Type: "tmpfs", Destination: "/var/www/skelet/env", RW: true},
	})
	want := []string{
		"--mount", `type=bind,"src=/home/me/my,shop",dst=/var/www/skelet`,
		"--mount", "type=bind,src=/dev/null,dst=/var/www/skelet/.env,readonly",
		"--mount", "type=volume,src=skelet_shop-node-modules,dst=/var/www/skelet/node_modules",
		"--mount", "type=tmpfs,dst=/var/www/skelet/env",
	}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("mountFlags = %q, %v\nwant %q", got, err, want)
	}
	if _, err := mountFlags([]mount{{Type: "npipe", Destination: "/x"}}); err == nil {
		t.Error("an unknown mount kind was dropped, not refused")
	}
}
