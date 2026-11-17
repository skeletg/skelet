// Package pluginadd is skelet plugin add and install: the ops CLI fetches the Go half into core and records the
// plugin's UI packages, then this installs those packages into the dashboard and every storefront that loads plugins,
// and regenerates - so both halves of a plugin arrive with one command
package pluginadd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/skeletg/skelet/cmd/skelet/internal/container"
	"github.com/skeletg/skelet/cmd/skelet/internal/gen"
	"github.com/skeletg/skelet/cmd/skelet/internal/plugins"
	"github.com/skeletg/skelet/cmd/skelet/internal/pnpm"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const AddAbout = `Installs a plugin published as a Go module, both halves: skelet plugin add <module>[@version] [--slug s].
The ops CLI fetches the Go module and compiles it in; the npm packages its skelet-plugin.json names go into the
dashboard and every storefront that loads plugins; then skelet gen. Flags after the module pass to the ops CLI.`

const InstallAbout = `Like skelet plugin add, for a plugin that isn't a Go module: skelet plugin install <git-url|dir>[@ref].
The source is vendored into plugins/<slug>/, and its UI packages installed the same way.`

// The seams a test swaps - each of these reaches a container, the network or the dev stack
var (
	opsCLI = func(root string, args ...string) error { return container.CLICall(root, args...).Run() }
	pnpmIn = func(root, app string, args ...string) error { return pnpm.Run(root, app, nil, args...) }
	regen  = func(root string) int { return gen.Run(root, nil) }
)

func Add(root string, args []string) int     { return run(root, "add", args) }
func Install(root string, args []string) int { return run(root, "install", args) }

func run(root, verb string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: skelet plugin %s <source>[@version] [flags]\n", verb)
		return 2
	}
	sys.HandleInterrupts()
	before, _, err := plugins.Load(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet plugin:", err)
		return 1
	}
	if err := opsCLI(root, append([]string{"plugin", verb}, args...)...); err != nil {
		return max(sys.ExitCode(err), 1)
	}
	after, _, err := plugins.Load(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet plugin:", err)
		return 1
	}
	for _, in := range Packages(root, Changed(before, after), after) {
		fmt.Printf("==> pnpm add %s (%s)\n", in.Spec, in.App)
		if err := pnpmIn(root, in.App, "add", in.Spec); err != nil {
			fmt.Fprintf(os.Stderr, "skelet plugin: installing %s into %s: %v\n", in.Spec, in.App, err)
			return max(sys.ExitCode(err), 1)
		}
	}
	return regen(root)
}

// Changed is the plugins the ops CLI just added or changed - new entries, and ones whose module or UI moved
func Changed(before, after *plugins.Set) []string {
	var out []string
	for slug, e := range after.Plugins {
		if was, ok := before.Plugins[slug]; !ok || was != e {
			out = append(out, slug)
		}
	}
	sort.Strings(out)
	return out
}

// Package is one package going into one app - "dashboard" or a storefront's name
type Package struct{ App, Spec string }

// Packages is what the changed, enabled plugins need: each dashboard package into the dashboard, each storefront
// package into every storefront with the plugin loader. A storefront without it, like the Astro template, draws no
// plugin UI, so it gets nothing
func Packages(root string, slugs []string, set *plugins.Set) []Package {
	var shops []string
	for _, sf := range storefronts.List(root) {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(sf.Dir), "app", "plugins", "loaded.ts")); err == nil {
			shops = append(shops, sf.Name)
		}
	}
	var out []Package
	for _, slug := range slugs {
		e, ok := set.Plugins[slug]
		if !ok || !e.Enabled {
			continue
		}
		if e.Dashboard != "" {
			out = append(out, Package{App: "dashboard", Spec: e.Dashboard})
		}
		if e.Storefront != "" {
			for _, shop := range shops {
				out = append(out, Package{App: shop, Spec: e.Storefront})
			}
		}
	}
	return slices.Clip(out)
}
