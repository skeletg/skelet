package route

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeRoot is a checkout with one storefront, enough for the pnpm app lookup
func fakeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "apps", "channels", "vetrina")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestResolveRoutes(t *testing.T) {
	root := fakeRoot(t)
	cases := []struct {
		args   []string
		script string
		want   []string
		notice string
	}{
		{[]string{"up", "--lean"}, "dev", []string{"--lean"}, ""},
		{[]string{"sh"}, "bash", nil, ""},
		{[]string{"logs", "app"}, "logs", []string{"app"}, ""},
		{[]string{"pnpm", "dashboard", "run", "lint"}, "dash", []string{"run", "lint"}, ""},
		{[]string{"pnpm", "vetrina", "run", "dev"}, "channel", []string{"vetrina", "run", "dev"}, ""},
		{[]string{"npm", "dashboard", "x"}, "dash", []string{"x"}, "(skelet npm is skelet pnpm now - running pnpm)"},
		{[]string{"db", "migrate", "up", "--signals"}, "migrate-signals", []string{"up"}, ""},
		{[]string{"db", "migrate", "--signals", "status"}, "migrate-signals", []string{"status"}, ""},
		{[]string{"db", "migrate", "status"}, "migrate", []string{"status"}, ""},
		{[]string{"db", "export", "out.dump"}, "db-export", []string{"out.dump"}, ""},
		{[]string{"db", "shell"}, "psql", nil, ""},
		{[]string{"gen"}, "codegen", nil, ""},
		{[]string{"gen", "--check"}, "codegen", []string{"--check"}, ""},
		{[]string{"gen", "sqlc", "x"}, "sqlc", []string{"x"}, ""},
		{[]string{"check"}, "check", nil, ""},
		{[]string{"check", "codegen", "--no-frontend"}, "codegen", []string{"--check", "--no-frontend"}, ""},
		{[]string{"check", "pins"}, "pin-check", nil, ""},
		{[]string{"test"}, "test", nil, ""},
		{[]string{"test", "--changed"}, "test", []string{"--changed"}, ""},
		{[]string{"test", "e2e", "-run", "X"}, "test-e2e", []string{"-run", "X"}, ""},
		{[]string{"test", "storefront", "vetrina"}, "test-storefront", []string{"vetrina"}, ""},
		{[]string{"add", "channel", "shop"}, "new", []string{"channel", "shop"}, ""},
		{[]string{"new", "channel", "shop"}, "new", []string{"channel", "shop"}, "(skelet new channel is skelet add channel now - running it)"},
		{[]string{"rm", "channel", "shop", "-y"}, "remove", []string{"channel", "shop", "-y"}, ""},
		{[]string{"search", "status"}, "opensearch", []string{"status"}, ""},
		{[]string{"release"}, "push-images", nil, ""},
		{[]string{"provider", "http://x"}, "provider-check", []string{"http://x"}, ""},
	}
	for _, c := range cases {
		a := Resolve(root, c.args)
		if a.Script != c.script || !slices.Equal(a.Args, c.want) || a.Notice != c.notice || a.Code != 0 {
			t.Errorf("%q -> %+v, want bin/%s %q notice %q", c.args, a, c.script, c.want, c.notice)
		}
	}
}

func TestResolveRefusals(t *testing.T) {
	root := fakeRoot(t)
	cases := []struct {
		args   []string
		stderr string
	}{
		{[]string{"pnpm"}, "skelet pnpm <app> <pnpm args> - apps: dashboard, vetrina"},
		{[]string{"pnpm", "nope"}, `skelet pnpm: unknown app "nope" - apps: dashboard, vetrina`},
		{[]string{"db"}, "skelet db migrate [--signals] up|rollback|status|new <name> | export | import | shell"},
		{[]string{"db", "--signals"}, "skelet db migrate [--signals] up|rollback|status|new <name> | export | import | shell"},
		{[]string{"nope"}, `Unknown command "nope" - try: skelet help`},
		{[]string{"new", "my-shop", "--yes"}, newProject},
	}
	for _, c := range cases {
		a := Resolve(root, c.args)
		if a.Script != "" || a.Stderr != c.stderr || a.Code != 2 {
			t.Errorf("%q -> %+v, want refusal %q", c.args, a, c.stderr)
		}
	}
}

func TestResolveUsage(t *testing.T) {
	for _, args := range [][]string{nil, {""}, {"help"}, {"-h"}, {"--help"}, {"help", ""}} {
		if a := Resolve(fakeRoot(t), args); a.Stdout != Usage() || a.Code != 0 || a.Script != "" {
			t.Errorf("%q did not print the usage", args)
		}
	}
}

func TestGoVerbsResolveToTheirHandler(t *testing.T) {
	for _, verb := range []string{"setup", "doctor", "hooks"} {
		if a := Resolve(fakeRoot(t), []string{verb, "--x"}); a.Go == nil || a.Script != "" || !slices.Equal(a.Args, []string{"--x"}) {
			t.Errorf("%s -> %+v", verb, a)
		}
	}
}

// Every path that resolves a route - plain, sub-route, flagged, app - hands a Go verb its handler
func TestGoRoutesSurviveEveryPath(t *testing.T) {
	root := fakeRoot(t)
	saved := slices.Clone(routes)
	t.Cleanup(func() { routes = saved })
	called := ""
	verb := func(name string) func(string, []string) int {
		return func(string, []string) int { called = name; return 0 }
	}
	for i, r := range routes {
		switch {
		case r.Verb == "check" && r.Sub == "pins":
			routes[i] = Route{Verb: "check", Sub: "pins", Go: verb("pins"), About: "x"}
		case r.Verb == "db" && r.Flag == "--signals":
			routes[i] = Route{Verb: "db", Sub: "migrate", Flag: "--signals", Go: verb("signals"), About: "x"}
		case r.Verb == "db" && r.Sub == "export":
			routes[i] = Route{Verb: "db", Sub: "export", Go: verb("export"), About: "x"}
		case r.Verb == "pnpm" && r.Sub == AnyApp:
			routes[i] = Route{Verb: "pnpm", Sub: AnyApp, Go: verb("channel"), About: "x"}
		}
	}
	for args, want := range map[string]string{
		"check pins": "pins", "db migrate --signals status": "signals", "db export f": "export", "pnpm vetrina i": "channel",
	} {
		called = ""
		a := Resolve(root, strings.Fields(args))
		if a.Go == nil {
			t.Errorf("%q resolved to %+v - the Go handler was dropped", args, a)
			continue
		}
		a.Go(root, a.Args)
		if called != want {
			t.Errorf("%q ran %q, want %q", args, called, want)
		}
	}
}
