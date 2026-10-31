// Package compose gives Go's own docker compose calls the environment bin/_lib gives the scripts: the profiles that
// follow .skelet-apps, .skelet-tools and SIGNALS_ENABLED, and the uid containers run as. _lib is read rather than
// copied, so the two can't drift - and nothing here leaks into a script's environment, where an inherited
// COMPOSE_PROFILES would override what the script works out for itself
package compose

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/skeletg/skelet/apps/skelet/internal/sys"
)

// Env is read after setup writes the selection files, since the profiles follow them
func Env(root string) ([]string, error) {
	cmd := exec.Command("bash", "-c", `. bin/_lib && printf '%s\n%s\n%s\n' "$COMPOSE_PROFILES" "$SKELET_UID" "$SKELET_GID"`)
	cmd.Dir = root
	cmd.Env = sys.Env()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading bin/_lib: %w", err)
	}
	v := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(v) != 3 {
		return nil, fmt.Errorf("reading bin/_lib: unexpected output %q", out)
	}
	return append(sys.Env(), "COMPOSE_PROFILES="+v[0], "SKELET_UID="+v[1], "SKELET_GID="+v[2]), nil
}

// Command is docker compose run from the checkout with env
func Command(root string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir, cmd.Env = root, env
	return cmd
}
