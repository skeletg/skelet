package commands

import (
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/tools/go/ast/astutil"
)

var pluginInstallCmd = &cobra.Command{
	Use:   "install <git-url|local-dir>[@ref]",
	Short: "Vendor an out-of-tree plugin into plugins/ and compile it in",
	Long: `Fetches a plugin's source and installs it as plugins/<slug>/, so a plugin
can live in its own repository without forking this one.

  bin/cli plugin install https://github.com/acme/skelet-plugin-fraud@v1.2.0
  bin/cli plugin install git@github.com:acme/plugins.git --dir fraud --slug fraud
  bin/cli plugin install ../local-checkout/my-plugin

What it does, in order:

  1. Clones the URL at the ref (or copies the local dir), taking --dir within it.
  2. Verifies the source registers a plugin and warns on internal/* imports.
  3. Copies it to plugins/<slug>/, dropping .git, go.mod, go.sum and vendor/ -
     the code compiles as part of this module from here on.
  4. Rewrites the source's own module path in imports to this module's path,
     so a plugin with subpackages survives the move.
  5. Enables it (plugins.json + firstparty), runs go mod tidy to pull the
     plugin's dependencies in, and smoke-builds it.

A failed smoke build parks the plugin back out of the build (plugins/_<slug>)
so the tree keeps compiling - inspect, fix, then plugin enable.

A plugin shipping graphql/ needs bin/codegen before it builds, so the smoke
build is skipped for those - the checklist at the end says what is left.

Local-dir sources must be visible to this process - bin/cli runs in the app
container, which mounts the repo, so keep the source inside it or install by
URL.

This vendors CODE you are choosing to compile into core with full privileges.
Read it first - capabilities in the manifest are a disclosure, not a sandbox.`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginInstall,
}

func init() {
	PluginCmd.AddCommand(pluginInstallCmd)
	pluginInstallCmd.Flags().String("slug", "", "Directory name under plugins/ (default: source basename)")
	pluginInstallCmd.Flags().String("dir", "", "Subdirectory of the source that holds the plugin")
	pluginInstallCmd.Flags().Bool("force", false, "Overwrite an existing plugins/<slug>/ directory")
}

func runPluginInstall(cmd *cobra.Command, args []string) error {
	source, ref := splitSourceRef(args[0])
	subdir, _ := cmd.Flags().GetString("dir")
	slugFlag, _ := cmd.Flags().GetString("slug")
	force, _ := cmd.Flags().GetBool("force")

	staged, cleanup, err := stageSource(source, ref)
	if err != nil {
		return err
	}
	defer cleanup()
	if subdir != "" {
		staged = filepath.Join(staged, filepath.FromSlash(subdir))
		if st, err := os.Stat(staged); err != nil || !st.IsDir() {
			return fmt.Errorf("--dir %s: not a directory in the source", subdir)
		}
	}

	slug := slugFlag
	if slug == "" {
		slug = deriveSlug(source, subdir)
	}
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("slug %q must match [a-z][a-z0-9-]{1,39} - pass --slug to pick one", slug)
	}
	if slug == "firstparty" {
		return fmt.Errorf("slug %q is reserved for the generated registration package", slug)
	}

	if err := verifyRegisters(staged); err != nil {
		return err
	}
	if bad := internalImports(staged); len(bad) > 0 {
		fmt.Printf("WARNING: imports of core internals - the plugin contract is SDK-only, and bin/lint's depguard will refuse these unless allow-listed:\n")
		for _, imp := range bad {
			fmt.Printf("  %s\n", imp)
		}
		fmt.Println()
	}

	dest := filepath.Join("plugins", slug)
	parkedDest := filepath.Join("plugins", "_"+slug)
	for _, d := range []string{dest, parkedDest} {
		if _, err := os.Stat(d); err == nil {
			if !force {
				return fmt.Errorf("%s already exists (pass --force to overwrite)", d)
			}
			if err := os.RemoveAll(d); err != nil {
				return err
			}
		}
	}

	srcModule := sourceModulePath(staged)
	if err := copyPluginTree(staged, dest); err != nil {
		return err
	}
	if srcModule != "" && srcModule != "github.com/skeletg/skelet" {
		n, err := rewriteSelfImports(dest, srcModule, "github.com/skeletg/skelet/plugins/"+slug)
		if err != nil {
			return err
		}
		if n > 0 {
			fmt.Printf("  rewrote %d import(s) of %s\n", n, srcModule)
		}
	}
	fmt.Printf("  installed %s -> %s\n", args[0], dest)
	fmt.Println()

	if err := setPluginState(slug, true); err != nil {
		return err
	}
	if out, err := exec.Command("go", "mod", "tidy").CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy after install: %w\n%s", err, out)
	}

	hasGraphql := false
	if st, err := os.Stat(filepath.Join(dest, "graphql")); err == nil && st.IsDir() {
		hasGraphql = true
	}
	if !hasGraphql {
		if out, err := exec.Command("go", "build", "./"+filepath.ToSlash(dest)+"/...").CombinedOutput(); err != nil {
			// Park it so one broken install can't stop the whole module compiling
			if parkErr := setPluginState(slug, false); parkErr != nil {
				return fmt.Errorf("smoke build failed AND parking failed (%w) - remove %s by hand:\n%s", parkErr, dest, out)
			}
			return fmt.Errorf("smoke build failed - parked as plugins/_%s, fix and run: bin/cli plugin enable %s\n%s", slug, slug, out)
		}
	}

	fmt.Println()
	fmt.Println("Plugin installed. Next steps:")
	fmt.Println()
	step := 0
	printStep := func(lines ...string) {
		step++
		fmt.Printf("  %d. %s\n", step, lines[0])
		for _, l := range lines[1:] {
			fmt.Printf("     %s\n", l)
		}
	}
	printStep("Audit it: bin/cli plugin list shows the manifest, but capabilities",
		"are a disclosure, not a sandbox - read the code")
	if hasGraphql {
		printStep("bin/codegen - it ships graphql/, which must merge into the schema",
			"before anything builds")
		printStep("If it adds PUBLIC queries or mutations, the authz tripwires will name them:",
			"add reviewed entries to graph/admin/authz_coverage_test.go,",
			"graph/front/authz_coverage_test.go and graph/front/schema_exposure_test.go")
	} else {
		printStep("bin/codegen - regenerates the docs/config references it registers")
	}
	if _, err := os.Stat(filepath.Join(dest, "migrations")); err == nil {
		printStep(fmt.Sprintf("bin/cli plugin migrate up %s - it ships migrations", slug))
	}
	printStep("Rebuild")
	return nil
}

