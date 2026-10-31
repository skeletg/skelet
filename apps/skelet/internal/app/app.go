// Package app decides what a skelet invocation does: a checkout build runs the command, the released binary hands a
// checkout its own CLI so it never drives a Skelet of a different version
package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/skeletg/skelet/apps/skelet/internal/checkout"
	"github.com/skeletg/skelet/apps/skelet/internal/route"
	"github.com/skeletg/skelet/apps/skelet/internal/sys"
)

type Config struct {
	Version string
	Mode    string
}

func Main(cfg Config, args []string) int {
	if cfg.Mode != "checkout" {
		return global(cfg, args)
	}
	root := os.Getenv("SKELET_ROOT")
	if root == "" {
		cwd, _ := os.Getwd()
		var ok bool
		if root, ok = checkout.Find(cwd); !ok {
			fmt.Fprintln(os.Stderr, "skelet: not inside a Skelet checkout")
			return 2
		}
	}
	if len(args) > 0 && args[0] == "version" {
		fmt.Printf("skelet %s (checkout %s)\n", cfg.Version, root)
		return 0
	}
	return run(root, route.Resolve(root, args))
}

func run(root string, a route.Action) int {
	if a.Notice != "" {
		fmt.Fprintln(os.Stderr, a.Notice)
	}
	if a.Script == "" {
		fmt.Print(a.Stdout)
		if a.Stderr != "" {
			fmt.Fprintln(os.Stderr, a.Stderr)
		}
		return a.Code
	}
	path := filepath.Join(root, "bin", a.Script)
	return execFailed(path, sys.Exec(path, append([]string{path}, a.Args...), childEnv()))
}

func global(cfg Config, args []string) int {
	if os.Getenv("SKELET_SHIM") != "" {
		fmt.Fprintln(os.Stderr, "skelet: a release build is sitting in this checkout's CLI cache - remove .skelet/bin and run ./skelet again")
		return 1
	}
	if len(args) > 0 && args[0] == "version" {
		fmt.Printf("skelet %s\n", cfg.Version)
		return 0
	}
	cwd, _ := os.Getwd()
	root, ok := checkout.Find(cwd)
	if !ok {
		fmt.Fprintln(os.Stderr, "skelet: not inside a Skelet checkout - cd into one")
		return 2
	}
	// Through bash rather than the file itself, so a shim that lost its +x bit (WSL clones) still runs
	bash, err := exec.LookPath("bash")
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet: bash is needed to run the checkout's front door")
		return 1
	}
	shim := filepath.Join(root, "skelet")
	return execFailed(shim, sys.Exec(bash, append([]string{"bash", shim}, args...), os.Environ()))
}

// The shim's own variables stay out of the scripts, which see the environment they always did
func childEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "SKELET_ROOT=") && !strings.HasPrefix(kv, "SKELET_SHIM=") {
			env = append(env, kv)
		}
	}
	return env
}

func execFailed(path string, err error) int {
	fmt.Fprintf(os.Stderr, "skelet: %s: %v\n", path, err)
	if errors.Is(err, fs.ErrNotExist) {
		return 127
	}
	return 126
}
