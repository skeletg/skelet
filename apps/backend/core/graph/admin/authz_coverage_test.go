package admin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The admin auth middleware is non-blocking, so a resolver with no guard answers ANONYMOUS callers - anything genuinely public belongs in publicResolvers with a reason, not silence

// Resolver name -> why it needs no guard, asserted to still exist and still be ungated
var publicResolvers = map[string]string{
	"AdminLogin":                "pre-auth by definition - this is where claims come from",
	"RequestAdminPasswordReset": "pre-auth, and silent on whether the address exists",
	"ResetAdminPassword":        "pre-auth, the reset token is the credential",
	"RefreshToken":              "pre-auth, the refresh cookie is the credential",
	"Logout":                    "clears the session, safe to call without one",

	"DevAdmins":      "refuses unless r.DevMode",
	"DevImpersonate": "refuses unless r.DevMode",

	// The gate lives in the plugin's own resolver, reached here only through pluginhost.Do
}

// A field resolver hangs off a parent the caller already had to be authorized to hold, so demanding its own guard on each one would be noise - the risk is the OTHER direction: one added later that quietly reads a domain its parent's gate never covered (inventory through a product, say, or payouts through an order) So every ungated field resolver is written down with why the parent's gate is enough, and a new one fails this test until someone makes that call deliberately
var parentGatedFieldResolvers = map[string]string{
	// Same aggregate as the parent - these read the parent row's own columns or its directly owned children
	"Order.Fulfillments":                 "shipments of this order, under orders.view with the parent",
	"Order.PaymentTransactions":          "this order's own ledger rows, no gateway credentials among them",
	"Order.Returns":                      "RMAs opened against this order",
	"Order.TaxLines":                     "the parent order's own tax breakdown",
	"Order.Shipments":                    "the packages the order was placed as - its own rows",
	"Order.TrackingURL":                  "formats a URL from the parent's carrier and tracking number",
	"Order.AppliedPromotions":            "which codes this order consumed - order history, not the promotion catalogue",
	"Category.ProductCount":              "a count over the parent category",
	"Category.FacetOverrides":            "the parent category's own filter-presentation rows",
	"Collection.FacetOverrides":          "the parent collection's own filter-presentation rows",
	"Category.Content":                   "the parent category's own translation rows",
	"Collection.Content":                 "the parent collection's own translation rows",
	"Product.Content":                    "the parent product's own translation rows",
	"Category.Listings":                  "filters the listings already on the parent through the caller's channel scope, fetching nothing",
	"Attribute.Descriptions":             "the parent attribute's own localized copy",
	"AttributeValue.Descriptions":        "the parent value's own localized copy",
	"CMSPageType.PageCount":              "a count over the parent page type",
	"Product.Downloads":                  "digital assets attached to the parent product",
	"Product.ProductOptions":             "the parent product's own option set",
	"Product.OptionRules":                "the parent product's own option rules",
	"Product.OptionTemplates":            "names and counts of the templates the parent product uses",
	"Product.ConfiguratorLayout":         "one setting on the parent product",
	"OptionTemplate.ProductOptions":      "the parent template's own option set",
	"OptionTemplate.OptionRules":         "the parent template's own option rules",
	"OptionTemplate.Products":            "the products using the parent template, which catalog.view already lists",
	"OptionTemplate.ProductTypes":        "the product types handing the parent template on - global configuration catalog.view lists",
	"Product.TypeOptionTemplates":        "names and counts of the templates the parent product's type hands it",
	"Product.PartOf":                     "reads memberships off the parent, no service call",
	"PromotionRule.Codes":                "coupon codes belonging to the parent rule",
	"PromotionRule.Contents":             "the parent rule's own localized copy",
	"PromotionRule.LabelMedia":           "formats a URL from a field on the parent",
	"PromotionRule.BannerMedia":          "formats a URL from a field on the parent",
	"ProductMemberships.Configurable":    "reads the parent membership struct, no service call",
	"ProductMemberships.Bundles":         "reads the parent membership struct, no service call",
	"ProductMemberships.GroupedProducts": "reads the parent membership struct, no service call",

	// Global reference data - catalogues an admin needs to render the parent at all, and secret from nobody
	"Country.Regions":                "global country/region catalogue",
	"InventoryLocation.Country":      "global country catalogue, not inventory data",
	"CategoryChannelListing.Channel": "channel name and currency, needed to label the parent listing",
	"ProductType.ReviewQuestions":    "the parent type's curated question set - configuration, not review content",
	"ProductType.OptionTemplates":    "names and counts of the templates the parent type hands its products",

	// Same subject as the parent, which is the thing customers.view already grants
	"Customer.Consents": "marketing opt-in state for the parent customer only, keyed by its id",

	// Already public: the storefront serves both of these to anonymous callers for the same product,
	// so gating the admin copy on inventory.view would move no data out of reach
	"Product.SalableQuantity":       "the front schema exposes salableQuantity unauthenticated",
	"Product.InStock":               "the front schema exposes inStock unauthenticated",
	"ProductChannelListing.InStock": "Product.InStock pinned to a listing the parent already filtered to the admin's permitted channels",
}

