package testrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/verbtest"
)

func TestE2E(t *testing.T) {
	root := t.TempDir()
	d := verbtest.NewDocker(t, stackUp)
	if _, code := d.Run("e2e", root, "-run", "X"); code != 0 {
		t.Fatalf("exit %d:\n%s", code, strings.Join(cmds(d), "\n"))
	}
	calls := cmds(d)
	if missing := inOrder(calls, []string{
		"DROP DATABASE skelet_e2e WITH (FORCE)",
		"ALTER DATABASE skelet_e2e SET skelet.e2e = 'allowed'",
		"DROP DATABASE skelet_e2e_signals WITH (FORCE)",
		"DROP DATABASE IF EXISTS skelet_e2e_signals",
		"MODIFY COMMENT 'skelet.e2e=allowed'",
		"ALTER DATABASE skelet RESET skelet.e2e",
		"--url postgres://skelet:skelet@db:5432/skelet_e2e?sslmode=disable",
		"--url postgres://skelet:skelet@signals-db:5432/skelet_e2e_signals?sslmode=disable",
		"--url clickhouse://skelet:skelet@clickhouse:9000/skelet_e2e_signals",
		"-e DATABASE_URL=postgres://skelet:skelet@db:5432/skelet_e2e?sslmode=disable app go run ./cmd/cli plugin migrate up",
		"-w /var/www/backend/signals app go build -o /tmp/skelet-e2e-signals ./cmd/signals",
		"exec -T -d", "SIGNALS_GRPC_PORT=18092 -e SIGNALS_PORT=18090",
		"curl -sf http://localhost:18090/healthz",
		"-e SKELET_TESTDB_SHARED=1 -e DATABASE_URL=postgres://skelet:skelet@db:5432/skelet_e2e?sslmode=disable",
		"E2E_SIGNALS_GRPC_ADDR=127.0.0.1:18092",
		"app go test ./internal/e2e/... ./plugins/... -count=1 -timeout 40m -run X",
		"-w /var/www/backend/signals -e SKELET_TESTDB_SHARED=1",
		"app go test ./internal/e2e/... -count=1 -timeout 40m -run X",
		"pkill -f /tmp/skelet-e2e-signals",
	}); missing != "" {
		t.Errorf("no %q in order in:\n%s", missing, strings.Join(calls, "\n"))
	}
	// Core's tests and the Signals process share one contract secret
	if strings.Count(strings.Join(calls, "\n"), "SIGNALS_SERVICE_SECRET=e2e-service-secret") != 2 && os.Getenv("SIGNALS_SERVICE_SECRET") == "" {
		t.Errorf("the secret isn't shared:\n%s", strings.Join(calls, "\n"))
	}
	if anyContains(calls, "skelet_test") || anyContains(calls, "DROP DATABASE skelet ") {
		t.Errorf("e2e touched another suite's or the dev databases:\n%s", strings.Join(calls, "\n"))
	}
	if _, err := os.Stat(lockPath("test-e2e", root)); !os.IsNotExist(err) {
		t.Error("the run left its lock behind")
	}

	keep := verbtest.NewDocker(t, stackUp)
	if _, code := keep.Run("e2e", root, "--keep"); code != 0 || anyContains(cmds(keep), "DROP DATABASE") {
		t.Errorf("--keep: exit %d:\n%s", code, strings.Join(cmds(keep), "\n"))
	}
}

func TestE2ERefusals(t *testing.T) {
	// ClickHouse is the spine the suite runs on - under --lean it refuses before touching a database
	lean := verbtest.NewDocker(t, `case "$*" in "compose ps --services --status running") echo app ;; esac`)
	if _, code := lean.Run("e2e", t.TempDir()); code != 1 || anyContains(cmds(lean), "DATABASE") {
		t.Errorf("without clickhouse: exit %d, calls %q", code, lean.Calls())
	}
	down := verbtest.NewDocker(t, "")
	if _, code := down.Run("e2e", t.TempDir()); code != 1 || len(down.Calls()) != 1 {
		t.Errorf("stack down: exit %d, calls %q", code, down.Calls())
	}

	// A Signals process that never answers: its log, a failure, and no suite - and it's still stopped
	sick := verbtest.NewDocker(t, stackUp+"\ncase \"$*\" in *curl*) exit 7 ;; esac")
	if _, code := sick.Run("e2e", t.TempDir()); code != 1 || anyContains(cmds(sick), "go test") ||
		!anyContains(cmds(sick), "cat /tmp/skelet-e2e-signals.log") || !strings.Contains(cmds(sick)[len(cmds(sick))-1], "pkill") {
		t.Errorf("unhealthy: exit %d:\n%s", code, strings.Join(cmds(sick), "\n"))
	}

	// Core's suite failing ends the run with its code, before the Signals module's
	failing := verbtest.NewDocker(t, stackUp+"\ncase \"$*\" in *\"go test ./internal/e2e/... ./plugins/...\"*) exit 5 ;; esac")
	if _, code := failing.Run("e2e", t.TempDir()); code != 5 || anyContains(cmds(failing), "-w /var/www/backend/signals -e SKELET_TESTDB_SHARED") {
		t.Errorf("core failing: exit %d:\n%s", code, strings.Join(cmds(failing), "\n"))
	}
}

