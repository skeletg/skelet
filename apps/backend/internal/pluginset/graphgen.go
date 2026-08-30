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
)

// Surfaces gqlgen builds, and the sdk constant each one is reached through
var surfaces = []struct{ name, sdkConst, pkg string }{
	{"admin", "sdk.GraphAdmin", "admin"},
	{"front", "sdk.GraphFront", "front"},
}

// Field a plugin's SDL adds to Query or Mutation
type sdlField struct {
	slug    string
	field   string
	enabled bool
}

// A resolver method gqlgen generated, with its signature kept verbatim so nothing here has to reimplement
// gqlgen's SDL-to-Go type mapping - it reads back what gqlgen already decided
type method struct {
	recv    string // queryResolver or mutationResolver
	name    string // KlarnaCreateSession
	field   string // klarnaCreateSession
	params  string // ctx context.Context, amountCents int, ...
	args    string // ctx, amountCents, ...
	results string // (*model.KlarnaSession, error)
	slug    string
	pkgs    []string // packages the signature refers to, so imports can be narrowed
}

var (
	// gqlgen writes "// KlarnaCreateSession is the resolver for the klarnaCreateSession field." - the exact
	// GraphQL name, which beats guessing at its capitalisation rules
	fieldDoc = regexp.MustCompile(`is the resolver for the (\w+) field`)
	// A field line inside an extend block: two spaces, a lowercase name, then args or a colon
	sdlFieldLine = regexp.MustCompile(`^\s{2}([a-z]\w*)\s*[(:]`)
	pkgRef       = regexp.MustCompile(`\b([a-z][a-zA-Z0-9_]*)\.`)
)

// GenerateGraph rewrites every plugin-owned resolver file and the plugin_resolvers.go beside it, so adding a
// GraphQL surface to a plugin means writing SDL and a resolver in the plugin, and touching no core Go at all
// gqlgen must have run first - it owns the signatures, this owns the bodies
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
		if err := writePluginResolvers(root, s.name, s.pkg, methods); err != nil {
			return err
		}
	}
	return nil
}

// Which Query/Mutation fields each enabled plugin claims on this surface
func sdlFields(root string, set *Set, surface string) (map[string]sdlField, error) {
	owned := map[string]sdlField{}
	found, err := Discover(root)
	if err != nil {
		return nil, err
	}
	for _, slug := range found {
		files, err := filepath.Glob(filepath.Join(root, pluginsDir, slug, "graphql", surface, "*.graphqls"))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			inExtend := false
			for line := range strings.SplitSeq(string(src), "\n") {
				trimmed := strings.TrimSpace(line)
				switch {
				case strings.HasPrefix(trimmed, "extend type Query") || strings.HasPrefix(trimmed, "extend type Mutation"):
					inExtend = true
					continue
				case trimmed == "}":
					inExtend = false
					continue
				}
				if !inExtend {
					continue
				}
				if m := sdlFieldLine.FindStringSubmatch(line); m != nil {
					if prev, dup := owned[m[1]]; dup {
						return nil, fmt.Errorf("%s field %q claimed by both %s and %s", surface, m[1], prev.slug, slug)
					}
					owned[m[1]] = sdlField{slug: slug, field: m[1], enabled: set.Enabled(slug)}
				}
			}
		}
	}
	return owned, nil
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
		delegates := bytes.Contains(raw, []byte("pluginhost.Do("))

		var mine, foreign, disabled int
		var ms []method
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			recv := recvType(fn.Recv.List[0].Type)
			if recv != "queryResolver" && recv != "mutationResolver" {
				continue
			}
			field := fieldName(fn)
			o, ok := owned[field]
			if !ok {
				foreign++
				continue
			}
			if !o.enabled {
				disabled++
				continue
			}
			mine++
			m, err := describe(fset, fn, recv, field, o.slug)
			if err != nil {
				return nil, err
			}
			ms = append(ms, m)
		}
		// A delegation file whose plugin is no longer enabled, or whose directory has gone entirely, cannot compile:
		// its SDL left the schema with the plugin and took the generated model types with it. Only a file that
		// actually delegates is removed, so a core resolver is never mistaken for an orphan
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
		src, err := renderResolvers(pkg, sdkConst, ms, imports)
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

func describe(fset *token.FileSet, fn *ast.FuncDecl, recv, field, slug string) (method, error) {
	params, args, err := paramList(fset, fn.Type.Params)
	if err != nil {
		return method{}, err
	}
	results, err := exprList(fset, fn.Type.Results)
	if err != nil {
		return method{}, err
	}
	sig := params + " " + results
	return method{
		recv: recv, name: fn.Name.Name, field: field, slug: slug,
		params: params, args: args, results: results,
		pkgs: pkgRefs(sig),
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
