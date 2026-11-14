package admin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// A library resolver that never asks which half a file is in is how a private one gets out, so each reaches requirePrivate or says why not
var halfBlindLibraryResolvers = map[string]string{
	"MediaPrivateArea": "says whether the install has a private store, and nothing of what is in it",
	"MediaUploadTypes": "a list of MIME types",
	"RevokeMediaShare": "takes a share back on media.share alone, and reads no file",
}

var libraryResolversElsewhere = []string{"GenerateMediaAltText"}

func TestEveryLibraryResolverAsksWhichHalf(t *testing.T) {
	funcs, _ := parsePackageFuncs(t)
	names := append(entryPointsDeclaredIn(t, "media.resolvers.go"), libraryResolversElsewhere...)
	if len(names) < 15 {
		t.Fatalf("found only %d library resolvers - the parser is not seeing media.resolvers.go", len(names))
	}
	for _, name := range names {
		asks := reachesCall(name, "requirePrivate", funcs, 0, map[string]bool{})
		reason, exempt := halfBlindLibraryResolvers[name]
		switch {
		case exempt && asks:
			t.Errorf("%s is listed as needing no private-half check (%s) but makes one - drop it from the list", name, reason)
		case !exempt && !asks:
			t.Errorf("%s never reaches requirePrivate - a caller without the private half's grant could use it on a private file", name)
		}
	}
	for name := range halfBlindLibraryResolvers {
		if _, ok := funcs[name]; !ok {
			t.Errorf("%s is listed but no longer exists", name)
		}
	}
}

func TestOnlyTheGatedHelperReadsAPrivateFile(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), ".GetAnyAsset(") {
			seen++
			if name != "media_helpers.go" {
				t.Errorf("%s calls GetAnyAsset - go through libraryAsset, which checks the private half's grant", name)
			}
		}
		if strings.Contains(string(src), "MediaLibrarySvc.Open(") {
			t.Errorf("%s reads a library file's bytes - that belongs to the file route, behind its own gate", name)
		}
	}
	if seen != 1 {
		t.Fatalf("GetAnyAsset is called in %d files, want exactly media_helpers.go", seen)
	}
}

func reachesCall(name, target string, funcs map[string]funcInfo, depth int, seen map[string]bool) bool {
	if depth > 4 || seen[name] {
		return false
	}
	seen[name] = true
	fn, ok := funcs[name]
	if !ok {
		return false
	}
	for _, callee := range fn.calls {
		if callee == target || reachesCall(callee, target, funcs, depth+1, seen) {
			return true
		}
	}
	return false
}

func entryPointsDeclaredIn(t *testing.T, file string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var out []string
	for _, decl := range parsed.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil && resolverKind(fn) != "" {
			out = append(out, fn.Name.Name)
		}
	}
	return out
}
