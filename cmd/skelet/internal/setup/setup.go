// Package setup is skelet setup: the guided first run, safe to re-run. Answers become env files with generated
// secrets, the app selection, the dev tools and the plugin set, and the dev target then boots and migrates
package setup

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/gitx"
	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
	"github.com/skeletg/skelet/cmd/skelet/internal/scaffold"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
	"github.com/skeletg/skelet/cmd/skelet/internal/tui"
)

const Flags = "[core signals dashboard <storefront>...] [--small] [--shared FILE] [--yes] [--all | --backend-only | --apps LIST|none] [--tools LIST|none] [--keep-code] [--profile FILE]"

const About = `The guided first run, and safe to re-run - each run defaults to how the clone is set up now,
read from .env and the env files. Plain, it sets up a dev clone: it asks which frontends run - on the host, with
skelet dev - the suites, the search engine, the payment gateways, whether to hide the admin API and which dev tools to
run, writes the env files with generated secrets - a value that isn't a placeholder is never replaced - the root .env
and plugins.json, then boots the stack, migrates, installs the host apps and offers a first admin login.

Named apps set up a server with just those: skelet setup core signals, skelet setup signals, skelet setup dashboard
nextjs-vetrina. Each named app gets its env file, and nothing else is assumed to run on the machine. Core makes the
values it shares with the others; an app on a machine without core takes them from core instead - from --shared, from
what it already holds, or asked for - and with --yes it refuses rather than make them up.

  core signals dashboard <storefront>       the apps that run on this machine - a server setup
  --small                                   core in one container: no worker or media tiers, no Valkey
  --shared FILE                             core's values for a machine without core - a copy of its .env does

  --all, --backend-only, --apps LIST|none   which frontends this clone runs - the backend always does. A
                                            template runs only when named, or with --all
  --tools LIST|none                         dev tools: mailpit, webhook-tester, grafana, garage-webui
  --shop NAME, --template NAME              a storefront of your own, copied from a template (default: shop,
                                            and the template docker-compose.yml runs) - offered when you have none
  --keep-code                               leave deselected apps' source in the working tree
  --yes, -y                                 take the defaults and ask nothing
  --profile FILE                            replay the answers in FILE (implies --yes)`

// forkShop makes a project's first shop - a seam for the tests, which have no compose to validate against
var forkShop = scaffold.Fork

