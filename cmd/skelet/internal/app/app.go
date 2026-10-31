// Package app decides what a skelet invocation does: a checkout build runs the command, the released binary hands a
// checkout its own CLI so it never drives a Skelet of a different version
package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/skeletg/skelet/cmd/skelet/internal/checkout"
	"github.com/skeletg/skelet/cmd/skelet/internal/global"
	"github.com/skeletg/skelet/cmd/skelet/internal/route"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
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
	if a.Go != nil {
		return a.Go(root, a.Args)
	}
	if a.Script == "" {
		fmt.Print(a.Stdout)
		if a.Stderr != "" {
			fmt.Fprintln(os.Stderr, a.Stderr)
		}
		return a.Code
	}
	path := filepath.Join(root, "bin", a.Script)
	return sys.Replace(path, append([]string{path}, a.Args...), sys.Env())
}
