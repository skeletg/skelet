package gates

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A repo that passes every structure rule: scripts that parse and run, a route table over the floor, templates for
// every env_file and required variable, and a well-formed changelog
func repoTree(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	routes := `{Verb: "up", Script: "dev"},` + "\n"
	for i := range 29 {
		routes += "\t{Verb: \"v" + strings.Repeat("x", i+1) + "\", Go: stack.Down},\n"
	}
	put(t, root, map[string]string{
		"skelet":                             "#!/usr/bin/env bash\nexec true\n",
		"bin/_lib":                           "export X=1\n",
		"bin/dev":                            "#!/usr/bin/env bash\n. bin/_lib\ndocker compose up -d\n",
		"bin/check":                          "#!/usr/bin/env bash\nexec ./skelet check\n",
		"cmd/skelet/install.sh":              "#!/bin/sh\necho hi\n",
		"cmd/skelet/internal/route/table.go": "package route\n\nvar routes = []Route{\n\t" + routes + "}\n",
		"docker-compose.yml":                 "services:\n  app:\n    env_file: env/go.env\n",
		"docker-compose.prod.yml":            "x: ${NEEDED:?set it}\n",
		"env/go.env.example":                 "A=1\n",
		".env.example":                       "# NEEDED=\n",
		"README.md":                          "Run bin/dev.\n",
		"changelog/0.1.0.md":                 "Ran bin/long-gone back then.\n",
		"CHANGELOG.md":                       "## [Unreleased]\n\n## Releases\n\n- [0.1.0](changelog/0.1.0.md) - 2026-01-01\n",
		"changelog/unreleased/README.md":     "fragments\n",
		"cmd/skelet/internal/hooks/hooks.go": "// recognises the hook bin/install-hooks wrote\n",
	})
	for _, s := range []string{"skelet", "bin/_lib", "bin/dev", "bin/check"} {
		if err := os.Chmod(filepath.Join(root, s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
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
	for _, line := range []string{"ok    4 scripts", "ok    1 routed scripts all present", "ok    every bin script is reachable", "ok    no dangling references", "==> repo-check OK"} {
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
		{"a script without +x", func(t *testing.T, root string) { _ = os.Chmod(filepath.Join(root, "bin/dev"), 0o644) },
			"FAIL  bin/dev is not executable (git update-index --chmod=+x bin/dev)"},
		{"a script that doesn't parse", func(t *testing.T, root string) {
			put(t, root, map[string]string{"bin/dev": "#!/usr/bin/env bash\nif then\n"})
		}, "FAIL  bin/dev is not valid bash"},
		{"an installer with a bashism", func(t *testing.T, root string) {
			put(t, root, map[string]string{"cmd/skelet/install.sh": "#!/bin/sh\nx=(a b)\n"})
		}, "FAIL  cmd/skelet/install.sh is not valid sh"},
		{"a route to a missing script", func(t *testing.T, root string) {
			src, _ := os.ReadFile(filepath.Join(root, "cmd/skelet/internal/route/table.go"))
			put(t, root, map[string]string{"cmd/skelet/internal/route/table.go": strings.Replace(string(src), `Script: "dev"`, `Script: "gone"`, 1)})
		}, "FAIL  the route table sends a verb to bin/gone, which is missing or not executable"},
		{"a script with no verb", func(t *testing.T, root string) {
			put(t, root, map[string]string{"bin/orphan": "#!/usr/bin/env bash\n"})
			_ = os.Chmod(filepath.Join(root, "bin/orphan"), 0o755)
		}, "FAIL  bin/orphan has no skelet verb"},
		{"a table under the floor", func(t *testing.T, root string) {
			put(t, root, map[string]string{"cmd/skelet/internal/route/table.go": "package route\n"})
		}, "FAIL  only 0 routes found in cmd/skelet/internal/route/table.go"},
		{"a container-creating script without _lib", func(t *testing.T, root string) {
			put(t, root, map[string]string{"bin/dev": "#!/usr/bin/env bash\ndocker compose run --rm app true\n"})
			_ = os.Chmod(filepath.Join(root, "bin/dev"), 0o755)
		}, "FAIL  bin/dev creates containers without sourcing bin/_lib"},
		{"a reference to a deleted script", func(t *testing.T, root string) {
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
