package front

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The storefront schema is mostly public on purpose, so "every resolver is gated" - right for the admin schema - is wrong here
// What holds instead: a MUTATION establishes who is acting, via customer auth or the cart bearer credential, or it is writing on behalf of nobody

var publicMutations = map[string]string{
	// Pre-auth by definition - this is where a session comes from
	"CustomerRegister":     "creates the account",
	"CustomerLogin":        "issues the session",
	"CustomerRefreshToken": "the refresh cookie is the credential",
	"CustomerLogout":       "clears a session, safe without one",
	"VerifyEmail":          "the emailed token is the credential",
	"RequestPasswordReset": "silent on whether the address exists",
	"ResetPassword":        "the reset token is the credential",

	// Gateway handshakes - they resolve a cart and return a client token, touching no customer record

	// Anonymous by design - rate-limited elsewhere, nothing account-shaped
	"PresignWebFormUpload":           "issues a scoped upload URL for a public form",
	"RequestProductOptionFileUpload": "issues a scoped upload URL for a cart line",
	"SubmitWebForm":                  "public form submission, rate-limited and scoped to the resolved store",
	"CreateGuestCart":                "mints the anonymous cart whose uuid every later call presents as the credential",

	// Guest contribution is the feature - the service decides whether an anonymous author is allowed, and both land moderated rather than published
	"SubmitProductReview":   "guests allowed only while reviews/policy/require_authentication is off - enforced in SubmitReview",
	"SubmitProductQuestion": "guest questions are a feature, name required, queued for moderation",

	// Reads gateway truth for one order and captures only that order's own authorized amount - the unguessable uuidv7 is what a guest receipt already leans on
	"FinalizeOrderPayment": "no session by design, and it cannot be aimed at another account",
}

// Most front field resolvers serve catalog, review or CMS data that is public by design, so demanding a gate on
// them would be 30-odd entries of noise - the schema is what decides what a public caller may ask for
// The narrow slice that matters is a field hanging off a CUSTOMER-SCOPED parent, where the parent object's own
// credential is the only thing between a caller and another person's data
var customerScopedParents = map[string]bool{"Customer": true, "Order": true}

// Every field resolver on one of those parents, with what makes it safe - a new one fails until someone says
var customerScopedFields = map[string]string{
	// Re-checks the parent id against the caller, so a Customer for anyone else yields nothing
	"Customer.Consents":           "returns empty unless obj.ID is the authenticated caller",
	"Customer.StoreCreditBalance": "returns 0 unless obj.ID is the authenticated caller",

	// An Order only reaches this surface through myOrder (customer session) or guestOrder (uuid + email, and
	// customer_id IS NULL), so the parent IS the credential and these read that same order's own rows
	"Order.Returns":             "RMAs on the parent order",
	"Order.ReturnableLines":     "remaining returnable quantities on the parent order",
	"Order.TrackingURL":         "formats a URL from the parent's carrier and tracking number",
	"Order.AmountDue":           "arithmetic on the parent order's own totals",
	"Order.PaymentInstructions": "offline-payment copy from config, keyed by the parent's channel and provider",
}

type funcInfo struct {
	identifies bool
	calls      []string
	// Set when the body is a generated pluginhost.Do delegation - the caller check then lives in that
	// plugin's own resolver, and authz_plugin_test.go follows it there rather than trusting it
	pluginSlug string
}

func TestEveryFrontMutationEstablishesACaller(t *testing.T) {
	funcs, mutations := parseFrontFuncs(t)

	for name := range mutations {
		if reason, ok := publicMutations[name]; ok {
			if reason == "" {
				t.Errorf("%s is allowlisted with no reason", name)
			}
			continue
		}
		if identifies(name, funcs, 0, map[string]bool{}) {
			continue
		}
		// A plugin mutation names its caller on the far side of the delegation - follow it, so a
		// plugin is held to this rather than exempted from it
		if slug := funcs[name].pluginSlug; slug != "" {
			if !pluginIdentifies(t, slug, name, 0, map[string]bool{}) {
				t.Errorf("front mutation %s is served by plugin %q, whose resolver establishes no caller - take sdk.CustomerID and refuse a nil in the plugin", name, slug)
			}
			continue
		}
		t.Errorf("front mutation %s establishes no caller - require customer auth, resolve the cart, or add it to publicMutations with a reason", name)
	}
}

// A stale allowlist is how a guard gets dropped later without anyone noticing
func TestPublicMutationAllowlistIsCurrent(t *testing.T) {
	funcs, mutations := parseFrontFuncs(t)

	for name := range publicMutations {
		if _, ok := mutations[name]; !ok {
			t.Errorf("publicMutations lists %s, which is no longer a front mutation - drop the entry", name)
			continue
		}
		if identifies(name, funcs, 0, map[string]bool{}) {
			t.Errorf("%s is allowlisted as public but now identifies its caller - drop the entry so the check is enforced", name)
		}
	}
}

func TestEveryCustomerScopedFrontFieldIsAccountedFor(t *testing.T) {
	fields := parseFrontFieldResolvers(t)

	seen := 0
	for key := range fields {
		parent, _, _ := strings.Cut(key, ".")
		if !customerScopedParents[parent] {
			continue
		}
		seen++
		reason, ok := customerScopedFields[key]
		if !ok {
			t.Errorf("front field resolver %s hangs off a customer-scoped parent but is not accounted for - "+
				"check obj.ID against the caller, or record in customerScopedFields what makes the parent's credential enough", key)
			continue
		}
		if reason == "" {
			t.Errorf("%s is listed in customerScopedFields with no reason", key)
		}
	}
	if seen == 0 {
		t.Fatal("found no field resolvers on a customer-scoped parent - the parser or customerScopedParents is wrong")
	}
}

