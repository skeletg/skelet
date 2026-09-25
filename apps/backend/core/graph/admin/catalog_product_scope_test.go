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

// Options, values, bundle structure, grouped members, kit components, media and variant mode are ONE row per product, shared by every channel the product is listed in - so a bare any-scope catalog.manage there lets an admin scoped to channel A edit a product listed only in channel B. That was a real gap across 25 mutations Every mutation declared in these files must reach a channel-aware gate; a bare RequirePermission fails here
var productRowResolverFiles = []string{
	"catalog_product_media.resolvers.go",
	"catalog_product_options.resolvers.go",
	"catalog_product_structure.resolvers.go",
	"catalog_products.resolvers.go",
	"product_variant_mode.resolvers.go",
}

// Reached directly or through the requireCatalogManage* helpers - the first two widen to every listing channel,
// RequireChannelPermission is for the per-listing mutations that name their channel, and Global outranks both
var channelAwareGates = map[string]bool{
	"RequireAllListedChannelsPermission": true,
	"RequireChannelPermission":           true,
	"RequireGlobalPermission":            true,
}

// Product TYPES are global configuration shared by every channel, so their writes need the global grant - see globalOnlyResolverFiles
func TestProductRowMutationsAreChannelScoped(t *testing.T) {
	funcs, resolvers := parsePackageFuncs(t)

	checked := 0
	for _, file := range productRowResolverFiles {
		names := mutationsDeclaredIn(t, file)
		if len(names) == 0 {
			t.Errorf("%s declares no mutations - was it renamed or split? update productRowResolverFiles", file)
			continue
		}
		for _, name := range names {
			if _, ok := resolvers[name]; !ok {
				continue // an unexported helper on the same receiver
			}
			checked++
			if reachesChannelAwareGate(name, funcs, 0, map[string]bool{}) {
				continue
			}
			t.Errorf("%s (%s) gates on an any-scope permission only - the rows it writes render on every channel "+
				"listing the product, so gate it with r.requireCatalogManageOnAllListingChannels (or the "+
				"requireCatalogManageFor<child> helper matching the id it takes) before the write", name, file)
		}
	}
	if checked < 50 {
		t.Fatalf("only checked %d mutations - the parser is not seeing the product resolvers", checked)
	}
}

func reachesChannelAwareGate(name string, funcs map[string]funcInfo, depth int, seen map[string]bool) bool {
	if depth > 4 || seen[name] {
		return false
	}
	seen[name] = true
	fn, ok := funcs[name]
	if !ok {
		return false
	}
	for _, gate := range fn.authGates {
		if channelAwareGates[gate] {
			return true
		}
	}
	for _, callee := range fn.calls {
		if reachesChannelAwareGate(callee, funcs, depth+1, seen) {
			return true
		}
	}
	return false
}

func mutationsDeclaredIn(t *testing.T, file string) []string {
	t.Helper()
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("%s is listed in productRowResolverFiles but not readable: %v", file, err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var out []string
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		ident, ok := star.X.(*ast.Ident)
		if !ok || ident.Name != "mutationResolver" || !fn.Name.IsExported() {
			continue
		}
		out = append(out, fn.Name.Name)
	}
	return out
}

// A helper that looked up the child row AFTER the write would gate nothing, so each one is asserted to take
// the child's id and reach the product gate - the names are the contract the mutations above rely on
func TestChildRowGateHelpersReachTheProductGate(t *testing.T) {
	funcs, _ := parsePackageFuncs(t)

	for _, name := range []string{
		"requireCatalogManageForProductOption",
		"requireCatalogManageForProductOptionValue",
		"requireCatalogManageForBundleOption",
		"requireCatalogManageForBundleSelection",
		"requireCatalogManageForGroupedMember",
		"requireCatalogManageForKitComponent",
	} {
		fn, ok := funcs[name]
		if !ok {
			t.Errorf("%s is gone - the mutations that call it now gate on nothing; keep it or re-gate them", name)
			continue
		}
		if !strings.Contains(strings.Join(fn.calls, ","), "requireCatalogManage") {
			t.Errorf("%s no longer reaches the product-global gate", name)
		}
	}
}

// Content shared across channels: every mutation gates on each channel the row renders on, on the channel it names, or globally
var sharedContentResolverFiles = []string{
	"ai.resolvers.go",
	"blog.resolvers.go",
	"catalog_attributes.resolvers.go",
	"catalog_categories.resolvers.go",
	"cms_pages.resolvers.go",
	"cms_revisions.resolvers.go",
	"cms_schema.resolvers.go",
	"cms_workflow.resolvers.go",
	"inventory.resolvers.go",
	"media.resolvers.go",
	"menus.resolvers.go",
}

// A new row is listed nowhere yet, and an upload adds an asset without changing any existing one
var unlistedCreates = map[string]bool{
	"CreateBlogPost":          true,
	"CreateCMSPage":           true,
	"CreateCategory":          true,
	"CreateMenu":              true,
	"CreateMediaDirectory":    true,
	"RequestMediaAssetUpload": true,
	"ConfirmMediaAssetUpload": true,
	"GenerateMediaAltText":    true,
}

func TestSharedContentMutationsAreChannelScoped(t *testing.T) {
	funcs, resolvers := parsePackageFuncs(t)
	checked := 0
	for _, file := range sharedContentResolverFiles {
		for _, name := range mutationsDeclaredIn(t, file) {
			if _, ok := resolvers[name]; !ok || unlistedCreates[name] {
				continue
			}
			checked++
			if !reachesChannelAwareGate(name, funcs, 0, map[string]bool{}) {
				t.Errorf("%s (%s) gates on an any-scope permission only - the row it writes renders on channels the caller may not manage", name, file)
			}
		}
	}
	if checked < 100 {
		t.Fatalf("only checked %d mutations - the parser is not seeing the content resolvers", checked)
	}
}

// Install-wide configuration: a channel-scoped grant never reaches it, per docs/permissions.md
var globalOnlyResolverFiles = []string{
	"catalog_collections.resolvers.go",
	"catalog_product_types.resolvers.go",
	"cms_blocks.resolvers.go",
	"countries.resolvers.go",
	"currencies.resolvers.go",
	"locales.resolvers.go",
}

func TestGlobalOnlyMutationsNeedTheGlobalGrant(t *testing.T) {
	funcs, resolvers := parsePackageFuncs(t)
	for _, file := range globalOnlyResolverFiles {
		names := mutationsDeclaredIn(t, file)
		if len(names) == 0 {
			t.Errorf("%s declares no mutations - update globalOnlyResolverFiles", file)
		}
		for _, name := range names {
			if _, ok := resolvers[name]; !ok {
				continue
			}
			if !reachesGate(name, "RequireGlobalPermission", funcs, 0, map[string]bool{}) {
				t.Errorf("%s (%s) must gate on RequireGlobalPermission - the row is shared by every channel", name, file)
			}
		}
	}
}

func reachesGate(name, gate string, funcs map[string]funcInfo, depth int, seen map[string]bool) bool {
	if depth > 4 || seen[name] {
		return false
	}
	seen[name] = true
	fn, ok := funcs[name]
	if !ok {
		return false
	}
	if slices.Contains(fn.authGates, gate) {
		return true
	}
	for _, callee := range fn.calls {
		if reachesGate(callee, gate, funcs, depth+1, seen) {
			return true
		}
	}
	return false
}
