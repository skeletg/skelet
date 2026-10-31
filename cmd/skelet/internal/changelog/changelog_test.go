package changelog

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The outputs below are what the bash changelog script printed for this same tree before it was retired

func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range map[string]string{
		"CHANGELOG.md": "# Changelog\n\nIntro.\n\n## [Unreleased]\n\n## Releases\n\n- [0.2.0](changelog/0.2.0.md) - 2026-10-18\n" +
			"- [0.1.0](changelog/0.1.0.md) - 2026-09-01\n\n[Unreleased]: https://github.com/skeletg/skelet/compare/v0.2.0...develop\n",
		"changelog/0.1.0.md":                    "# [0.1.0]\n",
		"changelog/0.2.0.md":                    "# [0.2.0]\n",
		"changelog/unreleased/README.md":        "# Unreleased changes\n",
		"changelog/unreleased/alpha.added.md":   "- **First.** One.\n",
		"changelog/unreleased/beta.added.md":    "\n\n- **Second.** Two,\n  wrapped.\n\n  \n",
		"changelog/unreleased/gamma.fixed.md":   "- **A fix.**\n",
		"changelog/unreleased/intro.summary.md": "An opening paragraph.\n",
	} {
		write(t, root, name, body)
	}
	return root
}

func write(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const body = "An opening paragraph.\n\n### Added\n\n- **First.** One.\n\n- **Second.** Two,\n  wrapped.\n\n### Fixed\n\n- **A fix.**"

func TestPreviewAndCheck(t *testing.T) {
	root := tree(t)
	if got := assemble(root); got != body {
		t.Errorf("preview:\n%s\nwant:\n%s", got, body)
	}
	var out bytes.Buffer
	if code := Check(root, &out); code != 0 || out.String() != "  ok    4 fragments well-formed, 2 releases listed, Unreleased empty\n" {
		t.Errorf("check: exit %d\n%s", code, out.String())
	}
}

func TestCheckFailures(t *testing.T) {
	root := tree(t)
	write(t, root, "changelog/unreleased/Bad_Name.md", "x\n")
	write(t, root, "changelog/unreleased/thing.misc.md", "- y\n")
	write(t, root, "changelog/unreleased/head.added.md", "## heading\n")
	write(t, root, "changelog/unreleased/plain.fixed.md", "no dash\n")
	write(t, root, "changelog/9.9.9.md", "# [9.9.9]\n")
	index, _ := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	s := strings.Replace(string(index), "- [0.1.0](changelog/0.1.0.md) - 2026-09-01", "- [0.0.1](changelog/0.0.1.md) - 2026-01-01", 1)
	s = strings.Replace(s, "## [Unreleased]\n", "## [Unreleased]\n\n### Added\n- sneaked in\n", 1)
	write(t, root, "CHANGELOG.md", s)
	want := `  FAIL  changelog/unreleased/Bad_Name.md - name it <slug>.<section>.md, e.g. filesystem-storage.added.md
  FAIL  changelog/unreleased/head.added.md - no headings, the section comes from the file name
  FAIL  changelog/unreleased/plain.fixed.md - must start with a "- " entry
  FAIL  changelog/unreleased/thing.misc.md - "misc" is not one of: added changed deprecated removed fixed security summary
  FAIL  CHANGELOG.md has entries under [Unreleased] - move them into changelog/unreleased/
  FAIL  changelog/0.1.0.md is not listed under Releases in CHANGELOG.md
  FAIL  changelog/9.9.9.md is not listed under Releases in CHANGELOG.md
  FAIL  CHANGELOG.md lists 0.0.1, but changelog/0.0.1.md does not exist
`
	var out bytes.Buffer
	if code := Check(root, &out); code != 1 || out.String() != want {
		t.Errorf("exit %d\n%s\nwant:\n%s", code, out.String(), want)
	}
}

func TestRelease(t *testing.T) {
	root := tree(t)
	if _, err := exec.LookPath("git"); err == nil {
		for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init"}} {
			if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %s", args, out)
			}
		}
	}
	stdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	code := release(root, []string{"0.3.0", "2026-11-01"})
	os.Stdout = stdout
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	file, _ := os.ReadFile(filepath.Join(root, "changelog/0.3.0.md"))
	if want := "# [0.3.0] - 2026-11-01\n\n" + body + "\n\n[0.3.0]: https://github.com/skeletg/skelet/compare/v0.2.0...v0.3.0\n"; string(file) != want {
		t.Errorf("release file:\n%s\nwant:\n%s", file, want)
	}
	index, _ := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	want := "# Changelog\n\nIntro.\n\n## [Unreleased]\n\n## Releases\n\n- [0.3.0](changelog/0.3.0.md) - 2026-11-01\n" +
		"- [0.2.0](changelog/0.2.0.md) - 2026-10-18\n- [0.1.0](changelog/0.1.0.md) - 2026-09-01\n\n" +
		"[Unreleased]: https://github.com/skeletg/skelet/compare/v0.3.0...develop\n"
	if string(index) != want {
		t.Errorf("CHANGELOG.md:\n%s\nwant:\n%s", index, want)
	}
	left, _ := os.ReadDir(filepath.Join(root, "changelog/unreleased"))
	if len(left) != 1 || left[0].Name() != "README.md" {
		t.Errorf("left in unreleased: %v", left)
	}
	for args, want := range map[string]int{"0.3.0": 1, "3.0": 2, "": 2} {
		if got := release(root, strings.Fields(args)); got != want {
			t.Errorf("release %q: exit %d, want %d", args, got, want)
		}
	}
	if got := release(root, []string{"0.4.0"}); got != 1 {
		t.Errorf("a release with no fragments: exit %d, want 1", got)
	}
}