// The ref rides after the last @ past the final slash, so scp-style git@host:path URLs survive
func splitSourceRef(s string) (source, ref string) {
	at := strings.LastIndex(s, "@")
	if at > strings.LastIndex(s, "/") && at > strings.LastIndex(s, ":") {
		return s[:at], s[at+1:]
	}
	return s, ""
}

func looksLikeGitURL(s string) bool {
	return strings.Contains(s, "://") || strings.HasPrefix(s, "git@") || strings.HasSuffix(s, ".git")
}

// Returns a directory holding the source - a temp clone for URLs, the directory itself for local paths
func stageSource(source, ref string) (dir string, cleanup func(), err error) {
	noop := func() {}
	if !looksLikeGitURL(source) {
		st, err := os.Stat(source)
		if err != nil || !st.IsDir() {
			return "", noop, fmt.Errorf("source %q is neither a git URL nor a local directory", source)
		}
		if ref != "" {
			return "", noop, fmt.Errorf("a @ref only makes sense with a git URL - check out %s in %s instead", ref, source)
		}
		return source, noop, nil
	}

	tmp, err := os.MkdirTemp("", "skelet-plugin-*")
	if err != nil {
		return "", noop, err
	}
	cleanup = func() { os.RemoveAll(tmp) }

	if ref != "" {
		// Branches and tags clone shallow; a bare commit can't, so fall back to a full clone + checkout
		if out, err := exec.Command("git", "clone", "--depth", "1", "--branch", ref, source, tmp).CombinedOutput(); err != nil {
			_ = os.RemoveAll(tmp)
			if err := os.MkdirAll(tmp, 0o755); err != nil {
				return "", cleanup, err
			}
			if out2, err2 := exec.Command("git", "clone", source, tmp).CombinedOutput(); err2 != nil {
				cleanup()
				return "", noop, fmt.Errorf("git clone %s: %w\n%s", source, err2, out2)
			}
			if out2, err2 := exec.Command("git", "-C", tmp, "checkout", ref).CombinedOutput(); err2 != nil {
				cleanup()
				return "", noop, fmt.Errorf("checkout %s: %w\n%s (shallow attempt said: %s)", ref, err2, out2, out)
			}
		}
	} else if out, err := exec.Command("git", "clone", "--depth", "1", source, tmp).CombinedOutput(); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("git clone %s: %w\n%s", source, err, out)
	}
	return tmp, cleanup, nil
}

func deriveSlug(source, subdir string) string {
	base := subdir
	if base == "" {
		base = strings.TrimSuffix(source, ".git")
	}
	base = filepath.Base(filepath.FromSlash(base))
	// scp-style URLs keep host:user in Base when the path has no slash
	if i := strings.LastIndex(base, ":"); i >= 0 {
		base = base[i+1:]
	}
	return strings.ToLower(base)
}

// Same test pluginset.Discover applies to the tree, run before anything is copied
func verifyRegisters(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return err
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if strings.Contains(string(src), "sdk.RegisterPlugin(") {
			return nil
		}
	}
	return fmt.Errorf("no sdk.RegisterPlugin( call in %s - not a plugin root (is the plugin in a subdirectory? pass --dir)", dir)
}

func internalImports(dir string) []string {
	var bad []string
	seen := map[string]bool{}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(p, "github.com/skeletg/skelet/internal/") && !seen[p] {
				seen[p] = true
				bad = append(bad, p)
			}
		}
		return nil
	})
	return bad
}

// The staged repo's own module path, empty when it has no go.mod - vendored code joins THIS module,
// so imports of that path have to move with it
func sourceModulePath(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func copyPluginTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dest, 0o755)
		}
		base := filepath.Base(rel)
		if d.IsDir() {
			if base == ".git" || base == "vendor" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dest, rel), 0o755)
		}
		// The module boundary dissolves on install - keeping these would split the build in two
		if rel == "go.mod" || rel == "go.sum" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dest, rel), b, 0o644)
	})
}

// Rewrites imports of the source repo's own module path (self-imports across its subpackages) onto
// the vendored location, via the AST so string literals and comments are never touched
func rewriteSelfImports(dir, oldModule, newModule string) (int, error) {
	rewritten := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		changed := false
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if p != oldModule && !strings.HasPrefix(p, oldModule+"/") {
				continue
			}
			if astutil.RewriteImport(fset, f, p, newModule+strings.TrimPrefix(p, oldModule)) {
				changed = true
				rewritten++
			}
		}
		if !changed {
			return nil
		}
		out, err := os.Create(path)
		if err != nil {
			return err
		}
		defer out.Close()
		return format.Node(out, fset, f)
	})
	return rewritten, err
}
