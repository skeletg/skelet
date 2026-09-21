package admin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/pluginset"
)

// A plugin's admin fields reach core only through a generated pluginhost.Do delegation, whose body carries no auth.Require* of its own
// so the coverage gate would either fail every plugin field or, if allowlisted, wave them all through unchecked

// What counts as a gate on the plugin side. PermittedChannels is included because the host checks the permission is held BEFORE resolving scope - it refuses an unpermitted caller rather than handing back a bypass, which is what makes it safe for a listing to lean on
var pluginAuthGates = map[string]bool{
	"RequirePermission":        true,
	"RequireChannelPermission": true,
	"PermittedChannels":        true,
}

// Reads the slug out of a generated delegation: pluginhost.Do(ctx, sdk.GraphAdmin, "<slug>", "<field>", ...)
func delegatedPluginSlug(body *ast.BlockStmt) string {
	slug := ""
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 3 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Do" {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "pluginhost" {
			return true
		}
		lit, ok := call.Args[2].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		slug = strings.Trim(lit.Value, `"`)
		return false
	})
	return slug
}

// Where a plugin's Go source actually is. In-tree it is plugins/<slug>; as a module it is wherever
// the go tool resolved the dependency to, which honours replace directives and workspaces
func pluginSourceDir(t *testing.T, slug string) string {
	t.Helper()
	set, err := pluginset.Load("../..")
	if err != nil {
		t.Fatalf("load plugins.json: %v", err)
	}
	if set.InTree(slug) {
		return filepath.Join("../../plugins", slug)
	}
	module := set.Plugins[slug].Module
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", module)
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("resolve module %s for plugin %q: %v", module, slug, err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		t.Fatalf("module %s resolved to no directory", module)
	}
	return dir
}

// Every method and func in one plugin's package, by name, with the same guard/call shape core uses -
// so a guard reached through a plugin's own helper counts, just as it does here
func parsePluginFuncs(t *testing.T, slug string) map[string]funcInfo {
	t.Helper()
	dir := pluginSourceDir(t, slug)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("plugin %s: read %s: %v", slug, dir, err)
	}
	fset := token.NewFileSet()
	out := map[string]funcInfo{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("plugin %s: parse %s: %v", slug, name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			info := out[fn.Name.Name]
			gates := pluginBodyGates(fn.Body)
			info.guards = info.guards || len(gates) > 0
			info.authGates = append(info.authGates, gates...)
			info.calls = append(info.calls, pluginBodyCalls(fn.Body)...)
			out[fn.Name.Name] = info
		}
	}
	return out
}

// sdk.Require*/PermittedChannels only - a plugin cannot import auth, so these are the whole
// vocabulary available to it
func pluginBodyGates(body *ast.BlockStmt) []string {
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "sdk" {
			return true
		}
		if pluginAuthGates[sel.Sel.Name] {
			out = append(out, sel.Sel.Name)
		}
		return true
	})
	return out
}

// Bare helper(...) calls - a plugin's resolvers are methods on a stateless struct, so its helpers are
// package funcs rather than methods on a receiver
func pluginBodyCalls(body *ast.BlockStmt) []string {
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if fn, ok := call.Fun.(*ast.Ident); ok {
				out = append(out, fn.Name)
			}
		}
		return true
	})
	return out
}

// Whether the plugin serving this field guards it, directly or through its own helpers
func pluginGates(t *testing.T, slug, field string) bool {
	t.Helper()
	funcs := parsePluginFuncs(t, slug)
	if len(funcs) == 0 {
		t.Fatalf("plugin %s: parsed no functions - the gate is not seeing its source", slug)
	}
	if _, ok := funcs[field]; !ok {
		return false // the plugin does not serve the field its SDL declares - boot refuses it too
	}
	return gates(field, funcs, 0, map[string]bool{})
}
