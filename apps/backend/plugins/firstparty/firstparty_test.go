package firstparty

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/skeletg/skelet/internal/pluginset"

	pluginhost "github.com/skeletg/skelet/internal/plugin"
)

func TestEveryEnabledPluginRegisters(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "firstparty.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse firstparty.go: %v", err)
	}
	var want []string
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if dir, ok := strings.CutPrefix(path, "github.com/skeletg/skelet/plugins/"); ok {
			want = append(want, dir)
		}
	}
	// Zero imports is the shipped default, so the guard checks generated imports against the
	// decision file rather than refusing emptiness
	set, err := pluginset.Load("../..")
	if err != nil {
		t.Fatalf("load plugins.json: %v", err)
	}
	if enabled := set.EnabledSlugs(); len(want) != len(enabled) {
		t.Fatalf("firstparty.go imports %v but plugins.json enables %v - run bin/codegen", want, enabled)
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
