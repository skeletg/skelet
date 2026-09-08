package admin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
)

// SIGNALS_ENABLED=false has to leave the store sellable and its transactional mail flowing, so the gate is per file and anything still reachable with the suite off is written down below with why

// Resolver file -> the suite it belongs to, every resolver in it gated
var signalsGatedFiles = map[string]string{
	"analytics.resolvers.go":      "the event spine",
	"campaign.resolvers.go":       "Pulse broadcasts",
	"deliverability.resolvers.go": "Pulse sending reputation",
	"experiment.resolvers.go":     "Prism",
	"lens.resolvers.go":           "Lens",
	"synapse.resolvers.go":        "Synapse",
}

// Resolver file -> why its resolvers stay reachable with the suite off
var signalsExemptFiles = map[string]string{
	"flow.resolvers.go": "flows deliver transactional mail, and the engine already parks the marketing ones itself",
}

func TestEverySignalsSuiteResolverIsGated(t *testing.T) {
	byFile := parseResolversByFile(t)
	for file, suite := range signalsGatedFiles {
		fns, ok := byFile[file]
		if !ok {
			t.Errorf("%s is listed as %s but has no resolvers - rename it here or drop the entry", file, suite)
			continue
		}
		for name, gated := range fns {
			if !gated {
				t.Errorf("%s in %s (%s) answers callers with SIGNALS_ENABLED=false - add requireSignals, or move the file to signalsExemptFiles with a reason", name, file, suite)
			}
		}
	}
}

func TestSignalsExemptResolversStayReachable(t *testing.T) {
	byFile := parseResolversByFile(t)
	for file, why := range signalsExemptFiles {
		fns, ok := byFile[file]
		if !ok {
			t.Errorf("%s is listed exempt but has no resolvers - rename it here or drop the entry", file)
			continue
		}
		for name, gated := range fns {
			if gated {
				t.Errorf("%s in %s calls requireSignals, but the file is exempt because %s", name, file, why)
			}
		}
	}
}

// File name -> resolver name -> whether its body calls requireSignals
func parseResolversByFile(t *testing.T) map[string]map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	out := map[string]map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".resolvers.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || resolverKind(fn) == "" {
				continue
			}
			if out[name] == nil {
				out[name] = map[string]bool{}
			}
			out[name][fn.Name.Name] = callsRequireSignals(fn.Body)
		}
	}
	// Both tests read every assertion off this map, so an empty one would pass them without checking anything
	for file := range signalsGatedFiles {
		if len(out[file]) == 0 {
			t.Fatalf("no resolvers parsed out of %s - the parser is not seeing the package", file)
		}
	}
	return out
}

func callsRequireSignals(body *ast.BlockStmt) bool {
	return slices.Contains(bodyCalls(body), "requireSignals")
}
