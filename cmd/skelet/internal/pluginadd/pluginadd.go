// Package pluginadd is skelet plugin add and install: the ops CLI fetches the Go half into core and records the
// plugin's UI packages, then this asks, installs the packages the apps are missing into the dashboard and every
// storefront that loads plugins, and regenerates - so both halves of a plugin arrive with one command
package pluginadd

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/skeletg/skelet/cmd/skelet/internal/container"
	"github.com/skeletg/skelet/cmd/skelet/internal/gen"
	"github.com/skeletg/skelet/cmd/skelet/internal/plugins"
	"github.com/skeletg/skelet/cmd/skelet/internal/pnpm"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const AddAbout = `Installs a plugin published as a Go module, both halves: skelet plugin add <module>[@version] [--slug s] [--yes].
The ops CLI fetches the Go module and compiles it in. Then, after asking, the npm packages its skelet-plugin.json
names go into the dashboard and every storefront that loads plugins, and skelet gen runs - which is the first time
the plugin's code runs on this machine, so read what it ships before answering. --yes skips the question. Run it
again to finish an install that stopped halfway. Other flags pass to the ops CLI.`

const InstallAbout = `Like skelet plugin add, for a plugin that isn't a Go module: skelet plugin install <git-url|dir>[@ref] [--yes].
The source is vendored into plugins/<slug>/, and its UI packages installed the same way.`

// The seams a test swaps - each of these reaches a container, the network, the dev stack or the keyboard
var (
	opsCLI  = func(root string, args ...string) error { return container.CLICall(root, args...).Run() }
	pnpmIn  = func(root, app string, args ...string) error { return pnpm.Run(root, app, nil, args...) }
	regen   = func(root string) int { return gen.Run(root, nil) }
	confirm = askOnTerminal
)

func Add(root string, args []string) int     { return run(root, "add", args) }
func Install(root string, args []string) int { return run(root, "install", args) }

func run(root, verb string, args []string) int {
	yes := false
	args = slices.DeleteFunc(slices.Clone(args), func(a string) bool {
		if a == "--yes" || a == "-y" {
			yes = true
		}
		return a == "--yes" || a == "-y"
	})
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: skelet plugin %s <source>[@version] [--yes] [flags]\n", verb)
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
	missing, clashes, err := Missing(root, after)
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet plugin:", err)
		return 1
	}
	fmt.Println()
	if slugs := Changed(before, after); len(slugs) > 0 {
		fmt.Printf("==> fetched and recorded: %s\n", strings.Join(slugs, ", "))
	}
	fmt.Println("Next, on this machine:")
	for _, in := range missing {
		fmt.Printf("  pnpm add %s, in %s\n", in.Spec, in.App)
	}
	fmt.Println("  skelet gen, which compiles the enabled plugins in and runs their code")
	for _, c := range clashes {
		fmt.Println("  note:", c)
	}
	if !yes {
		ok, asked := confirm("Go on?")
		switch {
		case !asked:
			fmt.Fprintf(os.Stderr, "Stopped before installing - there's no terminal to ask on. Read what the plugin ships, then run this again with --yes.\n")
			return 1
		case !ok:
			fmt.Fprintf(os.Stderr, "Stopped before installing. Run this again to go on, or back out with skelet cli plugin disable <slug>.\n")
			return 1
		}
	}
	for _, in := range missing {
		fmt.Printf("==> pnpm add %s (%s)\n", in.Spec, in.App)
		if err := pnpmIn(root, in.App, "add", in.Spec); err != nil {
			fmt.Fprintf(os.Stderr, "skelet plugin: installing %s into %s: %v - run this again once that's sorted, it picks up from here\n", in.Spec, in.App, err)
			return max(sys.ExitCode(err), 1)
		}
	}
	return regen(root)
}

// asked is false when there's nobody to ask
func askOnTerminal(question string) (ok, asked bool) {
	if !sys.IsTerminal(os.Stdin.Fd()) {
		return false, false
	}
	fmt.Print(question + " [y/N] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, true
	}
	return false, true
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

// Missing is the packages the enabled plugins want that an app doesn't depend on yet - every enabled plugin's, not
// only the one just added, so a run that stopped halfway, a plugin enabled later and a storefront forked since are
// all finished by the next one. A package an app already has is never touched: a plugin's manifest can add to an app
// and can't move what's there, so one naming next or zod changes nothing. Where the versions differ that's a clash,
// reported with the command that would move it
func Missing(root string, set *plugins.Set) (missing []Package, clashes []string, err error) {
	for _, app := range gen.UIApps(root) {
		declared, err := app.Declared(root)
		if err != nil {
			return nil, nil, err
		}
		for _, w := range app.Wanted(set) {
			name, want := plugins.PackageName(w.Spec), plugins.PackageVersion(w.Spec)
			has, ok := declared[name]
			switch {
			case !ok:
				missing = append(missing, Package{App: app.Name, Spec: w.Spec})
			case pinned(want) && !strings.Contains(has, ":") && has != want:
				clashes = append(clashes, fmt.Sprintf("%s already has %s at %s and plugin %s asks for %s - left alone; to move it: skelet pnpm %s add %s",
					app.Name, name, has, w.Slug, want, app.Name, w.Spec))
			}
		}
	}
	return missing, clashes, nil
}

// A version or range, as against a tag like latest, which says nothing about what's installed
func pinned(version string) bool {
	return version != "" && !unicode.IsLetter(rune(version[0]))
}
