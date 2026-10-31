// Package gen is skelet gen: every generator in order - the Signals contract, the plugin set, sqlc, gqlgen, the plugin
// GraphQL bridge, the published schemas and doc tables, the edge security headers, and the frontends' typed queries
package gen

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/legacy"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const About = `Regenerates everything: the Signals gRPC contract (buf), the plugin set, sqlc, both gqlgen schemas, the
plugin GraphQL bridge, the remote-provider contract, the docs reference tables and the edge security
headers - then the dashboard's and every storefront's typed queries, which need the backend up.

  --check        fail if anything was stale, listing what regeneration changed
  --no-frontend  stop before the two steps that need a live backend - the half CI runs

skelet check codegen is gen --check.`

// What --check compares - files mixing hand-written code (*.resolvers.go, *.gql.ts) stay out
var generated = []string{
	"apps/backend/core/plugins/firstparty/",
	"apps/backend/core/graph/admin/plugin_resolvers.go",
	"apps/backend/core/graph/front/plugin_resolvers.go",
	"apps/backend/core/internal/db/sqlc/",
	"apps/backend/signals/internal/db/sqlc/",
	"apps/backend/core/signalsapi/v1/",
	"apps/backend/core/graph/admin/generated.go",
	"apps/backend/core/graph/admin/model/",
	"apps/backend/core/graph/front/generated.go",
	"apps/backend/core/graph/front/model/",
	"apps/backend/core/internal/remoteprovider/contract/",
	"apps/dashboard/nginx.conf.template",
	"deploy/caddy/Caddyfile",
	"docs/permissions.md",
	"docs/webhooks.md",
	"docs/settings.md",
	"apps/dashboard/src/graphql/schema.types.ts",
	"apps/dashboard/src/",
	// Every storefront, not the demo by name - a broad pathspec costs nothing and can't go stale
	"apps/channels/",
}

// Check is skelet check codegen
func Check(root string, args []string) int { return Run(root, append([]string{"--check"}, args...)) }

func Run(root string, args []string) int {
	check, frontend := false, true
	for _, a := range args {
		switch a {
		case "--check":
			check = true
		case "--no-frontend":
			frontend = false
		default:
			fmt.Printf("Unknown flag: %s (try --check, --no-frontend)\n", a)
			return 2
		}
	}
	sys.HandleInterrupts()
	var before map[string]string
	if check {
		before = snapshot(root)
	}
	if code := backend(root); code != 0 {
		return code
	}
	if frontend {
		if code := typescript(root, check); code != 0 {
			return code
		}
	}
	suffix := ""
	if !frontend {
		suffix = " (backend-side only, --no-frontend)"
	}
	if !check {
		fmt.Printf("==> codegen complete%s\n", suffix)
		return 0
	}
	if stale := changed(before, snapshot(root)); len(stale) > 0 {
		fmt.Println("==> codegen check: STALE - generated files changed:")
		for _, f := range stale {
			fmt.Println(f)
		}
		fmt.Println("\nRun `skelet gen`, stage the regenerated files, and commit again.")
		return 1
	}
	fmt.Printf("==> codegen check: clean%s\n", suffix)
	return 0
}

func backend(root string) int {
	steps := []struct {
		title string
		run   func() error
	}{
		// The core-to-Signals wire contract, before anything that might use its types
		{"buf (signals grpc contract)", func() error {
			if err := legacy.Run(root, "buf", "lint", "/var/www/proto"); err != nil {
				return err
			}
			return legacy.Run(root, "buf", "generate", "--template", "/var/www/proto/buf.gen.yaml", "/var/www/proto")
		}},
		// Before gqlgen: plugins.json decides which plugins compile in
		{"plugin set", func() error { return goRun(root, "plugingen") }},
		// sqlc writes its outputs but never removes one, so a deleted query would leave its Go behind where --check
		// can't see it - its own files go first
		{"sqlc generate", func() error {
			if err := clearSqlc(root); err != nil {
				return err
			}
			return legacy.Run(root, "sqlc", "generate")
		}},
		// Before gqlgen: a removed or disabled plugin leaves delegation files referencing model types its SDL took
		// with it, and gqlgen type-checks the graph package first. plugingraph only parses, so it runs on the stale tree
		{"plugin graph (prune)", func() error { return goRun(root, "plugingraph") }},
		{"gqlgen admin", func() error { return legacy.Run(root, "gqlgen", "generate", "--config", "gqlgen.admin.yml") }},
		{"gqlgen front", func() error { return legacy.Run(root, "gqlgen", "generate", "--config", "gqlgen.front.yml") }},
		// gqlgen stubs each plugin field; this turns the stubs into delegation through the runtime registry, so a plugin
		// author writes SDL and a resolver in their own package and touches no core Go
		{"plugin graph", func() error { return goRun(root, "plugingraph") }},
		// Read out of the adapter structs themselves, so a renamed payload type fails here, not in an integrator's build
		{"remote provider contract", func() error { return goRun(root, "providerschema") }},
		// Permissions, webhook events and settings, read back out of the registries after every init() has run
		{"docs reference tables", func() error { return goRun(root, "docsref") }},
		// The nginx and Caddy copies of the CSP and Permissions-Policy, from lib/httpx/securityheaders.go
		{"security headers", func() error { return goRun(root, "secheadgen") }},
	}
	for _, s := range steps {
		fmt.Printf("==> %s\n", s.title)
		if err := s.run(); err != nil {
			return max(sys.ExitCode(err), 1)
		}
	}
	return 0
}

// goRun runs one of core's generators on the host Go
func goRun(root, generator string) error {
	cmd := exec.Command("go", "run", "./cmd/"+generator)
	cmd.Dir = filepath.Join(root, "apps", "backend", "core")
	cmd.Env = append(sys.Env(), "LC_ALL=C.UTF-8", "LANG=C.UTF-8")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return sys.Run(cmd)
}

var sqlcHeader = regexp.MustCompile(`(?m)^// Code generated by sqlc\. DO NOT EDIT\.`)

