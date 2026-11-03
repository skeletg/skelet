//go:build unix

package sys

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCleanupsRunLastFirstAndOnce(t *testing.T) {
	var got []string
	AtExit(func() { got = append(got, "first") })
	AtExit(func() { got = append(got, "second") })
	runCleanups()
	runCleanups()
	if strings.Join(got, ",") != "second,first" {
		t.Errorf("ran %v", got)
	}
}

// An interrupt with no child running exits 130 - after the cleanups, which os.Exit alone would skip
func TestInterruptRunsTheCleanups(t *testing.T) {
	if os.Getenv("SKELET_INTERRUPT_PROBE") == "1" {
		HandleInterrupts()
		AtExit(func() { os.Stdout.WriteString("cleaned\n") })
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestInterruptRunsTheCleanups$")
	cmd.Env = append(os.Environ(), "SKELET_INTERRUPT_PROBE=1")
	out, err := cmd.Output()
	if code := ExitCode(err); code != 130 || !strings.Contains(string(out), "cleaned") {
		t.Errorf("exit %d, output %q - want 130 after the cleanup ran", code, out)
	}
}
