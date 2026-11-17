package pluginset

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	gqlast "github.com/vektah/gqlparser/v2/ast"
	gqlparser "github.com/vektah/gqlparser/v2/parser"
)

// Surfaces gqlgen builds, and the sdk constant each one is reached through
var surfaces = []struct{ name, sdkConst, pkg string }{
	{"admin", "sdk.GraphAdmin", "admin"},
	{"front", "sdk.GraphFront", "front"},
}

// Field a plugin's SDL adds to Query or Mutation, or to one of core's own object types
type sdlField struct {
	slug    string
	typ     string // the core type it extends, empty for Query and Mutation
	field   string
	enabled bool
}

// The key a field is owned under - case folded, since a field named acmeUrl comes back from gqlgen as AcmeURL
func ownedKey(typ, field string) string {
	if typ == "" {
		return strings.ToLower(field)
	}
	return typ + "." + strings.ToLower(field)
}

// A resolver method gqlgen generated, with its signature kept verbatim so nothing here has to reimplement
// gqlgen's SDL-to-Go type mapping - it reads back what gqlgen already decided
type method struct {
	recv    string // queryResolver, mutationResolver, or a core type's resolver like productResolver
	name    string // KlarnaCreateSession
	field   string // klarnaCreateSession, or Product.acmeWarranty on a core type
	impl    string // the plugin's method - the name itself on Query and Mutation, prefixed with the type on a core type (ProductAcmeWarranty), so two types' fields never clash
	parents string // a core type's field is batched: the parameter holding its parents
	params  string // ctx context.Context, amountCents int, ...
	args    string // ctx, amountCents, ...
	results string // (*model.KlarnaSession, error)
	slug    string
	imports map[string]string // name to path for each package the signature refers to, read from gqlgen's own imports
}

var (
	// gqlgen writes "// KlarnaCreateSession is the resolver for the klarnaCreateSession field." - the exact
	// GraphQL name, which beats guessing at its capitalisation rules
	fieldDoc = regexp.MustCompile(`is the resolver for the (\w+) field`)
	pkgRef   = regexp.MustCompile(`\b([a-z][a-zA-Z0-9_]*)\.`)
)

// GenerateGraph rewrites every plugin-owned resolver file and the plugin_resolvers.go beside it, so adding a GraphQL surface to a plugin means writing SDL and a resolver in the plugin, and touching no core Go at all gqlgen must have run first - it owns the signatures, this owns the bodies
func GenerateGraph(root string) error {
	set, err := Load(root)
	if err != nil {
		return err
	}
	for _, s := range surfaces {
		owned, err := sdlFields(root, set, s.name)
		if err != nil {
			return err
		}
		methods, err := rewriteResolverFiles(root, s.name, s.sdkConst, s.pkg, owned)
		if err != nil {
			return err
		}
		if err := writePluginResolvers(root, s.name, s.sdkConst, s.pkg, methods); err != nil {
			return err
		}
	}
	return nil
}

// Which Query/Mutation fields, and which fields on core's own types, each enabled plugin claims on this surface
func sdlFields(root string, set *Set, surface string) (map[string]sdlField, error) {
	owned := map[string]sdlField{}
	for _, slug := range set.EnabledSlugs() {
		base := filepath.Join(root, pluginsDir, slug)
		if !set.InTree(slug) {
			base = filepath.Join(root, StageDir, slug)
		}
		files, err := filepath.Glob(filepath.Join(base, "graphql", surface, "*.graphqls"))
		if err != nil {
			return nil, err
		}
		var docs []*gqlast.SchemaDocument
		own := map[string]bool{} // types the plugin defines itself - extending those adds nothing to core's
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			doc, err := gqlparser.ParseSchema(&gqlast.Source{Name: f, Input: string(src)})
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", f, err)
			}
			docs = append(docs, doc)
			for _, def := range doc.Definitions {
				own[def.Name] = true
			}
		}
		for _, doc := range docs {
			for _, ext := range doc.Extensions {
				typ := ext.Name
				switch {
				case typ == "Query" || typ == "Mutation":
					typ = ""
				case own[typ]:
					continue
				case ext.Kind != gqlast.Object:
					return nil, fmt.Errorf("%s extends %s %s - a plugin adds fields to core's object types only", slug, strings.ToLower(string(ext.Kind)), typ)
				}
				for _, fd := range ext.Fields {
					if typ != "" {
						if err := coreTypeField(slug, typ, fd); err != nil {
							return nil, err
						}
					}
					key := ownedKey(typ, fd.Name)
					if prev, dup := owned[key]; dup {
						return nil, fmt.Errorf("%s field %q claimed by both %s and %s", surface, strings.TrimPrefix(typ+"."+fd.Name, "."), prev.slug, slug)
					}
					owned[key] = sdlField{slug: slug, typ: typ, field: fd.Name, enabled: set.Enabled(slug)}
				}
			}
		}
	}
	return owned, nil
}

