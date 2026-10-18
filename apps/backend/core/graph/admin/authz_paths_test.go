package admin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The admin middleware answers anonymous callers, so every resolver path that returns data has to pass a checked permission gate first
var conditionallyGatedResolvers = map[string]string{}

var authOnlyGuards = map[string]bool{"RequireAuth": true, "RequireActor": true, "RequireAdmin": true}

// Resolvers any signed-in caller may call, where one of authOnlyGuards is the whole gate
var authOnlyResolvers = map[string]string{
	"Channels":       "the scope selector needs the channel roster, filtered to the caller's grants",
	"ResolveURL":     "resolves a storefront path for the editor, the data behind it is gated where it is read",
	"MyPreferences":  "the caller's own preference rows",
	"AiStatus":       "whether the assistant is configured, so the nav can show it",
	"InventoryMode":  "the install's inventory mode, read by every admin screen that shows stock",
	"ServerFeatures": "feature switches the nav gates on",

	"Me":                 "the caller's own admin row and sessions",
	"ChangePassword":     "the caller's own password, behind the current one",
	"SetMyPreference":    "the caller's own preference rows",
	"LogoutOtherDevices": "the caller's own refresh sessions",
	"AdminTotpSetup":     "the caller's own second factor",
	"AdminTotpEnable":    "the caller's own second factor",
	"AdminTotpDisable":   "the caller's own second factor, behind a current code",
	"AdminActions":       "filters the action catalogue through the caller's grants",
	"SignalsInsights":    "digestPermsFor narrows every section to the caller's grants on the channel",
	"SignalsChat":        "digestPermsFor narrows every section to the caller's grants on the channel",

	"SaveChatConversation":   "own rows only, keyed by the caller's admin id",
	"DeleteChatConversation": "own rows only, keyed by the caller's admin id",
	"ChatConversations":      "own rows only, keyed by the caller's admin id",
	"ChatConversation":       "own rows only, keyed by the caller's admin id",
}

func TestEveryAdminResolverGatesEveryPath(t *testing.T) {
	bodies, resolvers := parseResolverBodies(t, ".")
	for name := range resolvers {
		if _, ok := conditionallyGatedResolvers[name]; ok {
			continue
		}
		if _, ok := publicResolvers[name]; ok {
			continue
		}
		body := bodies[name]
		if body == nil || delegatedPluginSlug(body) != "" {
			continue
		}
		_, authOnly := authOnlyResolvers[name]
		if leak, ok := firstUngatedReturn(body, bodies, authOnly); ok {
			t.Errorf("resolver %s can return data before a checked permission gate runs (%s) - bind the gate's error and return on it before the first return that hands anything back", name, leak)
		}
	}
}

func TestConditionallyGatedAllowlistIsCurrent(t *testing.T) {
	bodies, resolvers := parseResolverBodies(t, ".")
	for name, reason := range conditionallyGatedResolvers {
		if reason == "" {
			t.Errorf("%s is allowlisted with no reason", name)
		}
		if _, ok := resolvers[name]; !ok {
			t.Errorf("conditionallyGatedResolvers lists %s, which is no longer a resolver", name)
			continue
		}
		if _, ok := firstUngatedReturn(bodies[name], bodies, false); !ok {
			t.Errorf("%s now gates every path - drop it from conditionallyGatedResolvers", name)
		}
	}
}

func TestAuthOnlyAllowlistIsCurrent(t *testing.T) {
	bodies, resolvers := parseResolverBodies(t, ".")
	for name, reason := range authOnlyResolvers {
		if reason == "" {
			t.Errorf("%s is allowlisted with no reason", name)
		}
		if _, ok := resolvers[name]; !ok {
			t.Errorf("authOnlyResolvers lists %s, which is no longer a resolver", name)
			continue
		}
		if _, ok := firstUngatedReturn(bodies[name], bodies, false); !ok {
			t.Errorf("%s now holds a permission gate on every path - drop it from authOnlyResolvers", name)
		}
	}
}

