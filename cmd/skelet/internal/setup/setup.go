// Package setup is skelet setup: the guided first run, safe to re-run. Answers become env files with generated
// secrets, the app selection, the dev tools and the plugin set, and the dev target then boots and migrates
package setup

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/skeletg/skelet/cmd/skelet/internal/gitx"
	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const Flags = "[--yes] [--all | --backend-only | --apps LIST|none] [--docker-apps LIST|none] [--tools LIST|none] [--keep-code] [--profile FILE]"

const About = `The guided first run, and safe to re-run - each run defaults to the last one's answers, kept in
.skelet-profile. It asks which frontends this clone runs and whether in Docker or on the host, the
target (dev, small or prod), the suites, the search engine, the payment gateways, whether to hide
the admin API and which dev tools to run. Then it writes that target's env files with generated
secrets - a value that isn't a placeholder is never replaced - the root .env, plugins.json and the
selection files. For dev it boots the stack, migrates, installs the host apps and offers a first
admin login.

  --all, --backend-only, --apps LIST|none   which frontends this clone runs - the backend always does
  --docker-apps LIST|none                   which of those run in Docker rather than on the host
  --tools LIST|none                         dev tools: mailpit, webhook-tester, grafana, garage-webui
  --keep-code                               leave deselected apps' source in the working tree
  --yes, -y                                 take the defaults and ask nothing
  --profile FILE                            replay the answers in FILE (implies --yes)`

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

	from := filepath.Join(root, profile.File)
	if f.profile != "" {
		from = f.profile
	}
	prof, err := profile.Load(from)
	if err != nil {
		return fail(err)
	}
	ask := prompter{on: !f.yes && sys.IsTerminal(os.Stdin.Fd()), in: bufio.NewReader(os.Stdin), out: os.Stderr}
	a, err := answer(root, c, f, prof, from, ask, os.Stdout)
	if errors.Is(err, errRefused) {
		return 1
	}
	if err != nil {
		return fail(err)
	}
	w, err := Write(root, a, c, rand.Reader, os.Stdout)
	if err != nil {
		return fail(err)
	}
	if err := tree(root, a, c); err != nil {
		return fail(err)
	}

	if a.target != "dev" {
		fmt.Printf("==> Files written. %s target: fill the hostnames and credentials in env/%s.env,\n", a.target, a.target)
		fmt.Printf("    then deploy with docker-compose.%s.yml\n", a.target)
		return 0
	}
	if f.noBoot {
		fmt.Println("==> Files written - start the stack with skelet up")
		return 0
	}
	return boot(root, a, w, ask)
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
