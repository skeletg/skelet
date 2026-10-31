package compose

import (
	"os/exec"
	"strings"
	"testing"
)

// The profiles and uid come from bin/_lib itself, and the shim's own variables stay out
func TestEnvReadsLib(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	t.Setenv("SKELET_SHIM", "1")
	t.Setenv("COMPOSE_PROFILES", "mine")
	lib, err := Load("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	env := lib.Env()
	seen := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		seen[k] = v
	}
	if _, ok := seen["SKELET_SHIM"]; ok {
		t.Error("the shim's variable reached compose")
	}
	if seen["COMPOSE_PROFILES"] != "mine" {
		t.Errorf("COMPOSE_PROFILES = %q - _lib keeps one you exported, and so must this", seen["COMPOSE_PROFILES"])
	}
	if seen["SKELET_UID"] == "" || seen["SKELET_GID"] == "" {
		t.Errorf("no uid from _lib: %q %q", seen["SKELET_UID"], seen["SKELET_GID"])
	}
}
