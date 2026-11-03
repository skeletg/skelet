package launch

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const moduleRoot = "../.."

// A project tree holding a copy of this module at cmd/skelet, with the build cache kept but the launcher's own cache
// moved into the test's temp dir
func project(t *testing.T) string {
	t.Helper()
	gocache, err := exec.Command("go", "env", "GOCACHE").Output()
	if err != nil {
		t.Skip("no go to build with")
	}
	t.Setenv("GOCACHE", strings.TrimSpace(string(gocache)))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	dst := filepath.Join(root, "cmd", "skelet")
	err = filepath.WalkDir(moduleRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(moduleRoot, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), raw, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// The flags the launcher builds with have to produce a checkout build - if main.mode were renamed, -X would set
// nothing and the result would be a release binary launching itself
func TestTheLaunchersBuildIsACheckoutBuild(t *testing.T) {
	root := project(t)
	bin, err := Build(root, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "version")
	cmd.Env = append(os.Environ(), "SKELET_ROOT="+root)
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "(checkout "+root+")") {
		t.Errorf("%v: %s - want the checkout build's version line", err, out)
	}
	again, err := Build(root, io.Discard)
	if err != nil || again != bin {
		t.Errorf("an unchanged tree built again: %s then %s (%v)", bin, again, err)
	}
}

func TestConcurrentFirstRunsAgree(t *testing.T) {
	root := project(t)
	var wg sync.WaitGroup
	got := make([]string, 4)
	errs := make([]error, 4)
	for i := range got {
		wg.Go(func() { got[i], errs[i] = Build(root, io.Discard) })
	}
	wg.Wait()
	for i := range got {
		if errs[i] != nil || got[i] != got[0] {
			t.Errorf("run %d: %s, %v - want %s", i, got[i], errs[i], got[0])
		}
	}
}

// A cmd/skelet that stops compiling falls back to the last build that did, so doctor and the commit hook still run
func TestABrokenSourceRunsTheLastGoodBuild(t *testing.T) {
	root := project(t)
	good, err := Build(root, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(root, "cmd", "skelet", "main.go")
	if err := os.WriteFile(main, []byte("package main\nfunc main() { broken }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if got, err := Build(root, &log); err != nil || got != good || !strings.Contains(log.String(), "running the last build that did") {
		t.Errorf("got %s, %v, log %q - want the last good build with a warning", got, err, log.String())
	}
	t.Setenv("SKELET_STRICT", "1")
	if _, err := Build(root, io.Discard); err == nil || !strings.Contains(err.Error(), "building cmd/skelet failed") {
		t.Errorf("SKELET_STRICT: %v - want the compile error", err)
	}
}

func TestKeyFollowsTheSourceOnly(t *testing.T) {
	src := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module x/cmd/skelet\n\ngo 1.27.3\n")
	write("main.go", "package main\n")
	base, _ := Key(src, "1.27.3")
	for rel, changes := range map[string]bool{
		"main_test.go":         false,
		"internal/testdata/x":  false,
		".skelet-local":        false,
		"skelet":               false,
		"internal/x/embed.txt": true,
		"main.go":              true,
	} {
		write(rel, "changed "+rel)
		now, _ := Key(src, "1.27.3")
		if (now != base) != changes {
			t.Errorf("changing %s: key moved %v, want %v", rel, now != base, changes)
		}
		base = now
	}
	if other, _ := Key(src, "1.28.0"); other == base {
		t.Error("the Go version isn't part of the key")
	}
}

func TestPruneKeepsTheNewestAndEveryLastGood(t *testing.T) {
	dir := t.TempDir()
	var builds []string
	for i := range keep + 3 {
		b := filepath.Join(dir, "skelet-"+strings.Repeat(string(rune('a'+i)), 4))
		if err := os.WriteFile(b, nil, 0o755); err != nil {
			t.Fatal(err)
		}
		builds = append(builds, b)
	}
	remember(dir, "/some/project", builds[0])
	for i, b := range builds {
		touch(b)
		_ = os.Chtimes(b, mtimeOf(i), mtimeOf(i))
	}
	prune(dir)
	left, _ := filepath.Glob(filepath.Join(dir, "skelet-*"))
	if len(left) != keep+1 {
		t.Errorf("%d builds left, want the newest %d plus the protected oldest", len(left), keep)
	}
	if _, err := os.Stat(builds[0]); err != nil {
		t.Error("a project's last good build was pruned")
	}
}

// The i-th build is i hours newer than the first
func mtimeOf(i int) time.Time { return time.Unix(1_700_000_000, 0).Add(time.Duration(i) * time.Hour) }
