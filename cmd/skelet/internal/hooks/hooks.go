// Package hooks is skelet hooks: the opt-in pre-commit hook that runs skelet check before each commit
package hooks

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
)

const About = `Installs the pre-commit hook, which runs skelet check - the version pins, the repo structure and the
cross-app contracts, in about a second with no Docker or backend - before every commit. Deliberately not
check --codegen: needing a running backend to make a commit is a bad trade, so stale codegen is yours to
catch before a schema-touching PR. Skip it once with git commit --no-verify.

The hook finds the CLI itself - skelet on the PATH, ~/.local/bin/skelet, the skelet you installed it
with, then go run from the checkout - since git GUIs often start hooks without your shell's PATH.
An existing hook of your own is left alone unless you pass --force.

  --force   replace a pre-commit hook skelet didn't write`

// Marker is the line that says a hook is this version of skelet's
const Marker = "# skelet-hook 2"

// Kind is what a pre-commit hook is
type Kind int

const (
	Missing Kind = iota
	Current
	// Old is the hook bin/install-hooks wrote, which runs bin/check directly
	Old
	Foreign
)

// Script is the hook. cli is the skelet that installed it, tried after the PATH and ~/.local/bin, or empty
func Script(cli string) string {
	tries := `skelet "$HOME/.local/bin/skelet"`
	if cli != "" {
		tries += " '" + strings.ReplaceAll(cli, "'", `'\''`) + "'"
	}
	return `#!/bin/sh
` + Marker + ` - written by skelet hooks: runs skelet check before each commit (skip it once with --no-verify)
export LC_ALL=C.UTF-8 LANG=C.UTF-8
for s in ` + tries + `; do
    command -v "$s" >/dev/null 2>&1 && exec "$s" check
done
command -v go >/dev/null 2>&1 && exec go -C cmd/skelet run . check
echo "pre-commit: no skelet CLI and no Go to run skelet check - install the CLI, or skip it once with --no-verify" >&2
exit 1
`
}

// Path is where git looks for the pre-commit hook, worktrees and core.hooksPath included
func Path(root string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-path", "hooks/pre-commit")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return p, nil
}

// Classify reads the hook at path
func Classify(path string) Kind {
	raw, err := os.ReadFile(path)
	switch {
	case err != nil:
		return Missing
	case bytes.Contains(raw, []byte(Marker)):
		return Current
	case old(raw):
		return Old
	}
	return Foreign
}

// old matches the hook bin/install-hooks wrote by what it does rather than byte for byte, so a clone holding an
// older wording of its comments still counts
func old(raw []byte) bool {
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	return bytes.Contains(raw, []byte("# Installed by bin/install-hooks")) && strings.TrimSpace(lines[len(lines)-1]) == "exec ./bin/check"
}

func Run(root string, args []string) int {
	force := false
	for _, a := range args {
		if a != "--force" {
			fmt.Fprintf(os.Stderr, "skelet hooks: unknown flag %s - the only one is --force\n", a)
			return 2
		}
		force = true
	}
	path, err := Path(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Not a git working tree - refusing to install hooks.")
		return 1
	}
	if Classify(path) == Foreign && !force {
		fmt.Fprintf(os.Stderr, "%s is a hook of your own - skelet hooks --force replaces it\n", path)
		return 1
	}
	if err := write(path); err != nil {
		fmt.Fprintln(os.Stderr, "skelet hooks:", err)
		return 1
	}
	fmt.Printf("Pre-commit hook installed: %s\n", path)
	fmt.Println("It runs skelet check - under a second, no Docker and no backend needed.")
	fmt.Println("See what each gate is for with `skelet check --list`, or skip it once with `git commit --no-verify`.")
	return 0
}

func write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return envfile.WriteAtomic(path, []byte(Script(os.Getenv("SKELET_CLI"))), 0o755)
}

// Upgrade rewrites the hook bin/install-hooks wrote, which stops working once bin/ is gone. It costs one stat on every
// command, so it only looks where a plain clone keeps its hooks and leaves worktrees to skelet hooks
func Upgrade(root string, out io.Writer) {
	path := filepath.Join(root, ".git", "hooks", "pre-commit")
	if Classify(path) != Old {
		return
	}
	if err := write(path); err != nil {
		fmt.Fprintf(out, "skelet: couldn't update the pre-commit hook, which still runs bin/check: %v\n", err)
		return
	}
	fmt.Fprintln(out, "skelet: updated the pre-commit hook - it ran bin/check, and now runs skelet check")
}