// Fixtures named Leak* must be flagged and Gated* must pass, pinning what the walker accepts as a gate
func TestPathWalkerFixtures(t *testing.T) {
	bodies, resolvers := parseResolverBodies(t, "testdata/authzpaths")
	if len(resolvers) < 8 {
		t.Fatalf("found only %d fixture resolvers", len(resolvers))
	}
	for name := range resolvers {
		leak, flagged := firstUngatedReturn(bodies[name], bodies, false)
		switch {
		case strings.HasPrefix(name, "Leak") && !flagged:
			t.Errorf("fixture %s leaks but the walker passed it", name)
		case strings.HasPrefix(name, "Gated") && flagged:
			t.Errorf("fixture %s is gated but the walker flagged it (%s)", name, leak)
		}
	}
}

func parseResolverBodies(t *testing.T, dir string) (map[string]*ast.BlockStmt, map[string]bool) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	bodies := map[string]*ast.BlockStmt{}
	resolvers := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if _, dup := bodies[fn.Name.Name]; !dup {
				bodies[fn.Name.Name] = fn.Body
			}
			if resolverKind(fn) != "" {
				bodies[fn.Name.Name] = fn.Body
				resolvers[fn.Name.Name] = true
			}
		}
	}
	return bodies, resolvers
}

// The first return that can hand back data while no gate has run on its path, or false when every path is gated
func firstUngatedReturn(body *ast.BlockStmt, helpers map[string]*ast.BlockStmt, allowAuth bool) (string, bool) {
	w := pathWalker{helpers: helpers, seen: map[string]bool{}, allowAuth: allowAuth}
	_, leak := w.block(body.List, false, nil)
	return leak, leak != ""
}

type pathWalker struct {
	helpers   map[string]*ast.BlockStmt
	seen      map[string]bool
	allowAuth bool
}

// pending holds the names bound to a gate's error that no if-check has consumed yet
func (w pathWalker) block(stmts []ast.Stmt, gated bool, outer map[string]bool) (bool, string) {
	pending := maps.Clone(outer)
	if pending == nil {
		pending = map[string]bool{}
	}
	for _, s := range stmts {
		g, leak := w.stmt(s, gated, pending)
		if leak != "" {
			return g, leak
		}
		gated = gated || g
	}
	return gated, ""
}

func (w pathWalker) stmt(s ast.Stmt, gated bool, pending map[string]bool) (bool, string) {
	switch s := s.(type) {
	case *ast.ReturnStmt:
		if !gated && !benignReturn(s) && !w.returnsGate(s) {
			return false, "return " + exprString(s)
		}
		return true, "" // this path ends here, gated or refused
	case *ast.IfStmt:
		local := maps.Clone(pending)
		pre := gated
		if s.Init != nil {
			g, leak := w.stmt(s.Init, pre, local)
			if leak != "" {
				return g, leak
			}
			pre = pre || g
		}
		thenPre, elsePre := pre, pre
		switch w.errCheck(s.Cond, local) {
		case gateFails:
			elsePre = true
		case gatePasses:
			thenPre = true
		}
		thenG, leak := w.block(s.Body.List, thenPre, local)
		if leak != "" {
			return false, leak
		}
		elseG := elsePre
		if s.Else != nil {
			g, leak := w.stmt(s.Else, elsePre, local)
			if leak != "" {
				return false, leak
			}
			elseG = g
		}
		return thenG && elseG, ""
	case *ast.BlockStmt:
		return w.block(s.List, gated, pending)
	case *ast.SwitchStmt:
		local := maps.Clone(pending)
		pre := gated
		if s.Init != nil {
			g, _ := w.stmt(s.Init, pre, local)
			pre = pre || g
		}
		all, hasDefault := true, false
		for _, c := range s.Body.List {
			cc := c.(*ast.CaseClause)
			if cc.List == nil {
				hasDefault = true
			}
			g, leak := w.block(cc.Body, pre, local)
			if leak != "" {
				return false, leak
			}
			all = all && g
		}
		return pre || (all && hasDefault), ""
	case *ast.TypeSwitchStmt:
		for _, c := range s.Body.List {
			if _, leak := w.block(c.(*ast.CaseClause).Body, gated, pending); leak != "" {
				return false, leak
			}
		}
		return gated, ""
	case *ast.ForStmt:
		if _, leak := w.block(s.Body.List, gated, pending); leak != "" {
			return false, leak
		}
		return gated, ""
	case *ast.RangeStmt:
		if _, leak := w.block(s.Body.List, gated, pending); leak != "" {
			return false, leak
		}
		return gated, ""
	case *ast.AssignStmt:
		w.bind(s.Lhs, s.Rhs, pending)
	case *ast.DeclStmt:
		gd, ok := s.Decl.(*ast.GenDecl)
		if !ok {
			return false, ""
		}
		for _, spec := range gd.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				lhs := make([]ast.Expr, len(vs.Names))
				for i, n := range vs.Names {
					lhs[i] = n
				}
				w.bind(lhs, vs.Values, pending)
			}
		}
	}
	return false, ""
}

