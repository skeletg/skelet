package gates

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A tree holding a small copy of everything the contracts gate reads, every contract kept, in a git repo whose main
// has no proto yet
func contractTree(t *testing.T) string {
	t.Helper()
	root := repoTreeBare(t)
	put(t, root, map[string]string{
		blockkit: "// SYNC SENTINEL: types agree with apps/dashboard/src/features/cms/page-blocks.ts\npackage blockkit\n\nconst (\n" +
			"\tTypeHero   = \"hero\"\n\tTypeText   = \"text\"\n\tTypeRef    = \"ref\"\n)\n\nvar BuiltinTypes = map[string]bool{\n\tTypeHero: true, TypeText: true,\n\tTypeRef: true,\n}\n",
		dashBlocks: "export const BLOCKS = [\n  {\n    type: 'hero',\n  },\n  {\n    type: 'text',\n  },\n  {\n    type: 'ref',\n  },\n]\n",
		"apps/channels/shop/app/components/cms/CmsBlocks.tsx": "const REGISTRY: Record<string, BlockRenderer> = {\n  hero: Hero,\n  text: Text,\n};\n",
		responseCache: "var frontCacheableOps = map[string]bool{\n\t\"Products\": true,\n\t\"Menu\": true,\n}\n",
		"apps/channels/shop/app/lib/public-ops.ts":      "export const PUBLIC_OPS = new Set([\n  \"Menu\",\n  \"Products\",\n])\n",
		dashLexical + "/image.tsx":                      "class ImageNode {\n  static getType() {\n    return 'image'\n  }\n}\n",
		dashLexical + "/structured.tsx":                 "type StructuredBlockType =\n  | 'pros-cons'\n  | 'faq'\n\nexport {}\n",
		"apps/channels/shop/app/lib/lexical-render.tsx": "switch (t) {\n  case \"image\":\n  case \"pros-cons\":\n  case \"faq\":\n}\n",
		dashOptVis: "export function visible(o: { a?: string }) {\n  return o.a === \"x\"; // shown when x\n}\n",
		"apps/channels/shop/app/lib/optionVisibility.ts": "export function visible(o: {a?:string}) { return o.a === 'x' }\n",
		secHeaders: "package httpx\n\nconst ContentSecurityPolicy = \"default-src 'self'; \" +\n\t\"img-src 'self'\"\n\nconst PermissionsPolicy = `camera=(), geolocation=()`\n",
		nginxConf:  "add_header Content-Security-Policy \"default-src 'self'; img-src 'self' ${UPLOAD_ORIGINS}\" always;\nadd_header Permissions-Policy \"camera=(), geolocation=()\" always;\n",
		caddyfile:  "header Content-Security-Policy \"default-src 'self'; img-src 'self' {$UPLOAD_ORIGINS}\"\nheader Permissions-Policy \"camera=(), geolocation=()\"\n",
		"apps/backend/core/internal/rules/schema.go": "package rules\n\nvar s = `{\"x-widget\": \"segment-picker\"}`\n",
		widgetReg: "export const FORM_WIDGETS = {\n  'segment-picker': SegmentPicker,\n}\n",
	})
	git(t, root, "add", "-A")
	git(t, root, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")
	return root
}

// repoTreeBare is a git repo on main with nothing in it
func repoTreeBare(t *testing.T) string {
	t.Helper()
	root := repoTree(t)
	git(t, root, "checkout", "-q", "-b", "main")
	return root
}

func contractsOut(t *testing.T, root string) (string, int) {
	t.Helper()
	t.Setenv("CI", "")
	var out bytes.Buffer
	code := contractCheck(root, &out)
	return out.String(), code
}

func TestContractsHealthy(t *testing.T) {
	out, code := contractsOut(t, contractTree(t))
	if code != 0 || strings.Contains(out, "FAIL") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, line := range []string{
		"ok    1 referenced paths resolve",
		"ok    backend BuiltinTypes == dashboard palette (3 types)",
		"ok    shop renderer covers every type but the declared gaps (ref)",
		"ok    shop PUBLIC_OPS == frontCacheableOps (2 ops)",
		"ok    shop renders all 3 authored node types and variants",
		"ok    shop evaluator matches the dashboard's, modulo formatting",
		"ok    Content-Security-Policy matches " + nginxConf,
		"ok    Permissions-Policy matches " + caddyfile,
		"ok    all 1 requested widgets are registered",
		"ok    main has no proto yet",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("no %q in:\n%s", line, out)
		}
	}
}

