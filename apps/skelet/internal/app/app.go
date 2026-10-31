// Package app decides what a skelet invocation does: a checkout build runs the command, the released binary hands a
// checkout its own CLI so it never drives a Skelet of a different version
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/skeletg/skelet/apps/skelet/internal/checkout"
	"github.com/skeletg/skelet/apps/skelet/internal/global"
	"github.com/skeletg/skelet/apps/skelet/internal/route"
	"github.com/skeletg/skelet/apps/skelet/internal/sys"
)

type Config struct {
	Version string
	Mode    string
}

func Main(cfg Config, args []string) int {
	if cfg.Mode != "checkout" {
		return global.Main(cfg.Version, args)
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
	return sys.Replace(path, append([]string{path}, a.Args...), childEnv())
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