func clearSqlc(root string) error {
	for _, dir := range []string{"apps/backend/core/internal/db/sqlc", "apps/backend/signals/internal/db/sqlc"} {
		files, _ := filepath.Glob(filepath.Join(root, dir, "*.go"))
		for _, f := range files {
			if raw, err := os.ReadFile(f); err == nil && sqlcHeader.Match(raw) {
				if err := os.Remove(f); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// typescript regenerates the frontends' typed queries, which introspect the running backend
func typescript(root string, check bool) int {
	fmt.Println("==> dashboard codegen")
	// The real GraphQL endpoint, not /health, which says ok before gqlgen's handlers rewire after an Air rebuild -
	// and ADMIN_API_SLUG moves that endpoint, so the probe follows it
	probe := "http://localhost:8080/api/admin/graphql"
	if v, ok, _ := envfile.Get(filepath.Join(root, ".env"), "ADMIN_API_SLUG"); ok && strings.TrimSpace(v) != "" {
		probe = "http://localhost:8080/api/admin/" + strings.TrimSpace(v) + "/graphql"
	}
	if !ready(probe) {
		fmt.Printf("    The backend's admin GraphQL isn't answering at %s after 30s.\n", probe)
		fmt.Println("    Start it with `skelet up` and re-run `skelet gen`.")
		if check {
			// stale TS bindings are exactly what --check is for
			return 1
		}
		fmt.Println("    Skipping dashboard + storefront codegen.")
		return 0
	}
	// Keyed on the source being here, not on .skelet-apps - an app run on the host still wants its bindings fresh
	if _, err := os.Stat(filepath.Join(root, "apps", "dashboard", "package.json")); err == nil {
		url := os.Getenv("GRAPHQL_CODEGEN_URL")
		if url == "" {
			url = probe
		}
		if code := retried("dashboard", probe, func() error {
			cmd := legacy.Command(root, "dash", "run", "codegen")
			cmd.Env = append(cmd.Env, "GRAPHQL_CODEGEN_URL="+url)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			return sys.Run(cmd)
		}); code != 0 {
			return code
		}
	} else {
		fmt.Println("    (skipped - dashboard source not in this checkout; see skelet setup)")
	}
	fmt.Println("==> storefront codegen")
	manifests, _ := filepath.Glob(filepath.Join(root, "apps", "channels", "*", "package.json"))
	slices.Sort(manifests)
	if len(manifests) == 0 {
		fmt.Println("    (no storefronts in this checkout; see skelet setup)")
	}
	for _, m := range manifests {
		name := filepath.Base(filepath.Dir(m))
		if code := retried(name, probe, func() error { return legacy.Run(root, "channel", name, "run", "codegen") }); code != 0 {
			return code
		}
	}
	return 0
}

// retried runs a TS codegen step again once after a failure - Air sometimes queues a second rebuild mid-run
func retried(label, probe string, run func() error) int {
	if run() == nil {
		return 0
	}
	fmt.Printf("    %s codegen hit a backend restart; re-probing and retrying once...\n", label)
	ready(probe)
	if err := run(); err != nil {
		return max(sys.ExitCode(err), 1)
	}
	return 0
}

// ready probes the admin GraphQL endpoint for up to 30 seconds
func ready(url string) bool {
	client := &http.Client{Timeout: time.Second}
	for range 30 {
		resp, err := client.Post(url, "application/json", strings.NewReader(`{"query":"{__typename}"}`))
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 400 {
				return true
			}
		}
		time.Sleep(time.Second)
	}
	return false
}

// snapshot is every generated file that differs from the index or isn't tracked at all, by its content - so a run
// that writes a new file counts, and one that only touches files already dirty before it doesn't
func snapshot(root string) map[string]string {
	git := func(args ...string) []string {
		cmd := exec.Command("git", append(args, append([]string{"--"}, generated...)...)...)
		cmd.Dir = root
		out, _ := cmd.Output()
		return strings.Fields(string(out))
	}
	state := map[string]string{}
	for _, f := range append(git("diff", "--name-only"), git("ls-files", "--others", "--exclude-standard")...) {
		raw, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			state[f] = "deleted"
			continue
		}
		sum := sha256.Sum256(raw)
		state[f] = hex.EncodeToString(sum[:])
	}
	return state
}

// changed is every path whose state differs between the two snapshots, sorted
func changed(before, after map[string]string) []string {
	var out []string
	for f, v := range after {
		if before[f] != v {
			out = append(out, f)
		}
	}
	for f := range before {
		if _, ok := after[f]; !ok {
			out = append(out, f)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