// A stale entry is how this list stops describing the code
func TestCustomerScopedFieldListIsCurrent(t *testing.T) {
	fields := parseFrontFieldResolvers(t)

	for key := range customerScopedFields {
		if _, ok := fields[key]; !ok {
			t.Errorf("customerScopedFields lists %s, which is no longer a front field resolver - drop the entry", key)
			continue
		}
		if parent, _, _ := strings.Cut(key, "."); !customerScopedParents[parent] {
			t.Errorf("customerScopedFields lists %s, whose parent is no longer customer-scoped - drop the entry", key)
		}
	}
}

// Field resolvers keyed as Type.Field - the by-name map would merge same-named methods on different parents
func parseFrontFieldResolvers(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	fset := token.NewFileSet()
	out := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if key := frontFieldResolverKey(fn); key != "" {
				out[key] = true
			}
		}
	}
	if len(out) < 25 {
		t.Fatalf("found only %d field resolvers - the parser is not seeing the package", len(out))
	}
	return out
}

// "Order.Returns" for a field resolver, empty for entry points and plain funcs
func frontFieldResolverKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 || !fn.Name.IsExported() {
		return ""
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return ""
	}
	ident, ok := star.X.(*ast.Ident)
	if !ok {
		return ""
	}
	switch ident.Name {
	case "queryResolver", "mutationResolver", "subscriptionResolver":
		return ""
	}
	base, found := strings.CutSuffix(ident.Name, "Resolver")
	if !found || base == "" { // the root *Resolver and its exported helpers are not schema fields
		return ""
	}
	return strings.ToUpper(base[:1]) + base[1:] + "." + fn.Name.Name
}

func identifies(name string, funcs map[string]funcInfo, depth int, seen map[string]bool) bool {
	if depth > 4 || seen[name] {
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
		if identifies(callee, funcs, depth+1, seen) {
			return true
		}
	}
	return false
}

func parseFrontFuncs(t *testing.T) (funcs map[string]funcInfo, mutations map[string]bool) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	fset := token.NewFileSet()
	funcs = map[string]funcInfo{}
	mutations = map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			info := funcs[fn.Name.Name]
			info.identifies = info.identifies || bodyIdentifies(fn.Body)
			info.calls = append(info.calls, bodyCalls(fn.Body)...)
			if slug := delegatedPluginSlug(fn.Body); slug != "" {
				info.pluginSlug = slug
			}
			funcs[fn.Name.Name] = info

			if isFrontMutation(fn) {
				mutations[fn.Name.Name] = true
			}
		}
	}
	if len(mutations) < 30 {
		t.Fatalf("found only %d front mutations - the parser is not seeing the package", len(mutations))
	}
	return funcs, mutations
}

func isFrontMutation(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) != 1 || !fn.Name.IsExported() {
		return false
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "mutationResolver"
}

// Customer identity, or the cart bearer credential checked against the resolved store
// A bare auth.OptionalCustomerID establishes nothing - it hands back nil for an anonymous caller - so it counts only through refusesAnonymous, which wants the nil check that actually returns
func bodyIdentifies(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "auth" && strings.HasPrefix(sel.Sel.Name, "RequireCustomer") {
			found = true
		}
		// The cart uuid and the signed voter token are the other two credentials on this surface - both resolve fail-closed, so reaching either is as good as customer auth
		if sel.Sel.Name == "assertCartOnCurrentStore" || sel.Sel.Name == "voterFromContext" {
			found = true
		}
		return true
	})
	return found || refusesAnonymous(body)
}

// The hand-rolled equivalent of RequireCustomer: take the optional id, then return early when it came back nil
// Passing auth.OptionalCustomerID straight into a service call is the OTHER idiom and deliberately doesn't count - there the cart uuid is the credential and the id is only an ownership discriminator
func refusesAnonymous(body *ast.BlockStmt) bool {
	optional := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 || !isOptionalCustomerID(assign.Rhs[0]) {
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
	refuses := false
	ast.Inspect(body, func(n ast.Node) bool {
		stmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		cmp, ok := stmt.Cond.(*ast.BinaryExpr)
		if !ok || cmp.Op != token.EQL {
			return true
		}
		lhs, lok := cmp.X.(*ast.Ident)
		rhs, rok := cmp.Y.(*ast.Ident)
		if !lok || !rok || !optional[lhs.Name] || rhs.Name != "nil" {
			return true
		}
		for _, s := range stmt.Body.List {
			if _, isReturn := s.(*ast.ReturnStmt); isReturn {
				refuses = true
			}
		}
		return true
	})
	return refuses
}

func isOptionalCustomerID(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "OptionalCustomerID" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "auth"
}

func bodyCalls(body *ast.BlockStmt) []string {
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			out = append(out, fn.Name)
		case *ast.SelectorExpr:
			if recv, ok := fn.X.(*ast.Ident); ok && recv.Name == "r" {
				out = append(out, fn.Sel.Name)
			}
		}
		return true
	})
	return out
}
