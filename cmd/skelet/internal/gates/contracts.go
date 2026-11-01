package gates

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
)

const ContractsAbout = `The SYNC comments name contracts that span two or three apps; this is the mechanical half, so drift
fails a build: every path a SYNC comment names still exists, the CMS block types agree across the
backend, the dashboard and each storefront, the anonymous-cacheable operations match, every lexical
node the dashboard authors has a storefront renderer, the conditional-option evaluators are the same
code, the security headers match across the copies that must, every x-widget a Go schema asks for is
registered, and the core-to-Signals proto stays compatible with main. Each extractor fails when it
finds nothing, so a moved file can't turn it into a no-op.`

const (
	blockkit      = "apps/backend/core/internal/blockkit/blockkit.go"
	responseCache = "apps/backend/core/internal/wiring/response_cache.go"
	dashBlocks    = "apps/dashboard/src/features/cms/page-blocks.ts"
	dashLexical   = "apps/dashboard/src/features/catalog/lexical-blocks"
	dashOptVis    = "apps/dashboard/src/features/carts/optionVisibility.ts"
	secHeaders    = "apps/backend/core/lib/httpx/securityheaders.go"
	nginxConf     = "apps/dashboard/nginx.conf.template"
	caddyfile     = "deploy/caddy/Caddyfile"
	widgetReg     = "apps/dashboard/src/features/promotions/widgets/registry.ts"
	// The storefront logic every template shares - checked once, here, rather than per storefront
	storefrontPkg = "packages/storefront-core"
	publicOps     = storefrontPkg + "/public-ops.ts"
	reactPkg      = "packages/storefront-react"
	pkgOptVis     = storefrontPkg + "/optionVisibility.ts"
	// This gate's own source talks about SYNC comments, so it stays out of the scan for them
	self = "cmd/skelet/internal/gates"
)

// Renderers a storefront may lack: ref is a backend-side block whose storefront half was never built - drop an entry
// the day one lands
var rendererGaps = []string{"ref"}

func Contracts(root string, _ []string) int { return contractCheck(root, os.Stdout) }

func contractCheck(root string, out io.Writer) int {
	r := &report{out: out}
	c := contracts{root: root, r: r}
	c.syncPaths()
	c.blockTypes()
	c.cacheableOps()
	c.lexicalNodes()
	c.evaluators()
	c.frameworkFree()
	c.headers()
	c.widgets()
	c.proto()
	fmt.Fprintln(out)
	if r.failed {
		fmt.Fprintln(out, "==> contract-check FAILED")
	} else {
		fmt.Fprintln(out, "==> contract-check OK")
	}
	return r.code()
}

type contracts struct {
	root string
	r    *report
}

func (c contracts) skip(format string, a ...any) { fmt.Fprintf(c.r.out, "  --    "+format+"\n", a...) }

func (c contracts) read(rel string) string {
	raw, _ := os.ReadFile(filepath.Join(c.root, rel))
	return string(raw)
}

func (c contracts) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.root
	out, err := cmd.Output()
	return string(out), err
}

// storefronts is every storefront - the templates and the project's own
func (c contracts) storefronts() []storefronts.Storefront { return storefronts.List(c.root) }

var (
	syncLine  = regexp.MustCompile(`(//|#).*\bSYNC\b`)
	syncToken = regexp.MustCompile(`[A-Za-z0-9_.*-]+/[A-Za-z0-9_./*-]+`)
)

// syncPaths - a renamed file leaves the comment behind, which is how a contract quietly loses one of its ends. Glob
// and module-relative forms both count as resolved
func (c contracts) syncPaths() {
	c.r.section("SYNC comments point at paths that exist")
	tracked, _ := c.git("ls-files")
	files := strings.Split(strings.TrimSpace(tracked), "\n")
	hits, _ := c.git("grep", "-h", "SYNC", "--", ".", ":!"+self)
	var tokens []string
	for line := range strings.SplitSeq(hits, "\n") {
		if !syncLine.MatchString(line) {
			continue
		}
		for _, t := range syncToken.FindAllString(line, -1) {
			if t = strings.TrimRight(t, ".,)"); !slices.Contains(tokens, t) {
				tokens = append(tokens, t)
			}
		}
	}
	slices.Sort(tokens)
	if len(tokens) == 0 {
		c.r.bad("found no SYNC comments at all - did the marker change?")
		return
	}
	stale := false
	for _, t := range tokens {
		esc := strings.ReplaceAll(regexp.QuoteMeta(t), `\*`, `[^/]*`)
		re := regexp.MustCompile(`(^|/)` + esc + `(/|$)`)
		if !slices.ContainsFunc(files, re.MatchString) {
			c.r.bad("SYNC comment points at a path that doesn't exist: %s", t)
			stale = true
		}
	}
	if !stale {
		c.r.pass("%d referenced paths resolve", len(tokens))
	}
}

