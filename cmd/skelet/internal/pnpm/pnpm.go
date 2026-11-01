// Package pnpm runs pnpm for a frontend: with the host's pnpm when the app has a host install, since the gates should
// check the tree the dev server uses, else in the app's container
package pnpm

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/container"
	"github.com/skeletg/skelet/cmd/skelet/internal/node"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const About = `pnpm for a frontend: skelet pnpm dashboard run lint, skelet pnpm <storefront> add <pkg>, or for one of the
packages the storefronts share: skelet pnpm storefront-core test. The host's
pnpm runs it when the app has a host install - the container keeps node_modules in a volume of its own, and
checking a different tree than the dev server runs is how the two drift apart. Otherwise it runs in the
app's container, exec'd when it's running, else a one-off. SKELET_FORCE_DOCKER=1 always uses the container.`

func Dashboard(root string, args []string) int { return replace(root, "dashboard", args) }

// Channel is a storefront's pnpm - the route puts the storefront's name first
func Channel(root string, args []string) int { return replace(root, args[0], args[1:]) }

// Package is pnpm in one of packages/ - the route puts the package's directory name first
func Package(root string, args []string) int {
	call, err := dirCall(root, "packages/"+args[0], args[1:]...)
	if err != nil {
		return 1
	}
	return call.Replace()
}

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

// WorkspaceLockfile brings the root lockfile up to date with the workspace's manifests, in a throwaway container on the
// storefronts' Node image. It needs no storefront checked out, built or running - a shop just forked from a template
// setup left out of the working tree can count on none. env/ is masked, as in a storefront's own container
func WorkspaceLockfile(root string) error {
	argv := append(append([]string{"docker", "run", "--rm"}, compose.RunAs()...), compose.UserFlags()...)
	argv = append(argv, "-e", "HOME=/tmp", "-e", "NPM_CONFIG_UPDATE_NOTIFIER=false", "-v", root+":/w", "--tmpfs", "/w/env", "-w", "/w", nodeImage(root),
		"npx", "-y", "pnpm@"+pinnedPnpm(root), "install", "--lockfile-only")
	return sys.Call{Dir: root, Path: sys.Lookup("docker"), Argv: argv, Env: sys.Env()}.Run()
}

// nodeImage is the storefronts' Node, by the .nvmrc the pins gate holds every Dockerfile to
func nodeImage(root string) string {
	raw, _ := os.ReadFile(filepath.Join(root, ".nvmrc"))
	if v := strings.TrimSpace(string(raw)); v != "" {
		return "node:" + v + "-alpine"
	}
	return "node:lts-alpine"
}

var packageManager = regexp.MustCompile(`"packageManager":\s*"pnpm@([0-9][0-9.]*)"`)

// pinnedPnpm is the pnpm a workspace manifest pins - the pins gate holds them to one
func pinnedPnpm(root string) string {
	for _, pat := range []string{"packages/*/package.json", "templates/*/package.json", "apps/channels/*/package.json"} {
		manifests, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pat)))
		slices.Sort(manifests)
		for _, m := range manifests {
			raw, _ := os.ReadFile(m)
			if v := packageManager.FindSubmatch(raw); v != nil {
				return string(v[1])
			}
		}
	}
	return "latest"
}

var errUnusable = errors.New("pnpm can't run this app")

// RunDir runs pnpm in a workspace package's directory, which has no container of its own: with the host's pnpm when the
// workspace is installed there, else in the first storefront's container, pointed at the package
func RunDir(root, dir string, args ...string) error {
	call, err := dirCall(root, dir, args...)
	if err != nil {
		return err
	}
	return call.Run()
}

func dirCall(root, dir string, args ...string) (sys.Call, error) {
	abs := filepath.Join(root, filepath.FromSlash(dir))
	if os.Getenv("SKELET_FORCE_DOCKER") == "" && node.Tree(abs) == "pnpm" {
		if path, err := exec.LookPath("pnpm"); err == nil {
			return sys.Call{Dir: abs, Path: path, Argv: append([]string{"pnpm"}, args...), Env: sys.Env()}, nil
		}
	}
	list := storefronts.List(root)
	if len(list) == 0 {
		fmt.Fprintf(os.Stderr, "no storefront checked out to run %s's pnpm in - its container is where the workspace is installed\n", dir)
		return sys.Call{}, errUnusable
	}
	flags := append(compose.UserFlags(), "-w", storefronts.Mount+"/"+dir)
	return container.In(root, compose.Target{Service: list[0].Name, TTY: true, Flags: flags}, append([]string{"pnpm"}, args...)...), nil
}

// Call is pnpm for app with args, or why it can't run - said on stderr
func Call(root, app string, env []string, args ...string) (sys.Call, error) {
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
	return container.In(root, compose.Target{Service: service, TTY: true, Flags: flags}, append([]string{"pnpm"}, args...)...), nil
}
