package doctor

import (
	"bytes"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/hooks"
)

// The host tools doctor calls, as scripts answering from FAKE_* variables. A tool left out of the map is missing:
// PATH holds these and the few real commands bin/_lib needs, nothing else
var fakes = map[string]string{
	"docker": `case "$*" in
  "info") [ -z "$FAKE_DOCKER_DOWN" ] ;;
  "info -f {{.SecurityOptions}}") [ -z "$FAKE_DOCKER_DOWN" ] && echo "[name=seccomp]" ;;
  "compose version --short") echo 2.30.0 ;;
  "compose ps --status running --format {{.Service}}") printf '%s\n' $FAKE_RUNNING ;;
  "compose ps --format {{.Publishers}}") echo "$FAKE_PUBLISHERS" ;;
  "compose exec -T clickhouse"*) echo "${FAKE_SPINE-1}" ;;
  "compose port "*) [ -n "$FAKE_SF_PORT" ] && echo "0.0.0.0:$FAKE_SF_PORT" ;;
  *) exit 1 ;;
esac`,
	"git": `case "$*" in
  "--version") echo "git version ${FAKE_GIT-2.43.0}" ;;
  "rev-parse --git-path hooks/pre-commit") echo .git/hooks/pre-commit ;;
  *) exit 1 ;;
esac`,
	"go": `case "$*" in
  "version") echo "go version go${FAKE_GO-1.27.3} linux/amd64" ;;
  "env GOTOOLCHAIN") echo "${FAKE_TOOLCHAIN-auto}" ;;
esac`,
	"skelet": `true`,
	"pnpm":   `echo "${FAKE_PNPM-12.6.0}"`,
	"node":   `echo "v${FAKE_NODE-24.1.0}"`,
}

