package gates

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// A repo that passes every structure rule: a POSIX installer, no bin/ mentions outside released changelogs and the
// files exempt on purpose, templates for every env_file and required variable, and a well-formed changelog
func repoTree(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	put(t, root, map[string]string{
		"cmd/skelet/install.sh":              "#!/bin/sh\necho hi\n",
		"docker-compose.yml":                 "services:\n  app:\n    env_file: env/go.env\n",
		"docker-compose.prod.yml":            "x: ${NEEDED:?set it}\n",
		"env/go.env.example":                 "A=1\n",
		".env.example":                       "# NEEDED=\n",
		"README.md":                          "Run skelet up. Installed to ~/.local/bin/skelet, node_modules/.bin/vite is fine.\n",
		"changelog/0.1.0.md":                 "Ran bin/long-gone back then.\n",
		"CHANGELOG.md":                       "## [Unreleased]\n\n## Releases\n\n- [0.1.0](changelog/0.1.0.md) - 2026-01-01\n",
		"changelog/unreleased/README.md":     "fragments\n",
		"cmd/skelet/internal/hooks/hooks.go": "// recognises the hook bin/install-hooks wrote\n",
	})
	git(t, root, "init", "-q")
	git(t, root, "add", "-A")
	return root
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
}

func structure(t *testing.T, root string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code := repoCheck(root, &out)
	return out.String(), code
}

func TestStructureHealthy(t *testing.T) {
	out, code := structure(t, repoTree(t))
	if code != 0 || strings.Contains(out, "FAIL") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, line := range []string{"ok    no dangling references", "ok    every env_file has a template", "==> repo-check OK"} {
		if !strings.Contains(out, line) {
			t.Errorf("no %q in:\n%s", line, out)
		}
	}
}

func TestStructureFailures(t *testing.T) {
	cases := []struct {
		name    string
		breakIt func(t *testing.T, root string)
		want    string
	}{
		{"an installer with a bashism", func(t *testing.T, root string) {
			put(t, root, map[string]string{"cmd/skelet/install.sh": "#!/bin/sh\nx=(a b)\n"})
		}, "FAIL  cmd/skelet/install.sh is not valid sh"},
		{"a reference to a retired script", func(t *testing.T, root string) {
			put(t, root, map[string]string{"apps/x/main.go": "// run bin/psql for that\n"})
			git(t, root, "add", "-A")
		}, "FAIL  referenced but missing: bin/psql"},
		{"an env_file without a template", func(t *testing.T, root string) {
			put(t, root, map[string]string{"docker-compose.small.yml": "env_file: env/small.env\n"})
		}, "FAIL  env/small.env has no committed env/small.env.example"},
		{"a required variable missing from .env.example", func(t *testing.T, root string) {
			put(t, root, map[string]string{"docker-compose.small.yml": "y: ${OTHER:?set it}\n"})
		}, "FAIL  compose refuses to start without OTHER, and .env.example never mentions it"},
		{"a malformed changelog fragment", func(t *testing.T, root string) {
			put(t, root, map[string]string{"changelog/unreleased/x.added.md": "no dash\n"})
		}, `FAIL  changelog/unreleased/x.added.md - must start with a "- " entry`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := repoTree(t)
			c.breakIt(t, root)
			out, code := structure(t, root)
			if code != 1 || !strings.Contains(out, c.want) || !strings.Contains(out, "==> repo-check FAILED") {
				t.Errorf("exit %d, want %q in:\n%s", code, c.want, out)
			}
		})
	}
}
