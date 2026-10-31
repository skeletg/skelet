package sys

import (
	"slices"
	"testing"
)

func TestScriptsDoNotSeeTheShimVariables(t *testing.T) {
	t.Setenv("SKELET_ROOT", "/x")
	t.Setenv("SKELET_SHIM", "1")
	t.Setenv("SKELET_APPS", "dashboard")
	env := Env()
	for _, kv := range env {
		if kv == "SKELET_ROOT=/x" || kv == "SKELET_SHIM=1" {
			t.Errorf("%s reached the script", kv)
		}
	}
	if !slices.Contains(env, "SKELET_APPS=dashboard") {
		t.Error("an unrelated SKELET_ variable was dropped")
	}
}
