//go:build linux

package sys

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("SKELET_EXEC_PROBE") == "1" {
		err := Exec("/bin/sh", []string{"sh", "-c", "grep -E '^Sig(Blk|Ign):' /proc/self/status"}, os.Environ())
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	os.Exit(m.Run())
}

func masks(t *testing.T, status []byte) (blocked, ignored uint64) {
	t.Helper()
	sc := bufio.NewScanner(bytes.NewReader(status))
	for sc.Scan() {
		name, val, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSpace(val), 16, 64)
		if err != nil {
			continue
		}
		switch name {
		case "SigBlk":
			blocked = n
		case "SigIgn":
			ignored = n
		}
	}
	return blocked, ignored
}

// A script exec'd through the front door must still stop on Ctrl-C - a blocked or newly ignored SIGINT would carry
// across the exec into it
func TestExecLeavesSIGINTDeliverable(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "SKELET_EXEC_PROBE=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	own, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	const sigint = uint64(1) << 1
	blocked, ignored := masks(t, out)
	_, ownIgnored := masks(t, own)
	if blocked&sigint != 0 {
		t.Errorf("SIGINT is blocked in the exec'd program")
	}
	if ignored&sigint != 0 && ownIgnored&sigint == 0 {
		t.Errorf("SIGINT is ignored in the exec'd program but not in its parent")
	}
}
