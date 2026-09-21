package plugin

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// gqlgen binds a scalar it has no mapping for to Go `string` and says nothing (plugin/modelgen/models.go, the ast.Scalar branch of the no-user-defined-model path)
// so a plugin shipping `scalar Money` gets silent string semantics and finds out when cents arrive over the wire quoted

// The five gqlgen injects itself when they appear as scalars - codegen/config/config.go, extraBuiltins
var gqlgenExtraBuiltins = map[string]struct{}{
	"Time": {}, "Int64": {}, "Map": {}, "Upload": {}, "Any": {},
}

var (
	scalarDecl = regexp.MustCompile(`(?m)^\s*scalar\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// The models: block is flat YAML - a top-level two-space key is a type name
	modelsEntry = regexp.MustCompile(`(?m)^  ([A-Za-z_][A-Za-z0-9_]*):`)
)

func TestEveryDeclaredScalarIsBound(t *testing.T) {
	root := repoBackendRoot(t)
	for _, surface := range []struct{ cfg, coreGlob, pluginGlob string }{
		{"gqlgen.admin.yml", "graph/schema/admin/*.graphqls", "plugins/*/graphql/admin/*.graphqls"},
		{"gqlgen.front.yml", "graph/schema/front/*.graphqls", "plugins/*/graphql/front/*.graphqls"},
	} {
		bound := boundTypes(t, filepath.Join(root, surface.cfg))
		for _, pattern := range []string{surface.coreGlob, surface.pluginGlob} {
			for _, file := range globFiles(t, root, pattern) {
				for _, name := range declaredScalars(t, file) {
					if _, ok := bound[name]; ok {
						continue
					}
					if _, ok := gqlgenExtraBuiltins[name]; ok {
						continue
					}
					rel, _ := filepath.Rel(root, file)
					t.Errorf("%s declares `scalar %s` with no binding in %s - gqlgen will silently resolve it to Go string.\n"+
						"Either add a models: entry for it, or reuse an already-bound scalar (JSON, Int64, Time).",
						rel, name, surface.cfg)
				}
			}
		}
	}
}

// Guards the guard: if gqlgen's config layout changes so the models: block stops parsing, every scalar would read as unbound and the test
// would fail loudly - but if it read as EMPTY it would pass vacuously, so assert the known bindings are actually seen
func TestScalarBindingParserStillWorks(t *testing.T) {
	bound := boundTypes(t, filepath.Join(repoBackendRoot(t), "gqlgen.admin.yml"))
	for _, want := range []string{"JSON", "UUID", "Int64"} {
		if _, ok := bound[want]; !ok {
			t.Fatalf("parser found no binding for %q - the models: block layout changed and this test has gone vacuous", want)
		}
	}
}

func boundTypes(t *testing.T, cfgPath string) map[string]struct{} {
	t.Helper()
	src, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read %s: %v", cfgPath, err)
	}
	body := string(src)
	idx := strings.Index(body, "\nmodels:")
	if idx < 0 {
		t.Fatalf("%s has no models: block", cfgPath)
	}
	out := map[string]struct{}{}
	for _, m := range modelsEntry.FindAllStringSubmatch(body[idx:], -1) {
		out[m[1]] = struct{}{}
	}
	return out
}

func declaredScalars(t *testing.T, path string) []string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []string
	for _, m := range scalarDecl.FindAllStringSubmatch(string(src), -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

func globFiles(t *testing.T, root, pattern string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, pattern))
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	return files
}

// The package sits two levels down from apps/backend
func repoBackendRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "gqlgen.admin.yml")); err != nil {
		t.Fatalf("could not locate apps/backend from %s: %v", wd, err)
	}
	return root
}