func Run(root string, args []string) int {
	c, err := scan(root)
	if err != nil {
		return fail(err)
	}
	f, err := parseFlags(args, c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	sys.HandleInterrupts()

	prof, from := current(root, c), ".env"
	if f.profile != "" {
		from = f.profile
		if prof, err = profile.Load(from); err != nil {
			return fail(err)
		}
	}
	ask := prompter{on: !f.yes && sys.IsTerminal(os.Stdin.Fd()), in: bufio.NewReader(os.Stdin), out: os.Stderr}
	if visual(f) {
		ask.ui = tui.Terminal()
		sys.AtExit(tui.Restore)
	}
	a, ok, err := interview(root, c, f, prof, from, ask, os.Stdout)
	if errors.Is(err, errRefused) {
		return 1
	}
	if err != nil {
		return fail(err)
	}
	if !ok {
		fmt.Println("Nothing written.")
		return 1
	}
	st, err := newStepper(root, ask.ui)
	if err != nil {
		return fail(err)
	}
	defer st.close()
	var w written
	if code := st.run("Writing the env files and your answers", func() int {
		if w, err = Write(root, a, c, rand.Reader, os.Stdout); err != nil {
			return fail(err)
		}
		return 0
	}); code != 0 {
		return code
	}
	// After the env files - the copy checks the compose files it adds to, and compose needs them - and before the boot,
	// so the stack starts with the new shop in it
	if a.shop != "" {
		if code := st.run("Copying "+a.shop+" from "+a.shopTemplate, func() int { return forkShop(root, a.shop, a.shopTemplate) }); code != 0 {
			fmt.Fprintf(os.Stderr, "==> %s wasn't made - skelet add channel %s makes it once the rest is fixed\n", a.shop, a.shop)
			return code
		}
	}
	if title := treeChange(root, a, c); title != "" {
		if code := st.run(title, func() int {
			if err := tree(root, a, c); err != nil {
				return fail(err)
			}
			return 0
		}); code != 0 {
			return code
		}
	}

	if a.target != "dev" {
		ask.section(os.Stdout, "Files written. Fill in the hostnames and credentials, then start what runs here:", starts(a)...)
		return 0
	}
	if f.noBoot {
		ask.section(os.Stdout, "Files written - start the stack with skelet up")
		return 0
	}
	return boot(root, a, w, ask, st)
}

// interview asks, and when the questions are drawn shows the answers for a go-ahead - a change goes round again
func interview(root string, c catalog, f flags, prof profile.Profile, from string, ask prompter, out io.Writer) (answers, bool, error) {
	for range 10 {
		a, err := answer(root, c, f, prof, from, ask, out)
		if err != nil || ask.ui == nil {
			return a, true, err
		}
		act, err := ask.ui.Review("Review", review(a, c))
		ask.answered(err)
		switch act {
		case tui.Go:
			return a, true, nil
		case tui.Quit:
			return a, false, nil
		}
		ask.prior = &a
	}
	return answers{}, false, nil
}

func review(a answers, c catalog) [][2]string {
	list := func(l []string, none string) string {
		if len(l) == 0 {
			return none
		}
		return strings.Join(l, ", ")
	}
	rows := [][2]string{{"target", a.target}, {"apps", list(a.apps, "backend only")}}
	if a.target == "server" {
		rows[1][0] = "on this machine"
		if a.runs(appCore) {
			rows = append(rows, [2]string{"core", map[bool]string{true: "one container", false: "with its worker and media tiers"}[a.small]})
		}
	}
	if a.shop != "" {
		rows = append(rows, [2]string{"new shop", a.shop + ", copied from " + a.shopTemplate})
	}
	var suites []string
	if a.signals {
		suites = append(suites, "Signals")
	}
	if a.ai {
		suites = append(suites, "AI")
	}
	admin := "/api/admin/graphql"
	if a.adminSlug {
		admin = "an unguessable path"
	}
	if a.hasCore() {
		rows = append(rows, [2]string{"suites", list(suites, "none")}, [2]string{"search", a.search},
			[2]string{"gateways", list(a.gateways, "none")}, [2]string{"admin API", admin})
	}
	if a.target == "dev" {
		rows = append(rows, [2]string{"dev tools", list(a.tools, "none")})
	}
	if t := treeChange(c.root, a, c); t != "" {
		rows = append(rows, [2]string{"working tree", t})
	}
	return rows
}

func starts(a answers) []string {
	var out []string
	switch {
	case a.runs(appCore) && a.runs(appSignals):
		out = append(out, "    core, signals  docker compose -f docker-compose.prod.yml up -d  (--profile edge adds the bundled edge)")
	case a.runs(appCore):
		out = append(out, "    core           cd apps/backend/core && docker compose up -d  (docker-compose.small.yml adds the bundled edge)")
	case a.runs(appSignals):
		out = append(out, "    signals        cd apps/backend/signals && docker compose up -d")
	}
	for _, app := range frontends(a.apps) {
		if app == "dashboard" {
			out = append(out, "    dashboard      pnpm run build in apps/dashboard - dist/ is the site")
			continue
		}
		out = append(out, fmt.Sprintf("    %-14s pnpm run build, then pnpm run start, in its directory", app))
	}
	return out
}

func treeChange(root string, a answers, c catalog) string {
	switch {
	case !a.prune || !gitx.IsWorkTree(root):
		return ""
	case len(excluded(a, c)) == 0:
		if gitx.SparseOn(root) {
			return "Restoring the full working tree"
		}
		return ""
	case a.removeDeselected:
		return "Leaving deselected apps out of the working tree"
	}
	return ""
}

// tree makes the working tree follow the selection: a deselected app's source leaves through sparse-checkout - it
// stays in history and comes back when it's selected again
func tree(root string, a answers, c catalog) error {
	if !a.prune || !gitx.IsWorkTree(root) {
		return nil
	}
	out := excluded(a, c)
	if len(out) == 0 {
		if gitx.SparseOn(root) {
			fmt.Println("==> Restoring the full working tree (git sparse-checkout disable)...")
			return gitx.SparseDisable(root)
		}
		return nil
	}
	if !a.removeDeselected {
		return nil
	}
	fmt.Println("==> Removing deselected apps from the working tree (git sparse-checkout)...")
	if err := gitx.SparseSet(root, append([]string{"/*"}, out...)); err != nil {
		return err
	}
	fmt.Println("    (still in git history - select them again with skelet setup --apps ... to restore)")
	return nil
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "skelet setup:", err)
	return 1
}
