package pluginset

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Tests read the real tree - the invariant below is about this checkout, not a fixture
const root = "../.."

// A plugin that ships SDL cannot be half-disabled: its own resolvers return core's generated model types, so
// dropping its schema drops the models its package still references and nothing compiles
// The failure is loud now that the schema list is the enabled set rather than a directory glob, but it lands as
// a compile error inside the plugin, which is a worse place to read it than here
func TestPluginsShippingSchemaAreEnabled(t *testing.T) {
	set, err := Load(root)
	if err != nil {
		t.Fatalf("load %s: %v", StateFile, err)
	}
	found, err := Discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}

	for _, slug := range found {
		sdl, err := filepath.Glob(filepath.Join(root, pluginsDir, slug, "graphql", "*", "*.graphqls"))
		if err != nil {
			t.Fatalf("glob %s: %v", slug, err)
		}
		if len(sdl) == 0 || set.Enabled(slug) {
			continue
		}
		t.Errorf("%s is disabled but still ships %d SDL file(s) that gqlgen merges into the schema - "+
			"move its graphql/ directory out of the tree and regenerate, or re-enable it", slug, len(sdl))
	}
}

// firstparty lives under plugins/ and is not one - anything else that grows in there must not be either
func TestDiscoverOnlyFindsRealPlugins(t *testing.T) {
	found, err := Discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if slices.Contains(found, "firstparty") {
		t.Error("firstparty was discovered as a plugin - it is the generated import list")
	}
	if !slices.Contains(found, "example") {
		t.Error("example was not discovered - it registers, so it must be decided on in plugins.json even at 0")
	}
	set, err := Load(root)
	if err != nil {
		t.Fatalf("load %s: %v", StateFile, err)
	}
	for _, slug := range found {
		if _, ok := set.Plugins[slug]; !ok {
			t.Errorf("%s registers a plugin but %s does not mention it", slug, StateFile)
		}
	}
}

// The zero-plugin case is the one that breaks the build: core wiring calls ValidatePluginResolvers
// unconditionally, so removing plugin_resolvers.go instead of emptying it leaves the symbol
// undeclared - an empty set must still generate a compilable stub
func TestWritePluginResolversEmptySetGeneratesStub(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "graph", "front"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writePluginResolvers(dir, "front", "front", nil); err != nil {
		t.Fatalf("empty set: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "graph", "front", "plugin_resolvers.go"))
	if err != nil {
		t.Fatalf("stub not written: %v", err)
	}
	src := string(b)
	if !strings.Contains(src, "func ValidatePluginResolvers() error") {
		t.Errorf("stub misses the symbol wiring depends on:\n%s", src)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "plugin_resolvers.go", b, 0); err != nil {
		t.Errorf("stub does not parse: %v", err)
	}
	// idempotent when the file is already the stub, and when it was never written at all it must not
	// error either - the pre-fix code returned os.Remove's ENOENT here
	if err := writePluginResolvers(dir, "front", "front", nil); err != nil {
		t.Errorf("second empty-set write: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "graph", "front", "plugin_resolvers.go")); err != nil {
		t.Fatal(err)
	}
	if err := writePluginResolvers(dir, "front", "front", nil); err != nil {
		t.Errorf("empty-set write with no prior file: %v", err)
	}
}
