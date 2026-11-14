// Package pnpm runs the host's pnpm for a frontend
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
	"github.com/skeletg/skelet/cmd/skelet/internal/gitx"
	"github.com/skeletg/skelet/cmd/skelet/internal/node"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const About = `pnpm for a frontend, with the host's pnpm: skelet pnpm dashboard run lint, skelet pnpm <storefront> add <pkg>,
or for one of the packages the storefronts share: skelet pnpm storefront-core test. skelet pnpm --lockfile brings the
workspace's pnpm-lock.yaml up to date in a throwaway container, for a checkout with no pnpm installed.`

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

// Run runs pnpm for app as a child, for a verb with more to do
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

// RunDir runs pnpm in a workspace package's directory
func RunDir(root, dir string, args ...string) error {
	call, err := dirCall(root, dir, args...)
	if err != nil {
		return err
	}
	return call.Run()
}

func dirCall(root, dir string, args ...string) (sys.Call, error) {
	abs := filepath.Join(root, filepath.FromSlash(dir))
	path, err := host(root, filepath.FromSlash(dir))
	if err != nil {
		return sys.Call{}, err
	}
	return sys.Call{Dir: abs, Path: path, Argv: append([]string{"pnpm"}, args...), Env: sys.Env()}, nil
}

func host(root, rel string) (string, error) {
	if path, err := exec.LookPath("pnpm"); err == nil {
		return path, nil
	}
	manifest, _ := os.ReadFile(filepath.Join(root, rel, "package.json"))
	_, problem := node.Pnpm(root, "", node.Pin(manifest))
	fmt.Fprintf(os.Stderr, "The frontends run on the host, which needs pnpm: %s\n", problem)
	return "", errUnusable
}

// Call is pnpm for app with args, or why it can't run - said on stderr
func Call(root, app string, env []string, args ...string) (sys.Call, error) {
	rel := filepath.Join("apps", "dashboard")
	if app != "dashboard" {
		rel = filepath.Join("apps", "channels", app)
		if s, ok := storefronts.Find(root, app); ok {
			rel = filepath.FromSlash(s.Dir)
		}
	}
	dir := filepath.Join(root, rel)
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		fmt.Fprintf(os.Stderr, "%s is not in this checkout - removed by skelet setup's app selection?\n", rel)
		fmt.Fprintln(os.Stderr, "Bring it back with: skelet setup --all (or --apps ...)")
		return sys.Call{}, errUnusable
	}
	if node.Tree(dir) == "npm" {
		fmt.Fprintf(os.Stderr, "%s/node_modules is an npm install from before the switch to pnpm - replace it:\n", rel)
		fmt.Fprintf(os.Stderr, "    rm -rf %s/node_modules && (cd %s && pnpm install)\n", rel, rel)
		return sys.Call{}, errUnusable
	}
	if node.Unwritable(dir) {
		fmt.Fprintf(os.Stderr, "%s/node_modules isn't yours to write - a frontend container left it behind. skelet fixperms gives it back\n", rel)
		return sys.Call{}, errUnusable
	}
	path, err := host(root, rel)
	if err != nil {
		return sys.Call{}, err
	}
	return sys.Call{Dir: dir, Path: path, Argv: append([]string{"pnpm"}, args...), Env: append(sys.Env(), env...)}, nil
}
