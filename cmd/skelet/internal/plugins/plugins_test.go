package plugins

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
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

// Core's pluginset reads and writes the same file - held to its fixture, so neither side drops what the other wrote
func TestPluginsFileRoundTripsLikePluginsets(t *testing.T) {
	want, err := os.ReadFile(filepath.Join(repoRoot, "apps/backend/core/internal/pluginset/testdata/plugins.roundtrip.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, File), want, 0o644); err != nil {
		t.Fatal(err)
	}
	set, _, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := set.Marshal(); err != nil || string(got) != string(want) {
		t.Errorf("a load and marshal rewrote the file (%v):\n%s", err, got)
	}
}

// What may go to pnpm add is one rule in two modules that can't import each other
func TestPackageSpecIsPluginsets(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot, "apps/backend/core/internal/pluginset/ui.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "regexp.MustCompile(`"+PackageSpec.String()+"`)") {
		t.Errorf("pluginset's packageSpec is no longer %s", PackageSpec)
	}
	for spec, name := range map[string]string{"@acme/x@^1.2.0": "@acme/x", "@acme/x": "@acme/x", "x@latest": "x", "x": "x"} {
		if got := PackageName(spec); got != name || !PackageSpec.MatchString(spec) {
			t.Errorf("PackageName(%q) = %q, valid %v", spec, got, PackageSpec.MatchString(spec))
		}
	}
	if PackageVersion("@acme/x@^1.2.0") != "^1.2.0" || PackageVersion("@acme/x") != "" {
		t.Error("PackageVersion misreads a scope's @ as the version's")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []string{"zod@npm:evil-pkg", "x@https://example.test/x.tgz", "x';alert(1)//", "--registry=x"} {
		raw := `{"plugins": {"fraud": {"enabled": true, "dashboard": ` + strconv.Quote(spec) + `}}}`
		if err := os.WriteFile(filepath.Join(root, File), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Load(root); err == nil {
			t.Errorf("plugins.json naming %q was loaded", spec)
		}
	}
}
