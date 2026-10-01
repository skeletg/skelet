// Command providerschema publishes the remote-provider wire contract: one JSON Schema and one example request per kind, generated from the adapter structs themselves The payload types are unexported and spread across ~14 consumer packages, so this reads them with go/types rather than reflection - a renamed or deleted type fails the run, which is the drift gate the hand-written docs never had
package main

import (
	"encoding/json"
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// One wire operation - most kinds have exactly one, search has two
type op struct {
	Kind string // remoteprovider kind string
	Name string // file stem; the kind itself for single-op kinds
	Pkg  string // package dir relative to apps/backend
	Req  string // request type as declared
	Resp string // response type as declared
	Doc  string
}

// The registry the generator walks - adding a kind means adding a line here, and a typo in
// a type name is a build-time failure rather than a silently missing schema
var ops = []op{
	{"shipping_carrier", "shipping_carrier", "internal/shipping", "remoteRateRequest", "remoteRateResponse",
		"Rate a basket to a destination. Returns the shipping options the shopper picks from."},
	{"tax_provider", "tax_provider", "internal/order", "remoteTaxRequest", "remoteTaxResponse",
		"Calculate tax for an order. A null taxCents is treated as a provider failure, not as zero tax."},
	{"address_validator", "address_validator", "internal/address", "remoteAddressRequest", "remoteAddressResponse",
		"Validate and optionally normalize an address. Note this shape uses `state`, unlike the tax and shipping addresses which use `region`."},
	{"promotion_condition", "promotion_condition", "internal/promotion", "remoteConditionRequest", "remoteConditionResponse",
		"Decide whether a promotion rule matches the cart. Hot path - set a cache TTL."},
	{"promotion_action", "promotion_action", "internal/promotion", "remoteActionRequest", "remoteActionResponse",
		"Compute the discount a matched promotion rule applies."},
	{"fraud_risk", "fraud_risk", "internal/order", "remoteFraudRequest", "remoteFraudResponse",
		"Score an order for fraud. Only the exact decision \"reject\" blocks the order; anything else approves."},
	{"currency_rate", "currency_rate", "internal/currency", "remoteRateRequest", "remoteRateResponse",
		"Supply FX rates for a base currency."},
	{"pricing", "pricing", "internal/wiring", "remotePricingRequest", "remotePricingResponse",
		"Override a line's unit price. Null or negative means no override - keep the catalog price."},
	{"recommendation", "recommendation", "internal/recommend", "remoteRecRequest", "remoteRecResponse",
		"Recommend related products for a product page."},
	{"reviews", "reviews", "internal/review", "remoteReviewRequest", "remoteReviewResponse",
		"Supply review aggregates from an external review platform."},
	{"search", "search", "internal/search", "remoteSearchRequest", "remoteSearchResponse",
		"Free-text product search (the typeahead and /search paths)."},
	{"search", "search.browse", "internal/search", "remoteBrowseRequest", "remoteBrowseResponse",
		"Faceted browse - the category and filter listing path. Same kind as search; a provider registered for `search` must implement both operations."},
	{"customer_segments", "customer_segments", "internal/wiring", "remoteSegmentRequest", "remoteSegmentResponse",
		"Return the segment slugs a customer belongs to."},
	{"returns_policy", "returns_policy", "internal/order", "remoteReturnRequest", "remoteReturnResponse",
		"Approve or refuse a return request."},
	{"inventory_availability", "inventory_availability", "internal/inventory", "remoteAvailRequest", "remoteAvailResponse",
		"Report salable quantity per product. -1 means unlimited."},
	{"shipping_tracking", "shipping_tracking", "internal/shipping", "remoteTrackRequest", "remoteTrackResponse",
		"Report carrier tracking status for a shipment."},
}

func main() {
	outDir := "internal/remoteprovider/contract"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := run(outDir); err != nil {
		fmt.Fprintln(os.Stderr, "providerschema:", err)
		os.Exit(1)
	}
}

func run(outDir string) error {
	dirs := map[string]bool{}
	for _, o := range ops {
		dirs["./"+o.Pkg] = true
	}
	patterns := make([]string, 0, len(dirs))
	for d := range dirs {
		patterns = append(patterns, d)
	}
	sort.Strings(patterns)

	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return fmt.Errorf("load packages: %w", err)
	}
	byDir := map[string]*packages.Package{}
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return fmt.Errorf("package %s: %w", p.PkgPath, p.Errors[0])
		}
		// PkgPath tail matches the internal/... dir we asked for
		for d := range dirs {
			if strings.HasSuffix(p.PkgPath, strings.TrimPrefix(d, "./")) {
				byDir[strings.TrimPrefix(d, "./")] = p
			}
		}
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	written := 0
	for _, o := range ops {
		pkg, ok := byDir[o.Pkg]
		if !ok {
			return fmt.Errorf("%s: package not loaded", o.Pkg)
		}
		reqT, err := lookupStruct(pkg, o.Req)
		if err != nil {
			return fmt.Errorf("%s request: %w", o.Name, err)
		}
		respT, err := lookupStruct(pkg, o.Resp)
		if err != nil {
			return fmt.Errorf("%s response: %w", o.Name, err)
		}

		g := &gen{defs: map[string]any{}, seen: map[string]bool{}}
		reqSchema := g.schemaFor(reqT, o.Req)
		respSchema := g.schemaFor(respT, o.Resp)

		doc := map[string]any{
			"$schema":     "https://json-schema.org/draft/2020-12/schema",
			"title":       o.Name,
			"description": o.Doc,
			"x-kind":      o.Kind,
			"x-source":    fmt.Sprintf("%s.%s -> %s.%s", o.Pkg, o.Req, o.Pkg, o.Resp),
			"properties": map[string]any{
				"request":  reqSchema,
				"response": respSchema,
			},
		}
		if len(g.defs) > 0 {
			doc["$defs"] = g.defs
		}
		if err := writeJSON(filepath.Join(outDir, o.Name+".schema.json"), doc); err != nil {
			return err
		}
		// The example is what bin/provider-check posts by default, so it has to satisfy the
		// request schema rather than just illustrate it
		if err := writeJSON(filepath.Join(outDir, o.Name+".request.json"), g.example(reqT)); err != nil {
			return err
		}
		written += 2
	}
	fmt.Printf("providerschema: wrote %d files for %d operations into %s\n", written, len(ops), outDir)
	return nil
}

