package sdk

// Two gates guard the plugin-facing surface: TestAPISurface records every exported declaration in api.json, so drift shows up in review and demands a Version bump
// TestAPICompatibility is the authoritative one, running apidiff against the last release tag, which knows Go's real rules where a line diff does not - it skips until a v* tag exists

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/exp/apidiff"
	"golang.org/x/tools/go/packages"
)

var updateSurface = flag.Bool("update-sdk-api", false, "rewrite api.json with the current sdk surface")

const (
	surfaceFile   = "api.json"
	regenerateCmd = "go test . -run TestAPISurface -update-sdk-api"
	// Every package a plugin author may import - this module is the SDK, and sdktest is public
	// contract too (plugins unit-test against the fake host) so it is gated alongside
	publicPattern = "./..."
	archivePrefix = "apps/backend/sdk/"
	// Where the SDK lived while it was a package of core's module - tags from then are compared through it
	legacyArchivePrefix = "apps/backend/"
	modulePath          = "github.com/skeletg/skelet/apps/backend/sdk"
	// apidiff names symbols relative to the path it is handed, which is the module's parent so the root package reads as ./sdk
	versionConstChange = "./sdk.Version:"
)

// The checked-in record - Packages is keyed by import path so adding or dropping a public package
// shows up as a diff rather than silently changing nothing
type surface struct {
	Version  string              `json:"version"`
	Packages map[string][]string `json:"packages"`
}

func TestAPISurface(t *testing.T) {
	cur := surface{Version: Version, Packages: surfaceOf(loadPackages(t, ".", publicPattern))}
	if *updateSurface {
		writeSurfaceFile(t, cur)
		return
	}

	raw, err := os.ReadFile(surfaceFile)
	if err != nil {
		t.Fatalf("read %s: %v - bootstrap the baseline with: %s", surfaceFile, err, regenerateCmd)
	}
	var old surface
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatalf("parse %s: %v - regenerate: %s", surfaceFile, err, regenerateCmd)
	}
	if old.Version != Version {
		t.Fatalf("sdk.Version is %s but %s records %s - version and baseline move together: %s", Version, surfaceFile, old.Version, regenerateCmd)
	}

	removed, added := diffSurfaces(old.Packages, cur.Packages)
	if len(removed) == 0 && len(added) == 0 {
		return
	}
	oldV, err := parseSemver(old.Version)
	if err != nil {
		t.Fatalf("%s version %q: %v", surfaceFile, old.Version, err)
	}
	var b strings.Builder
	if len(removed) > 0 {
		fmt.Fprintf(&b, "removed or changed declarations:\n  %s\n", strings.Join(removed, "\n  "))
	}
	if len(added) > 0 {
		fmt.Fprintf(&b, "additions:\n  %s\n", strings.Join(added, "\n  "))
	}
	t.Fatalf("sdk surface drifted from %s:\n%sbump %s, then regenerate: %s\nTestAPICompatibility carries the authoritative breaking/compatible verdict", surfaceFile, b.String(), bumpWanted(oldV, len(removed) > 0), regenerateCmd)
}

