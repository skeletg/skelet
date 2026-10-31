package hooks

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The one hook bin/install-hooks ever wrote, so existing clones are recognised
const oldHook = `#!/bin/sh
# Installed by bin/install-hooks. sh on purpose - bash whines about the locale first Deliberately NOT bin/check --codegen - needing a
# running backend to make a commit is a bad trade, so codegen staleness is on you to check before a schema-touching PR
export LC_ALL=C.UTF-8 LANG=C.UTF-8
exec ./bin/check
`

func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return root
}

func hook(t *testing.T, root, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestClassify(t *testing.T) {
	root := repo(t)
	p := filepath.Join(root, ".git", "hooks", "pre-commit")
	for body, want := range map[string]Kind{
		oldHook:                         Old,
		Script("/usr/local/bin/skelet"): Current,
		"#!/bin/sh\nexec ./bin/check\n": Foreign,
		"#!/bin/sh\nnpm test\n":         Foreign,
	} {
		hook(t, root, body, 0o755)
		if got := Classify(p); got != want {
			t.Errorf("Classify(%q) = %v, want %v", body[:20], got, want)
		}
	}
	_ = os.Remove(p)
	if Classify(p) != Missing {
		t.Error("no hook isn't Missing")
	}
}

func TestUpgradeRewritesOnlyTheOldHook(t *testing.T) {
	root := repo(t)
	t.Setenv("SKELET_CLI", "/opt/skelet's/skelet")
	p := hook(t, root, oldHook, 0o755)
	var out bytes.Buffer
	Upgrade(root, &out)
	raw, _ := os.ReadFile(p)
	if string(raw) != Script("/opt/skelet's/skelet") || !strings.Contains(out.String(), "updated the pre-commit hook") {
		t.Errorf("hook now:\n%s\noutput: %s", raw, out.String())
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm()&0o111 == 0 {
		t.Error("the upgraded hook isn't executable")
	}

	mine := "#!/bin/sh\nexec ./bin/check\n"
	hook(t, root, mine, 0o755)
	out.Reset()
	Upgrade(root, &out)
	if raw, _ := os.ReadFile(p); string(raw) != mine || out.Len() != 0 {
		t.Error("Upgrade touched a hook it didn't write")
	}
}

func TestRunKeepsAForeignHookWithoutForce(t *testing.T) {
	root := repo(t)
	p := hook(t, root, "#!/bin/sh\nnpm test\n", 0o755)
	if code := Run(root, nil); code != 1 {
		t.Errorf("exit %d over a foreign hook", code)
	}
	if code := Run(root, []string{"--force"}); code != 0 {
		t.Errorf("--force: exit %d", code)
	}
	if Classify(p) != Current {
		t.Error("--force didn't install the hook")
	}
	if code := Run(root, []string{"--bogus"}); code != 2 {
		t.Errorf("a bad flag: exit %d", code)
	}
}

// The hook runs in sh with no skelet anywhere and no Go: it has to say why the commit is blocked
func TestHookExplainsWhenItCantFindTheCLI(t *testing.T) {
	root := repo(t)
	p := hook(t, root, Script(""), 0o755)
	cmd := exec.Command("/bin/sh", p)
	cmd.Dir = root
	cmd.Env = []string{"PATH=/nonexistent", "HOME=" + t.TempDir()}
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "no skelet CLI and no Go") {
		t.Errorf("err %v, output %q", err, out)
	}
}

// With a skelet on the PATH the hook runs it with check, and nothing after
func TestHookRunsTheCLI(t *testing.T) {
	root := repo(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "skelet"), []byte("#!/bin/sh\necho \"ran skelet $*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := hook(t, root, Script(""), 0o755)
	cmd := exec.Command("/bin/sh", p)
	cmd.Dir = root
	cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + t.TempDir()}
	if out, err := cmd.CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "ran skelet check" {
		t.Errorf("err %v, output %q", err, out)
	}
}
