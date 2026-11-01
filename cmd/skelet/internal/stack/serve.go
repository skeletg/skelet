package stack

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/node"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const ServeAbout = `A frontend's dev server on the host: skelet serve <dashboard|channel> [dev server args].

It reads the root .env first and hands it to the dev server, so the channel id lives in one place -
Next reads .env files only from its own directory, and a second copy of STOREFRONT_CHANNEL_ID in the
app would drift from the one compose reads. Installs the app's dependencies when they're missing, and
refuses while the app's container holds the port. Arguments go to the dev server with no -- in front,
since pnpm passes that through verbatim.`

// Dev defaults that match the compose service, used only when the root .env is silent
var serveDefaults = []string{"NEXT_PUBLIC_BACKEND_URL=http://localhost:8080", "STOREFRONT_REVALIDATE_SECRET=dev-revalidate-secret-change-me"}

func Serve(root string, args []string) int {
	app := ""
	if len(args) > 0 {
		app, args = args[0], args[1:]
	}
	dir, port := "", "3000"
	switch app {
	case "":
		return serveUsage(root)
	case "dashboard":
		dir, port = filepath.Join("apps", "dashboard"), "5173"
	default:
		s, ok := storefronts.Find(root, app)
		if !ok {
			fmt.Fprintf(os.Stderr, "No such app: %s\n", app)
			return serveUsage(root)
		}
		dir = filepath.FromSlash(s.Dir)
	}
	manifest, err := os.ReadFile(filepath.Join(root, dir, "package.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s has no package.json - see skelet setup\n", dir)
		return 1
	}
	env, err := serveEnv(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "skelet serve:", err)
		return 1
	}
	// A container on the same port is the confusing failure - the dev server picks another and nothing says why
	// the page you opened is stale
	if compose.Load(root).Running(app) {
		fmt.Fprintf(os.Stderr, "The %s container is running and holds port %s - stop it first: docker compose stop %s\n", app, port, app)
		return 1
	}
	if _, problem := node.Pnpm(root, dir, node.Pin(manifest)); problem != "" {
		fmt.Fprintf(os.Stderr, "Can't serve %s: %s\n", app, problem)
		return 1
	}
	abs := filepath.Join(root, dir)
	switch node.Tree(abs) {
	case "none":
		fmt.Printf("==> installing %s deps\n", app)
		install := exec.Command("pnpm", "install", "--frozen-lockfile")
		install.Dir, install.Env = abs, env
		install.Stdin, install.Stdout, install.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := sys.Run(install); err != nil {
			return sys.ExitCode(err)
		}
	case "npm":
		fmt.Fprintf(os.Stderr, "%s/node_modules is an npm install from before the switch to pnpm - replace it:\n", dir)
		fmt.Fprintf(os.Stderr, "    rm -rf %s/node_modules && (cd %s && pnpm install)\n", dir, dir)
		return 1
	}
	fmt.Printf("==> %s on the host (http://localhost:%s)\n", app, port)
	return sys.Call{Dir: abs, Path: sys.Lookup("pnpm"), Argv: append([]string{"pnpm", "run", "dev"}, args...), Env: env}.Replace()
}

// serveEnv is the caller's environment with the root .env laid over it, as compose reads that file - parsed, not
// sourced - and the dev defaults filling whatever is still empty
func serveEnv(root string) ([]string, error) {
	pairs, err := envfile.Pairs(filepath.Join(root, ".env"))
	if err != nil {
		return nil, err
	}
	env := append(sys.Env(), "LC_ALL=C.UTF-8", "LANG=C.UTF-8")
	for _, kv := range pairs {
		env = setEnv(env, kv, true)
	}
	for _, kv := range serveDefaults {
		env = setEnv(env, kv, false)
	}
	return env, nil
}

// setEnv puts KEY=value in env - over what's there, or only when that is missing or empty
func setEnv(env []string, kv string, override bool) []string {
	key := kv[:strings.Index(kv, "=")+1]
	i := slices.IndexFunc(env, func(e string) bool { return strings.HasPrefix(e, key) })
	switch {
	case i < 0:
		return append(env, kv)
	case override || env[i] == key:
		env[i] = kv
	}
	return env
}

func serveUsage(root string) int {
	apps := append([]string{"dashboard"}, storefronts.Names(root)...)
	fmt.Fprintln(os.Stderr, "usage: skelet serve <app> [dev server args]")
	fmt.Fprintln(os.Stderr, "  "+strings.Join(apps, ", "))
	return 2
}
