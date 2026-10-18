package apperr_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Static sweep across internal/ and lib/: once a package registers ANY sentinel with apperr.RegisterSentinel, every `Err* = errors.New(...)` it declares must be registered too - a forgotten one surfaces as extensions.code = INTERNAL on what should be a typed error, which nothing else catches Packages with zero registrations are exempt: their errors never cross the GraphQL edge (or haven't yet - the first registration in a package opts the whole package in)
func TestSentinelRegistrationCoverage(t *testing.T) {
	var pkgDirs []string
	for _, root := range []string{"../../internal", "../../lib"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				pkgDirs = append(pkgDirs, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(pkgDirs) < 50 {
		t.Fatalf("scanned only %d package dirs, the roots are wrong", len(pkgDirs))
	}
	for _, pkgDir := range pkgDirs {
		declared, registered, opted := scanPackage(t, pkgDir)
		if !opted {
			continue
		}
		var missing []string
		for name := range declared {
			if !registered[name] {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("package %s registers sentinels but misses: %s (add them to its RegisterSentinel init)", pkgDir, strings.Join(missing, ", "))
		}
	}
}

// declared = top-level Err* vars assigned errors.New; registered = names passed to apperr.RegisterSentinel; opted = any RegisterSentinel call at all
func scanPackage(t *testing.T, dir string) (declared, registered map[string]bool, opted bool) {
	t.Helper()
	declared, registered = map[string]bool{}, map[string]bool{}
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		astf, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, decl := range astf.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if !strings.HasPrefix(name.Name, "Err") || i >= len(vs.Values) {
						continue
					}
					if call, ok := vs.Values[i].(*ast.CallExpr); ok && isSelector(call.Fun, "errors", "New") {
						declared[name.Name] = true
					}
				}
			}
		}
		ast.Inspect(astf, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isSelector(call.Fun, "apperr", "RegisterSentinel") || len(call.Args) == 0 {
				return true
			}
			opted = true
			if ident, ok := call.Args[0].(*ast.Ident); ok {
				registered[ident.Name] = true
			}
			return true
		})
	}
	return declared, registered, opted
}

func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg
}