// Records the error variable a gate call binds, and forgets any pending name the assignment overwrites
func (w pathWalker) bind(lhs, rhs []ast.Expr, pending map[string]bool) {
	for _, l := range lhs {
		if id, ok := l.(*ast.Ident); ok {
			delete(pending, id.Name)
		}
	}
	if len(rhs) != 1 || len(lhs) == 0 || !w.isGateCall(rhs[0]) {
		return
	}
	if id, ok := lhs[len(lhs)-1].(*ast.Ident); ok && id.Name != "_" {
		pending[id.Name] = true
	}
}

type gateCheck int

const (
	noCheck gateCheck = iota
	gateFails
	gatePasses
)

// gateFails when the then-branch runs whenever the gate refused, gatePasses when it runs only once the gate allowed
func (w pathWalker) errCheck(e ast.Expr, pending map[string]bool) gateCheck {
	switch e := e.(type) {
	case *ast.ParenExpr:
		return w.errCheck(e.X, pending)
	case *ast.BinaryExpr:
		switch e.Op {
		case token.LOR:
			if w.errCheck(e.X, pending) == gateFails || w.errCheck(e.Y, pending) == gateFails {
				return gateFails
			}
		case token.LAND:
			if w.errCheck(e.X, pending) == gatePasses || w.errCheck(e.Y, pending) == gatePasses {
				return gatePasses
			}
		case token.NEQ, token.EQL:
			if !w.gateErrAgainstNil(e.X, e.Y, pending) && !w.gateErrAgainstNil(e.Y, e.X, pending) {
				return noCheck
			}
			if e.Op == token.NEQ {
				return gateFails
			}
			return gatePasses
		}
	}
	return noCheck
}

func (w pathWalker) gateErrAgainstNil(v, other ast.Expr, pending map[string]bool) bool {
	if id, ok := other.(*ast.Ident); !ok || id.Name != "nil" {
		return false
	}
	if id, ok := v.(*ast.Ident); ok {
		return pending[id.Name]
	}
	return w.isGateCall(v)
}

// A return that hands back a gate's own error, with zero values beside it, is the gate
func (w pathWalker) returnsGate(s *ast.ReturnStmt) bool {
	if len(s.Results) == 0 || !w.isGateCall(s.Results[len(s.Results)-1]) {
		return false
	}
	for _, r := range s.Results[:len(s.Results)-1] {
		if !zeroValue(r) {
			return false
		}
	}
	return true
}

