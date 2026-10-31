// Package global is the released skelet on your PATH. Outside a project it starts one; inside one it hands the command
// to that project's own CLI, so an installed binary never drives a Skelet of a different version
package global

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/skeletg/skelet/apps/skelet/internal/checkout"
	"github.com/skeletg/skelet/apps/skelet/internal/doctor"
	"github.com/skeletg/skelet/apps/skelet/internal/project"
	"github.com/skeletg/skelet/apps/skelet/internal/sys"
)

// Seams for the tests: replacing the process ends a test binary, and fetching a release needs a remote
var (
	replace   = sys.Replace
	newSource = func(repo string) project.Source { return project.Git{URL: repo} }
)

const usage = `skelet - start and run Skelet projects

  new <dir>   fetch a Skelet release into <dir> and run its setup   [skelet new --help]
  doctor      check this machine has what Skelet needs
  version     this binary's version
  help        this screen

Every other command runs inside a project - cd into one and run skelet help there.
`

const newUsage = `skelet new <dir> [--version vX.Y.Z] [--repo URL] [--force] [setup flags]

Fetches a Skelet release into <dir> as a git checkout and runs its setup, handing it every
flag not listed here (--yes takes the defaults). Skelet is the checkout's upstream remote,
and main is your branch, starting at the release.

  --version   the release to start from - by default this binary's own, else the newest
  --repo      fetch from a fork or a mirror instead of ` + project.Upstream + `
  --force     allow <dir> inside another Skelet project
`

func Main(version string, args []string) int {
	if os.Getenv("SKELET_SHIM") != "" {
		fmt.Fprintln(os.Stderr, "skelet: a release build is sitting in this checkout's CLI cache - remove .skelet/bin and run ./skelet again")
		return 1
	}
	verb := ""
	if len(args) > 0 {
		verb = args[0]
	}
	switch {
	case verb == "version":
		fmt.Printf("skelet %s\n", version)
		return 0
	// new channel is a project's own verb, the old name for add channel
	case verb == "new" && len(args) > 1 && args[1] != "channel":
		return create(version, args[1:])
	}
	cwd, _ := os.Getwd()
	if root, ok := checkout.Find(cwd); ok {
		return handOff(root, args)
	}
	switch {
	case verb == "" || verb == "help" || verb == "-h" || verb == "--help":
		fmt.Print(usage)
		return 0
	case verb == "new" && len(args) == 1:
		fmt.Fprint(os.Stderr, newUsage)
		return 2
	case verb == "doctor":
		return doctor.Machine(args[1:])
	}
	fmt.Fprintf(os.Stderr, "skelet: %s runs inside a Skelet project - cd into one, or start one with skelet new <dir>\n",
		strings.Join(args[:min(2, len(args))], " "))
	return 2
}

func create(version string, args []string) int {
	o, err := project.ParseArgs(args)
	if errors.Is(err, project.ErrHelp) {
		fmt.Print(newUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "skelet new: %v\n", err)
		return 2
	}
	dir, tag, err := project.Create(context.Background(), newSource(o.Repo), o, version, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skelet new: %v\n", err)
		return 1
	}
	if _, err := os.Stat(filepath.Join(dir, "apps", "skelet", "go.mod")); err != nil {
		fmt.Fprintf(os.Stderr, "skelet new: %s predates the Go CLI, so its setup is the bash one - it needs OpenSSL 1.1.1+, which stock macOS lacks (brew install openssl)\n", tag)
	}
	// The shim acts on the checkout you stand in, so setup runs in the new project whatever path it was reached by
	if err := os.Chdir(dir); err != nil {
		fmt.Fprintf(os.Stderr, "skelet new: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "==> running setup in %s\n", dir)
	return handOff(dir, append([]string{"setup"}, o.Setup...))
}

func handOff(root string, args []string) int {
	// Through bash rather than the file itself, so a shim that lost its +x bit (WSL clones) still runs
	bash, err := exec.LookPath("bash")
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet: a Skelet project's front door is a bash script, and there's no bash on the PATH")
		return 1
	}
	return replace(bash, append([]string{"bash", filepath.Join(root, "skelet")}, args...), os.Environ())
}
