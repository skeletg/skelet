// Package legacy runs the bin/ scripts a Go verb still leans on while they're being ported. They get the environment
// the CLI was started with and nothing the CLI worked out: bin/_lib keeps an inherited COMPOSE_PROFILES, so passing
// Go's computed one down would override what the script decides for itself, --lean included
package legacy

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

// Run runs bin/<script> from the checkout with the terminal attached, as a child HandleInterrupts waits for
func Run(root, script string, args ...string) error {
	cmd := Command(root, script, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return sys.Run(cmd)
}

// Command is bin/<script> ready to run, for a caller that wants its output
func Command(root, script string, args ...string) *exec.Cmd {
	cmd := exec.Command(filepath.Join(root, "bin", script), args...)
	cmd.Dir, cmd.Env = root, sys.Env()
	return cmd
}
