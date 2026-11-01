package plugins

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const repoRoot = "../../../.."

func plugin(t *testing.T, root, slug, src string) {
	t.Helper()
	dir := filepath.Join(root, Dir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

const registers = "package p\nfunc init() { sdk.RegisterPlugin(nil) }\n"

func TestDiscoverIsPluginsetsRule(t *testing.T) {
	root := t.TempDir()
	plugin(t, root, "example", registers)
	plugin(t, root, "payment-stub", registers)
	plugin(t, root, "_parked", registers)
	plugin(t, root, ".modules", registers)
	plugin(t, root, "firstparty", "package firstparty\n")
	plugin(t, root, "helpers", "package helpers\n")
	got, err := Discover(root)
	if want := []string{"example", "payment-stub"}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Discover = %v, %v - want %v", got, err, want)
	}
}

func TestDecideMergesGatewayPicksOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"plugins": {"example": 1, "_example": 0, "payment-old": 1, "loyalty": {"enabled": true, "module": "example.com/loyalty"}}}`
	if err := os.WriteFile(filepath.Join(root, File), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	set, before, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	dropped := set.Decide([]string{"example", "payment-old", "payment-new", "fresh"}, []string{"payment-new"})
	if !reflect.DeepEqual(dropped, []string{"_example"}) {
		t.Errorf("dropped %v", dropped)
	}
	want := map[string]Entry{
		"example":     {Enabled: true},
		"payment-old": {Enabled: false},
		"payment-new": {Enabled: true},
		"fresh":       {Enabled: false},
		"loyalty":     {Enabled: true, Module: "example.com/loyalty"},
	}
	if !reflect.DeepEqual(set.Plugins, want) {
		t.Errorf("decided %v\nwant %v", set.Plugins, want)
	}
	data, _ := set.Marshal()
	if changed, err := Save(root, data, before); !changed || err != nil {
		t.Errorf("Save: %v, %v", changed, err)
	}
	again, raw, _ := Load(root)
	if !reflect.DeepEqual(again.Plugins, want) {
		t.Errorf("reloaded %v", again.Plugins)
	}
	if changed, _ := Save(root, data, raw); changed {
		t.Error("an identical write counted as a change")
	}
}

// The committed file round-trips byte for byte, so a setup that picks nothing new leaves it out of git status
func TestCommittedFileRoundTrips(t *testing.T) {
	set, raw, err := Load(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	found, err := Discover(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	set.Decide(found, nil)
	data, _ := set.Marshal()
	if string(data) != string(raw) {
		t.Errorf("re-encoded:\n%s\ncommitted:\n%s", data, raw)
	}
}
