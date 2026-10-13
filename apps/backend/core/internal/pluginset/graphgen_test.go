package pluginset

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Ownership is read off the parsed schema, so indentation and layout can't hide a field and a lookalike type name can't claim one
func TestSDLFieldsReadsEveryLayout(t *testing.T) {
	root := writeTree(t, map[string]string{
		StateFile: `{"plugins": {"acme": {"enabled": true}}}`,
		"plugins/acme/graphql/admin/a.graphqls": "extend type Query {\n\tacmeTabbed: Int\n    acmeDeep(\n      id: ID!\n    ): String\n}\n" +
			"extend type Mutation { acmeOneLine: Boolean }\n" +
			"type QueryResult { notAField: Int }\nextend type QueryResult { alsoNot: Int }\n",
	})
	set, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := sdlFields(root, set, "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"acmeTabbed", "acmeDeep", "acmeOneLine"} {
		if o, ok := owned[f]; !ok || o.slug != "acme" || !o.enabled {
			t.Errorf("%s not owned by acme: %+v", f, o)
		}
	}
	for _, f := range []string{"notAField", "alsoNot"} {
		if _, ok := owned[f]; ok {
			t.Errorf("%s is not a Query or Mutation field but was claimed", f)
		}
	}
}

func TestSDLFieldsRefusesATwiceClaimedField(t *testing.T) {
	root := writeTree(t, map[string]string{
		StateFile:                              `{"plugins": {"one": {"enabled": true}, "two": {"enabled": true}}}`,
		"plugins/one/graphql/front/a.graphqls": "extend type Query { shared: Int }\n",
		"plugins/two/graphql/front/b.graphqls": "extend type Query { shared: Int }\n",
	})
	set, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sdlFields(root, set, "front"); err == nil || !strings.Contains(err.Error(), "claimed by both") {
		t.Errorf("want a double-claim error, got %v", err)
	}
}

// The interface file imports what the signatures use, read from gqlgen's file - an upload argument used to leave graphql unimported
func TestGeneratedResolversImportWhatTheirSignaturesUse(t *testing.T) {
	root := writeTree(t, map[string]string{
		"graph/admin/acme.resolvers.go": `package admin

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
)

// AcmeUpload is the resolver for the acmeUpload field.
func (r *mutationResolver) AcmeUpload(ctx context.Context, file graphql.Upload) (*model.AcmeResult, error) {
	panic("not implemented")
}
`,
	})
	owned := map[string]sdlField{"acmeUpload": {slug: "acme", field: "acmeUpload", enabled: true}}
	ms, err := rewriteResolverFiles(root, "admin", "sdk.GraphAdmin", "admin", owned)
	if err != nil {
		t.Fatal(err)
	}
	if err := writePluginResolvers(root, "admin", "sdk.GraphAdmin", "admin", ms); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"acme.resolvers.go", "plugin_resolvers.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "graph", "admin", name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := map[string]bool{}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			got[p] = true
		}
		for _, want := range []string{"github.com/99designs/gqlgen/graphql", "github.com/skeletg/skelet/apps/backend/core/graph/admin/model"} {
			if !got[want] {
				t.Errorf("%s does not import %s", name, want)
			}
		}
	}
}

func TestLoadRefusesUnsafeEntries(t *testing.T) {
	for name, state := range map[string]string{
		"traversal slug": `{"plugins": {"../core": {"enabled": true}}}`,
		"digit slug":     `{"plugins": {"1pay": {"enabled": true}}}`,
		"reserved slug":  `{"plugins": {"firstparty": {"enabled": true}}}`,
		"flag module":    `{"plugins": {"acme": {"enabled": true, "module": "-toolexec=/tmp/x"}}}`,
	} {
		if _, err := Load(writeTree(t, map[string]string{StateFile: state})); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	if _, err := Load(writeTree(t, map[string]string{StateFile: `{"plugins": {"payment-acme": {"enabled": true, "module": "github.com/acme/payment-acme"}}}`})); err != nil {
		t.Errorf("a valid module entry refused: %v", err)
	}
}

func TestSaveLeavesNoTempBehind(t *testing.T) {
	root := writeTree(t, map[string]string{StateFile: `{"plugins": {}}`})
	set, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	set.Set("acme", true)
	if err := set.Save(root); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("plugins/ holds %d entries after a save, want just plugins.json", len(entries))
	}
	again, err := Load(root)
	if err != nil || !again.Enabled("acme") {
		t.Errorf("saved set did not round-trip: %v", err)
	}
}

// A write that can't land must leave neither a half-written file nor its temp behind
func TestAtomicWriteCleansUpOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "plugins.json")
	if err := os.MkdirAll(filepath.Join(target, "occupied"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(target, []byte("{}")); err == nil {
		t.Fatal("rename over a non-empty directory succeeded")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("a failed write left %d entries behind, want only the original", len(entries))
	}
}