type funcInfo struct {
	guards    bool     // calls auth.Require* directly
	authGates []string // which ones - a bare RequirePermission is a weaker gate than the channel-aware ones
	calls     []string // package funcs and resolver methods it invokes
	// Set when the body is a generated pluginhost.Do delegation - the guard then lives in that
	// plugin's own resolver, and authz_plugin_test.go follows it there rather than trusting it
	pluginSlug string
}

func TestEveryAdminResolverIsGated(t *testing.T) {
	funcs, resolvers := parsePackageFuncs(t)

	for name, kind := range resolvers {
		if reason, ok := publicResolvers[name]; ok {
			if reason == "" {
				t.Errorf("%s is allowlisted with no reason", name)
			}
			continue
		}
		if gates(name, funcs, 0, map[string]bool{}) {
			continue
		}
		// A plugin field carries no guard HERE by construction - follow the delegation and demand one
		// on the far side, so a plugin is held to the same standard rather than exempted from it
		if slug := funcs[name].pluginSlug; slug != "" {
			if !pluginGates(t, slug, name) {
				t.Errorf("%s resolver %s is served by plugin %q, whose resolver has no sdk.Require*/PermittedChannels guard - gate it in the plugin", kind, name, slug)
			}
			continue
		}
		t.Errorf("%s resolver %s has no permission guard - add an auth.Require* check, or allowlist it in publicResolvers with a reason", kind, name)
	}
}

// A stale allowlist is how a guard gets dropped later - every entry must still be a real, still-ungated resolver
func TestPublicResolverAllowlistIsCurrent(t *testing.T) {
	funcs, resolvers := parsePackageFuncs(t)

	for name := range publicResolvers {
		if _, ok := resolvers[name]; !ok {
			t.Errorf("publicResolvers lists %s, which is no longer a Query/Mutation resolver - drop the entry", name)
			continue
		}
		if gates(name, funcs, 0, map[string]bool{}) {
			t.Errorf("%s is allowlisted as public but now guards itself - drop the entry so the guard is enforced", name)
		}
	}
}

func TestEveryAdminFieldResolverIsAccountedFor(t *testing.T) {
	funcs, _ := parsePackageFuncs(t)
	fields := parseFieldResolvers(t)

	for key, info := range fields {
		if info.guards {
			continue
		}
		reached := false
		for _, callee := range info.calls {
			if gates(callee, funcs, 1, map[string]bool{}) {
				reached = true
				break
			}
		}
		if reached {
			continue
		}
		if reason, ok := parentGatedFieldResolvers[key]; ok {
			if reason == "" {
				t.Errorf("%s is listed in parentGatedFieldResolvers with no reason", key)
			}
			continue
		}
		t.Errorf("field resolver %s is neither gated nor accounted for - add an auth.Require* check if it reads "+
			"beyond what the parent's gate covers, or record in parentGatedFieldResolvers why the parent's gate is enough", key)
	}
}