// A field on a core type answers for a whole list of parents in one call, and is nullable, since the type is on every
// channel's schema and the plugin isn't on every channel - those answer null
func coreTypeField(slug, typ string, fd *gqlast.FieldDefinition) error {
	where := fmt.Sprintf("%s's field %s.%s", slug, typ, fd.Name)
	if fd.Type.NonNull {
		return fmt.Errorf("%s is non-null - make it nullable, so a channel the plugin is off on can answer null", where)
	}
	dir := fd.Directives.ForName("goField")
	on := func(arg string) bool {
		if dir == nil {
			return false
		}
		a := dir.Arguments.ForName(arg)
		return a != nil && a.Value != nil && a.Value.Raw == "true"
	}
	if !on("forceResolver") || !on("batch") {
		return fmt.Errorf("%s needs @goField(forceResolver: true, batch: true), so the plugin answers once per list of %ss rather than once per row", where, typ)
	}
	return nil
}

// Rewrites each resolver file whose every method is plugin-owned - a file mixing core and plugin fields is an
// error rather than something to clobber, since gqlgen names these after the schema file and a collision is possible
func rewriteResolverFiles(root, surface, sdkConst, pkg string, owned map[string]sdlField) ([]method, error) {
	paths, err := filepath.Glob(filepath.Join(root, "graph", surface, "*.resolvers.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	var all []method
	for _, path := range paths {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		imports := importsOf(file)
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		delegates := bytes.Contains(raw, []byte("pluginhost.Do(")) || bytes.Contains(raw, []byte("pluginhost.DoBatch("))

		var mine, foreign, disabled int
		var ms []method
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			recv := recvType(fn.Recv.List[0].Type)
			typ, ok := resolverType(recv)
			if !ok {
				continue
			}
			o, ok := owned[ownedKey(typ, fieldName(fn))]
			if !ok {
				foreign++
				continue
			}
			if !o.enabled {
				disabled++
				continue
			}
			mine++
			m, err := describe(fset, fn, recv, o, imports)
			if err != nil {
				return nil, err
			}
			ms = append(ms, m)
		}
		// A delegation file whose plugin is no longer enabled, or whose directory has gone entirely, cannot compile: its SDL left the schema with the plugin and took the generated model types with it. Only a file that actually delegates is removed, so a core resolver is never mistaken for an orphan
		if mine == 0 {
			if delegates && (disabled > 0 || foreign > 0) {
				if err := os.Remove(path); err != nil {
					return nil, err
				}
			}
			continue
		}
		if foreign > 0 {
			return nil, fmt.Errorf("%s mixes %d plugin-owned and %d core resolvers - rename the plugin's schema file so gqlgen gives it its own", path, mine, foreign)
		}
		src, err := renderResolvers(pkg, sdkConst, ms)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", path, err)
		}
		if err := os.WriteFile(path, src, 0o644); err != nil {
			return nil, err
		}
		all = append(all, ms...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].slug != all[j].slug {
			return all[i].slug < all[j].slug
		}
		return all[i].field < all[j].field
	})
	return all, nil
}

