// Package build is skelet build: what a deployment runs, for the plugin set plugins.json decides - core's image with
// the plugins compiled in, and the dashboard and storefronts with their UI packages bundled
package build

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/gen"
	"github.com/skeletg/skelet/cmd/skelet/internal/pnpm"
	"github.com/skeletg/skelet/cmd/skelet/internal/release"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const About = `Builds what a deployment runs, with the plugins plugins.json enables: skelet build [core] [signals] [dashboard]
[<storefront>...] [--tag T]. With no names, core and the dashboard - the two every plugin reaches. Regenerates first
(skelet gen's backend half), so a plugin just added or removed is in or out, then builds each: a backend as the image
skelet-<app>:<tag>, prod by default, which is what its compose service runs when CORE_IMAGE or SIGNALS_IMAGE is unset,
and a frontend with its own pnpm run build. skelet release pushes the backends' images to a registry instead.`

// The seams a test swaps - each reaches docker, pnpm or the generators
var (
	regen  = func(root string) int { return gen.Run(root, []string{"--no-frontend"}) }
	image  = release.Image
	pnpmIn = func(root, app string, args ...string) error { return pnpm.Run(root, app, nil, args...) }
)

func Run(root string, args []string) int {
	tag, names := "prod", []string{}
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--tag" && i+1 < len(args):
			tag = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--tag="):
			tag = strings.TrimPrefix(args[i], "--tag=")
		case strings.HasPrefix(args[i], "-"):
			fmt.Fprintf(os.Stderr, "skelet build: unknown flag %s\n", args[i])
			return 2
		default:
			names = append(names, args[i])
		}
	}
	if len(names) == 0 {
		names = []string{"core", "dashboard"}
	}
	shops := storefronts.Names(root)
	for _, n := range names {
		if !slices.Contains(release.Images(), n) && n != "dashboard" && !slices.Contains(shops, n) {
			fmt.Fprintf(os.Stderr, "skelet build: nothing called %s - %s, dashboard, or a storefront: %s\n",
				n, strings.Join(release.Images(), ", "), strings.Join(shops, ", "))
			return 2
		}
	}
	sys.HandleInterrupts()

	fmt.Println("==> regenerating for the plugin set")
	if code := regen(root); code != 0 {
		return code
	}
	var built []string
	for _, n := range names {
		switch {
		case slices.Contains(release.Images(), n):
			if code := image(root, n, tag); code != 0 {
				return code
			}
			built = append(built, fmt.Sprintf("%-12s image skelet-%s:%s", n, n, tag))
		default:
			fmt.Printf("==> build %s\n", n)
			if err := pnpmIn(root, n, "run", "build"); err != nil {
				return max(sys.ExitCode(err), 1)
			}
			built = append(built, fmt.Sprintf("%-12s %s", n, output(n)))
		}
	}
	fmt.Println("\nBuilt:")
	for _, b := range built {
		fmt.Println("  " + b)
	}
	return 0
}

// Where a frontend's build lands - the dashboard is static files any host serves, a storefront its framework's output
func output(app string) string {
	if app == "dashboard" {
		return "apps/dashboard/dist - static files, served by the edge Caddy or any static host"
	}
	return "its framework's build output, run with its start script"
}