// A stale entry is how this list stops describing the code - every one must still be a real, still-ungated field resolver
func TestParentGatedFieldResolverListIsCurrent(t *testing.T) {
	funcs, _ := parsePackageFuncs(t)
	fields := parseFieldResolvers(t)

	for key := range parentGatedFieldResolvers {
		info, ok := fields[key]
		if !ok {
			t.Errorf("parentGatedFieldResolvers lists %s, which is no longer a field resolver - drop the entry", key)
			continue
		}
		if info.guards {
			t.Errorf("%s now guards itself - drop the entry so the guard is enforced", key)
			continue
		}
		for _, callee := range info.calls {
			if gates(callee, funcs, 1, map[string]bool{}) {
				t.Errorf("%s now reaches a guard via %s - drop the entry so the guard is enforced", key, callee)
				break
			}
		}
	}
}

// Field resolvers keyed as Type.Field, each with its OWN guard/call info - the by-name map would merge
// same-named methods on different parents and hide exactly the case this is meant to catch
func parseFieldResolvers(t *testing.T) map[string]funcInfo {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	fset := token.NewFileSet()
	out := map[string]funcInfo{}
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
			key := fieldResolverKey(fn)
			if key == "" {
				continue
			}
			info := out[key]
			gates := bodyAuthGates(fn.Body)
			info.guards = info.guards || len(gates) > 0
			info.authGates = append(info.authGates, gates...)
			info.calls = append(info.calls, bodyCalls(fn.Body)...)
			out[key] = info
		}
	}
	if len(out) < 20 {
		t.Fatalf("found only %d field resolvers - the parser is not seeing the package", len(out))
	}
	return out
}

// "Order.Fulfillments" for a field resolver, empty for query/mutation/subscription entry points and plain funcs
func fieldResolverKey(fn *ast.FuncDecl) string {
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

// Guards directly, or reaches a guard through the package helpers it calls
func gates(name string, funcs map[string]funcInfo, depth int, seen map[string]bool) bool {
	if depth > 4 || seen[name] {
		return false
	}
	seen[name] = true
	fn, ok := funcs[name]
	if !ok {
		return false
	}
	if fn.guards {
		return true
	}
	for _, callee := range fn.calls {
		if gates(callee, funcs, depth+1, seen) {
			return true
		}
	}
	return false
}

// funcs is every declaration by name, resolvers just the Query/Mutation entry points
func parsePackageFuncs(t *testing.T) (funcs map[string]funcInfo, resolvers map[string]string) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	fset := token.NewFileSet()
	funcs = map[string]funcInfo{}
	resolvers = map[string]string{}
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
			info := funcs[fn.Name.Name] // a name can be both a helper and a method - merge
			gates := bodyAuthGates(fn.Body)
			info.guards = info.guards || len(gates) > 0
			info.authGates = append(info.authGates, gates...)
			info.calls = append(info.calls, bodyCalls(fn.Body)...)
			if slug := delegatedPluginSlug(fn.Body); slug != "" {
				info.pluginSlug = slug
			}
			funcs[fn.Name.Name] = info

			if kind := resolverKind(fn); kind != "" {
				resolvers[fn.Name.Name] = kind
			}
		}
	}
	if len(resolvers) < 100 {
		t.Fatalf("found only %d Query/Mutation resolvers - the parser is not seeing the package", len(resolvers))
	}
	return funcs, resolvers
}

// "query" or "mutation" for an entry point, empty for field resolvers and plain funcs
func resolverKind(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	if !fn.Name.IsExported() { // unexported methods on the same receiver are helpers, not schema fields
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
	case "queryResolver":
		return "query"
	case "mutationResolver":
		return "mutation"
	}
	return ""
}

// Every auth.Require* the body calls, in source order - only these count as a gate; LoadPermittedChannels deliberately returns Bypass for a caller with no claims and cannot fail, so accepting it here passed a resolver that answered anonymous callers with every channel's rows
func bodyAuthGates(body *ast.BlockStmt) []string {
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "auth" {
			return true
		}
		if strings.HasPrefix(sel.Sel.Name, "Require") {
			out = append(out, sel.Sel.Name)
		}
		return true
	})
	return out
}

// Only bare helper(...) and r.Method(...) - service calls like r.OrderSvc.Get would just add noise
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
