//go:build unix

package sys

import (
	"os"
	"os/exec"
)

// Call is a command a verb runs, either as its last step - replacing the CLI, so the tty, signals and exit code are
// the command's own - or as a child another verb waits on
type Call struct {
	Dir  string
	Path string
	Argv []string
	Env  []string
}

// Replace becomes the command, from Dir. It returns only when that failed
func (c Call) Replace() int {
	if c.Dir != "" {
		return ReplaceIn(c.Dir, c.Path, c.Argv, c.Env)
	}
	return Replace(c.Path, c.Argv, c.Env)
}

// Cmd is the command ready to run as a child
func (c Call) Cmd() *exec.Cmd {
	cmd := exec.Command(c.Path, c.Argv[1:]...)
	cmd.Dir, cmd.Env = c.Dir, c.Env
	return cmd
}

// Run runs it as a child with the terminal attached, for a verb that has more to do afterwards
func (c Call) Run() error {
	cmd := c.Cmd()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return Run(cmd)
}

// Lookup resolves name on the PATH, leaving it bare when it isn't there so the exec fails with the shell's 127
func Lookup(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}