// between is the lines after the first one matching start, up to the next one matching end - the awk ranges the bash
// gate drew its lists from
func between(text string, start, end *regexp.Regexp) []string {
	var out []string
	on := false
	for line := range strings.SplitSeq(text, "\n") {
		switch {
		case !on && start.MatchString(line):
			on = true
		case on && end.MatchString(line):
			on = false
		case on:
			out = append(out, line)
		}
	}
	return out
}

// set is the unique matches of re, group 1 when it has one, sorted
func set(lines []string, re *regexp.Regexp) []string {
	var out []string
	for _, l := range lines {
		for _, m := range re.FindAllStringSubmatch(l, -1) {
			v := m[0]
			if len(m) > 1 {
				v = m[1]
			}
			if v != "" && !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	slices.Sort(out)
	return out
}

// minus is what a has and b lacks
func minus(a, b []string) []string {
	var out []string
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}

// sides prints what each list has that the other lacks, under a failure
func (c contracts) sides(left, right []string) {
	for _, v := range minus(left, right) {
		fmt.Fprintf(c.r.out, "          < %s\n", v)
	}
	for _, v := range minus(right, left) {
		fmt.Fprintf(c.r.out, "          > %s\n", v)
	}
}

var (
	typeConst    = regexp.MustCompile(`^\tType[A-Za-z]+ +=`)
	builtinStart = regexp.MustCompile(`var BuiltinTypes = map\[string\]bool\{`)
	closeBrace   = regexp.MustCompile(`^\}`)
	typeName     = regexp.MustCompile(`^Type[A-Za-z]+$`)
	dashType     = regexp.MustCompile(`^    type: '([a-z_]+)'`)
	registry     = regexp.MustCompile(`const REGISTRY: Record<string, BlockRenderer> = \{`)
	registryEnd  = regexp.MustCompile(`^\};`)
	registryKey  = regexp.MustCompile(`^  ([a-z_]+):`)
	fieldSplit   = regexp.MustCompile(`[\s,:]+`)
)

func (c contracts) goBlockTypes() []string {
	text := c.read(blockkit)
	values := map[string]string{}
	for line := range strings.SplitSeq(text, "\n") {
		if typeConst.MatchString(line) {
			if f := strings.Fields(line); len(f) >= 3 {
				values[f[0]] = strings.ReplaceAll(f[2], `"`, "")
			}
		}
	}
	var out []string
	for _, line := range between(text, builtinStart, closeBrace) {
		for _, part := range fieldSplit.Split(line, -1) {
			if v := values[part]; typeName.MatchString(part) && v != "" && !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	slices.Sort(out)
	return out
}

func (c contracts) blockTypes() {
	c.r.section("CMS block types agree three ways")
	goTypes := c.goBlockTypes()
	dash := set(strings.Split(c.read(dashBlocks), "\n"), dashType)
	if len(goTypes) == 0 || len(dash) == 0 {
		c.r.bad("block-type extraction came back empty - %s or %s moved", blockkit, dashBlocks)
		return
	}
	if slices.Equal(goTypes, dash) {
		c.r.pass("backend BuiltinTypes == dashboard palette (%d types)", len(goTypes))
	} else {
		c.r.bad("backend BuiltinTypes != dashboard palette")
		c.sides(goTypes, dash)
	}
	for _, sf := range c.storefronts() {
		name := sf.Name
		f := filepath.Join(filepath.FromSlash(sf.Dir), "app", "components", "cms", "CmsBlocks.tsx")
		if _, err := os.Stat(filepath.Join(c.root, f)); err != nil {
			c.skip("%s has no CMS renderer registry", name)
			continue
		}
		sf := set(between(c.read(f), registry, registryEnd), registryKey)
		if len(sf) == 0 {
			c.r.bad("%s: renderer registry extraction came back empty", name)
			continue
		}
		before := c.r.failed
		if unknown := minus(sf, goTypes); len(unknown) > 0 {
			c.r.bad("%s renders types the backend rejects: %s", name, strings.Join(unknown, " "))
		}
		for _, g := range minus(goTypes, sf) {
			if !slices.Contains(rendererGaps, g) {
				c.r.bad("%s has no renderer for backend type '%s' (add one, or list it in rendererGaps)", name, g)
			}
		}
		if c.r.failed == before {
			c.r.pass("%s renderer covers every type but the declared gaps (%s)", name, strings.Join(rendererGaps, " "))
		}
	}
}

var (
	cacheStart = regexp.MustCompile(`var frontCacheableOps = map\[string\]bool\{`)
	setStart   = regexp.MustCompile(`new Set\(\[`)
	setEnd     = regexp.MustCompile(`^\]\)`)
	quotedOp   = regexp.MustCompile(`"([A-Za-z]+)"`)
)

// cacheableOps - opt-out on both sides, so a name on one list and not the other means the storefront strips a token
// for a query the backend then declines to serve from cache
func (c contracts) cacheableOps() {
	c.r.section("anonymous-cacheable operations agree")
	ops := set(between(c.read(responseCache), cacheStart, closeBrace), quotedOp)
	if len(ops) == 0 {
		c.r.bad("frontCacheableOps extraction came back empty - %s moved", responseCache)
		return
	}
	if !c.inPackage(publicOps) {
		return
	}
	sf := set(between(c.read(publicOps), setStart, setEnd), quotedOp)
	switch {
	case len(sf) == 0:
		c.r.bad("PUBLIC_OPS extraction came back empty - %s changed shape", publicOps)
	case slices.Equal(ops, sf):
		c.r.pass("storefront-core PUBLIC_OPS == frontCacheableOps (%d ops)", len(ops))
	default:
		c.r.bad("storefront-core PUBLIC_OPS != frontCacheableOps")
		c.sides(ops, sf)
	}
}

// inPackage says the storefront package holds f. A checkout without the package - a backend-only sparse clone - has
// nothing to check; one with the package and not the file has a stale path here, and fails rather than going quiet
func (c contracts) inPackage(f string) bool {
	if _, err := os.Stat(filepath.Join(c.root, f)); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(c.root, storefrontPkg)); err != nil {
		c.skip("no %s in this checkout", storefrontPkg)
		return false
	}
	c.r.bad("%s is gone but %s is here - the path in cmd/skelet/internal/gates/contracts.go is stale", f, storefrontPkg)
	return false
}

var (
	getType      = regexp.MustCompile(`getType\(\)`)
	returnType   = regexp.MustCompile(`return .[a-z-]+.`)
	notNodeChar  = regexp.MustCompile(`[^a-z-]`)
	variantStart = regexp.MustCompile(`^type StructuredBlockType =`)
	variantLine  = regexp.MustCompile(`^ *\| `)
	quotedName   = regexp.MustCompile(`'([a-z-]+)'`)
	caseName     = regexp.MustCompile(`case "([a-z-]+)"`)
)

// lexicalNodes is one direction on purpose: a node the dashboard can author but a channel can't render silently
// blanks part of a blog post, while the reverse is only dead code
func (c contracts) lexicalNodes() {
	c.r.section("every dashboard lexical node has a storefront renderer")
	// The node type is the first return after a getType(), as one stream across the files the way awk read them
	files, _ := filepath.Glob(filepath.Join(c.root, dashLexical, "*.tsx"))
	slices.Sort(files)
	var nodes []string
	waiting := false
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		for line := range strings.SplitSeq(string(raw), "\n") {
			switch {
			case getType.MatchString(line):
				waiting = true
			case waiting && returnType.MatchString(line):
				nodes = append(nodes, strings.TrimPrefix(notNodeChar.ReplaceAllString(line, ""), "return"))
				waiting = false
			}
		}
	}
	var variants []string
	on := false
	for line := range strings.SplitSeq(c.read(filepath.Join(dashLexical, "structured.tsx")), "\n") {
		switch {
		case !on && variantStart.MatchString(line):
			on = true
		case on && !variantLine.MatchString(line):
			on = false
		case on:
			variants = append(variants, set([]string{line}, quotedName)...)
		}
	}
	slices.Sort(nodes)
	nodes = slices.Compact(nodes)
	slices.Sort(variants)
	variants = slices.Compact(variants)
	authored := len(nodes) + len(variants)
	if authored < 2 {
		c.r.bad("lexical node extraction came back empty - %s moved", dashLexical)
		return
	}
	all := append(slices.Clone(nodes), variants...)
	slices.Sort(all)
	all = slices.Compact(all)
	for _, sf := range c.storefronts() {
		name := sf.Name
		f := filepath.Join(filepath.FromSlash(sf.Dir), "app", "lib", "lexical-render.tsx")
		if _, err := os.Stat(filepath.Join(c.root, f)); err != nil {
			c.skip("%s has no lexical renderer", name)
			continue
		}
		if missing := minus(all, set(strings.Split(c.read(f), "\n"), caseName)); len(missing) > 0 {
			c.r.bad("%s has no case for: %s", name, strings.Join(missing, " "))
		} else {
			c.r.pass("%s renders all %d authored node types and variants", name, authored)
		}
	}
}

var importSpec = regexp.MustCompile(`(?:\bfrom|\bimport)\s*\(?\s*["']([^"'./][^"']*)["']`)

// frameworkFree - the core runs under any framework only while it imports none, and the shared hooks under any React
// framework only while they import no Next. A type import counts: it ties the package's types to the framework
func (c contracts) frameworkFree() {
	c.r.section("the storefront core imports no framework")
	for _, p := range []struct {
		dir    string
		banned []string
	}{
		{storefrontPkg, []string{"next", "react", "react-dom", "next-intl", "server-only"}},
		{reactPkg, []string{"next", "next-intl"}},
	} {
		if _, err := os.Stat(filepath.Join(c.root, p.dir)); err != nil {
			c.skip("no %s in this checkout", p.dir)
			continue
		}
		files, before := 0, c.r.failed
		_ = filepath.WalkDir(filepath.Join(c.root, p.dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || (d.IsDir() && d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			if d.IsDir() || !(strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")) {
				return nil
			}
			files++
			raw, _ := os.ReadFile(path)
			rel, _ := filepath.Rel(c.root, path)
			for _, m := range importSpec.FindAllStringSubmatch(string(raw), -1) {
				if root, _, _ := strings.Cut(m[1], "/"); slices.Contains(p.banned, root) {
					c.r.bad("%s imports %s", filepath.ToSlash(rel), m[1])
				}
			}
			return nil
		})
		if c.r.failed == before {
			c.r.pass("%s imports none of %s (%d files)", p.dir, strings.Join(p.banned, ", "), files)
		}
	}
}

var lineComment = regexp.MustCompile(`//.*`)

// normEvaluator is an evaluator with comments, quote style, optional-property marks, whitespace and semicolons gone -
// formatting differs between the two frontends, semantics must not
func normEvaluator(text string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(text, "\n") {
		line = lineComment.ReplaceAllString(line, "")
		line = strings.ReplaceAll(strings.ReplaceAll(line, `"`, "'"), "?:", ":")
		b.WriteString(line)
	}
	return strings.Map(func(r rune) rune {
		if r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' {
			return -1
		}
		return r
	}, b.String())
}

// evaluators - three ports of internal/cart/options_visibility.go exist because three runtimes evaluate it, and the
// server rejects a filled hidden option, so drift is a dead end for the shopper
func (c contracts) evaluators() {
	c.r.section("the conditional-option evaluators are the same code")
	if _, err := os.Stat(filepath.Join(c.root, dashOptVis)); err != nil {
		c.r.bad("%s is gone - update cmd/skelet/internal/gates/contracts.go", dashOptVis)
		return
	}
	if !c.inPackage(pkgOptVis) {
		return
	}
	if normEvaluator(c.read(pkgOptVis)) == normEvaluator(c.read(dashOptVis)) {
		c.r.pass("storefront-core evaluator matches the dashboard's, modulo formatting")
	} else {
		c.r.bad("storefront-core evaluator has diverged from %s - diff them and reconcile against internal/cart/options_visibility.go", dashOptVis)
	}
}

var (
	cspConst   = regexp.MustCompile(`^const ContentSecurityPolicy =`)
	ppConst    = regexp.MustCompile("const PermissionsPolicy = `([^`]*)`")
	nginxCSP   = regexp.MustCompile(`add_header Content-Security-Policy "([^"]*)"`)
	nginxPP    = regexp.MustCompile(`add_header Permissions-Policy "(.*)" always`)
	caddyCSP   = regexp.MustCompile(`Content-Security-Policy "([^"]*)"`)
	caddyPP    = regexp.MustCompile(`Permissions-Policy "(.*)"$`)
	whitespace = regexp.MustCompile(`\s+`)
)

func squeeze(s string) string { return strings.TrimSpace(whitespace.ReplaceAllString(s, " ")) }

// matches joins every group-1 match of re, line by line, as grep -o listed them
func matches(text string, re *regexp.Regexp) []string {
	var out []string
	for line := range strings.SplitSeq(text, "\n") {
		for _, m := range re.FindAllStringSubmatch(line, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

func (c contracts) goCSP() string {
	var parts []string
	on := false
	for line := range strings.SplitSeq(c.read(secHeaders), "\n") {
		if !on && !cspConst.MatchString(line) {
			continue
		}
		on = true
		part := strings.TrimSuffix(strings.TrimPrefix(line, "const ContentSecurityPolicy = "), "+")
		parts = append(parts, strings.ReplaceAll(part, `"`, ""))
		if !strings.HasSuffix(line, "+") {
			break
		}
	}
	return squeeze(strings.Join(parts, "\n"))
}

// headers - one policy, up to three files: the Go middleware is the source and skelet gen rewrites the nginx and Caddy
// copies from it. Checked here too, so a hand-edited copy or a skipped codegen run still fails
func (c contracts) headers() {
	c.r.section("security headers match across the copies that must")
	goCSP := c.goCSP()
	goPP := squeeze(strings.Join(matches(c.read(secHeaders), ppConst), "\n"))
	nginx := c.read(nginxConf)
	unescape := func(s []string) string { return strings.ReplaceAll(strings.Join(s, "\n"), `\"`, `"`) }
	c.compare("Content-Security-Policy", nginxConf, goCSP, squeeze(withoutFirst(matches(nginx, nginxCSP), " ${UPLOAD_ORIGINS}")))
	c.compare("Permissions-Policy", nginxConf, goPP, squeeze(unescape(matches(nginx, nginxPP))))
	// A fork that brings its own proxy may delete the bundled Caddyfile, so a genuine absence skips. A stale path must
	// not: the file is tracked and sparse-checkout never touches deploy/, so missing almost always means the path below
	// moved - and skipping would quietly drop the third copy while reporting green
	if _, err := os.Stat(filepath.Join(c.root, caddyfile)); err == nil {
		caddy := c.read(caddyfile)
		c.compare("Content-Security-Policy", caddyfile, goCSP, squeeze(withoutFirst(matches(caddy, caddyCSP), " {$UPLOAD_ORIGINS}")))
		c.compare("Permissions-Policy", caddyfile, goPP, squeeze(unescape(matches(caddy, caddyPP))))
	} else if tracked, _ := c.git("ls-files", "deploy/Caddyfile", "deploy/*/Caddyfile"); strings.TrimSpace(tracked) != "" {
		c.r.bad("%s is missing but a Caddyfile is tracked at %s - the path in cmd/skelet/internal/gates/contracts.go is stale",
			caddyfile, strings.SplitN(strings.TrimSpace(tracked), "\n", 2)[0])
	} else {
		c.skip("no %s - the edge profile isn't in this checkout", caddyfile)
	}
}

// withoutFirst drops the first placeholder from each matched line, as sed did line by line, and joins them
func withoutFirst(lines []string, placeholder string) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.Replace(l, placeholder, "", 1)
	}
	return strings.Join(out, "\n")
}

func (c contracts) compare(label, where, backend, other string) {
	switch {
	case backend == "" || other == "":
		c.r.bad("%s extraction came back empty - %s or %s moved", label, secHeaders, where)
	case backend == other:
		c.r.pass("%s matches %s", label, where)
	default:
		c.r.bad("%s differs between %s and %s - edit the Go copy and run skelet gen", label, secHeaders, where)
		split := func(s string) []string {
			return strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' })
		}
		c.sides(split(backend), split(other))
	}
}

var (
	widgetHint  = regexp.MustCompile(`"x-widget": *"([a-z-]*)"`)
	widgetStart = regexp.MustCompile(`^export const FORM_WIDGETS`)
	widgetKey   = regexp.MustCompile(`^  '([a-z-]+)'`)
)

// widgets - SchemaForm falls back to a plain text box when a widget key is missing, so a backend asking for one nobody
// registered looks fine and quietly makes the operator type slugs
func (c contracts) widgets() {
	c.r.section("every x-widget a Go params schema asks for is registered in the dashboard")
	hits, _ := c.git("grep", "-ho", `"x-widget": *"[a-z-]*"`, "--", "apps/backend/core/internal/**/*.go")
	requested := set(strings.Split(hits, "\n"), widgetHint)
	registered := set(between(c.read(widgetReg), widgetStart, closeBrace), widgetKey)
	switch {
	case len(requested) == 0:
		c.r.bad("found no x-widget hints at all - did the schemas move?")
	case len(registered) == 0:
		c.r.bad("FORM_WIDGETS extraction came back empty - %s moved", widgetReg)
	default:
		if missing := minus(requested, registered); len(missing) > 0 {
			c.r.bad("Go schemas ask for widgets the dashboard does not register: %s", strings.Join(missing, " "))
		} else {
			c.r.pass("all %d requested widgets are registered", len(requested))
		}
	}
}

// proto - core and Signals deploy separately, so the proto must stay compatible with main's, read as a tarball on
// stdin. Skipped when the stack is down, except in CI, which runs buf on the host
func (c contracts) proto() {
	c.r.section("the core-to-Signals proto stays backward compatible")
	ci := os.Getenv("CI") != ""
	hasProto := func(ref string) bool { _, err := c.git("cat-file", "-e", ref+":proto/buf.yaml"); return err == nil }
	baseline := ""
	for _, ref := range []string{"origin/main", "main"} {
		if _, err := c.git("rev-parse", "--verify", "-q", ref+"^{commit}"); err == nil {
			baseline = ref
			break
		}
	}
	switch {
	case baseline == "" && ci:
		c.r.bad("no main branch to compare the proto against - the checkout needs origin/main (fetch-depth: 0)")
		return
	case baseline == "":
		c.skip("no main or origin/main here - the proto compatibility check has nothing to compare against")
		return
	case !hasProto(baseline):
		// Nothing deployed carries the contract yet, so hold a branch to develop's instead
		dev := ""
		for _, ref := range []string{"develop", "origin/develop"} {
			if hasProto(ref) {
				dev = ref
				break
			}
		}
		if dev == "" {
			c.r.pass("%s has no proto yet - nothing deployed to stay compatible with", baseline)
			return
		}
		baseline = dev
	}
	lib := compose.Load(c.root)
	if !ci && !lib.Running("app") {
		c.skip("proto compatibility against %s skipped - the app container isn't running (skelet up, then skelet check contracts)", baseline)
		return
	}
	tar, err := exec.Command("git", "-C", c.root, "archive", "--format=tar", baseline, "proto").Output()
	if err != nil {
		c.r.bad("couldn't read %s's proto: %v", baseline, err)
		return
	}
	var buf *exec.Cmd
	if ci {
		buf = exec.Command("go", "tool", "github.com/bufbuild/buf/cmd/buf", "breaking", "../../../proto", "--against", "-#format=tar,subdir=proto")
		buf.Dir = filepath.Join(c.root, "apps", "backend", "core")
	} else {
		buf = compose.Command(c.root, lib.Env(), "exec", "-T", "app", "go", "tool", "github.com/bufbuild/buf/cmd/buf",
			"breaking", "/var/www/proto", "--against", "-#format=tar,subdir=proto")
	}
	buf.Stdin = bytes.NewReader(tar)
	if out, err := buf.CombinedOutput(); err != nil {
		c.r.bad("the Signals proto breaks compatibility with %s:", baseline)
		for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
			fmt.Fprintf(c.r.out, "        %s\n", line)
		}
		return
	}
	c.r.pass("no breaking changes against %s", baseline)
}