// The authoritative gate - apidiff knows Go's compatibility rules, so an added interface method is
// reported incompatible rather than counted as an addition the way a line diff sees it
func TestAPICompatibility(t *testing.T) {
	tag, err := lastReleaseTag()
	if err != nil {
		t.Skipf("no release tag to compare against (%v) - this gate starts enforcing once the first v* tag is cut", err)
	}

	oldDir, oldPattern := materializeAt(t, tag)
	oldPkgs := loadPackages(t, oldDir, oldPattern)
	if len(oldPkgs) == 0 {
		t.Skipf("%s has no %s packages - nothing to compare", tag, publicPattern)
	}
	newPkgs := loadPackages(t, ".", publicPattern)

	modPath := path.Dir(modulePath)
	report := apidiff.ModuleChanges(
		&apidiff.Module{Path: modPath, Packages: oldPkgs},
		&apidiff.Module{Path: modPath, Packages: newPkgs},
	)
	if len(report.Changes) == 0 {
		return
	}

	var incompatible, compatible []string
	for _, c := range report.Changes {
		if strings.HasPrefix(c.Message, versionConstChange) {
			continue // the release marker itself moves every release by design - apidiff is right that a const value change is breaking, it just is not news here
		}
		if c.Compatible {
			compatible = append(compatible, c.Message)
			continue
		}
		incompatible = append(incompatible, c.Message)
	}

	if len(incompatible) == 0 && len(compatible) == 0 {
		return // everything apidiff reported was the release marker itself
	}

	oldV, err := versionOf(oldPkgs, modPath)
	if err != nil {
		t.Fatalf("read sdk.Version at %s: %v", tag, err)
	}
	curV, err := parseSemver(Version)
	if err != nil {
		t.Fatalf("sdk.Version %q: %v", Version, err)
	}
	if admitsChange(oldV, curV, len(incompatible) > 0) {
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "apidiff vs %s (sdk.Version %s -> %s):\n", tag, semverText(oldV), Version)
	if len(incompatible) > 0 {
		fmt.Fprintf(&b, "  BREAKING:\n    %s\n", strings.Join(incompatible, "\n    "))
	}
	if len(compatible) > 0 {
		fmt.Fprintf(&b, "  compatible:\n    %s\n", strings.Join(compatible, "\n    "))
	}
	t.Fatalf("%sbump %s", b.String(), bumpWanted(oldV, len(incompatible) > 0))
}

// semver has no String of its own and it is unexported, so formatting lives here rather than widening the package
func semverText(v semver) string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// Refuses a rewrite the current Version does not admit - this is where the bump discipline is
// actually enforced, since the checker above only ever sees baseline and code already in sync
func writeSurfaceFile(t *testing.T, cur surface) {
	t.Helper()
	if raw, err := os.ReadFile(surfaceFile); err == nil {
		var old surface
		if err := json.Unmarshal(raw, &old); err != nil {
			t.Fatalf("parse %s: %v", surfaceFile, err)
		}
		curV, err := parseSemver(Version)
		if err != nil {
			t.Fatalf("sdk.Version %q: %v", Version, err)
		}
		oldV, err := parseSemver(old.Version)
		if err != nil {
			t.Fatalf("%s version %q: %v", surfaceFile, old.Version, err)
		}
		removed, added := diffSurfaces(old.Packages, cur.Packages)
		breaking := len(removed) > 0
		if (breaking || len(added) > 0) && !admitsChange(oldV, curV, breaking) {
			detail := added
			if breaking {
				detail = removed
			}
			t.Fatalf("refusing to record this change under v%s (baseline v%s) - bump %s first:\n  %s", Version, old.Version, bumpWanted(oldV, breaking), strings.Join(detail, "\n  "))
		}
	}
	out, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(surfaceFile, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	var n int
	for _, lines := range cur.Packages {
		n += len(lines)
	}
	t.Logf("wrote %s: v%s, %d packages, %d declarations", surfaceFile, Version, len(cur.Packages), n)
}

// Pre-1.0 the minor carries breaking changes and the patch carries additions - from 1.0 it is the usual major/minor split. bumpWanted and admitsChange are the same rule read two ways, so the checker's advice and the updater's refusal can never disagree
func bumpWanted(old semver, breaking bool) string {
	switch {
	case old.Major == 0 && breaking:
		return "the minor component of sdk.Version - pre-1.0, breaking rides the minor"
	case old.Major == 0:
		return "at least the patch component of sdk.Version"
	case breaking:
		return "the major component of sdk.Version"
	default:
		return "at least the minor component of sdk.Version"
	}
}

func admitsChange(old, cur semver, breaking bool) bool {
	if cur.compare(old) <= 0 {
		return false
	}
	if !breaking {
		return true
	}
	if old.Major == 0 {
		return cur.Major > 0 || cur.Minor > old.Minor
	}
	return cur.Major > old.Major
}

// git archive resolves pathspecs against the working directory and this test runs from sdk/, so every
// git call is anchored at the repo root instead
func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Most recent v* tag reachable from HEAD - the error is reported as a skip, since "no releases yet"
// is a legitimate state and must not read as a failure
func lastReleaseTag() (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", fmt.Errorf("locate repo root: %w", err)
	}
	out, err := exec.Command("git", "-C", root, "describe", "--tags", "--abbrev=0", "--match", "v*").Output()
	if err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			return "", fmt.Errorf("git describe: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	tag := strings.TrimSpace(string(out))
	if tag == "" {
		return "", errors.New("git describe returned nothing")
	}
	return tag, nil
}

// Extracts the tagged SDK with a go.mod beside it so deps resolve from the module cache, and returns the module root and the pattern naming its packages
// A tag whose SDK sits under core's go.mod is materialized as core's root with the sdk tree under it
func materializeAt(t *testing.T, tag string) (dir, pattern string) {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Skipf("locate repo root: %v", err)
	}
	prefix, pattern := archivePrefix, publicPattern
	paths := []string{strings.TrimSuffix(archivePrefix, "/")}
	if exec.Command("git", "-C", root, "cat-file", "-e", tag+":"+archivePrefix+"go.mod").Run() != nil {
		prefix, pattern = legacyArchivePrefix, "./sdk/..."
		paths = []string{prefix + "go.mod", prefix + "go.sum", prefix + "sdk"}
	}
	dir = t.TempDir()
	cmd := exec.Command("git", append([]string{"-C", root, "archive", "--format=tar", tag}, paths...)...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			t.Skipf("git archive %s: %s - cannot compare against this tag", tag, strings.TrimSpace(string(ee.Stderr)))
		}
		t.Skipf("git archive %s: %v", tag, err)
	}

	tr := tar.NewReader(bytes.NewReader(out))
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read archive of %s: %v", tag, err)
		}
		rel, ok := strings.CutPrefix(h.Name, prefix)
		if !ok || rel == "" {
			continue
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				t.Fatal(err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(dst)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir, pattern
}

func loadPackages(t *testing.T, dir, pattern string) []*types.Package {
	t.Helper()
	cfg := &packages.Config{
		Dir:  dir,
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps,
	}
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		t.Fatalf("load %s in %s: %v", pattern, dir, err)
	}
	out := make([]*types.Package, 0, len(pkgs))
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			t.Fatalf("load %s: %v", p.PkgPath, p.Errors)
		}
		out = append(out, p.Types)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path() < out[j].Path() })
	return out
}

