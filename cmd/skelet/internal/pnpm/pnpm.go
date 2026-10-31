// Package pnpm runs pnpm for a frontend: with the host's pnpm when the app has a host install, since the gates should
// check the tree the dev server uses, else in the app's container
package pnpm

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/container"
	"github.com/skeletg/skelet/cmd/skelet/internal/node"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const About = `pnpm for a frontend: skelet pnpm dashboard run lint, skelet pnpm <storefront> add <pkg>. The host's
pnpm runs it when the app has a host install - the container keeps node_modules in a volume of its own, and
checking a different tree than the dev server runs is how the two drift apart. Otherwise it runs in the
app's container, exec'd when it's running, else a one-off. SKELET_FORCE_DOCKER=1 always uses the container.`

func Dashboard(root string, args []string) int { return replace(root, "dashboard", args) }

// Channel is a storefront's pnpm - the route puts the storefront's name first
func Channel(root string, args []string) int { return replace(root, args[0], args[1:]) }

func replace(root, app string, args []string) int {
	call, err := Call(root, app, nil, args...)
	if err != nil {
		return 1
	}
	return call.Replace()
}

// Run runs pnpm for app as a child, for a verb with more to do. env is added only on the host, where the app's
// config can read it - a container has its own
func Run(root, app string, env []string, args ...string) error {
	call, err := Call(root, app, env, args...)
	if err != nil {
		return err
	}
	return call.Run()
}

// Lockfile brings the workspace lockfile up to date with the manifests through app's pnpm. Only a resolve, so a one-off
// container starts none of the services the app's depends on
func Lockfile(root, app string) error {
	call, err := call(root, app, nil, true, "install", "--lockfile-only")
	if err != nil {
		return err
	}
	return call.Run()
}

var errUnusable = errors.New("pnpm can't run this app")

// RunDir runs pnpm in a workspace package's directory, which has no container of its own: with the host's pnpm when the
// workspace is installed there, else in the first storefront's container, pointed at the package
func RunDir(root, dir string, args ...string) error {
	abs := filepath.Join(root, filepath.FromSlash(dir))
	if os.Getenv("SKELET_FORCE_DOCKER") == "" && node.Tree(abs) == "pnpm" {
		if path, err := exec.LookPath("pnpm"); err == nil {
			return sys.Call{Dir: abs, Path: path, Argv: append([]string{"pnpm"}, args...), Env: sys.Env()}.Run()
		}
	}
	list := storefronts.List(root)
	if len(list) == 0 {
		fmt.Fprintf(os.Stderr, "no storefront to run %s's pnpm in\n", dir)
		return errUnusable
	}
	flags := append(compose.UserFlags(), "-w", storefronts.Mount+"/"+dir)
	return container.In(root, compose.Target{Service: list[0].Name, TTY: true, Flags: flags}, append([]string{"pnpm"}, args...)...).Run()
}

// Call is pnpm for app with args, or why it can't run - said on stderr
func Call(root, app string, env []string, args ...string) (sys.Call, error) {
	return call(root, app, env, false, args...)
}

func call(root, app string, env []string, noDeps bool, args ...string) (sys.Call, error) {
	// A storefront's container starts at the workspace root, so pnpm is pointed at the storefront's own directory
	rel, service, flags := filepath.Join("apps", "dashboard"), app, compose.UserFlags()
	if app != "dashboard" {
		rel = filepath.Join("apps", "channels", app)
		if s, ok := storefronts.Find(root, app); ok {
			rel, flags = filepath.FromSlash(s.Dir), append(flags, "-w", s.ContainerDir())
		}
	}
	dir := filepath.Join(root, rel)
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		fmt.Fprintf(os.Stderr, "%s is not in this checkout - removed by skelet setup's app selection?\n", rel)
		fmt.Fprintln(os.Stderr, "Bring it back with: skelet setup --all (or --apps ...)")
		return sys.Call{}, errUnusable
	}
	if os.Getenv("SKELET_FORCE_DOCKER") == "" {
		switch node.Tree(dir) {
		case "npm":
			fmt.Fprintf(os.Stderr, "%s/node_modules is an npm install from before the switch to pnpm - replace it:\n", rel)
			fmt.Fprintf(os.Stderr, "    rm -rf %s/node_modules && (cd %s && pnpm install)\n", rel, rel)
			return sys.Call{}, errUnusable
		case "pnpm":
			if path, err := exec.LookPath("pnpm"); err == nil {
				return sys.Call{Dir: dir, Path: path, Argv: append([]string{"pnpm"}, args...), Env: append(sys.Env(), env...)}, nil
			}
		}
	}
	return container.In(root, compose.Target{Service: service, TTY: true, NoDeps: noDeps, Flags: flags}, append([]string{"pnpm"}, args...)...), nil
}
