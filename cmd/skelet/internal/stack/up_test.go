package stack

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/verbtest"
)

func put(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// devClone is a checkout set up for the dev target, with a dashboard and one storefront on the host
func devClone(t *testing.T) string {
	root := t.TempDir()
	put(t, root, map[string]string{
		"env/go.env":                      "SIGNALS_SERVICE_SECRET=first\nEMAIL_LINK_SECRET=els\nSIGNALS_SERVICE_SECRET=svc\n",
		"env/db.env":                      "POSTGRES_USER=skelet\n",
		"env/signals.env.example":         "JWT_SECRET=old\nSIGNALS_ENCRYPTION_KEY=\nSIGNALS_PORT=8090\n",
		"apps/dashboard/package.json":     "{}",
		"apps/channels/shop/package.json": "{}",
	})
	return root
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func config(port int) string {
	return `"compose config --format json") echo '{"services":{"db":{"ports":[{"target":5432,"published":"` + strconv.Itoa(port) + `"}]}}}' ;;`
}

func unset(t *testing.T, names ...string) {
	for _, n := range names {
		t.Setenv(n, "")
		_ = os.Unsetenv(n)
	}
}

func TestUp(t *testing.T) {
	unset(t, "COMPOSE_PROFILES", "SKELET_SIDECARS", "SKELET_TOOLS", "BUILDX_NO_DEFAULT_ATTESTATIONS")
	root := devClone(t)
	d := verbtest.NewDocker(t, `case "$*" in
`+config(freePort(t))+`
*"ps --services --status running") echo dashboard ;;
"compose up"*) echo "attestations off: $BUILDX_NO_DEFAULT_ATTESTATIONS" ;;
esac`)
	out, code := d.Run("up", root)
	calls := strings.Join(d.Calls(), "\n")
	if code != 0 || !strings.Contains(calls, "compose up -d --build|search,signals") {
		t.Fatalf("exit %d:\n%s", code, calls)
	}
	// Without an attestation an unchanged image keeps its ID, so up recreates nothing
	if !strings.Contains(out, "attestations off: 1\n") {
		t.Errorf("up builds with the default attestation:\n%s", out)
	}
	t.Setenv("BUILDX_NO_DEFAULT_ATTESTATIONS", "0")
	if out, _ := d.Run("up", root); !strings.Contains(out, "attestations off: 0\n") {
		t.Errorf("your own setting was overridden:\n%s", out)
	}
	unset(t, "BUILDX_NO_DEFAULT_ATTESTATIONS")
	// The frontends run on the host: how to start them, and the dashboard's leftover container stopped
	for _, want := range []string{"skelet serve dashboard", "skelet serve shop", "stopping the dashboard container"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(calls, "compose --profile dashboard --profile shop ps --services --status running") ||
		!strings.Contains(calls, "compose --profile dashboard stop dashboard") || strings.Contains(calls, "stop shop") {
		t.Errorf("leftovers:\n%s", calls)
	}
	// env/signals.env from its template: the retired secret gone, its own key made, core's shared values copied as
	// compose reads them - the last line of a key that's there twice
	sig := read(t, filepath.Join(root, "env/signals.env"))
	if strings.Contains(sig, "JWT_SECRET") || !strings.Contains(sig, "SIGNALS_SERVICE_SECRET=svc\n") ||
		!strings.Contains(sig, "EMAIL_LINK_SECRET=els\n") || strings.Contains(sig, "SIGNALS_ENCRYPTION_KEY=\n") {
		t.Errorf("env/signals.env:\n%s", sig)
	}
	if fi, _ := os.Stat(filepath.Join(root, "env/signals.env")); fi.Mode().Perm() != 0o600 {
		t.Errorf("env/signals.env is %v, holds secrets", fi.Mode().Perm())
	}
	// Naming a service brings up just that, and says nothing about the frontends
	if out, _ := d.Run("up", root, "mailpit"); strings.Contains(out, "skelet serve") {
		t.Errorf("up mailpit talked about the frontends:\n%s", out)
	}
	// Run again, it's the same file - the generated key stays
	if _, code := d.Run("up", root); code != 0 || read(t, filepath.Join(root, "env/signals.env")) != sig {
		t.Errorf("a re-run changed env/signals.env")
	}
}

// --lean drops the sidecars and tools, but apps in Docker still start - and with apps in Docker nothing is said about
// the host or stopped
func TestUpLean(t *testing.T) {
	unset(t, "COMPOSE_PROFILES", "SKELET_SIDECARS", "SKELET_TOOLS")
	root := devClone(t)
	put(t, root, map[string]string{".skelet-apps": "dashboard\n", ".skelet-tools": "mailpit\n"})
	d := verbtest.NewDocker(t, `case "$*" in `+config(freePort(t))+` esac`)
	out, code := d.Run("up", root, "--lean")
	calls := strings.Join(d.Calls(), "\n")
	if code != 0 || !strings.Contains(calls, "compose up -d --build|dashboard") || strings.Contains(out, "skelet serve") || strings.Contains(calls, "stop") {
		t.Errorf("exit %d:\n%s\n%s", code, out, calls)
	}
	// The picked tool stays wired, lean or not
	if v, _, _ := envfile.Get(filepath.Join(root, "env/go.env"), "SMTP_HOST"); v != "mailpit" {
		t.Errorf("SMTP_HOST=%q", v)
	}
	// Arguments go to compose up, and naming a service says nothing about the frontends
	if out, _ := d.Run("up", root, "mailpit"); strings.Contains(out, "skelet serve") || !strings.Contains(strings.Join(d.Calls(), "\n"), "compose up -d --build mailpit") {
		t.Errorf("up mailpit:\n%s", strings.Join(d.Calls(), "\n"))
	}
}

func TestUpRefusals(t *testing.T) {
	// No dev env files: nothing is asked of docker
	for _, prof := range []string{"", "SKELET_TARGET=small\n"} {
		root := t.TempDir()
		if prof != "" {
			put(t, root, map[string]string{".skelet-profile": prof})
		}
		d := verbtest.NewDocker(t, "")
		if _, code := d.Run("up", root); code != 1 || len(d.Calls()) != 0 {
			t.Errorf("%q: exit %d, calls %q", prof, code, d.Calls())
		}
	}
	// A port something else holds, with no terminal to ask on - the stack doesn't start
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	held := l.Addr().(*net.TCPAddr).Port
	d := verbtest.NewDocker(t, `case "$*" in `+config(held)+` esac`)
	if _, code := d.Run("up", devClone(t)); code != 1 || strings.Contains(strings.Join(d.Calls(), "\n"), "compose up") {
		t.Errorf("held port: exit %d, calls %q", code, d.Calls())
	}
	// ...unless this project's own container is what holds it
	ours := verbtest.NewDocker(t, `case "$*" in `+config(held)+`
"compose ps --format {{.Ports}}") echo "127.0.0.1:`+strconv.Itoa(held)+`->5432/tcp" ;; esac`)
	if _, code := ours.Run("up", devClone(t)); code != 0 {
		t.Errorf("our own port: exit %d, calls %q", code, ours.Calls())
	}
	// compose failing is up's exit code
	failing := verbtest.NewDocker(t, `case "$*" in "compose up"*) exit 4 ;; esac`)
	if _, code := failing.Run("up", devClone(t)); code != 4 {
		t.Errorf("compose up failing: exit %d", code)
	}
}

func TestSyncToolEnv(t *testing.T) {
	root := t.TempDir()
	put(t, root, map[string]string{
		"env/go.env":      "SMTP_HOST=\nOTEL_EXPORTER_OTLP_ENDPOINT=\n",
		"env/signals.env": "SMTP_HOST=smtp.example.com\nOTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318\n",
	})
	if err := syncToolEnv(root, "webhook-tester,mailpit,grafana"); err != nil {
		t.Fatal(err)
	}
	want := "SMTP_HOST=mailpit\nOTEL_EXPORTER_OTLP_ENDPOINT=http://grafana:4318\nSMTP_PORT=1025\nOTEL_TRACES_EXPORTER=otlp\nOTEL_METRICS_EXPORTER=otlp\n"
	if got := read(t, filepath.Join(root, "env/go.env")); got != want {
		t.Errorf("picked:\n%s\nwant\n%s", got, want)
	}
	// A relay and a collector of your own are yours
	if got := read(t, filepath.Join(root, "env/signals.env")); got != "SMTP_HOST=smtp.example.com\nOTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318\n" {
		t.Errorf("own values touched:\n%s", got)
	}
	// Unpicked, the tools' own values are unwired and nothing else moves
	if err := syncToolEnv(root, "mailpit-ish"); err != nil {
		t.Fatal(err)
	}
	want = "SMTP_HOST=\nOTEL_EXPORTER_OTLP_ENDPOINT=\nSMTP_PORT=1025\nOTEL_TRACES_EXPORTER=\nOTEL_METRICS_EXPORTER=\n"
	if got := read(t, filepath.Join(root, "env/go.env")); got != want {
		t.Errorf("unpicked:\n%s\nwant\n%s", got, want)
	}
	if got := read(t, filepath.Join(root, "env/signals.env")); !strings.HasPrefix(got, "SMTP_HOST=smtp.example.com\n") {
		t.Errorf("unpicked mailpit cleared a relay of your own:\n%s", got)
	}
}

// fakeDocker puts a docker on this process's PATH that answers config with db and valkey published where .env's
// knobs say - the environment preflight exports as it moves them
func fakeDocker(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
"compose config --format json") printf '{"services":{"db":{"ports":[{"target":5432,"published":"%s"}]},"valkey":{"ports":[{"target":6379,"published":"%s"}]},"app":{"ports":[{"target":8080,"published":"18080"}]},"nats":{"ports":[{"target":4222,"published":"15442"}]}}}' "${DB_HOST_PORT:-15432}" "${VALKEY_HOST_PORT:-16379}" ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	unset(t, "DB_HOST_PORT", "VALKEY_HOST_PORT", "COMPOSE_PROFILES")
}

// busy stubs the port probe: these ports are held, everything else is free
func busy(t *testing.T, ports ...int) {
	t.Helper()
	old := inUse
	t.Cleanup(func() { inUse = old })
	inUse = func(p int) bool { return slices.Contains(ports, p) }
}

func TestPreflightAsks(t *testing.T) {
	fakeDocker(t)
	ask := func(input string) (bool, string) {
		// A move exports its knob, which would carry into the next case
		_ = os.Unsetenv("DB_HOST_PORT")
		_ = os.Unsetenv("VALKEY_HOST_PORT")
		root := t.TempDir()
		ok := preflight(root, compose.Load(root), prompter{in: strings.NewReader(input), on: true})
		env, _ := os.ReadFile(filepath.Join(root, ".env"))
		return ok, string(env)
	}
	busy(t, 15432, 16379)
	// Input ending at a prompt quits instead of asking forever
	if ok, _ := ask(""); ok {
		t.Error("EOF at the first prompt passed")
	}
	if ok, _ := ask("1\n1\n"); ok {
		t.Error("EOF after checking again passed")
	}
	if ok, _ := ask("2\n"); ok {
		t.Error("EOF at the port prompt passed")
	}
	if ok, _ := ask("3\n"); ok {
		t.Error("quit passed")
	}
	// Two services moved in one round can't both get the same port - the second is told it's taken
	ok, env := ask("2\n16000\n2\n16000\n16001\n")
	if !ok || env != "DB_HOST_PORT=16000\nVALKEY_HOST_PORT=16001\n" {
		t.Errorf("two moves: %v\n%s", ok, env)
	}
	// Enter takes the suggestion: free, and not a port something else in the stack publishes - db's first one is nats's,
	// though nothing answers there yet - nor one this round moved to
	busy(t, 15432, 16379)
	ok, env = ask("2\n\n2\nabc\n+16389\n80\n15443\n18080\n\n")
	if !ok || env != "DB_HOST_PORT=15443\nVALKEY_HOST_PORT=16389\n" {
		t.Errorf("suggestions: %v\n%s", ok, env)
	}
	// app's port is part of its URLs: it can only be freed, and 2 quits rather than asking again
	probes := 0
	inUse = func(p int) bool {
		if p != 18080 {
			return false
		}
		probes++
		return probes <= 2
	}
	if ok, _ := ask("2\n1\n"); ok {
		t.Error("quitting over app's port passed")
	}
	busy(t, 18080)
	// Off a terminal it lists and fails without reading anything
	r := strings.NewReader("2\n16000\n")
	if preflight(t.TempDir(), compose.Load(t.TempDir()), prompter{in: r, on: false}) || r.Len() == 0 {
		t.Error("no terminal: passed, or read an answer")
	}
	// Freed while it asked: checking again passes
	probes = 0
	inUse = func(p int) bool {
		if p != 15432 {
			return false
		}
		probes++
		return probes <= 2
	}
	if ok, _ := ask("1\n"); !ok {
		t.Error("a freed port still failed")
	}
}
