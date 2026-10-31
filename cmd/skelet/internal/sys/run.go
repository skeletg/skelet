//go:build unix

package sys

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
)

var (
	children atomic.Int32

	cleanupMu sync.Mutex
	cleanups  []func()
)

// AtExit registers f to run when the CLI exits through Exit or an interrupt - os.Exit skips deferred calls, so a lock
// or a background process released in a defer would outlive Ctrl-C. Cleanups run last-registered first, once
func AtExit(f func()) {
	cleanupMu.Lock()
	defer cleanupMu.Unlock()
	cleanups = append(cleanups, f)
}

func runCleanups() {
	cleanupMu.Lock()
	fs := cleanups
	cleanups = nil
	cleanupMu.Unlock()
	for _, f := range slices.Backward(fs) {
		f()
	}
}

// Exit runs the AtExit cleanups, then exits with code
func Exit(code int) {
	runCleanups()
	os.Exit(code)
}

// HandleInterrupts is for a verb that runs several commands in turn. Ctrl-C reaches the running child from the
// terminal on its own, so this process waits for it and then exits the way it did; with no child running, an
// interrupt ends the verb at once. Notify rather than Ignore - an ignored signal would be inherited
func HandleInterrupts() { handleOnce.Do(handle) }

var handleOnce sync.Once

func handle() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		for s := range ch {
			if children.Load() == 0 {
				Exit(128 + int(s.(syscall.Signal)))
			}
		}
	}()
}

// Run starts cmd and waits for it, counted as a running child for HandleInterrupts
func Run(cmd *exec.Cmd) error {
	children.Add(1)
	defer children.Add(-1)
	return cmd.Run()
}

// ExitCode is the code a shell would report for a finished command: its own, or 128+n when signal n killed it
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return 1
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ee.ExitCode()
}

// Env is the environment the scripts get: this one, minus what the shim and the launcher pass the CLI
func Env() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "SKELET_ROOT", "SKELET_SHIM", "SKELET_LAUNCHED", "SKELET_CLI":
		default:
			env = append(env, kv)
		}
	}
	return env
}