func storefrontTree(t *testing.T, apps map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, manifest := range apps {
		dir := filepath.Join(root, "apps", "channels", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if manifest == "" {
			continue
		}
		for file, body := range map[string]string{"playwright.config.ts": "", "package.json": manifest} {
			if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

const pinned = `{"devDependencies": {"@playwright/test": "1.56.1"}}`

const storefrontUp = `case "$*" in
"compose ps -q shop") echo abc123 ;;
"inspect -f {{.State.Running}} abc123") echo true ;;
*"printenv ADMIN_API_SLUG") echo s3cret ;;
"inspect -f {{range .Config.Env}}{{println .}}{{end}} abc123") printf 'A=1\nSTOREFRONT_CHANNEL_ID=chan-1\n' ;;
"inspect -f {{json .Mounts}} abc123") echo '[{"Type":"tmpfs","Source":"","Destination":"/var/www/skelet/env","RW":true},{"Type":"volume","Name":"skelet_shop-node-modules","Destination":"/var/www/skelet/node_modules","RW":true},{"Type":"bind","Source":"/src","Destination":"/var/www/skelet","RW":true},{"Type":"bind","Source":"/certs","Destination":"/certs","RW":false}]' ;;
esac`

func TestStorefront(t *testing.T) {
	root := storefrontTree(t, map[string]string{"shop": pinned, "plain": ""})
	d := verbtest.NewDocker(t, storefrontUp)
	// The one storefront with browser tests needs no name, and an argument that isn't one goes to Playwright
	if _, code := d.Run("storefront", root, "--grep", "cart"); code != 0 {
		t.Fatalf("exit %d:\n%s", code, strings.Join(cmds(d), "\n"))
	}
	calls := cmds(d)
	run := calls[len(calls)-1]
	if missing := inOrder(calls, []string{
		"app go run ./cmd/cli admin create-superuser --email e2e-storefront-",
		"--first-name E2E --last-name Storefront",
		"run --rm",
		"-e E2E_ADMIN_GRAPHQL=http://app:8080/api/admin/s3cret/graphql",
		"-e E2E_CHANNEL_ID=chan-1",
		// The container's mounts as they are - the tmpfs over env/ included, which --volumes-from can't copy
		"--network container:abc123 --tmpfs /var/www/skelet/env -v skelet_shop-node-modules:/var/www/skelet/node_modules -v /src:/var/www/skelet -v /certs:/certs:ro --ipc=host -w /var/www/skelet/apps/channels/shop",
		"mcr.microsoft.com/playwright:v1.56.1-noble node node_modules/@playwright/test/cli.js test --grep cart",
	}); missing != "" || !strings.HasPrefix(run, "run --rm") {
		t.Errorf("no %q in order in:\n%s", missing, strings.Join(calls, "\n"))
	}
	// The superuser it made is the one the specs sign in as
	for _, c := range calls {
		if strings.Contains(c, "create-superuser") {
			fields := strings.Fields(c)
			for i, f := range fields {
				if f == "--email" && !strings.Contains(run, "E2E_ADMIN_EMAIL="+fields[i+1]) {
					t.Errorf("the run signs in as someone else than %s", fields[i+1])
				}
				if f == "--password" && !strings.Contains(run, "E2E_ADMIN_PASSWORD="+fields[i+1]) {
					t.Error("the run has another password")
				}
			}
		}
	}
	// Named explicitly, and without an admin slug the API sits at its plain path
	named := verbtest.NewDocker(t, strings.Replace(storefrontUp, "echo s3cret", "true", 1))
	if _, code := named.Run("storefront", root, "shop"); code != 0 || !anyContains(cmds(named), "E2E_ADMIN_GRAPHQL=http://app:8080/api/admin/graphql ") {
		t.Errorf("named: exit %d:\n%s", code, strings.Join(cmds(named), "\n"))
	}
}

func TestStorefrontRefusals(t *testing.T) {
	cases := []struct {
		name string
		apps map[string]string
		args []string
		code int
	}{
		{"two with tests and no name", map[string]string{"a": pinned, "b": pinned}, nil, 2},
		{"none with tests", map[string]string{"plain": ""}, nil, 1},
		{"named without tests", map[string]string{"shop": pinned, "plain": ""}, []string{"plain"}, 1},
		{"no exact pin", map[string]string{"shop": `{"devDependencies": {"@playwright/test": "^1.56.1"}}`}, nil, 1},
		{"not running", map[string]string{"other": pinned}, nil, 1},
	}
	// A container that exists but has stopped, and one whose mounts can't be read
	stopped := verbtest.NewDocker(t, strings.Replace(storefrontUp, "echo true", "echo false", 1))
	if _, code := stopped.Run("storefront", storefrontTree(t, map[string]string{"shop": pinned})); code != 1 || anyContains(cmds(stopped), "create-superuser") {
		t.Errorf("stopped: exit %d, calls %q", code, stopped.Calls())
	}
	blind := verbtest.NewDocker(t, strings.Replace(storefrontUp, `"inspect -f {{json .Mounts}} abc123")`, `"inspect -f {{json .Mounts}} abc123") exit 1 ;; "never")`, 1))
	if _, code := blind.Run("storefront", storefrontTree(t, map[string]string{"shop": pinned})); code != 1 || anyContains(cmds(blind), "create-superuser") {
		t.Errorf("unreadable mounts: exit %d, calls %q", code, blind.Calls())
	}
	for _, c := range cases {
		d := verbtest.NewDocker(t, storefrontUp)
		if _, code := d.Run("storefront", storefrontTree(t, c.apps), c.args...); code != c.code || anyContains(cmds(d), "create-superuser") {
			t.Errorf("%s: exit %d, want %d; calls %q", c.name, code, c.code, d.Calls())
		}
	}
}
