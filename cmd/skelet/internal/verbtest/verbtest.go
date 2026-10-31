// Package verbtest runs a verb the way the CLI does - in a process of its own, since most verbs end by becoming
// another program - against a fake docker that records what it was asked, so a test sees the exact command line
// and nothing real ever runs
package verbtest

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

// Main is a test package's TestMain: in the child it runs the verb named by the environment and exits with its code,
// through sys.Exit as the CLI does, so a verb's AtExit cleanups run
func Main(m *testing.M, verbs map[string]func(root string, args []string) int) {
	if name := os.Getenv("VERBTEST_VERB"); name != "" {
		var args []string
		if a := os.Getenv("VERBTEST_ARGS"); a != "" {
			args = strings.Split(a, "\x1f")
		}
		sys.Exit(verbs[name](os.Getenv("VERBTEST_ROOT"), args))
	}
	os.Exit(m.Run())
}

// Docker is the fake: each call appends its arguments and the COMPOSE_PROFILES it saw to a log. answer is shell
// run on every call, with $* the arguments - for ps queries that need a reply
type Docker struct {
	t   *testing.T
	dir string
	log string
}

func NewDocker(t *testing.T, answer string) *Docker {
	t.Helper()
	d := &Docker{t: t, dir: t.TempDir()}
	d.log = filepath.Join(d.dir, "calls")
	script := "#!/bin/sh\nprintf '%s|%s\\n' \"$*\" \"${COMPOSE_PROFILES-<unset>}\" >> '" + d.log + "'\n" + answer + "\n"
	if err := os.WriteFile(filepath.Join(d.dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

// Run runs verb with args from root in a child process, with the fake docker first on the PATH
func (d *Docker) Run(verb, root string, args ...string) (stdout string, code int) {
	d.t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "VERBTEST_VERB="+verb, "VERBTEST_ROOT="+root, "VERBTEST_ARGS="+strings.Join(args, "\x1f"),
		"PATH="+d.dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.Output()
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return string(out), ee.ExitCode()
	}
	if err != nil {
		d.t.Fatal(err)
	}
	return string(out), 0
}

// Calls is each docker call as "args|profiles"
func (d *Docker) Calls() []string {
	raw, _ := os.ReadFile(d.log)
	if len(raw) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
}
