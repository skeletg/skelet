// Package lint is skelet lint: gofmt and golangci-lint are hard gates on every Go module, and the frontends' eslint
// reports without blocking
package lint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/legacy"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const About = `gofmt and golangci-lint on every Go module - hard gates, run on the host - then each frontend's eslint,
which only reports. golangci-lint is always the pinned version, never a binary that happens to be on
the PATH, so local lint runs the same rules as CI.`

// Golangci is the pinned linter - skelet check pins holds the workflows to this version, and Renovate bumps it here
const Golangci = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0"

// Every Go module, each on its own go.mod - the Signals module replaces core's by relative path, so a lint run there
// type-checks core's public packages too
var modules = []string{"apps/backend/core", "apps/backend/signals", "apps/backend/sdk", "cmd/skelet"}

var goLine = regexp.MustCompile(`(?m)^go (\S+)`)

func Run(root string, _ []string) int {
	sys.HandleInterrupts()
	for _, mod := range modules {
		fmt.Printf("==> gofmt (%s)\n", mod)
		cmd := exec.Command("gofmt", "-l", ".")
		cmd.Dir = filepath.Join(root, mod)
		out, err := cmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "gofmt: %v\n", err)
			return 1
		}
		if s := strings.TrimSpace(string(out)); s != "" {
			fmt.Printf("gofmt would reformat these files:\n%s\nfix with: (cd %s && gofmt -w .)\n", s, mod)
			return 1
		}
	}
	// Recorded rather than fatal on the spot, so a lint error adds to the frontend results instead of hiding them
	backendDirty := false
	for _, mod := range modules {
		fmt.Printf("==> golangci-lint (%s)\n", mod)
		cmd := exec.Command("go", "run", Golangci, "run", "./...")
		cmd.Dir = filepath.Join(root, mod)
		cmd.Env = append(sys.Env(), "GOTOOLCHAIN="+toolchain(filepath.Join(root, mod, "go.mod")))
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if sys.Run(cmd) != nil {
			backendDirty = true
		}
	}
	frontendDirty := false
	fmt.Println("==> dashboard eslint (non-blocking)")
	if _, err := os.Stat(filepath.Join(root, "apps", "dashboard", "package.json")); err == nil {
		frontendDirty = legacy.Run(root, "dash", "run", "lint") != nil
	} else {
		fmt.Println("    (skipped - dashboard not in this checkout; see skelet setup)")
	}
	manifests, _ := filepath.Glob(filepath.Join(root, "apps", "channels", "*", "package.json"))
	slices.Sort(manifests)
	if len(manifests) == 0 {
		fmt.Println("==> storefront eslint: (no storefronts in this checkout; see skelet setup)")
	}
	for _, m := range manifests {
		name := filepath.Base(filepath.Dir(m))
		fmt.Printf("==> %s eslint (non-blocking)\n", name)
		if legacy.Run(root, "channel", name, "run", "lint") != nil {
			frontendDirty = true
		}
	}
	if frontendDirty {
		fmt.Println("==> frontend eslint has findings (non-blocking, not checked by CI either)")
	}
	if backendDirty {
		fmt.Println("==> lint FAILED - see the golangci-lint output above")
		return 1
	}
	fmt.Println("==> lint OK (backend)")
	return 0
}

// toolchain is the Go a module's directive names, for go run's GOTOOLCHAIN - go run pkg@version builds outside the
// main module with the host toolchain, and one older than the directive refuses to load the config. +auto still lets
// the module demand something newer
func toolchain(gomod string) string {
	raw, _ := os.ReadFile(gomod)
	m := goLine.FindSubmatch(raw)
	if m == nil {
		return "auto"
	}
	v := string(m[1])
	// go1.27 names no toolchain, go1.27.0 does
	if strings.Count(v, ".") < 2 {
		v += ".0"
	}
	return "go" + v + "+auto"
}
