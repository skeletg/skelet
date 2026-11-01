package compose

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// What bin/_lib exported for each case, recorded from the script itself with a fake docker before it was retired
var libCases = []struct {
	name                string
	env                 map[string]string
	apps, tools, goEnv  *string
	profiles            string
	wantApps, wantTools string
}{
	{name: "bare", profiles: "search,signals"},
	{name: "selected", apps: new("dashboard,vetrina-demo\n"), tools: new("mailpit,grafana\n"), profiles: "search,dashboard,vetrina-demo,mailpit,grafana,signals", wantApps: "dashboard,vetrina-demo", wantTools: "mailpit,grafana"},
	{name: "lean", env: map[string]string{"SKELET_SIDECARS": "", "SKELET_TOOLS": ""}, apps: new("dashboard\n"), tools: new("mailpit\n"), profiles: "dashboard", wantApps: "dashboard"},
	{name: "signals off", goEnv: new("SIGNALS_ENABLED=false\n"), profiles: "search"},
	{name: "signals off, loosely written", goEnv: new("  signals_enabled =  Off  \n"), profiles: "search"},
	{name: "signals off, commented out", goEnv: new("# SIGNALS_ENABLED=false\n"), profiles: "search,signals"},
	{name: "signals off on any line", goEnv: new("SIGNALS_ENABLED=true\nSIGNALS_ENABLED=no\n"), profiles: "search"},
	{name: "inherited", env: map[string]string{"COMPOSE_PROFILES": "mine"}, profiles: "mine"},
	{name: "inherited empty", env: map[string]string{"COMPOSE_PROFILES": ""}, profiles: ""},
	{name: "tools from env", env: map[string]string{"SKELET_TOOLS": "grafana"}, tools: new("mailpit\n"), profiles: "search,grafana,signals", wantTools: "grafana"},
	{name: "whitespace", apps: new(" dashboard ,\n vetrina-demo \n"), profiles: "search,dashboard,vetrina-demo,signals", wantApps: "dashboard,vetrina-demo"},
	{name: "empty apps file", apps: new(""), profiles: "search,signals"},
	{name: "custom sidecars", env: map[string]string{"SKELET_SIDECARS": "search,extra"}, profiles: "search,extra,signals"},
}

func TestLoadMatchesLib(t *testing.T) {
	for _, c := range libCases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			for name, body := range map[string]*string{".skelet-apps": c.apps, ".skelet-tools": c.tools, "env/go.env": c.goEnv} {
				if body == nil {
					continue
				}
				if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, name), []byte(*body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, k := range []string{"COMPOSE_PROFILES", "SKELET_SIDECARS", "SKELET_TOOLS"} {
				unset(t, k)
			}
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			l := Load(root)
			if l.Profiles != c.profiles || l.Apps != c.wantApps || l.Tools != c.wantTools {
				t.Errorf("profiles %q apps %q tools %q\n_lib said %q %q %q", l.Profiles, l.Apps, l.Tools, c.profiles, c.wantApps, c.wantTools)
			}
		})
	}
}

// unset clears key for one test, restoring it after - t.Setenv can only set
func unset(t *testing.T, key string) {
	t.Helper()
	if v, ok := os.LookupEnv(key); ok {
		t.Cleanup(func() { _ = os.Setenv(key, v) })
	}
	_ = os.Unsetenv(key)
}

func fakeDocker(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestIDsMatchLib(t *testing.T) {
	mine := []string{strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())}
	for _, c := range []struct {
		name, docker string
		preset       *string
		want         []string
		ok           bool
	}{
		{"rootful", `echo "[name=seccomp]"`, nil, mine, true},
		{"rootless", `echo "[name=rootless]"`, nil, []string{"0", "0"}, true},
		{"docker down", `exit 1`, nil, mine, true},
		{"empty SKELET_UID counts as unset", `echo "[name=seccomp]"`, new(""), mine, true},
		{"preset SKELET_UID is kept, gid and all", `echo "[name=rootless]"`, new("1234"), []string{"", ""}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			fakeDocker(t, c.docker)
			unset(t, "SKELET_UID")
			if c.preset != nil {
				t.Setenv("SKELET_UID", *c.preset)
			}
			uid, gid, ok := IDs()
			if uid != c.want[0] || gid != c.want[1] || ok != c.ok {
				t.Errorf("IDs = %q %q %v, want %v %v", uid, gid, ok, c.want, c.ok)
			}
		})
	}
}

func TestUserFlags(t *testing.T) {
	unset(t, "SKELET_NO_USER_MAP")
	if got := strings.Join(UserFlags(), " "); got != "-e LC_ALL=C.UTF-8 -e LANG=C.UTF-8" {
		t.Errorf("UserFlags = %q", got)
	}
	t.Setenv("SKELET_NO_USER_MAP", "1")
	if got := strings.Join(UserFlags(), " "); got != "-u 0:0 -e LC_ALL=C.UTF-8 -e LANG=C.UTF-8" {
		t.Errorf("with SKELET_NO_USER_MAP: %q", got)
	}
}

func TestEnvSetsWithoutDuplicating(t *testing.T) {
	t.Setenv("COMPOSE_PROFILES", "mine")
	t.Setenv("SKELET_SHIM", "1")
	env := Load(t.TempDir()).Env()
	var profiles []string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "COMPOSE_PROFILES="); ok {
			profiles = append(profiles, v)
		}
		if strings.HasPrefix(kv, "SKELET_SHIM=") {
			t.Error("the shim's variable reached compose")
		}
	}
	if !slices.Equal(profiles, []string{"mine"}) || !slices.Contains(env, "LC_ALL=C.UTF-8") {
		t.Errorf("COMPOSE_PROFILES entries %v, LC_ALL set %v", profiles, slices.Contains(env, "LC_ALL=C.UTF-8"))
	}
}

// Exact service names - the scripts' grep for skelet-vetrina-demo also matched skelet-vetrina-demo-eu
func TestExecOrRun(t *testing.T) {
	fakeDocker(t, `[ "$*" = "compose ps --services --status running" ] && printf 'app\nvetrina-demo-eu\n'`)
	l := Load(t.TempDir())
	if args, creates := l.ExecOrRun(Target{Service: "app", Flags: []string{"-w", "/x"}}, "go", "test"); creates ||
		strings.Join(args, " ") != "exec -w /x app go test" {
		t.Errorf("running service: %v, creates %v", args, creates)
	}
	if args, creates := l.ExecOrRun(Target{Service: "vetrina-demo", NoDeps: true, TTY: true}, "pnpm", "i"); !creates ||
		strings.Join(args, " ") != "run --rm --no-deps vetrina-demo pnpm i" {
		t.Errorf("stopped service (stdin isn't a terminal under go test, so no -it): %v, creates %v", args, creates)
	}
}
