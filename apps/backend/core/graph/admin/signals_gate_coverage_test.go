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
	"campaign.resolvers.go":           "Pulse broadcasts",
	"flow.resolvers.go":               "Pulse automation flows",
	"email.resolvers.go":              "the Pulse email builder's reusable blocks",
	"email_library.resolvers.go":      "the Pulse email library",
	"customer_profile.resolvers.go":   "the commerce panels of the Synapse profile page",
	"deliverability.resolvers.go":     "Pulse sending reputation",
	"prism_experiments.resolvers.go":  "Prism experiments",
	"pulse_engagement.resolvers.go":   "the Pulse engagement rollup",
	"event_destinations.resolvers.go": "event destinations",
	"signals_lens.resolvers.go":       "Lens web analytics",
	"signals_synapse.resolvers.go":    "Synapse customer analytics",
	"signals_segments.resolvers.go":   "customer segments",
}

// The suite-backed resolvers reach it through one of these, and each funnels into signalsCtx, which
// is where requireSignals actually runs - TestTheSignalsHelperCarriesTheGate holds that chain up
var signalsGateCarriers = []string{"requireSignals", "signalsCtx", "signalsScoped", "signalsChannel", "signalsWrite"}

// Resolver file -> why its resolvers stay reachable with the suite off - none today: Pulse is the
// suite's too, and core sends every transactional mail from its own templates with the switch off
var signalsExemptFiles = map[string]string{}

// Resolver -> why it stays reachable though the rest of its file is gated, for the ones a core surface calls
var signalsExemptResolvers = map[string]string{
	"SuggestEmailSubjects": "copy assist is core's AI service, which reads no builder state - gated on emails.manage and AI_ENABLED",
	"RewriteEmailText":     "the same copy assist, on a passage rather than a subject",
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
			if _, exempt := signalsExemptResolvers[name]; exempt || gated {
				continue
			}
			t.Errorf("%s in %s (%s) answers callers with SIGNALS_ENABLED=false - add requireSignals, or list it in signalsExemptResolvers with a reason", name, file, suite)
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

// An exemption that quietly grew a gate back is a core surface breaking with the suite off, which is what the list exists to stop
func TestSignalsExemptResolversStayUngated(t *testing.T) {
	byFile := parseResolversByFile(t)
	seen := map[string]bool{}
	for _, fns := range byFile {
		for name, gated := range fns {
			why, exempt := signalsExemptResolvers[name]
			if !exempt {
				continue
			}
			seen[name] = true
			if gated {
				t.Errorf("%s calls requireSignals, but it is exempt because %s", name, why)
			}
		}
	}
	for name := range signalsExemptResolvers {
		if !seen[name] {
			t.Errorf("%s is listed exempt but no longer exists - rename it here or drop the entry", name)
		}
	}
}

func callsRequireSignals(body *ast.BlockStmt) bool {
	calls := bodyCalls(body)
	return slices.ContainsFunc(signalsGateCarriers, func(c string) bool { return slices.Contains(calls, c) })
}

// The carriers above are only a gate because signalsCtx runs requireSignals - assert that rather than trust it
func TestTheSignalsHelperCarriesTheGate(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "signals_helpers.go", nil, 0)
	if err != nil {
		t.Fatalf("parse signals_helpers.go: %v", err)
	}
	var found bool
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "signalsCtx" || fn.Body == nil {
			continue
		}
		found = true
		if !slices.Contains(bodyCalls(fn.Body), "requireSignals") {
			t.Error("signalsCtx does not call requireSignals, so every Signals resolver answers with the suite switched off")
		}
	}
	if !found {
		t.Fatal("signalsCtx not found - the gate chain moved and this test no longer proves anything")
	}
}