// Reads the Version const out of an already type-checked sdk package, so the old side's version comes
// from the tag itself rather than from anything the working tree could have edited
func versionOf(pkgs []*types.Package, modPath string) (semver, error) {
	want := modPath + "/sdk"
	for _, p := range pkgs {
		if p.Path() != want {
			continue
		}
		c, ok := p.Scope().Lookup("Version").(*types.Const)
		if !ok {
			return semver{}, fmt.Errorf("%s: Version is not a const", want)
		}
		v, err := strconv.Unquote(c.Val().ExactString())
		if err != nil {
			return semver{}, err
		}
		return parseSemver(v)
	}
	return semver{}, fmt.Errorf("package %s not found", want)
}

func diffSurfaces(old, cur map[string][]string) (removed, added []string) {
	for pkg, curLines := range cur {
		r, a := diffLines(old[pkg], curLines)
		removed = append(removed, qualify(pkg, r)...)
		added = append(added, qualify(pkg, a)...)
	}
	for pkg, oldLines := range old {
		if _, still := cur[pkg]; !still {
			removed = append(removed, qualify(pkg, oldLines)...)
		}
	}
	sort.Strings(removed)
	sort.Strings(added)
	return removed, added
}

func qualify(pkg string, lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = path.Base(pkg) + ": " + l
	}
	return out
}

func diffLines(old, cur []string) (removed, added []string) {
	oldSet := make(map[string]bool, len(old))
	for _, l := range old {
		oldSet[l] = true
	}
	curSet := make(map[string]bool, len(cur))
	for _, l := range cur {
		curSet[l] = true
		if !oldSet[l] {
			added = append(added, l)
		}
	}
	for _, l := range old {
		if !curSet[l] {
			removed = append(removed, l)
		}
	}
	return removed, added
}

