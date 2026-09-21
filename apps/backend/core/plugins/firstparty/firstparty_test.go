package firstparty

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/pluginset"

	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
)

func TestEveryEnabledPluginRegisters(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "firstparty.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse firstparty.go: %v", err)
	}
	imported := map[string]bool{}
	for _, imp := range file.Imports {
		imported[strings.Trim(imp.Path.Value, `"`)] = true
	}
	// Zero imports is the shipped default, so the guard checks generated imports against the
	// decision file rather than refusing emptiness
	set, err := pluginset.Load("../..")
	if err != nil {
		t.Fatalf("load plugins.json: %v", err)
	}
	// Compared by import PATH, not by directory name: a module plugin is imported by its module path
	// and has no directory here at all, so a slug-shaped comparison would see none of them
	enabled := set.EnabledSlugs()
	want := make([]string, 0, len(enabled))
	for _, slug := range enabled {
		p := set.ImportPath(slug)
		want = append(want, slug)
		if !imported[p] {
			t.Errorf("plugins.json enables %q but firstparty.go does not import %s - run bin/codegen", slug, p)
		}
		delete(imported, p)
	}
	for extra := range imported {
		t.Errorf("firstparty.go imports %s, which plugins.json does not enable - run bin/codegen", extra)
	}

	pluginhost.Install()
	got := map[string]bool{}
	for _, m := range pluginhost.Manifests() {
		got[m.Slug] = true
	}

	for _, dir := range want {
		if !got[dir] {
			t.Errorf("plugin %q is imported but never registered - check its init() calls sdk.RegisterPlugin with slug %q", dir, dir)
		}
	}
}
