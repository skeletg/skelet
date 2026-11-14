package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

const base = `
type Query {
  product(id: ID!, locale: String): Product
  products: [Product!]!
  legacy: String @deprecated(reason: "gone next release")
}
interface Node { id: ID! }
type Product implements Node { id: ID! name: String! price: Int status: Status }
enum Status { ACTIVE ARCHIVED OLD @deprecated(reason: "x") }
union Hit = Product | Collection
type Collection { id: ID! }
input ProductInput { name: String! note: String }
scalar Money
`

func schema(t *testing.T, sdl string) *ast.Schema {
	t.Helper()
	s, err := gqlparser.LoadSchema(&ast.Source{Name: "x.graphqls", Input: sdl})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWhatBreaksAClient(t *testing.T) {
	cases := []struct {
		name, from, to string
		want           []string
	}{
		{"unchanged", base, base, nil},
		{"additions are safe", base, strings.Replace(base, "scalar Money", "scalar Money\ntype Extra { a: Int }", 1) + "\nextend type Query { extra: Extra }", nil},
		{"a deprecated field and enum value go quietly", base, strings.NewReplacer(`  legacy: String @deprecated(reason: "gone next release")`+"\n", "", ` OLD @deprecated(reason: "x")`, "").Replace(base), nil},
		{"a field removed", base, strings.Replace(base, " price: Int", "", 1), []string{"Product.price removed"}},
		{"an output tightened is safe, loosened is not", base, strings.NewReplacer("price: Int", "price: Int!", "name: String!", "name: String").Replace(base), []string{"Product.name is String now, was String!"}},
		{"arguments", base, strings.Replace(base, "product(id: ID!, locale: String)", "product(id: ID!, channel: ID!, sort: String)", 1),
			[]string{"Query.product(channel) added as required", "Query.product(locale) removed"}},
		{"an argument made required", base, strings.Replace(base, "locale: String)", "locale: String!)", 1), []string{"Query.product(locale) is String! now, was String"}},
		{"input fields", base, strings.Replace(base, "input ProductInput { name: String! note: String }", "input ProductInput { name: String! sku: String! tag: String }", 1),
			[]string{"ProductInput.note removed", "ProductInput.sku added as required"}},
		{"an enum value and a union member", base, strings.NewReplacer("ACTIVE ARCHIVED", "ACTIVE", "Product | Collection", "Product").Replace(base),
			[]string{"Hit no longer includes Collection", "Status.ARCHIVED removed"}},
		{"a type removed, a kind changed, an interface dropped", base,
			strings.NewReplacer("scalar Money", "", "implements Node ", "", "type Collection { id: ID! }", "input Collection { id: ID! }",
				"Product | Collection", "Product").Replace(base),
			[]string{"Collection is INPUT_OBJECT now, was OBJECT", "Hit no longer includes Collection", "Money removed", "Product no longer implements Node"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := diff(schema(t, c.from), schema(t, c.to)); !slices.Equal(got, c.want) {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

func baseline(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for name, body := range files {
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func TestRunHoldsBothSurfacesToTheBaselineAndTakesAcceptedBreaks(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("graph/schema/admin/a.graphqls", "type Query { a: Int }")
	write("graph/schema/front/f.graphqls", "type Query { f: Int }")
	old := map[string]string{
		"apps/backend/core/graph/schema/admin/a.graphqls": "type Query { a: Int gone: Int }",
		"apps/backend/core/graph/schema/front/f.graphqls": "type Query { f: Int also: Int }",
	}
	var out bytes.Buffer
	if code := run(baseline(t, old), dir, &out); code != 1 || out.String() != "admin: Query.gone removed\nfront: Query.also removed\n" {
		t.Errorf("exit %d:\n%s", code, out.String())
	}
	write(accepted, "# taken on purpose in 0.5\nadmin: Query.gone removed\nfront: Query.also removed\nfront: Query.never removed\n")
	out.Reset()
	if code := run(baseline(t, old), dir, &out); code != 0 || out.String() != "stale: front: Query.never removed\n" {
		t.Errorf("accepted: exit %d:\n%s", code, out.String())
	}
	out.Reset()
	if code := run(baseline(t, map[string]string{}), dir, &out); code != 0 || strings.Contains(out.String(), "\nadmin:") || strings.HasPrefix(out.String(), "admin:") {
		t.Errorf("a baseline with no schema yet: exit %d:\n%s", code, out.String())
	}
}
