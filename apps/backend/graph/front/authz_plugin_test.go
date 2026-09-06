package front

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/pluginset"
)

// A plugin's storefront mutations reach core through a generated pluginhost.Do delegation, whose body
// establishes no caller of its own - so the check above would either fail every plugin mutation or,
// if allowlisted, wave them all through
//
// So it FOLLOWS the delegation into the plugin's vendored source and demands the same thing there:
// the mutation names who is acting before it writes. On this surface that means sdk.CustomerID with
// a nil check that actually returns, since a plugin cannot import internal/auth

// Not a path built from the slug: an in-tree plugin sits under plugins/<slug>/, a module one in
// the module cache at a versioned path that differs per machine, and only plugins.json knows which
func pluginSourceDir(t *testing.T, slug string) string {
	t.Helper()
	set, err := pluginset.Load("../..")
	if err != nil {
		t.Fatalf("read plugins.json: %v", err)
	}
	dir, err := set.SourceDir("../..", slug)
	if err != nil {
		t.Fatalf("plugin %s: %v", slug, err)
	}
	return dir
}

// Reads the slug out of pluginhost.Do(ctx, sdk.GraphFront, "<slug>", "<field>", ...)
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

type pluginFuncInfo struct {
	identifies bool
	calls      []string
}

func parsePluginFrontFuncs(t *testing.T, slug string) map[string]pluginFuncInfo {
	t.Helper()
	dir := pluginSourceDir(t, slug)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("plugin %s: read %s: %v", slug, dir, err)
	}
	fset := token.NewFileSet()
	out := map[string]pluginFuncInfo{}
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
			info.identifies = info.identifies || pluginRefusesAnonymous(fn.Body)
			info.calls = append(info.calls, pluginBodyCalls(fn.Body)...)
			out[fn.Name.Name] = info
		}
	}
	return out
}

// The plugin-side twin of refusesAnonymous: take sdk.CustomerID, then return early when it came back
// nil. A bare sdk.CustomerID establishes nothing - it hands back nil for a guest - so only the nil
// check that actually RETURNS counts
func pluginRefusesAnonymous(body *ast.BlockStmt) bool {
	optional := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "CustomerID" {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "sdk" {
			return true
		}
		if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
			optional[ident.Name] = true
		}
		return true
	})
	if len(optional) == 0 {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok || ifStmt.Body == nil {
			return true
		}
		bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
		if !ok || bin.Op != token.EQL {
			return true
		}
		lhs, ok := bin.X.(*ast.Ident)
		if !ok || !optional[lhs.Name] {
			return true
		}
		if rhs, ok := bin.Y.(*ast.Ident); !ok || rhs.Name != "nil" {
			return true
		}
		for _, stmt := range ifStmt.Body.List {
			if _, isReturn := stmt.(*ast.ReturnStmt); isReturn {
				found = true
			}
		}
		return true
	})
	return found
}

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

// Directly, or through a plugin helper that does - companyAdmin is the shape that matters here
func pluginIdentifies(t *testing.T, slug, field string, depth int, seen map[string]bool) bool {
	t.Helper()
	funcs := parsePluginFrontFuncs(t, slug)
	if len(funcs) == 0 {
		t.Fatalf("plugin %s: parsed no functions - the check is not seeing its source", slug)
	}
	var walk func(string, int) bool
	walk = func(name string, d int) bool {
		if d > 4 || seen[name] {
			return false
		}
		seen[name] = true
		fn, ok := funcs[name]
		if !ok {
			return false
		}
		if fn.identifies {
			return true
		}
		for _, callee := range fn.calls {
			if walk(callee, d+1) {
				return true
			}
		}
		return false
	}
	return walk(field, depth)
}
