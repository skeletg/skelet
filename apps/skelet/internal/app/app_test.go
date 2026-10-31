package app

import (
	"slices"
	"testing"
)

func TestScriptsDoNotSeeTheShimVariables(t *testing.T) {
	t.Setenv("SKELET_ROOT", "/x")
	t.Setenv("SKELET_SHIM", "1")
	t.Setenv("SKELET_APPS", "dashboard")
	env := childEnv()
	for _, kv := range env {
		if kv == "SKELET_ROOT=/x" || kv == "SKELET_SHIM=1" {
			t.Errorf("%s reached the script", kv)
		}
	}
	if !slices.Contains(env, "SKELET_APPS=dashboard") {
		t.Error("an unrelated SKELET_ variable was dropped")
	}
}

// A release build that ended up in a checkout's CLI cache would hand off to the shim, which runs it again
func TestReleaseBuildRefusesToRunUnderTheShim(t *testing.T) {
	t.Setenv("SKELET_SHIM", "1")
	if code := Main(Config{Version: "v1.0.0", Mode: "global"}, []string{"up"}); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
}
