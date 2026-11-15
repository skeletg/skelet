// Command schemacompat lists what in graph/schema would break a client built against the baseline's schema, which it reads as a tar on stdin
package main

import (
	"archive/tar"
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

const accepted = "graph/schema/accepted-breaks"

var surfaces = []string{"admin", "front"}

func main() { os.Exit(run(os.Stdin, ".", os.Stdout)) }

func run(baseline io.Reader, dir string, out io.Writer) int {
	before, err := fromTar(baseline)
	if err != nil {
		fmt.Fprintln(out, "schemacompat: reading the baseline:", err)
		return 2
	}
	allowed, err := readAccepted(filepath.Join(dir, accepted))
	if err != nil {
		fmt.Fprintln(out, "schemacompat:", err)
		return 2
	}
	breaks, seen := 0, map[string]bool{}
	for _, surface := range surfaces {
		if len(before[surface]) == 0 {
			continue
		}
		after, err := fromDir(filepath.Join(dir, "graph", "schema", surface))
		if err != nil {
			fmt.Fprintln(out, "schemacompat:", err)
			return 2
		}
		old, gerr := gqlparser.LoadSchema(before[surface]...)
		if gerr != nil {
			fmt.Fprintf(out, "schemacompat: the baseline's %s schema: %v\n", surface, gerr)
			return 2
		}
		cur, gerr := gqlparser.LoadSchema(after...)
		if gerr != nil {
			fmt.Fprintf(out, "schemacompat: the %s schema: %v\n", surface, gerr)
			return 2
		}
		for _, f := range diff(old, cur) {
			line := surface + ": " + f
			if slices.Contains(allowed, line) {
				seen[line] = true
				continue
			}
			fmt.Fprintln(out, line)
			breaks++
		}
	}
	for _, line := range allowed {
		if !seen[line] {
			fmt.Fprintln(out, "stale: "+line)
		}
	}
	if breaks > 0 {
		return 1
	}
	return 0
}

func diff(old, cur *ast.Schema) []string {
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }
	for name, ot := range old.Types {
		if ot.BuiltIn {
			continue
		}
		nt, ok := cur.Types[name]
		switch {
		case !ok:
			add("%s removed", name)
			continue
		case nt.Kind != ot.Kind:
			add("%s is %s now, was %s", name, nt.Kind, ot.Kind)
			continue
		}
		switch ot.Kind {
		case ast.Object, ast.Interface:
			for _, of := range ot.Fields {
				nf := nt.Fields.ForName(of.Name)
				switch {
				case strings.HasPrefix(of.Name, "__"):
				case nf == nil && !deprecated(of.Directives):
					add("%s.%s removed", name, of.Name)
				case nf == nil:
				case !outputFits(of.Type, nf.Type):
					add("%s.%s is %s now, was %s", name, of.Name, nf.Type, of.Type)
				}
				if nf != nil {
					arguments(add, name+"."+of.Name, of.Arguments, nf.Arguments)
				}
			}
			for _, i := range ot.Interfaces {
				if !slices.Contains(nt.Interfaces, i) {
					add("%s no longer implements %s", name, i)
				}
			}
		case ast.InputObject:
			for _, of := range ot.Fields {
				nf := nt.Fields.ForName(of.Name)
				switch {
				case nf == nil && !deprecated(of.Directives):
					add("%s.%s removed", name, of.Name)
				case nf != nil && !inputFits(of.Type, nf.Type):
					add("%s.%s is %s now, was %s", name, of.Name, nf.Type, of.Type)
				}
			}
			for _, nf := range nt.Fields {
				if ot.Fields.ForName(nf.Name) == nil && nf.Type.NonNull && nf.DefaultValue == nil {
					add("%s.%s added as required", name, nf.Name)
				}
			}
		case ast.Enum:
			for _, ov := range ot.EnumValues {
				if nt.EnumValues.ForName(ov.Name) == nil && !deprecated(ov.Directives) {
					add("%s.%s removed", name, ov.Name)
				}
			}
		case ast.Union:
			for _, m := range ot.Types {
				if !slices.Contains(nt.Types, m) {
					add("%s no longer includes %s", name, m)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

func arguments(add func(string, ...any), field string, old, cur ast.ArgumentDefinitionList) {
	for _, oa := range old {
		na := cur.ForName(oa.Name)
		switch {
		case na == nil:
			add("%s(%s) removed", field, oa.Name)
		case !inputFits(oa.Type, na.Type):
			add("%s(%s) is %s now, was %s", field, oa.Name, na.Type, oa.Type)
		}
	}
	for _, na := range cur {
		if old.ForName(na.Name) == nil && na.Type.NonNull && na.DefaultValue == nil {
			add("%s(%s) added as required", field, na.Name)
		}
	}
}

// A client reading a field copes with it getting stricter, never looser
func outputFits(old, cur *ast.Type) bool {
	if (old.NonNull && !cur.NonNull) || (old.Elem == nil) != (cur.Elem == nil) {
		return false
	}
	if old.Elem != nil {
		return outputFits(old.Elem, cur.Elem)
	}
	return old.NamedType == cur.NamedType
}

func inputFits(old, cur *ast.Type) bool {
	if (cur.NonNull && !old.NonNull) || (old.Elem == nil) != (cur.Elem == nil) {
		return false
	}
	if old.Elem != nil {
		return inputFits(old.Elem, cur.Elem)
	}
	return old.NamedType == cur.NamedType
}

func deprecated(d ast.DirectiveList) bool { return d.ForName("deprecated") != nil }

func fromTar(r io.Reader) (map[string][]*ast.Source, error) {
	out := map[string][]*ast.Source{}
	t := tar.NewReader(r)
	for {
		h, err := t.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		_, rest, ok := strings.Cut(h.Name, "graph/schema/")
		surface, file, nested := strings.Cut(rest, "/")
		if !ok || !nested || !strings.HasSuffix(file, ".graphqls") || strings.Contains(file, "/") {
			continue
		}
		raw, err := io.ReadAll(t)
		if err != nil {
			return nil, err
		}
		out[surface] = append(out[surface], &ast.Source{Name: "baseline/" + rest, Input: string(raw)})
	}
}

func fromDir(dir string) ([]*ast.Source, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.graphqls"))
	if err != nil {
		return nil, err
	}
	var out []*ast.Source
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		out = append(out, &ast.Source{Name: f, Input: string(raw)})
	}
	return out, nil
}

func readAccepted(path string) ([]string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []string
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		if l := strings.TrimSpace(lines.Text()); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out, lines.Err()
}