func path(t *testing.T, missing ...string) {
	t.Helper()
	fake, real := t.TempDir(), t.TempDir()
	for name, body := range fakes {
		if !slices.Contains(missing, name) {
			if err := os.WriteFile(filepath.Join(fake, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range []string{"bash", "sh", "tr", "grep", "dirname", "id", "cat", "sed"} {
		if p, err := exec.LookPath(name); err == nil {
			if err := os.Symlink(p, filepath.Join(real, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Setenv("PATH", fake+string(os.PathListSeparator)+real)
}

func put(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A checkout that is healthy in every way doctor looks: dev env files, a profile matching them, the dashboard in
// Docker, a decided example plugin, a running stack and the hook installed
func healthy(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, "apps/backend/core/go.mod", "module x\n\ngo 1.27.3\n")
	put(t, root, "docker-compose.yml", "services:\n  app:\n    image: app:${APP_TAG}\n")
	put(t, root, ".env", "APP_TAG=dev\n")
	put(t, root, "env/go.env", "SIGNALS_ENABLED=true\nAI_ENABLED=true\n")
	put(t, root, "env/db.env", "")
	put(t, root, "env/signals.env", "")
	put(t, root, ".skelet-profile", "SKELET_TARGET=dev\nSKELET_SIGNALS=true\nSKELET_AI=true\nSKELET_SEARCH=opensearch\nSKELET_GATEWAYS=\n")
	put(t, root, ".skelet-apps", "dashboard\n")
	put(t, root, ".skelet-tools", "\n")
	put(t, root, "apps/dashboard/package.json", `{"packageManager": "pnpm@12.6.0"}`)
	put(t, root, "apps/backend/core/plugins/example/plugin.go", "package example\nfunc init() { sdk.RegisterPlugin(nil) }\n")
	put(t, root, "apps/backend/core/plugins/plugins.json", "{\n  \"plugins\": {\n    \"example\": {\n      \"enabled\": true\n    }\n  }\n}\n")
	put(t, root, ".git/hooks/pre-commit", hooks.Script(""))
	if err := os.Chmod(filepath.Join(root, ".git/hooks/pre-commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	path(t)
	t.Setenv("SKELET_UID", "1000")
	t.Setenv("SKELET_GID", "1000")
	t.Setenv("FAKE_RUNNING", "app db clickhouse signals")
	t.Setenv("FAKE_PUBLISHERS", "[{0.0.0.0 8080 8080 tcp}] [{127.0.0.1 5432 5432 tcp}] [{0.0.0.0 5173 5173 tcp}] [{0.0.0.0 9900 3900 tcp}] [{0.0.0.0 9902 3902 tcp}]")
	return root
}

func allUp() probes {
	return probes{
		open: func(string) bool { return true },
		get: func(url string) (int, string, error) {
			if strings.HasSuffix(url, "/health") {
				return 200, "ok", nil
			}
			return 404, "", nil
		},
	}
}

func doctor(t *testing.T, root string, p probes) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code := checkout(root, &out, p)
	return out.String(), code
}

func expect(t *testing.T, out string, lines ...string) {
	t.Helper()
	for _, l := range lines {
		if !strings.Contains(out, l) {
			t.Errorf("no line %q in:\n%s", l, out)
		}
	}
}

func TestAHealthyCheckoutIsAllOk(t *testing.T) {
	root := healthy(t)
	out, code := doctor(t, root, allUp())
	if code != 0 || strings.Contains(out, "  FAIL  ") || strings.Contains(out, "  warn  ") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	expect(t, out,
		"ok    docker daemon reachable",
		"ok    docker compose v2 present (2.30.0)",
		"ok    git 2.43.0",
		"ok    go 1.27.3",
		"ok    env/go.env",
		"ok    in Docker: dashboard",
		"ok    none - every frontend runs in Docker",
		"ok    target=dev signals=true ai=true search=opensearch",
		"ok    every compose interpolation has a value",
		"ok    1 decided, enabled: example",
		"ok    running: app clickhouse db signals",
		"ok    clickhouse spine up, schema present",
		"ok    signals process up",
		"ok    backend (:8080) answering",
		"ok    backend /health: ok",
		"ok    *.localhost resolves",
		"ok    pre-commit hook runs skelet check",
		"0 warnings, 0 failures",
	)
}

func TestProblemsAreReported(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, root string)
		code  int
		lines []string
	}{
		{"docker down", func(t *testing.T, _ string) { t.Setenv("FAKE_DOCKER_DOWN", "1") }, 1,
			[]string{"FAIL  docker daemon not reachable"}},
		{"a missing env file", func(t *testing.T, root string) { _ = os.Remove(filepath.Join(root, "env/db.env")) }, 1,
			[]string{"FAIL  env/db.env missing - run skelet setup"}},
		{"an optional env file", func(t *testing.T, root string) {
			put(t, root, ".skelet-profile", "SKELET_TARGET=small\n")
			put(t, root, "env/small.env", "")
			put(t, root, "docker-compose.small.yml", "services:\n  app:\n    env_file:\n      - path: env/small.env\n      - path: env/small-extra.env\n        required: false\n")
		}, 0, []string{"env files (small target)", "warn  env/small-extra.env missing - optional"}},
		{"profile against env drift", func(t *testing.T, root string) {
			put(t, root, ".skelet-profile", "SKELET_TARGET=dev\nSKELET_SIGNALS=false\nSKELET_AI=true\n")
		}, 0, []string{"warn  SIGNALS_ENABLED is true in env/go.env but the profile says false"}},
		{"an older profile", func(t *testing.T, root string) {
			put(t, root, ".skelet-profile", "SKELET_TARGET=dev\nSKELET_B2B=true\n")
		}, 0,
			[]string{"ok    target=dev signals=(unset) ai=(unset) search=(unset)"}},
		{"an unset interpolation", func(t *testing.T, root string) { put(t, root, ".env", "") }, 0,
			[]string{"warn  no value for: APP_TAG"}},
		{"signals without clickhouse", func(t *testing.T, _ string) { t.Setenv("FAKE_RUNNING", "app db signals") }, 1,
			[]string{"FAIL  SIGNALS_ENABLED is on but no clickhouse container"}},
		{"a missing spine schema", func(t *testing.T, _ string) { t.Setenv("FAKE_SPINE", "0") }, 1,
			[]string{"FAIL  clickhouse is up but signals.analytics_events is missing"}},
		{"signals off skips the suite checks", func(t *testing.T, root string) {
			put(t, root, "env/go.env", "SIGNALS_ENABLED=off\nAI_ENABLED=true\n")
			put(t, root, ".skelet-profile", "SKELET_TARGET=dev\n")
			t.Setenv("FAKE_RUNNING", "app db")
		}, 0, nil},
		// A selection naming a storefront that isn't here starts nothing, and says so
		{"an app selection naming a storefront that's gone", func(t *testing.T, root string) {
			put(t, root, ".skelet-apps", "dashboard,vetrina-demo\n")
			put(t, root, "templates/vetrina/package.json", "{}")
		}, 0, []string{`warn  .skelet-apps names "vetrina-demo", which isn't an app here - storefronts: vetrina`}},
		{"an override on the old storefront mount", func(t *testing.T, root string) {
			put(t, root, "docker-compose.override.yml", "services:\n  shop-eu:\n    volumes:\n      - ./apps/channels/shop:/var/www/storefront\n")
		}, 0, []string{"warn  docker-compose.override.yml mounts a storefront at /var/www/storefront"}},
		{"an override storefront forked before the read-only workspace", func(t *testing.T, root string) {
			put(t, root, "docker-compose.override.yml", "services:\n  shop:\n    volumes:\n      - .:/var/www/skelet\n")
		}, 0, []string{"warn  docker-compose.override.yml mounts the whole checkout writable"}},
		{"a parked plugin name", func(t *testing.T, root string) {
			put(t, root, "apps/backend/core/plugins/plugins.json", `{"plugins": {"example": 1, "_example": 0}}`)
		}, 1, []string{`FAIL  plugins.json names "_example"`, "warn  plugins.json uses the old 0/1 form"}},
		{"an undecided plugin", func(t *testing.T, root string) {
			put(t, root, "apps/backend/core/plugins/fresh/plugin.go", "package fresh\nfunc init() { sdk.RegisterPlugin(nil) }\n")
		}, 1, []string{"FAIL  plugins/fresh has no decision"}},
		{"an enabled plugin with no directory", func(t *testing.T, root string) {
			put(t, root, "apps/backend/core/plugins/plugins.json", `{"plugins": {"example": {"enabled": true}, "gone": {"enabled": true}}}`)
		}, 1, []string{"FAIL  plugins.json enables gone, but plugins/gone registers no plugin"}},
		{"an old git", func(t *testing.T, _ string) { t.Setenv("FAKE_GIT", "2.34.1") }, 0,
			[]string{"warn  git 2.34.1 is older than 2.35"}},
		{"no git", func(t *testing.T, _ string) { path(t, "git") }, 1, []string{"FAIL  no git"}},
		{"an old go that can't fetch", func(t *testing.T, _ string) {
			t.Setenv("FAKE_GO", "1.26.3")
			t.Setenv("FAKE_TOOLCHAIN", "local")
		}, 0, []string{"warn  go 1.26.3 is older than go.mod's 1.27.3"}},
		{"an old go that fetches", func(t *testing.T, _ string) { t.Setenv("FAKE_GO", "1.26.3") }, 0,
			[]string{"ok    go 1.26.3 - fetches 1.27.3"}},
		{"a host app with the wrong pnpm", func(t *testing.T, root string) {
			put(t, root, ".skelet-apps", "\n")
			put(t, root, ".nvmrc", "24\n")
			t.Setenv("FAKE_PNPM", "11.0.0")
			t.Setenv("FAKE_NODE", "22.3.0")
		}, 1, []string{
			"FAIL  pnpm in apps/dashboard answers 11.0.0, the apps pin 12.6.0",
			"warn  node 22.3.0, and .nvmrc pins 24",
			"warn  apps/dashboard not installed",
		}},
		{"a host app with an npm tree", func(t *testing.T, root string) {
			put(t, root, ".skelet-apps", "\n")
			put(t, root, "apps/dashboard/node_modules/react/index.js", "")
		}, 1, []string{"ok    pnpm 12.6.0", "FAIL  apps/dashboard/node_modules is an npm install"}},
		{"a port another stack holds", func(t *testing.T, _ string) {
			t.Setenv("FAKE_PUBLISHERS", "[{127.0.0.1 5432 5432 tcp}]")
		}, 0, []string{
			"warn  backend (:8080) answering, but NOT from this checkout",
			"warn  backend /health skipped - this checkout publishes no :8080",
		}},
		{"nothing running", func(t *testing.T, _ string) { t.Setenv("FAKE_RUNNING", "") }, 0,
			[]string{"warn  nothing running - start with skelet up"}},
		{"the old hook", func(t *testing.T, root string) {
			put(t, root, ".git/hooks/pre-commit", "#!/bin/sh\n# Installed by bin/install-hooks.\nexec ./bin/check\n")
		}, 1, []string{"FAIL  the pre-commit hook still runs bin/check, which is gone"}},
		{"a hook of your own that calls bin/", func(t *testing.T, root string) {
			put(t, root, ".git/hooks/pre-commit", "#!/bin/sh\nbin/lint\n")
		}, 0, []string{"warn  the pre-commit hook is your own and calls bin/ scripts"}},
		{"a hook that lost its +x", func(t *testing.T, root string) {
			_ = os.Chmod(filepath.Join(root, ".git/hooks/pre-commit"), 0o644)
		}, 0, []string{"warn  the pre-commit hook isn't executable"}},
		{"no hook", func(t *testing.T, root string) { _ = os.Remove(filepath.Join(root, ".git/hooks/pre-commit")) }, 0,
			[]string{"ok    no pre-commit hook - skelet hooks adds one"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := healthy(t)
			c.setup(t, root)
			out, code := doctor(t, root, allUp())
			if code != c.code {
				t.Errorf("exit %d, want %d", code, c.code)
			}
			expect(t, out, c.lines...)
		})
	}
}

// Only a lookup failure means *.localhost doesn't resolve - the bash doctor's fallback passed on curl's 000 too, so a
// refused connection or a dead name both read as resolving
func TestMediaLookup(t *testing.T) {
	root := healthy(t)
	p := allUp()
	p.get = func(url string) (int, string, error) {
		if strings.Contains(url, "media.localhost") {
			return 0, "", &net.DNSError{Err: "no such host", Name: "media.localhost", IsNotFound: true}
		}
		return 200, "ok", nil
	}
	out, _ := doctor(t, root, p)
	expect(t, out, "warn  media.localhost doesn't resolve here")

	p.get = func(url string) (int, string, error) {
		if strings.Contains(url, "media.localhost") {
			return 0, "", &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		}
		return 200, "ok", nil
	}
	out, _ = doctor(t, root, p)
	expect(t, out, "ok    *.localhost resolves")
}

func TestHealthNeedsAnOkBody(t *testing.T) {
	root := healthy(t)
	p := allUp()
	p.get = func(string) (int, string, error) { return 503, "db down", nil }
	out, _ := doctor(t, root, p)
	expect(t, out, "warn  backend /health not ok (db down)")
}

func TestVersionParsing(t *testing.T) {
	for in, want := range map[string]string{
		"git version 2.39.3 (Apple Git-145)": "2.39.3",
		"go version go1.27.3 darwin/arm64":   "1.27.3",
		"v24.1.0":                            "24.1.0",
	} {
		if got := dotted(version(in)); got != want {
			t.Errorf("version(%q) = %s, want %s", in, got, want)
		}
	}
	if !older(version("2.34.9"), minGit) || older(version("2.35.0"), minGit) || older(version("10.0"), minGit) {
		t.Error("older compares numbers wrong")
	}
}

func TestOutsideAProjectOnlyTheMachineIsChecked(t *testing.T) {
	healthy(t)
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	if code := machine(&out); code != 0 {
		t.Errorf("exit %d:\n%s", code, out.String())
	}
	var sections []string
	for line := range strings.Lines(out.String()) {
		if title, ok := strings.CutPrefix(line, "==> "); ok {
			sections = append(sections, strings.TrimSpace(title))
		}
	}
	if !slices.Equal(sections, []string{"docker", "tools", "disk"}) {
		t.Errorf("sections %v", sections)
	}
	expect(t, out.String(), "ok    go 1.27.3\n")
}

// The skelet on the PATH: an installed one is fine, none or a link to a checkout's ./skelet - which goes away - is
// worth a word, a dangling link included
func TestSkeletOnThePath(t *testing.T) {
	checkout := t.TempDir()
	put(t, checkout, "skelet", "#!/usr/bin/env bash\n")
	put(t, checkout, "cmd/skelet/go.mod", "module x/cmd/skelet\n")
	linked := t.TempDir()
	if err := os.Symlink(filepath.Join(checkout, "skelet"), filepath.Join(linked, "skelet")); err != nil {
		t.Fatal(err)
	}
	dangling := t.TempDir()
	if err := os.Symlink(filepath.Join(checkout, "gone", "skelet"), filepath.Join(dangling, "skelet")); err != nil {
		t.Fatal(err)
	}
	put(t, checkout, "gone/bin/_lib", "")
	_ = os.Remove(filepath.Join(checkout, "gone", "skelet"))
	installed := t.TempDir()
	put(t, installed, "skelet", "binary")
	for dir, want := range map[string]string{
		installed:   "ok    skelet " + filepath.Join(installed, "skelet"),
		linked:      "warn  skelet on the PATH links to " + filepath.Join(checkout, "skelet"),
		dangling:    "warn  skelet on the PATH links to " + filepath.Join(checkout, "gone", "skelet"),
		t.TempDir(): "warn  no skelet on the PATH",
	} {
		t.Setenv("PATH", dir)
		var out strings.Builder
		cli(&report{out: &out})
		if !strings.Contains(out.String(), want) {
			t.Errorf("PATH=%s: %q, want %q", dir, out.String(), want)
		}
	}
}
