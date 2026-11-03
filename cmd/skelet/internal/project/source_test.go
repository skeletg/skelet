package project

import "testing"

func TestIsRelease(t *testing.T) {
	for tag, want := range map[string]bool{
		"v0.2.3": true, "v10.0.12": true, "v0.0.0": true,
		"0.2.3": false, "v0.2": false, "v0.2.3.4": false, "v0.2.3-rc1": false, "v01.2.3": false, "v+1.2.3": false,
		"v": false, "": false, "apps/backend/core/v0.1.0": false, "v0..3": false,
	} {
		if got := IsRelease(tag); got != want {
			t.Errorf("IsRelease(%q) = %v", tag, got)
		}
	}
}

func TestNewestComparesNumbersNotText(t *testing.T) {
	got, ok := newest([]string{"v0.2.3", "v0.10.0", "v0.9.9", "v1.0.0-rc1", "apps/backend/core/v2.0.0", "nightly"})
	if !ok || got != "v0.10.0" {
		t.Errorf("newest = %q, %v - want v0.10.0", got, ok)
	}
	if got, ok := newest([]string{"v1.0.0-rc1", "apps/backend/sdk/v0.1.0"}); ok {
		t.Errorf("newest of no releases = %q", got)
	}
}
