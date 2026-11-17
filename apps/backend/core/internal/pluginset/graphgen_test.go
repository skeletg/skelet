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
		if o, ok := owned[ownedKey("", f)]; !ok || o.slug != "acme" || !o.enabled || o.field != f {
			t.Errorf("%s not owned by acme: %+v", f, o)
		}
	}
	for _, f := range []string{"notAField", "alsoNot"} {
		for key := range owned {
			if strings.HasSuffix(key, strings.ToLower(f)) {
				t.Errorf("%s is the plugin's own type's field but was claimed as %s", f, key)
			}
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
	owned := map[string]sdlField{ownedKey("", "acmeUpload"): {slug: "acme", field: "acmeUpload", enabled: true}}
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

// A field on a core type is claimed under its type, and refused unless it's batched and nullable
func TestSDLFieldsClaimsFieldsOnCoreTypes(t *testing.T) {
	ok := `extend type Product { acmeWarranty: AcmeWarranty @goField(forceResolver: true, batch: true) }
type AcmeWarranty { months: Int }
extend type AcmeWarranty { provider: String }
`
	root := writeTree(t, map[string]string{
		StateFile:                               `{"plugins": {"acme": {"enabled": true}}}`,
		"plugins/acme/graphql/admin/a.graphqls": ok,
	})
	set, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := sdlFields(root, set, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if o, found := owned[ownedKey("Product", "acmeWarranty")]; !found || o.typ != "Product" || o.field != "acmeWarranty" || len(owned) != 1 {
		t.Errorf("owned %+v - want Product.acmeWarranty alone, the plugin's own type left out", owned)
	}

	for name, tc := range map[string]struct{ sdl, want string }{
		"unbatched": {`extend type Product { acmeNote: String @goField(forceResolver: true) }`, "batch: true"},
		"bare":      {`extend type Product { acmeNote: String }`, "batch: true"},
		"non-null":  {`extend type Product { acmeNote: String! @goField(forceResolver: true, batch: true) }`, "nullable"},
		"input":     {`extend input ProductInput { acmeNote: String }`, "object types only"},
	} {
		t.Run(name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				StateFile:                               `{"plugins": {"acme": {"enabled": true}}}`,
				"plugins/acme/graphql/admin/a.graphqls": tc.sdl,
			})
			set, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := sdlFields(root, set, "admin"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %v, want an error about %q", err, tc.want)
			}
		})
	}
}

// gqlgen's batch stub on a core type becomes a DoBatch delegation to a type-prefixed plugin method, beside the core
// resolvers on the same type, which stay where they are
func TestCoreTypeFieldsDelegateInABatch(t *testing.T) {
	root := writeTree(t, map[string]string{
		"graph/admin/acme.resolvers.go": `package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
)

func (r *productResolver) AcmeURL(ctx context.Context, objs []*model.Product) ([]*string, error) {
	panic("not implemented")
}

func (r *queryResolver) AcmeStatus(ctx context.Context) (*string, error) {
	panic("not implemented")
}
`,
		"graph/admin/catalog.resolvers.go": `package admin

import "context"

func (r *productResolver) Options(ctx context.Context, obj *Product) ([]string, error) { return nil, nil }
`,
	})
	owned := map[string]sdlField{
		ownedKey("Product", "acmeUrl"): {slug: "acme", typ: "Product", field: "acmeUrl", enabled: true},
		ownedKey("", "acmeStatus"):     {slug: "acme", field: "acmeStatus", enabled: true},
	}
	ms, err := rewriteResolverFiles(root, "admin", "sdk.GraphAdmin", "admin", owned)
	if err != nil {
		t.Fatal(err)
	}
	if err := writePluginResolvers(root, "admin", "sdk.GraphAdmin", "admin", ms); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(root, "graph", "admin", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	got := read("acme.resolvers.go")
	for _, want := range []string{
		`pluginhost.DoBatch(ctx, sdk.GraphAdmin, "acme", "Product.acmeUrl", len(objs),`,
		`return impl.ProductAcmeURL(ctx, objs)`,
		`pluginhost.Do(ctx, sdk.GraphAdmin, "acme", "acmeStatus",`,
		`return impl.AcmeStatus(ctx)`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("delegation lacks %q:\n%s", want, got)
		}
	}
	if iface := read("plugin_resolvers.go"); !strings.Contains(iface, "ProductAcmeURL(ctx context.Context, objs []*model.Product) ([]*string, error)") {
		t.Errorf("interface lacks the type-prefixed method:\n%s", iface)
	}
	if !strings.Contains(read("catalog.resolvers.go"), "func (r *productResolver) Options") {
		t.Error("a core resolver on the same type was touched")
	}
}

// A departed plugin's batched delegations go with it, or gqlgen type-checks a file whose interface and model types left
// the schema with the plugin
func TestADepartedPluginsTypeFieldsArePruned(t *testing.T) {
	root := writeTree(t, map[string]string{
		"graph/admin/gone.resolvers.go": `package admin

func (r *productResolver) GoneWarranty(ctx context.Context, objs []*model.Product) ([]*model.GoneWarranty, error) {
	return pluginhost.DoBatch(ctx, sdk.GraphAdmin, "gone", "Product.goneWarranty", len(objs),
		func(ctx context.Context, impl goneAdmin) ([]*model.GoneWarranty, error) { return impl.ProductGoneWarranty(ctx, objs) })
}
`,
	})
	if _, err := rewriteResolverFiles(root, "admin", "sdk.GraphAdmin", "admin", map[string]sdlField{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "graph", "admin", "gone.resolvers.go")); !os.IsNotExist(err) {
		t.Errorf("the departed plugin's delegation file is still there: %v", err)
	}
}

// A plugin's own type gets a delegated field only where it asks for a resolver - a reference back to core's record -
// and the plumbing gqlgen writes for that type survives the rewrite
func TestOwnTypeFieldsPointingAtCoreDelegate(t *testing.T) {
	root := writeTree(t, map[string]string{
		StateFile: `{"plugins": {"acme": {"enabled": true}}}`,
		"plugins/acme/graphql/admin/a.graphqls": `type AcmeWarranty {
  months: Int!
  productId: ID!
  product: Product @goField(forceResolver: true, batch: true)
}
`,
		"graph/admin/a.resolvers.go": `package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
)

func (r *acmeWarrantyResolver) Product(ctx context.Context, objs []*model.AcmeWarranty) ([]*model.Product, error) {
	panic("not implemented")
}

func (r *Resolver) AcmeWarranty() AcmeWarrantyResolver { return &acmeWarrantyResolver{r} }

type acmeWarrantyResolver struct{ *Resolver }
`,
	})
	set, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := sdlFields(root, set, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 1 || owned[ownedKey("AcmeWarranty", "product")].typ != "AcmeWarranty" {
		t.Fatalf("owned %+v - want AcmeWarranty.product alone, its plain fields left to the model", owned)
	}
	if _, err := rewriteResolverFiles(root, "admin", "sdk.GraphAdmin", "admin", owned); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "graph", "admin", "a.resolvers.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`pluginhost.DoBatch(ctx, sdk.GraphAdmin, "acme", "AcmeWarranty.product", len(objs),`,
		`return impl.AcmeWarrantyProduct(ctx, objs)`,
		`func (r *Resolver) AcmeWarranty() AcmeWarrantyResolver`,
		`type acmeWarrantyResolver struct{ *Resolver }`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("rewritten file lacks %q:\n%s", want, b)
		}
	}
}
