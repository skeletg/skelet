package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/internal/pluginset"
)

var pluginAddCmd = &cobra.Command{
	Use:   "add <module>[@version]",
	Short: "Add a plugin as a Go module dependency and compile it in",
	Long: `Installs a plugin the way Go installs anything else: as a versioned module
dependency. Nothing is copied into this repo.

  bin/cli plugin add github.com/skeletg/lang-sr@v0.1.0
  bin/cli plugin add github.com/skeletg/lang-sr            # latest

A plugin with no published module is written in plugins/<slug>/ instead, by
plugin new. Those two are the only layouts - there is no replace-to-a-checkout
route, so a plugin's source is either in the module cache or in-tree.

What it does, in order:

  1. go get the module.
  2. Verifies the module registers a plugin, and warns on internal/* imports.
  3. Records it in plugins.json with its module path, and regenerates
     plugins/firstparty so the binary imports it.
  4. go mod tidy, then a smoke build.

Versus ` + "`plugin install`" + `, which copies source into plugins/<slug>/: this gives you a
version, a checksum in go.sum, one copy instead of two, and go get -u as the upgrade
path. Reach for install only when the plugin has no module - no go.mod, or no repo you
can fetch from.

A plugin shipping graphql/ needs bin/codegen before the tree builds, so the smoke
build is skipped for those and the checklist says what is left.

This compiles CODE into your binary with full privileges. Read it first - capabilities
in the manifest are a disclosure, not a sandbox.`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginAdd,
}

func init() {
	PluginCmd.AddCommand(pluginAddCmd)
	pluginAddCmd.Flags().String("slug", "", "Name to record it under (default: the module path's last element)")
}

func runPluginAdd(cmd *cobra.Command, args []string) error {
	slugFlag, _ := cmd.Flags().GetString("slug")

	module, version := splitModuleVersion(args[0])

	target := module
	if version != "" {
		target = module + "@" + version
	}
	fmt.Printf("  go get %s\n", target)
	if out, err := exec.Command("go", "get", target).CombinedOutput(); err != nil {
		return fmt.Errorf("go get %s: %w\n%s", target, err, out)
	}
	modDir, err := resolvedModuleDir(module)
	if err != nil {
		return err
	}

	slug := slugFlag
	if slug == "" {
		slug = path.Base(module)
	}
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("slug %q must match [a-z][a-z0-9-]{1,39} - pass --slug to pick one", slug)
	}
	if slug == "firstparty" {
		return fmt.Errorf("slug %q is reserved for the generated registration package", slug)
	}

	if err := verifyRegisters(modDir); err != nil {
		return err
	}
	if bad := internalImports(modDir); len(bad) > 0 {
		fmt.Println("WARNING: imports of core internals - the plugin contract is SDK-only, and a module")
		fmt.Println("plugin cannot import them at all once it is fetched rather than vendored:")
		for _, imp := range bad {
			fmt.Printf("  %s\n", imp)
		}
		fmt.Println()
	}

	set, err := pluginset.Load(".")
	if err != nil {
		return err
	}
	if existing, ok := set.Plugins[slug]; ok && existing.Module != "" && existing.Module != module {
		return fmt.Errorf("%q is already installed from %s - remove it first, or pass --slug", slug, existing.Module)
	}
	set.SetModule(slug, module)
	if err := set.Save("."); err != nil {
		return err
	}
	// Stage + regenerate before tidy: firstparty has to import the module, or tidy drops the require
	// as unused and the next build cannot find it
	if err := pluginset.Stage("."); err != nil {
		return err
	}
	if err := pluginset.Generate("."); err != nil {
		return err
	}
	if err := pluginset.WriteGraphSchemas("."); err != nil {
		return err
	}
	if out, err := exec.Command("go", "mod", "tidy").CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy after add: %w\n%s", err, out)
	}
	fmt.Printf("  %s enabled in %s\n", slug, pluginset.StateFile)

	hasGraphql := false
	if st, err := os.Stat(filepath.Join(modDir, "graphql")); err == nil && st.IsDir() {
		hasGraphql = true
	}
	if !hasGraphql {
		if out, err := exec.Command("go", "build", "./...").CombinedOutput(); err != nil {
			// Leave it recorded but off, so one bad add cannot stop the module compiling
			set.Set(slug, false)
			if saveErr := set.Save("."); saveErr != nil {
				return fmt.Errorf("smoke build failed AND disabling failed (%w):\n%s", saveErr, out)
			}
			_ = pluginset.Generate(".")
			_ = pluginset.WriteGraphSchemas(".")
			return fmt.Errorf("smoke build failed - left disabled, fix and run: bin/cli plugin enable %s\n%s", slug, out)
		}
	}

	fmt.Println()
	fmt.Println("Plugin added. Next steps:")
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
	if _, err := os.Stat(filepath.Join(modDir, "migrations")); err == nil {
		printStep(fmt.Sprintf("bin/cli plugin migrate up %s - it ships migrations", slug))
	}
	for _, half := range []struct{ dir, dest string }{
		{"_dashboard", "apps/dashboard/src/plugins/" + slug + "/"},
		{"_storefront", "apps/channels/<yours>/app/plugins/" + slug + "/"},
	} {
		if _, err := os.Stat(filepath.Join(modDir, half.dir)); err == nil {
			printStep(fmt.Sprintf("cp -r %s/%s %s", modDir, half.dir, half.dest),
				"the frontend half is TypeScript, so go get brings it down but cannot place it")
		}
	}
	printStep("Rebuild")
	return nil
}

// "mod@v1.2.3" -> ("mod", "v1.2.3"). The @ has to come after the last slash, so an scp-style URL survives
func splitModuleVersion(s string) (module, version string) {
	at := strings.LastIndex(s, "@")
	if at > strings.LastIndex(s, "/") {
		return s[:at], s[at+1:]
	}
	return s, ""
}

func resolvedModuleDir(module string) (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", module).Output()
	if err != nil {
		return "", fmt.Errorf("go list -m %s: %w", module, err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", fmt.Errorf("module %s resolved to no directory", module)
	}
	return dir, nil
}
