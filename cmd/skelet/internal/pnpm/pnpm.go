// Package pnpm runs pnpm for a frontend: with the host's pnpm when the app has a host install, since the gates should
// check the tree the dev server uses, else in the app's container
package pnpm

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/container"
	"github.com/skeletg/skelet/cmd/skelet/internal/gitx"
	"github.com/skeletg/skelet/cmd/skelet/internal/node"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const About = `pnpm for a frontend: skelet pnpm dashboard run lint, skelet pnpm <storefront> add <pkg>, or for one of the
packages the storefronts share: skelet pnpm storefront-core test. skelet pnpm --lockfile brings the workspace's
pnpm-lock.yaml up to date in a throwaway container, for a checkout with no storefront to run it in. The host's
pnpm runs it when the app has a host install - the container keeps node_modules in a volume of its own, and
checking a different tree than the dev server runs is how the two drift apart. Otherwise it runs in the
app's container, exec'd when it's running, else a one-off. SKELET_FORCE_DOCKER=1 always uses the container.
A storefront's container can't write the workspace, so there an add, remove or update runs in a throwaway
container over the manifests instead, and the dev server's install picks up the new lockfile.`

func Lockfile(root string, args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "skelet pnpm --lockfile takes no arguments")
		return 2
	}
	if err := WorkspaceLockfile(root); err != nil {
		fmt.Fprintln(os.Stderr, "skelet pnpm --lockfile:", err)
		return 1
	}
	fmt.Println("pnpm-lock.yaml is up to date with the workspace")
	return 0
}

func Dashboard(root string, args []string) int { return replace(root, "dashboard", args) }

// Channel is a storefront's pnpm - the route puts the storefront's name first
func Channel(root string, args []string) int { return replace(root, args[0], args[1:]) }

// Package is pnpm in one of packages/ - the route puts the package's directory name first
func Package(root string, args []string) int {
	dir := "packages/" + args[0]
	if _, onHost := hostPnpm(filepath.Join(root, filepath.FromSlash(dir))); !onHost && editsLockfile(args[1:]) {
		return edit(root, dir, args[1:])
	}
	call, err := dirCall(root, dir, args[1:]...)
	if err != nil {
		return 1
	}
	return call.Replace()
}

func replace(root, app string, args []string) int {
	if s, ok := storefronts.Find(root, app); ok && editsLockfile(args) {
		if _, onHost := hostPnpm(filepath.Join(root, filepath.FromSlash(s.Dir))); !onHost {
			return edit(root, s.Dir, args)
		}
	}
	call, err := Call(root, app, nil, args...)
	if err != nil {
		return 1
	}
	return call.Replace()
}

func editsLockfile(args []string) bool {
	var words []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			words = append(words, a)
		}
	}
	if len(words) == 0 {
		return false
	}
	switch words[0] {
	case "add", "remove", "rm", "uninstall", "un", "update", "up", "upgrade", "dedupe":
		return true
	case "install", "i":
		return len(words) > 1
	}
	return false
}

func edit(root, dir string, args []string) int {
	if err := scratch(root, dir, append(slices.Clone(args), "--lockfile-only")...); err != nil {
		fmt.Fprintln(os.Stderr, "skelet pnpm:", err)
		return 1
	}
	fmt.Printf("%s/package.json and pnpm-lock.yaml updated - a Docker dev server reinstalls from them on its own\n", dir)
	return 0
}

