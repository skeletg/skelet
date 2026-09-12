package admin

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

// A dependency root nobody reads is invisible - the field compiles, wiring keeps handing it a live service, and the only symptom is a package the surface no longer needs staying linked into it
// Both resolver packages are checked together because they drift for the same reason, and the storefront one has no test file of its own to hang this on

// Package dir (relative to this one) -> the surface it serves
var resolverPackages = map[string]string{
	".":        "admin",
	"../front": "storefront",
}

// "<surface>.<field>" -> why it stays though no resolver reads it
var unreadResolverFields = map[string]string{}

func TestEveryResolverFieldIsRead(t *testing.T) {
	for dir, surface := range resolverPackages {
		fields, read := resolverFieldsAndReads(t, dir)
		if len(fields) == 0 {
			t.Errorf("%s: found no Resolver struct - did the type move?", surface)
			continue
		}
		var dead []string
		for _, name := range fields {
			if read[name] {
				continue
			}
			if _, ok := unreadResolverFields[surface+"."+name]; ok {
				continue
			}
			dead = append(dead, name)
		}
		sort.Strings(dead)
		for _, name := range dead {
			t.Errorf("%s Resolver.%s is wired up but no resolver reads it - drop the field and its assignment, or "+
				"list it in unreadResolverFields as %q with a reason", surface, name, surface+"."+name)
		}
	}
}

// Asserted from the other side too, so an entry cannot outlive the field it excuses
func TestUnreadResolverFieldsStillExist(t *testing.T) {
	declared := map[string]bool{}
	for dir, surface := range resolverPackages {
		fields, read := resolverFieldsAndReads(t, dir)
		for _, name := range fields {
			declared[surface+"."+name] = true
			if read[name] {
				if _, ok := unreadResolverFields[surface+"."+name]; ok {
					t.Errorf("%s.%s is listed unread but a resolver reads it now - drop the entry", surface, name)
				}
			}
		}
	}
	for key := range unreadResolverFields {
		if !declared[key] {
			t.Errorf("%s is listed unread but no such Resolver field exists - drop the entry", key)
		}
	}
}

// Exported field names on the package's Resolver, plus every name the package reads off something
// A bare name match rather than a typed one - running on source alone, it errs toward calling a field read
func resolverFieldsAndReads(t *testing.T, dir string) ([]string, map[string]bool) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	var fields []string
	read := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || name == "generated.go" {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.TypeSpec:
				st, ok := node.Type.(*ast.StructType)
				if !ok || node.Name.Name != "Resolver" {
					return true
				}
				for _, f := range st.Fields.List {
					for _, id := range f.Names {
						if id.IsExported() {
							fields = append(fields, id.Name)
						}
					}
				}
			case *ast.SelectorExpr:
				// A composite-literal key is an Ident, not a selector, so wiring the field up is not reading it
				read[node.Sel.Name] = true
			}
			return true
		})
	}
	return fields, read
}