func surfaceOf(pkgs []*types.Package) map[string][]string {
	out := make(map[string][]string, len(pkgs))
	for _, p := range pkgs {
		out[p.Path()] = surfaceLines(p)
	}
	return out
}

// One sorted line per exported const, var, func, type, struct field, interface method and method - field- and
// method-level granularity so an addition diffs as an addition rather than mutating one giant type line into a removal
func surfaceLines(self *types.Package) []string {
	qual := func(p *types.Package) string {
		if p == self {
			return ""
		}
		return p.Name()
	}

	var lines []string
	scope := self.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		switch o := obj.(type) {
		case *types.Const:
			if name == "Version" {
				continue // tracked by the surface file's version field - listing its value would turn every bump into a breaking change
			}
			lines = append(lines, fmt.Sprintf("const %s %s = %s", name, types.TypeString(o.Type(), qual), o.Val().ExactString()))
		case *types.Var:
			lines = append(lines, fmt.Sprintf("var %s %s", name, types.TypeString(o.Type(), qual)))
		case *types.Func:
			lines = append(lines, fmt.Sprintf("func %s%s", name, sigString(o.Type().(*types.Signature), qual)))
		case *types.TypeName:
			lines = append(lines, typeLines(o, qual)...)
		}
	}
	sort.Strings(lines)
	return lines
}

func typeLines(o *types.TypeName, qual types.Qualifier) []string {
	name := o.Name()
	if o.IsAlias() {
		return []string{fmt.Sprintf("type %s = %s", name, types.TypeString(o.Type(), qual))}
	}
	named := o.Type().(*types.Named)
	var lines []string
	switch u := named.Underlying().(type) {
	case *types.Struct:
		lines = append(lines, fmt.Sprintf("type %s struct", name))
		for i := 0; i < u.NumFields(); i++ {
			f := u.Field(i)
			if !f.Exported() {
				continue
			}
			l := fmt.Sprintf("field %s.%s %s", name, f.Name(), types.TypeString(f.Type(), qual))
			if tag := u.Tag(i); tag != "" {
				l += " " + strconv.Quote(tag)
			}
			lines = append(lines, l)
		}
	case *types.Interface:
		lines = append(lines, fmt.Sprintf("type %s interface", name))
		for m := range u.Methods() {
			if !m.Exported() {
				continue
			}
			lines = append(lines, fmt.Sprintf("ifacemethod %s.%s%s", name, m.Name(), sigString(m.Type().(*types.Signature), qual)))
		}
	default:
		lines = append(lines, fmt.Sprintf("type %s %s", name, types.TypeString(u, qual)))
	}
	for sel := range types.NewMethodSet(types.NewPointer(named)).Methods() {
		m := sel.Obj().(*types.Func)
		if !m.Exported() {
			continue
		}
		sig := m.Type().(*types.Signature)
		lines = append(lines, fmt.Sprintf("method (%s) %s%s", types.TypeString(sig.Recv().Type(), qual), m.Name(), sigString(sig, qual)))
	}
	return lines
}

// Types only, no parameter names - a rename is not an API change and must not trip the gate
func sigString(sig *types.Signature, qual types.Qualifier) string {
	var b strings.Builder
	b.WriteByte('(')
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		typ := params.At(i).Type()
		if sig.Variadic() && i == params.Len()-1 {
			b.WriteString("...")
			typ = typ.(*types.Slice).Elem()
		}
		b.WriteString(types.TypeString(typ, qual))
	}
	b.WriteByte(')')
	res := sig.Results()
	switch res.Len() {
	case 0:
	case 1:
		b.WriteString(" " + types.TypeString(res.At(0).Type(), qual))
	default:
		b.WriteString(" (")
		for i := 0; i < res.Len(); i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(types.TypeString(res.At(i).Type(), qual))
		}
		b.WriteByte(')')
	}
	return b.String()
}
