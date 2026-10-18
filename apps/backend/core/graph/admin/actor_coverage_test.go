package admin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
)

// The admin API serves integration tokens as well as people, so a resolver that refuses a token has to say so on purpose
var humanOnly = map[string]string{
	"Me":                         "the signed-in admin's own record",
	"ChangePassword":             "a person's own credential",
	"AdminTotpSetup":             "a person's own second factor",
	"AdminTotpEnable":            "a person's own second factor",
	"AdminTotpDisable":           "a person's own second factor",
	"LogoutOtherDevices":         "a person's own sessions",
	"MyPreferences":              "preferences are kept per admin",
	"SetMyPreference":            "preferences are kept per admin",
	"notifScope":                 "the inbox and its read state are kept per admin",
	"MarkNotificationRead":       "the inbox and its read state are kept per admin",
	"chatHistory":                "chat history is kept per admin",
	"SignalsChat":                "a conversational UI whose cost is capped per admin",
	"MintBlogPostPreviewToken":   "a draft preview link opened by a person in a browser",
	"MintCMSPagePreviewToken":    "a draft preview link opened by a person in a browser",
	"ClaimCMSPage":               "assigns the page to the caller as its editor",
	"MasqueradeCustomer":         "a support person acting as a shopper",
	"UpdateAdmin":                "admin management compares against the caller's own grants",
	"DeleteAdmin":                "admin management compares against the caller's own grants",
	"requireGrantable":           "a grant ceiling is the caller's own grants",
	"requireRolesGrantable":      "a grant ceiling is the caller's own grants",
	"requireOutranks":            "rank is measured against the caller's own roles",
	"requireOutranksRoleMembers": "rank is measured against the caller's own roles",
	"requireRoleEditable":        "rank is measured against the caller's own roles",
	"hasGlobalPermission":        "the admin branch of Channels - tokens take tokenChannels",
	"Channels":                   "the admin branch only - tokens are answered by tokenChannels above it",
}

func TestResolversSayWhetherTheyServeIntegrations(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "generated.go" {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					switch authCall(n) {
					case "RequireAuth":
						t.Errorf("%s: %s calls auth.RequireAuth - use RequireActor to serve tokens too, or RequireAdmin and list it in humanOnly", fset.Position(n.Pos()), fn.Name.Name)
					case "RequireAdmin":
						found[fn.Name.Name] = true
					}
				case *ast.AssignStmt:
					// discarding ok leaves a nil *Claims for a token caller to dereference
					if len(n.Lhs) == 2 && len(n.Rhs) == 1 && isBlank(n.Lhs[1]) {
						if c, ok := n.Rhs[0].(*ast.CallExpr); ok && authCall(c) == "AdminFromContext" {
							t.Errorf("%s: %s ignores whether an admin is present - check ok, or use RequireAdmin", fset.Position(n.Pos()), fn.Name.Name)
						}
					}
				}
				return true
			})
		}
	}
	for name := range found {
		if _, ok := humanOnly[name]; !ok {
			t.Errorf("%s refuses integration tokens through auth.RequireAdmin but isn't in humanOnly - add it with the reason, or serve tokens with RequireActor", name)
		}
	}
	for name := range humanOnly {
		if !found[name] {
			t.Errorf("humanOnly lists %s, which no longer calls auth.RequireAdmin - drop the entry", name)
		}
	}
	if t.Failed() {
		t.Logf("human-only today: %v", slices.Sorted(maps.Keys(found)))
	}
}

func authCall(c *ast.CallExpr) string {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "auth" {
		return ""
	}
	return sel.Sel.Name
}

func isBlank(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "_"
}