// The schema type a resolver receiver serves - empty for Query and Mutation, false for anything that isn't a field resolver
func resolverType(recv string) (string, bool) {
	switch recv {
	case "queryResolver", "mutationResolver":
		return "", true
	case "subscriptionResolver":
		return "", false
	}
	base, ok := strings.CutSuffix(recv, "Resolver")
	if !ok || base == "" {
		return "", false
	}
	r := []rune(base)
	r[0] = unicode.ToUpper(r[0])
	return string(r), true
}

func recvType(e ast.Expr) string {
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// gqlgen's own doc line carries the GraphQL name; lowering the first rune is the fallback
func fieldName(fn *ast.FuncDecl) string {
	if fn.Doc != nil {
		if m := fieldDoc.FindStringSubmatch(fn.Doc.Text()); m != nil {
			return m[1]
		}
	}
	r := []rune(fn.Name.Name)
	if len(r) == 0 {
		return ""
	}
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

func describe(fset *token.FileSet, fn *ast.FuncDecl, recv string, o sdlField, avail map[string]string) (method, error) {
	params, args, err := paramList(fset, fn.Type.Params)
	if err != nil {
		return method{}, err
	}
	results, err := exprList(fset, fn.Type.Results)
	if err != nil {
		return method{}, err
	}
	field, impl, parents := o.field, fn.Name.Name, ""
	if o.typ != "" {
		field, impl = o.typ+"."+o.field, o.typ+fn.Name.Name
		// gqlgen's batch shape: (ctx, objs []*model.T, args...) ([]R, error)
		ps := fn.Type.Params.List
		if len(ps) < 2 || len(ps[1].Names) != 1 || !strings.HasPrefix(results, "([]") {
			return method{}, fmt.Errorf("%s is not a batch resolver - is @goField(batch: true) on %s?", fn.Name.Name, field)
		}
		if _, isSlice := ps[1].Type.(*ast.ArrayType); !isSlice {
			return method{}, fmt.Errorf("%s is not a batch resolver - is @goField(batch: true) on %s?", fn.Name.Name, field)
		}
		parents = ps[1].Names[0].Name
	}
	imports := map[string]string{}
	for _, p := range pkgRefs(params + " " + results) {
		path, ok := avail[p]
		if !ok {
			return method{}, fmt.Errorf("%s refers to package %q with no import in its file", fn.Name.Name, p)
		}
		imports[p] = path
	}
	return method{
		recv: recv, name: fn.Name.Name, field: field, impl: impl, parents: parents, slug: o.slug,
		params: params, args: args, results: results, imports: imports,
	}, nil
}

// Prints parameters as written plus the bare names to forward, so the delegation cannot drift from the signature
func paramList(fset *token.FileSet, fl *ast.FieldList) (string, string, error) {
	if fl == nil {
		return "", "", nil
	}
	var parts, names []string
	for _, f := range fl.List {
		typ, err := node(fset, f.Type)
		if err != nil {
			return "", "", err
		}
		if len(f.Names) == 0 {
			return "", "", fmt.Errorf("unnamed parameter of type %s - gqlgen always names them", typ)
		}
		var group []string
		for _, n := range f.Names {
			group = append(group, n.Name)
			names = append(names, n.Name)
		}
		parts = append(parts, strings.Join(group, ", ")+" "+typ)
	}
	return strings.Join(parts, ", "), strings.Join(names, ", "), nil
}

func exprList(fset *token.FileSet, fl *ast.FieldList) (string, error) {
	if fl == nil || len(fl.List) == 0 {
		return "", nil
	}
	var parts []string
	for _, f := range fl.List {
		typ, err := node(fset, f.Type)
		if err != nil {
			return "", err
		}
		n := len(f.Names)
		if n == 0 {
			n = 1
		}
		for range n {
			parts = append(parts, typ)
		}
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return "(" + strings.Join(parts, ", ") + ")", nil
}

func node(fset *token.FileSet, n ast.Node) (string, error) {
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, n); err != nil {
		return "", err
	}
	return b.String(), nil
}

func pkgRefs(s string) []string {
	var out []string
	for _, m := range pkgRef.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

func importsOf(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		name := path[strings.LastIndex(path, "/")+1:]
		if spec.Name != nil {
			name = spec.Name.Name
		}
		out[name] = path
	}
	return out
}
