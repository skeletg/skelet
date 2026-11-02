package gates

import (
	"fmt"
	"os"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/gen"
)

const CheckAbout = `Every gate that can fail a commit, in one command - what the pre-commit hook and CI both run, so
"skelet check passes" is the whole contract. About a second, no running backend: only the contracts
gate's proto step uses the app container, and skips when it's down.

  (no flags)  the three fast gates
  --codegen   also regenerate and diff (needs the container; add the backend for the TS half)
  --list      what each gate is for

Each gate runs on its own too: skelet check pins, structure, contracts, codegen.`

// A gate, what it protects, and how to run it alone
type gate struct {
	name, what, alone string
	run               func(root string) int
}

var checks = []gate{
	{"pins", "one version per pin across the compose files, Dockerfiles and workflows", "skelet check pins",
		func(root string) int { return pinCheck(root, os.Stdout) }},
	{"structure", "the installer, env templates, the changelog", "skelet check structure",
		func(root string) int { return repoCheck(root, os.Stdout) }},
	{"contracts", "the SYNC sentinels: lists that must agree across backend, dashboard and storefront", "skelet check contracts",
		func(root string) int { return contractCheck(root, os.Stdout) }},
}

func Check(root string, args []string) int {
	codegen := false
	for _, a := range args {
		switch a {
		case "--codegen":
			codegen = true
		case "--list":
			for _, g := range checks {
				fmt.Printf("  %-10s %-23s %s\n", g.name, g.alone, g.what)
			}
			fmt.Printf("  %-10s %-23s %s\n", "codegen", "skelet check codegen", "generated files match their sources (--codegen only)")
			return 0
		case "-h", "--help":
			fmt.Println(CheckAbout)
			return 0
		default:
			fmt.Printf("Unknown flag: %s\n%s\n", a, CheckAbout)
			return 2
		}
	}
	var failed []string
	for _, g := range checks {
		fmt.Printf("\n>>> %s (%s)\n", g.name, g.what)
		if g.run(root) != 0 {
			failed = append(failed, g.alone)
		}
	}
	// Last because it's the slow one, and because a stale-codegen failure is the only one here you fix by running a
	// command rather than editing a file
	if codegen {
		fmt.Println("\n>>> codegen (generated files match their sources)")
		if gen.Check(root, nil) != 0 {
			failed = append(failed, "skelet check codegen")
		}
	}
	fmt.Println()
	if len(failed) == 0 {
		fmt.Println("skelet check: all gates passed")
		return 0
	}
	fmt.Printf("skelet check: %d gate(s) failed - %s\n", len(failed), strings.Join(failed, ", "))
	fmt.Println("Each prints what to change; rerun the single one while you fix it.")
	return 1
}