func hostPnpm(dir string) (string, bool) {
	if os.Getenv("SKELET_FORCE_DOCKER") != "" || node.Tree(dir) != "pnpm" {
		return "", false
	}
	path, err := exec.LookPath("pnpm")
	return path, err == nil
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
// setup left out of the working tree can count on none
func WorkspaceLockfile(root string) error {
	if _, err := os.Stat(filepath.Join(root, "pnpm-workspace.yaml")); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return scratch(root, "", "install", "--lockfile-only")
}

// scratch runs pnpm over a throwaway copy of the workspace's manifests - a sparse member's from git's index, so the lockfile keeps it
func scratch(root, dir string, args ...string) error {
	image, err := nodeImage(root)
	if err != nil {
		return err
	}
	files, onDisk, err := workspaceFiles(root)
	if err != nil {
		return err
	}
	version := pinnedPnpm(files)
	if version == "" {
		return errors.New("no workspace package.json pins pnpm in its packageManager field")
	}
	tmp, err := sys.Scratch(root, "pnpm-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	for rel, raw := range files {
		dst := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			return err
		}
	}
	argv := append(append([]string{"docker", "run", "--rm"}, compose.RunAs()...), compose.UserFlags()...)
	argv = append(argv, "-e", "HOME=/tmp", "-e", "NPM_CONFIG_UPDATE_NOTIFIER=false", "-e", "pnpm_config_update_notifier=false",
		"-v", tmp+":/w", "-w", path.Join("/w", dir), image, "npx", "-y", "pnpm@"+version)
	if err := (sys.Call{Dir: root, Path: sys.Lookup("docker"), Argv: append(argv, args...), Env: sys.Env()}).Run(); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(tmp, lockfile)); err != nil {
		return fmt.Errorf("pnpm wrote no lockfile: %w", err)
	}
	for _, rel := range append([]string{lockfile}, onDisk...) {
		raw, err := os.ReadFile(filepath.Join(tmp, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		if bytes.Equal(raw, files[rel]) {
			continue
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

const lockfile = "pnpm-lock.yaml"

func workspaceFiles(root string) (files map[string][]byte, onDisk []string, err error) {
	files = map[string][]byte{}
	for _, f := range []string{"pnpm-workspace.yaml", lockfile, ".npmrc", "package.json"} {
		raw, err := os.ReadFile(filepath.Join(root, f))
		switch {
		case err == nil:
			files[f] = raw
		case f == "pnpm-workspace.yaml":
			return nil, nil, err
		}
	}
	var dirs []string
	for _, s := range storefronts.List(root) {
		dirs = append(dirs, s.Dir)
	}
	for _, dir := range append(dirs, storefronts.Packages(root)...) {
		rel := dir + "/package.json"
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, nil, err
		}
		files[rel], onDisk = raw, append(onDisk, rel)
	}
	for _, dir := range storefronts.Excluded(root) {
		rel := dir + "/package.json"
		raw, err := gitx.Indexed(root, rel)
		if err != nil {
			return nil, nil, fmt.Errorf("reading %s from git: %w", rel, err)
		}
		files[rel] = raw
	}
	return files, onDisk, nil
}

// nodeImage is the storefronts' Node, by the .nvmrc the pins gate holds every Dockerfile to
func nodeImage(root string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(root, ".nvmrc"))
	v := strings.TrimSpace(string(raw))
	if err != nil || v == "" {
		return "", errors.New(".nvmrc names no Node version - the storefronts' images are pinned by it")
	}
	return "node:" + v + "-alpine", nil
}

var packageManager = regexp.MustCompile(`"packageManager":\s*"pnpm@([0-9][0-9.]*)"`)

// pinnedPnpm is the pnpm a workspace manifest pins - the pins gate holds them to one
func pinnedPnpm(files map[string][]byte) string {
	names := slices.Sorted(maps.Keys(files))
	for _, f := range names {
		if path.Base(f) != "package.json" {
			continue
		}
		if v := packageManager.FindSubmatch(files[f]); v != nil {
			return string(v[1])
		}
	}
	return ""
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
	if path, ok := hostPnpm(abs); ok {
		return sys.Call{Dir: abs, Path: path, Argv: append([]string{"pnpm"}, args...), Env: sys.Env()}, nil
	}
	list := storefronts.List(root)
	if len(list) == 0 {
		fmt.Fprintf(os.Stderr, "no storefront checked out to run %s's pnpm in - its container is where the workspace is installed\n", dir)
		return sys.Call{}, errUnusable
	}
	flags := append(compose.UserFlags(), "-w", storefronts.Mount+"/"+dir)
	return container.In(root, compose.Target{Service: workspaceHost(list, compose.Load(root)).Name, TTY: true, Flags: flags}, append([]string{"pnpm"}, args...)...), nil
}

func workspaceHost(list []storefronts.Storefront, lib compose.Lib) storefronts.Storefront {
	running, selected := lib.RunningServices(), strings.Split(lib.Apps, ",")
	for _, names := range [][]string{running, selected} {
		if i := slices.IndexFunc(list, func(s storefronts.Storefront) bool { return slices.Contains(names, s.Name) }); i >= 0 {
			return list[i]
		}
	}
	return list[0]
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