func lookupStruct(pkg *packages.Package, name string) (*types.Struct, error) {
	obj := pkg.Types.Scope().Lookup(name)
	if obj == nil {
		return nil, fmt.Errorf("type %q not found in %s (renamed or removed?)", name, pkg.PkgPath)
	}
	st, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("type %q is %T, want a struct", name, obj.Type().Underlying())
	}
	return st, nil
}

type gen struct {
	defs map[string]any
	seen map[string]bool
}

func (g *gen) schemaFor(st *types.Struct, title string) map[string]any {
	props := map[string]any{}
	var required []string
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		name, omitempty, skip := jsonName(f, st.Tag(i))
		if skip {
			continue
		}
		props[name] = g.typeSchema(f.Type())
		if !omitempty && !isPointer(f.Type()) {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	out := map[string]any{"type": "object", "title": title, "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func jsonName(f *types.Var, tag string) (name string, omitempty, skip bool) {
	t := reflect.StructTag(tag).Get("json")
	if t == "-" {
		return "", false, true
	}
	parts := strings.Split(t, ",")
	name = parts[0]
	if name == "" {
		name = f.Name()
	}
	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty, false
}

func isPointer(t types.Type) bool {
	_, ok := t.(*types.Pointer)
	return ok
}

func (g *gen) typeSchema(t types.Type) any {
	switch u := t.(type) {
	case *types.Pointer:
		// Nullable by design - the adapters use pointers where "absent" differs from zero
		inner := g.typeSchema(u.Elem())
		if m, ok := inner.(map[string]any); ok {
			m["x-nullable"] = true
			return m
		}
		return inner
	case *types.Named:
		// json.RawMessage is an unconstrained JSON value, not a byte array
		if u.Obj().Name() == "RawMessage" {
			return map[string]any{"description": "arbitrary JSON"}
		}
		if st, ok := u.Underlying().(*types.Struct); ok {
			key := u.Obj().Pkg().Name() + "." + u.Obj().Name()
			if !g.seen[key] {
				g.seen[key] = true
				g.defs[key] = g.schemaFor(st, key)
			}
			return map[string]any{"$ref": "#/$defs/" + key}
		}
		return g.typeSchema(u.Underlying())
	case *types.Basic:
		return basicSchema(u)
	case *types.Slice:
		return map[string]any{"type": "array", "items": g.typeSchema(u.Elem())}
	case *types.Array:
		// A fixed-size Go array is a fixed-length JSON array, e.g. the 5-bucket review histogram
		n := int(u.Len())
		return map[string]any{"type": "array", "items": g.typeSchema(u.Elem()), "minItems": n, "maxItems": n}
	case *types.Map:
		return map[string]any{"type": "object", "additionalProperties": g.typeSchema(u.Elem())}
	case *types.Interface:
		return map[string]any{"description": "arbitrary JSON"}
	}
	return map[string]any{}
}

func basicSchema(b *types.Basic) map[string]any {
	switch b.Kind() {
	case types.Bool:
		return map[string]any{"type": "boolean"}
	case types.String:
		return map[string]any{"type": "string"}
	case types.Float32, types.Float64:
		return map[string]any{"type": "number"}
	case types.Byte:
		return map[string]any{"type": "integer"}
	}
	if b.Info()&types.IsInteger != 0 {
		return map[string]any{"type": "integer"}
	}
	return map[string]any{}
}

// A filled-in request the checker can actually POST - real-shaped placeholders beat nulls,
// since a provider that rejects an empty string tells you nothing useful
func (g *gen) example(st *types.Struct) map[string]any {
	out := map[string]any{}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		name, _, skip := jsonName(f, st.Tag(i))
		if skip {
			continue
		}
		out[name] = exampleValue(f.Type(), name, 0)
	}
	return out
}

func exampleValue(t types.Type, field string, depth int) any {
	if depth > 4 {
		return nil
	}
	switch u := t.(type) {
	case *types.Pointer:
		return exampleValue(u.Elem(), field, depth+1)
	case *types.Named:
		if u.Obj().Name() == "RawMessage" {
			return map[string]any{}
		}
		if st, ok := u.Underlying().(*types.Struct); ok {
			out := map[string]any{}
			for i := 0; i < st.NumFields(); i++ {
				f := st.Field(i)
				if !f.Exported() {
					continue
				}
				n, _, skip := jsonName(f, st.Tag(i))
				if skip {
					continue
				}
				out[n] = exampleValue(f.Type(), n, depth+1)
			}
			return out
		}
		return exampleValue(u.Underlying(), field, depth+1)
	case *types.Basic:
		return exampleBasic(u, field)
	case *types.Slice:
		return []any{exampleValue(u.Elem(), field, depth+1)}
	case *types.Array:
		out := make([]any, u.Len())
		for i := range out {
			out[i] = exampleValue(u.Elem(), field, depth+1)
		}
		return out
	case *types.Map:
		return map[string]any{"example-key": exampleValue(u.Elem(), field, depth+1)}
	}
	return nil
}

// Field-name-driven placeholders so the example reads like a real call
func exampleBasic(b *types.Basic, field string) any {
	f := strings.ToLower(field)
	switch {
	case b.Kind() == types.Bool:
		return false
	case b.Info()&types.IsString != 0:
		switch {
		case strings.HasSuffix(f, "id"):
			return "00000000-0000-0000-0000-000000000000"
		case f == "currency":
			return "USD"
		case f == "countrycode":
			return "US"
		case f == "postalcode":
			return "10001"
		case f == "city":
			return "New York"
		case f == "street1":
			return "1 Example St"
		case f == "sku":
			return "EXAMPLE-SKU"
		case f == "locale":
			return "en"
		case f == "base":
			return "USD"
		case f == "query":
			return "example"
		case strings.Contains(f, "slug"):
			return "example"
		case strings.Contains(f, "time"):
			return "2026-01-01T00:00:00Z"
		}
		return "example"
	case b.Info()&types.IsInteger != 0:
		switch {
		case strings.Contains(f, "cents"):
			return 1000
		case f == "limit":
			return 10
		case f == "offset":
			return 0
		}
		return 1
	case b.Info()&types.IsFloat != 0:
		if f == "quantity" {
			return 1
		}
		return 1
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
