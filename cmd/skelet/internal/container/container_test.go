package container

import (
	"slices"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/verbtest"
)

func TestMain(m *testing.M) {
	verbtest.Main(m, map[string]func(string, []string) int{"sh": Shell, "cli": CLI, "provider": Provider, "go": Go})
}

const appRunning = `[ "$*" = "compose ps --services --status running" ] && echo app; exit 0`

// exec when the app is up, a one-off run when it isn't - with the locale pinned, and no -it, since stdin here isn't a
// terminal. provider's arguments are a published contract and pass through untouched
func TestAppVerbs(t *testing.T) {
	user := "-e LC_ALL=C.UTF-8 -e LANG=C.UTF-8"
	for _, c := range []struct {
		verb, answer string
		args         []string
		want         string
	}{
		{"sh", appRunning, []string{"-c", "true"}, "compose exec " + user + " app bash -c true"},
		{"sh", "exit 0", nil, "compose run --rm " + user + " app bash"},
		{"cli", appRunning, []string{"admin", "list"}, "compose exec " + user + " app go run ./cmd/cli admin list"},
		{"provider", "exit 0", []string{"--url", "https://x", "--kind", "tax_provider"}, "compose run --rm " + user + " app go run ./cmd/providercheck --url https://x --kind tax_provider"},
		{"go", appRunning, []string{"test", "./..."}, "compose exec " + user + " -w /var/www/backend/core app go test ./..."},
		{"go", appRunning, []string{"--signals", "test", "./..."}, "compose exec " + user + " -w /var/www/backend/signals app go test ./..."},
		{"go", "exit 0", []string{"--sdk", "vet"}, "compose run --rm " + user + " -w /var/www/backend/sdk app go vet"},
		{"go", appRunning, nil, "compose exec " + user + " -w /var/www/backend/core app go"},
	} {
		d := verbtest.NewDocker(t, c.answer)
		_, code := d.Run(c.verb, t.TempDir(), c.args...)
		calls := d.Calls()
		last := ""
		if len(calls) > 0 {
			last, _, _ = strings.Cut(calls[len(calls)-1], "|")
		}
		if code != 0 || last != c.want {
			t.Errorf("%s %q: exit %d, docker got %q\nwant last %q", c.verb, c.args, code, calls, c.want)
		}
	}
}

// A one-off run creates a container, so it carries the uid compose's user: reads; exec doesn't need one
func TestOnlyARunWorksOutTheUID(t *testing.T) {
	t.Setenv("SKELET_UID", "")
	d := verbtest.NewDocker(t, `case "$*" in "compose run"*) echo "uid=$SKELET_UID" >&2;; esac; exit 0`)
	if _, code := d.Run("sh", t.TempDir()); code != 0 || !slices.ContainsFunc(d.Calls(), func(c string) bool { return strings.HasPrefix(c, "info ") }) {
		t.Errorf("exit %d - a run should ask docker info whether the engine is rootless: %q", code, d.Calls())
	}
}