func TestContractFailures(t *testing.T) {
	cases := []struct {
		name string
		edit map[string]string
		want string
	}{
		{"a SYNC comment naming a moved file", map[string]string{"x.go": "// SYNC: see apps/gone/file.ts\n"},
			"FAIL  SYNC comment points at a path that doesn't exist: apps/gone/file.ts"},
		{"a palette that drifted", map[string]string{dashBlocks: "    type: 'hero',\n    type: 'text',\n"},
			"FAIL  backend BuiltinTypes != dashboard palette"},
		{"a storefront missing a renderer", map[string]string{"apps/channels/shop/app/components/cms/CmsBlocks.tsx": "const REGISTRY: Record<string, BlockRenderer> = {\n  hero: Hero,\n};\n"},
			"FAIL  shop has no renderer for backend type 'text'"},
		{"a storefront rendering a type the backend rejects", map[string]string{"apps/channels/shop/app/components/cms/CmsBlocks.tsx": "const REGISTRY: Record<string, BlockRenderer> = {\n  hero: Hero,\n  text: Text,\n  video: Video,\n};\n"},
			"FAIL  shop renders types the backend rejects: video"},
		{"a public op the backend doesn't cache", map[string]string{"apps/channels/shop/app/lib/public-ops.ts": "new Set([\n  \"Menu\",\n  \"Products\",\n  \"Cart\",\n])\n"},
			"FAIL  shop PUBLIC_OPS != frontCacheableOps"},
		{"an authored node with no case", map[string]string{"apps/channels/shop/app/lib/lexical-render.tsx": "case \"image\":\ncase \"faq\":\n"},
			"FAIL  shop has no case for: pros-cons"},
		{"an evaluator that changed meaning", map[string]string{"apps/channels/shop/app/lib/optionVisibility.ts": "export function visible(o: {a?:string}) { return o.a !== 'x' }\n"},
			"FAIL  shop evaluator has diverged"},
		{"an edge CSP edited by hand", map[string]string{caddyfile: "header Content-Security-Policy \"default-src *\"\nheader Permissions-Policy \"camera=(), geolocation=()\"\n"},
			"FAIL  Content-Security-Policy differs between " + secHeaders + " and " + caddyfile},
		{"a widget nobody registered", map[string]string{"apps/backend/core/internal/rules/more.go": "package rules\n\nvar t = `{\"x-widget\": \"ghost\"}`\n"},
			"FAIL  Go schemas ask for widgets the dashboard does not register: ghost"},
		{"an extractor that finds nothing", map[string]string{responseCache: "package wiring\n"},
			"FAIL  frontCacheableOps extraction came back empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := contractTree(t)
			put(t, root, c.edit)
			git(t, root, "add", "-A")
			out, code := contractsOut(t, root)
			if code != 1 || !strings.Contains(out, c.want) {
				t.Errorf("exit %d, want %q in:\n%s", code, c.want, out)
			}
		})
	}
}

// A Caddyfile that moved fails rather than skipping - only a genuinely absent one is a fork's choice
func TestAMovedCaddyfileFails(t *testing.T) {
	root := contractTree(t)
	raw, _ := os.ReadFile(filepath.Join(root, caddyfile))
	put(t, root, map[string]string{"deploy/edge/Caddyfile": string(raw)})
	_ = os.Remove(filepath.Join(root, caddyfile))
	git(t, root, "add", "-A")
	if out, code := contractsOut(t, root); code != 1 || !strings.Contains(out, "is missing but a Caddyfile is tracked at deploy/edge/Caddyfile") {
		t.Errorf("exit %d:\n%s", code, out)
	}
}
