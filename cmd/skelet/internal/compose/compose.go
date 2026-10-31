// Package compose gives Go's own docker compose calls what bin/_lib gives the scripts: the profiles that follow
// .skelet-apps, .skelet-tools and SIGNALS_ENABLED, and the uid containers run as. _lib is read rather than copied, so
// the two can't drift - and nothing here leaks into a script's environment, where an inherited COMPOSE_PROFILES would
// override what the script works out for itself
package compose

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

// Lib is what bin/_lib works out
type Lib struct {
	Profiles, UID, GID string
	// Apps is what runs in Docker, Tools the dev tools picked
	Apps, Tools string
}

// Load reads _lib, after setup has written the selection files it follows
func Load(root string) (Lib, error) {
	cmd := exec.Command("bash", "-c", `. bin/_lib && printf '%s\n' "$COMPOSE_PROFILES" "$SKELET_UID" "$SKELET_GID" "$SKELET_APPS" "$SKELET_TOOLS"`)
	cmd.Dir = root
	cmd.Env = sys.Env()
	out, err := cmd.Output()
	if err != nil {
		return Lib{}, fmt.Errorf("reading bin/_lib: %w", err)
	}
	v := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(v) != 5 {
		return Lib{}, fmt.Errorf("reading bin/_lib: unexpected output %q", out)
	}
	return Lib{Profiles: v[0], UID: v[1], GID: v[2], Apps: v[3], Tools: v[4]}, nil
}

// Env is the environment for a compose call
func (l Lib) Env() []string {
	return append(sys.Env(), "COMPOSE_PROFILES="+l.Profiles, "SKELET_UID="+l.UID, "SKELET_GID="+l.GID)
}

// Command is docker compose run from the checkout with env
func Command(root string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir, cmd.Env = root, env
	return cmd
}
