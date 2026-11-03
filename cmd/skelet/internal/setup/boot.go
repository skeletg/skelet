package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/container"
	"github.com/skeletg/skelet/cmd/skelet/internal/gen"
	"github.com/skeletg/skelet/cmd/skelet/internal/node"
	"github.com/skeletg/skelet/cmd/skelet/internal/stack"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

// boot is the dev target's second half: the stack up, the schema in place, the plugin bridge regenerated when the
// set changed, the host apps installed, and the first admin login offered while you're still at the prompt
func boot(root string, a answers, w written, ask prompter, st *stepper) int {
	if code := st.run("Starting containers", func() int {
		fmt.Println("==> Starting containers...")
		// --lean: migrating needs only db and app, and the sidecars arrive with skelet up
		return stack.Up(root, []string{"--lean"})
	}); code != 0 {
		return code
	}
	lib := compose.Load(root)
	dc := func(args ...string) *exec.Cmd { return compose.Command(root, lib.Env(), args...) }

	if code := st.run("Waiting for the database", func() int {
		fmt.Println("==> Waiting for db...")
		wait := dc("exec", "db", "sh", "-c", "until pg_isready -U $POSTGRES_USER -d $POSTGRES_DB; do sleep 1; done")
		wait.Stdout, wait.Stderr = os.Stdout, os.Stderr
		return sys.ExitCode(sys.Run(wait))
	}); code != 0 {
		return code
	}

	// The app migrates as it boots - this waits for the same work under the same locks, so what follows finds the
	// schema in place
	if code := st.run("Running migrations", func() int {
		fmt.Println("==> Running migrations...")
		return sys.ExitCode(container.CLICall(root, "migrate").Run())
	}); code != 0 {
		return code
	}

	if a.signals {
		st.run("Waiting for the ClickHouse spine schema", func() int { return spine(dc) })
	}

	if w.pluginsChanged {
		st.run("Regenerating the plugin bridge", func() int {
			fmt.Println("==> Regenerating the plugin GraphQL delegation (skelet gen)...")
			if gen.Run(root, nil) != 0 {
				fmt.Println("    codegen failed - run skelet gen once the stack is warm")
				return 1
			}
			return 0
		})
	}

	for _, app := range a.apps {
		docker := slices.Contains(a.dockerApps, app)
		if docker && ask.ui != nil {
			ask.ui.Note(app + " runs in Docker and installs its own dependencies on start - skelet logs " + app)
			continue
		}
		if code := st.run("Installing "+app+" dependencies", func() int { return install(root, app, docker) }); code != 0 {
			return code
		}
	}

	// Defaults to no once a superuser exists, so a re-run doesn't push a second one
	if ask.on {
		count := dc("exec", "-T", "db", "sh", "-c", `psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "SELECT count(*) FROM admin_users WHERE is_superuser"`)
		raw, _ := count.Output()
		q, def := "Create your first admin login now?", true
		if n := strings.TrimSpace(string(raw)); n != "" && n != "0" {
			q, def = "A superuser already exists - create another?", false
		}
		if ask.yesNo("==> "+q, def) {
			if err := container.CLICall(root, "admin", "create-superuser").Run(); err != nil {
				fmt.Fprintln(os.Stderr, "    create-superuser failed - run it again with skelet cli admin create-superuser")
			}
		}
	}

	summary(a, w, dc, ask)
	return 0
}

// spine waits for the Signals process to migrate ClickHouse, so the first beacon has a table to land in
func spine(dc func(...string) *exec.Cmd) int {
	fmt.Println("==> Waiting for the ClickHouse spine schema...")
	user, pass := envOr("CLICKHOUSE_USER", "skelet"), envOr("CLICKHOUSE_PASSWORD", "skelet")
	for range 60 {
		out, _ := dc("exec", "-T", "clickhouse", "clickhouse-client", "--user="+user, "--password="+pass,
			"--query", "EXISTS TABLE signals.analytics_events").Output()
		if strings.TrimSpace(string(out)) == "1" {
			fmt.Println("    signals.analytics_events present")
			return 0
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Fprintln(os.Stderr, "    NOT ready after 2 minutes - the spine will write nowhere until this lands")
	fmt.Fprintln(os.Stderr, "    look at: skelet logs signals")
	return 1
}

// install puts a host app's dependencies in place. A Docker app's image installs on every start, into a volume the
// host can't see, so the host tree says nothing about it
func install(root, app string, inDocker bool) int {
	if inDocker {
		fmt.Printf("==> %s runs in Docker and installs its own dependencies on start - follow it with skelet logs %s\n", app, app)
		return 0
	}
	dir := filepath.Join(root, appDir(root, app))
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		return 0
	}
	fmt.Printf("==> Installing %s dependencies...\n", app)
	switch node.Tree(dir) {
	case "pnpm":
		fmt.Printf("    %s/node_modules is populated, skipping\n", appDir(root, app))
		return 0
	case "npm":
		fmt.Printf("    %s/node_modules is an npm install from before the switch to pnpm - replacing it\n", appDir(root, app))
		if err := os.RemoveAll(filepath.Join(dir, "node_modules")); err != nil {
			fmt.Fprintln(os.Stderr, "   ", err)
			return 1
		}
	}
	cmd := exec.Command("pnpm", "install", "--frozen-lockfile")
	cmd.Dir, cmd.Env = dir, sys.Env()
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return sys.ExitCode(sys.Run(cmd))
}

func summary(a answers, w written, dc func(...string) *exec.Cmd, ask prompter) {
	ask.section(os.Stdout, "Setup complete.")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  skelet up               # dev stack with hot reload + sidecars")
	fmt.Println("  skelet doctor           # check the shape you just picked")
	fmt.Println("  skelet hooks            # opt-in: pre-commit keeps generated code fresh")
	fmt.Println("  skelet cli admin create-superuser   # an admin login, if you skipped it above")
	fmt.Println()
	fmt.Println("Once skelet up is running:")
	if a.adminSlug {
		fmt.Printf("  Admin GraphQL           http://localhost:8080/api/admin/%s/graphql\n", w.adminSlug)
	} else {
		fmt.Println("  Backend playgrounds     http://localhost:8080/playground/{admin,front}")
	}
	for _, app := range a.apps {
		if app == "dashboard" {
			fmt.Println("  Dashboard               http://localhost:5173")
			continue
		}
		// at whatever port its own compose service publishes
		port := "3000"
		if out, err := dc("port", app, "3000").Output(); err == nil {
			if s := strings.TrimSpace(string(out)); s != "" {
				port = s[strings.LastIndex(s, ":")+1:]
			}
		}
		fmt.Printf("  %-23s http://localhost:%s\n", "Storefront "+app, port)
	}
	if host := a.hostApps(); len(host) > 0 {
		fmt.Println()
		fmt.Println("These run on the host, so skelet up does not start them:")
		for _, app := range host {
			fmt.Printf("  skelet serve %s\n", app)
		}
	}
	urls := map[string]string{
		"mailpit":        "Mailpit (caught email)  http://localhost:8025",
		"webhook-tester": "Webhook tester          http://localhost:8081",
		"grafana":        "Grafana                 http://localhost:3030  (admin / admin)",
		"garage-webui":   "Garage web UI           http://localhost:9901",
	}
	for _, t := range a.tools {
		fmt.Println("  " + urls[t])
	}
	if len(a.apps) == 0 {
		fmt.Println()
		fmt.Println("Backend-only clone: point your own frontend at the GraphQL APIs above.")
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
