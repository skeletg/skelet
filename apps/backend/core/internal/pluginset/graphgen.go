package pluginset

import (
	"bytes"
	"cmp"
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
	slug  string
	typ   string // the core type it extends, empty for Query and Mutation
	field string
}

// The key a field is owned under, the same from its GraphQL name and from the Go name gqlgen gave its resolver: case
// folded and without underscores, since acmeUrl comes back as AcmeURL and acme_url as AcmeURL too
func ownedKey(typ, field string) string {
	field = strings.ToLower(strings.ReplaceAll(field, "_", ""))
	if typ == "" {
		return field
	}
	return typ + "." + field
}

// What the generated delegations call - also how a file that delegates, and a method that does, are recognised
const (
	delegateCall      = "pluginhost.Do("
	delegateBatchCall = "pluginhost.DoBatch("
)

// A resolver method gqlgen generated, with its signature kept verbatim so nothing here has to reimplement
// gqlgen's SDL-to-Go type mapping - it reads back what gqlgen already decided
type method struct {
	recv    string // queryResolver, mutationResolver, or a core type's resolver like productResolver
	name    string // KlarnaCreateSession
	key     string // what its field is owned under
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
func GenerateGraph(root string) error { return generateGraph(root, false) }

// PruneGraph is the pass before gqlgen: it clears what a plugin that left, or a field that was renamed, left behind,
// so gqlgen type-checks a tree that compiles. The fields gqlgen hasn't met yet have no resolver to find, which is fine
// here and an error after it
func PruneGraph(root string) error { return generateGraph(root, true) }

func generateGraph(root string, pruning bool) error {
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
		if !pruning {
			if err := everyFieldResolved(s.name, owned, methods); err != nil {
				return err
			}
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
		claim := func(typ string, fd *gqlast.FieldDefinition) error {
			if typ != "" {
				if err := coreTypeField(slug, typ, fd); err != nil {
					return err
				}
			}
			// @goField(name:) renames the resolver gqlgen writes, and that's the name read back
			key := ownedKey(typ, cmp.Or(goFieldArg(fd, "name"), fd.Name))
			if prev, dup := owned[key]; dup {
				return fmt.Errorf("%s field %q claimed by both %s and %s", surface, strings.TrimPrefix(typ+"."+fd.Name, "."), prev.slug, slug)
			}
			owned[key] = sdlField{slug: slug, typ: typ, field: fd.Name}
			return nil
		}
		// The plugin's own types are its models - only a field it asks a resolver for is delegated, a product pointing
		// back at core's record, say. The rest are plain fields of the model
		ownResolved := func(def *gqlast.Definition) error {
			for _, fd := range def.Fields {
				if forcedResolver(fd) {
					if err := claim(def.Name, fd); err != nil {
						return err
					}
				}
			}
			return nil
		}
		for _, doc := range docs {
			for _, def := range doc.Definitions {
				if def.Kind == gqlast.Object {
					if err := ownResolved(def); err != nil {
						return nil, err
					}
				}
			}
			for _, ext := range doc.Extensions {
				typ := ext.Name
				switch {
				case typ == "Query" || typ == "Mutation":
					typ = ""
				case own[typ]:
					if err := ownResolved(ext); err != nil {
						return nil, err
					}
					continue
				case ext.Kind != gqlast.Object:
					return nil, fmt.Errorf("%s extends %s %s - a plugin adds fields to core's object types only", slug, strings.ToLower(string(ext.Kind)), typ)
				}
				for _, fd := range ext.Fields {
					if err := claim(typ, fd); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return owned, nil
}

func goFieldArg(fd *gqlast.FieldDefinition, arg string) string {
	dir := fd.Directives.ForName("goField")
	if dir == nil {
		return ""
	}
	if a := dir.Arguments.ForName(arg); a != nil && a.Value != nil {
		return a.Value.Raw
	}
	return ""
}

func forcedResolver(fd *gqlast.FieldDefinition) bool {
	return goFieldArg(fd, "forceResolver") == "true"
}

// After gqlgen every claimed field has a resolver it wrote. One that doesn't was named in a way this couldn't read
// back, and would be left as gqlgen's panicking stub with nothing checking for it at boot
func everyFieldResolved(surface string, owned map[string]sdlField, ms []method) error {
	found := map[string]bool{}
	impls := map[string]string{}
	for _, m := range ms {
		found[m.key] = true
		if prev, dup := impls[m.slug+" "+m.impl]; dup {
			return fmt.Errorf("%s: %s's fields %s and %s both come to the method %s - rename one", surface, m.slug, prev, m.field, m.impl)
		}
		impls[m.slug+" "+m.impl] = m.field
	}
	var lost []string
	for key, o := range owned {
		if !found[key] {
			lost = append(lost, fmt.Sprintf("%s's %s", o.slug, strings.TrimPrefix(o.typ+"."+o.field, ".")))
		}
	}
	if len(lost) > 0 {
		sort.Strings(lost)
		return fmt.Errorf("%s: gqlgen wrote no resolver this could match to %s", surface, strings.Join(lost, ", "))
	}
	return nil
}

// A field on a core type answers for a whole list of parents in one call, and is nullable, since the type is on every
// channel's schema and the plugin isn't on every channel - those answer null
func coreTypeField(slug, typ string, fd *gqlast.FieldDefinition) error {
	where := fmt.Sprintf("%s's field %s.%s", slug, typ, fd.Name)
	if fd.Type.NonNull {
		return fmt.Errorf("%s is non-null - make it nullable, so a channel the plugin is off on can answer null", where)
	}
	if goFieldArg(fd, "forceResolver") != "true" || goFieldArg(fd, "batch") != "true" {
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
		delegates := bytes.Contains(raw, []byte(delegateCall)) || bytes.Contains(raw, []byte(delegateBatchCall))

		var mine, foreign, stale int
		var ms []method
		var keep []string // the resolver plumbing gqlgen writes beside a plugin type's own field resolvers
		for _, decl := range file.Decls {
			if src, ok := plumbing(fset, decl); ok {
				keep = append(keep, src)
				continue
			}
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			recv := recvType(fn.Recv.List[0].Type)
			typ, ok := resolverType(recv)
			if !ok {
				continue
			}
			key := ownedKey(typ, fieldName(fn))
			o, ok := owned[key]
			if !ok {
				// A delegation nobody claims any more - its plugin left, or the field was renamed or removed. It goes,
				// where a core resolver would be an error to clobber
				if delegation(fset, fn) {
					stale++
				} else {
					foreign++
				}
				continue
			}
			mine++
			m, err := describe(fset, fn, recv, key, o, imports)
			if err != nil {
				return nil, err
			}
			ms = append(ms, m)
		}
		// A delegation file whose plugin is no longer enabled, or whose directory has gone entirely, cannot compile: its SDL left the schema with the plugin and took the generated model types with it. Only a file that actually delegates is removed, so a core resolver is never mistaken for an orphan
		if mine == 0 {
			if delegates && stale > 0 && foreign == 0 {
				if err := os.Remove(path); err != nil {
					return nil, err
				}
			}
			continue
		}
		if foreign > 0 {
			return nil, fmt.Errorf("%s mixes %d plugin-owned and %d core resolvers - rename the plugin's schema file so gqlgen gives it its own", path, mine, foreign)
		}
		src, err := renderResolvers(pkg, sdkConst, ms, keep)
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

// gqlgen's accessor and struct for a type with field resolvers - func (r *Resolver) AcmeWarranty() AcmeWarrantyResolver
// and type acmeWarrantyResolver struct{ *Resolver } - which land in the file defining the type, the plugin's own for its
// own types, and have to survive the rewrite. A file with several such types gets their structs as one grouped
// type ( ... ) block
func plumbing(fset *token.FileSet, decl ast.Decl) (string, bool) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil || len(d.Recv.List) != 1 || recvType(d.Recv.List[0].Type) != "Resolver" {
			return "", false
		}
	case *ast.GenDecl:
		if d.Tok != token.TYPE || len(d.Specs) == 0 {
			return "", false
		}
		for _, spec := range d.Specs {
			if ts, ok := spec.(*ast.TypeSpec); !ok || !strings.HasSuffix(ts.Name.Name, "Resolver") {
				return "", false
			}
		}
	default:
		return "", false
	}
	src, err := node(fset, decl)
	return src, err == nil
}

// Whether a resolver's body is one of the delegations this writes
func delegation(fset *token.FileSet, fn *ast.FuncDecl) bool {
	if fn.Body == nil {
		return false
	}
	body, err := node(fset, fn.Body)
	return err == nil && (strings.Contains(body, delegateCall) || strings.Contains(body, delegateBatchCall))
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

func describe(fset *token.FileSet, fn *ast.FuncDecl, recv, key string, o sdlField, avail map[string]string) (method, error) {
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
		batched := len(ps) >= 2 && len(ps[1].Names) == 1 && strings.HasPrefix(results, "([]")
		if batched {
			_, batched = ps[1].Type.(*ast.ArrayType)
		}
		if !batched {
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
		recv: recv, name: fn.Name.Name, key: key, field: field, impl: impl, parents: parents, slug: o.slug,
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