// A direct call to auth.Require*, or to a package helper whose every path gates
func (w pathWalker) isGateCall(e ast.Expr) bool {
	if p, ok := e.(*ast.ParenExpr); ok {
		return w.isGateCall(p.X)
	}
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		if pkg, ok := fn.X.(*ast.Ident); ok && pkg.Name == "auth" && strings.HasPrefix(fn.Sel.Name, "Require") {
			return !authOnlyGuards[fn.Sel.Name] || w.allowAuth
		}
		if recv, ok := fn.X.(*ast.Ident); ok && recv.Name == "r" {
			return w.helperGates(fn.Sel.Name)
		}
	case *ast.Ident:
		return w.helperGates(fn.Name)
	}
	return false
}

// A helper gates when every path through it has passed a checked gate or refused
func (w pathWalker) helperGates(name string) bool {
	if w.seen[name] {
		return false
	}
	body, ok := w.helpers[name]
	if !ok {
		return false
	}
	w.seen[name] = true
	defer delete(w.seen, name)
	gated, leak := w.block(body.List, false, nil)
	return gated && leak == ""
}

func benignReturn(s *ast.ReturnStmt) bool {
	if len(s.Results) == 0 {
		return true
	}
	last := s.Results[len(s.Results)-1]
	for _, r := range s.Results[:len(s.Results)-1] {
		if !zeroValue(r) {
			return false
		}
	}
	if id, ok := last.(*ast.Ident); ok && id.Name == "nil" {
		return len(s.Results) > 1 // a lone nil is a helper answering "allowed" before it checked anything
	}
	return errorish(last)
}

func errorish(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return strings.Contains(strings.ToLower(e.Name), "err")
	case *ast.SelectorExpr:
		return strings.HasPrefix(e.Sel.Name, "Err")
	case *ast.UnaryExpr:
		return errorish(e.X)
	case *ast.CompositeLit:
		return true // &auth.ScopedDenial{...} and friends
	case *ast.CallExpr:
		switch fn := e.Fun.(type) {
		case *ast.Ident:
			return strings.Contains(strings.ToLower(fn.Name), "err")
		case *ast.SelectorExpr:
			n := strings.ToLower(fn.Sel.Name)
			return strings.Contains(n, "err") || n == "new" || n == "join" || n == "wrap"
		}
	}
	return false
}

func zeroValue(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name == "nil" || e.Name == "false"
	case *ast.BasicLit:
		return e.Value == "0" || e.Value == `""`
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		return ok && pkg.Name == "uuid" && e.Sel.Name == "Nil"
	case *ast.UnaryExpr:
		return zeroValue(e.X)
	case *ast.CompositeLit:
		for _, el := range e.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if !zeroValue(kv.Value) {
					return false
				}
				continue
			}
			if !zeroValue(el) {
				return false
			}
		}
		return true
	}
	return false
}

func exprString(s *ast.ReturnStmt) string {
	parts := make([]string, 0, len(s.Results))
	for _, r := range s.Results {
		switch r := r.(type) {
		case *ast.Ident:
			parts = append(parts, r.Name)
		default:
			parts = append(parts, "...")
		}
	}
	return strings.Join(parts, ", ")
}

// Install-wide resources, where a grant held on one channel must never be enough (docs/permissions.md)
var globalOnlySlugs = []string{
	"admins.view", "admins.manage", "audit.view", "scheduler.view", "scheduler.manage", "cache.view", "cache.manage",
	"currencies.manage", "countries.manage", "locales.manage",
}

func TestGlobalOnlySlugsAreNeverGatedPerChannel(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	global := map[string]bool{}
	for _, s := range globalOnlySlugs {
		global[s] = true
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name == "RequireGlobalPermission" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "auth" || !strings.HasPrefix(sel.Sel.Name, "Require") {
				return true
			}
			for _, arg := range call.Args {
				if lit, ok := arg.(*ast.BasicLit); ok && global[strings.Trim(lit.Value, `"`)] {
					t.Errorf("%s: %s gates %s, an install-wide permission - use auth.RequireGlobalPermission", fset.Position(call.Pos()), sel.Sel.Name, lit.Value)
				}
			}
			return true
		})
	}
}
