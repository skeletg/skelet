package doctor

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/apps/skelet/internal/compose"
	"github.com/skeletg/skelet/apps/skelet/internal/envfile"
	"github.com/skeletg/skelet/apps/skelet/internal/node"
	"github.com/skeletg/skelet/apps/skelet/internal/plugins"
	"github.com/skeletg/skelet/apps/skelet/internal/profile"
	"github.com/skeletg/skelet/apps/skelet/internal/setup"
)

func checkout(root string, out io.Writer, p probes) int {
	r := &report{out: out}
	docker(r)
	tools(r, root)

	// Read first - which env files must exist depends on the target this clone was set up for
	prof, _ := profile.Load(filepath.Join(root, profile.File))
	target := prof["SKELET_TARGET"]
	if target != "small" && target != "prod" {
		target = "dev"
	}
	_, files, _ := setup.EnvFiles(root, target)
	optional := optionalEnv(root, target)
	r.section("env files (%s target)", target)
	for _, f := range files {
		switch {
		case exists(filepath.Join(root, f)):
			r.pass("%s", f)
		case slices.Contains(optional, f):
			r.note("%s missing - optional, compose runs without it", f)
		default:
			r.bad("%s missing - run skelet setup", f)
		}
	}

	r.section("app selection")
	inDocker := ""
	if raw, err := os.ReadFile(filepath.Join(root, ".skelet-apps")); err == nil {
		inDocker = strings.Join(strings.Fields(string(raw)), "")
		r.pass("in Docker: %s (.skelet-apps; change with skelet setup --docker-apps ...)", or(inDocker, "none - the frontends run on the host"))
	} else {
		r.pass("none in Docker - the frontends run on the host (skelet setup --docker-apps ... to containerize)")
	}
	hostFrontends(r, root, strings.Split(inDocker, ","))

	shape(r, root, prof, files)
	pluginSet(r, root)
	lib := containers(r, root)
	ours := ports(r, root, lib, p)
	endpoints(r, ours, p)
	hygiene(r, root)
	disk(r, root)
	return r.end()
}

// optionalEnv is the env_file entries marked required: false - compose runs without them, so a missing one only warns
func optionalEnv(root, target string) []string {
	var out []string
	path := regexp.MustCompile(`path:\s*(env/[a-z0-9.-]+\.env)`)
	for _, f := range []string{"docker-compose." + target + ".yml", "docker-compose.yml"} {
		raw, _ := os.ReadFile(filepath.Join(root, f))
		prev := ""
		for line := range strings.Lines(string(raw)) {
			if prev != "" && strings.Contains(line, "required: false") {
				out = append(out, prev)
			}
			prev = ""
			if m := path.FindStringSubmatch(line); m != nil {
				prev = m[1]
			}
		}
	}
	return out
}

func hostFrontends(r *report, root string, inDocker []string) {
	r.section("host frontends")
	var host []string
	dirs, _ := filepath.Glob(filepath.Join(root, "apps", "channels", "*"))
	for _, d := range append([]string{filepath.Join(root, "apps", "dashboard")}, dirs...) {
		if exists(filepath.Join(d, "package.json")) && !slices.Contains(inDocker, filepath.Base(d)) {
			rel, _ := filepath.Rel(root, d)
			host = append(host, rel)
		}
	}
	if len(host) == 0 {
		r.pass("none - every frontend runs in Docker")
		return
	}
	manifest, _ := os.ReadFile(filepath.Join(root, host[0], "package.json"))
	switch have, problem := node.Pnpm(root, host[0], node.Pin(manifest)); {
	case problem != "":
		r.bad("%s", problem)
	case have != "":
		r.pass("pnpm %s", have)
	default:
		r.pass("pnpm on PATH")
	}
	nvmrc, _ := os.ReadFile(filepath.Join(root, ".nvmrc"))
	want := strings.TrimSpace(string(nvmrc))
	if v, err := output("", "node", "--version"); err != nil {
		r.note("no node on PATH - the host frontends need Node %s (.nvmrc)", or(want, "-"))
	} else if have := version(v); want != "" && len(have) > 0 && dotted(have[:1]) != strings.Split(want, ".")[0] {
		r.note("node %s, and .nvmrc pins %s - the frontends are built and tested on that", dotted(have), want)
	} else {
		r.pass("node %s", dotted(have))
	}
	for _, d := range host {
		switch node.Tree(filepath.Join(root, d)) {
		case "none":
			r.note("%s not installed - skelet setup, or: cd %s && pnpm install", d, d)
		case "pnpm":
			r.pass("%s installed with pnpm", d)
		default:
			r.bad("%s/node_modules is an npm install from before the switch to pnpm - rm -rf it, then pnpm install", d)
		}
	}
}

var interpolation = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)\}`)

func shape(r *report, root string, prof profile.Profile, files []string) {
	r.section("shape")
	if !exists(filepath.Join(root, profile.File)) {
		r.note("no .skelet-profile - run skelet setup to record what this clone is meant to be")
	} else {
		get := func(k string) string {
			if v, ok := prof.Get(k); ok {
				return v
			}
			return "(unset)"
		}
		r.pass("target=%s signals=%s ai=%s search=%s", get("SKELET_TARGET"), get("SKELET_SIGNALS"), get("SKELET_AI"), get("SKELET_SEARCH"))
		r.pass("gateways: %s", or(prof["SKELET_GATEWAYS"], "none"))
		// The env file is what the backend reads - a profile that disagrees with it is the confusing state, since the
		// dashboard shows one shape and the API serves another
		for key, answer := range map[string]string{"SIGNALS_ENABLED": "SKELET_SIGNALS", "AI_ENABLED": "SKELET_AI"} {
			want, ok := prof.Get(answer)
			if !ok {
				continue
			}
			got := ""
			for _, f := range files {
				if v, found, _ := envfile.Get(filepath.Join(root, f), key); found {
					got = v
					break
				}
			}
			if got == want || (got == "" && want == "true") {
				continue
			}
			r.note("%s is %s in %s but the profile says %s - re-run skelet setup", key, or(got, "(unset, defaults true)"), files[0], want)
		}
	}

	// Every variable a compose file interpolates without a default has to come from somewhere, or the service quietly
	// gets an empty string - a feature that just does nothing
	raw, _ := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	var missing []string
	for _, m := range interpolation.FindAllSubmatch(raw, -1) {
		v := string(m[1])
		if _, ok, _ := envfile.Get(filepath.Join(root, ".env"), v); ok || os.Getenv(v) != "" || slices.Contains(missing, v) {
			continue
		}
		missing = append(missing, v)
	}
	slices.Sort(missing)
	if len(missing) == 0 {
		r.pass("every compose interpolation has a value")
	} else {
		r.note("no value for: %s - compose will use an empty string", strings.Join(missing, " "))
	}
}

// pluginSet holds plugins.json to the rules codegen enforces, so a broken decision shows here rather than as a gen run
// that refuses
func pluginSet(r *report, root string) {
	r.section("plugins")
	raw, err := os.ReadFile(filepath.Join(root, plugins.File))
	if err != nil {
		r.bad("%s is missing - codegen can't run without it", plugins.File)
		return
	}
	set, _, err := plugins.Load(root)
	if err != nil {
		r.bad("%v", err)
		return
	}
	found, _ := plugins.Discover(root)
	before := r.fail
	slugs := make([]string, 0, len(set.Plugins))
	for s := range set.Plugins {
		slugs = append(slugs, s)
	}
	slices.Sort(slugs)
	var on []string
	for _, s := range slugs {
		e := set.Plugins[s]
		switch {
		case !plugins.SlugPattern.MatchString(s) || s == "firstparty":
			r.bad("plugins.json names %q, which isn't a plugin slug - codegen refuses the whole file", s)
		case e.Enabled && e.Module == "" && !slices.Contains(found, s):
			r.bad("plugins.json enables %s, but plugins/%s registers no plugin", s, s)
		case e.Enabled:
			on = append(on, s)
		}
	}
	for _, s := range found {
		if _, ok := set.Plugins[s]; !ok {
			r.bad("plugins/%s has no decision in plugins.json - codegen won't run until it has one (skelet cli plugin enable|disable %s)", s, s)
		}
	}
	var shape struct {
		Plugins map[string]json.RawMessage `json:"plugins"`
	}
	if json.Unmarshal(raw, &shape) == nil {
		for _, v := range shape.Plugins {
			if len(v) > 0 && v[0] != '{' {
				r.note("plugins.json uses the old 0/1 form - it still loads, and skelet cli plugin rewrites it")
				break
			}
		}
	}
	if r.fail == before {
		r.pass("%d decided, enabled: %s", len(slugs), or(strings.Join(on, ", "), "none"))
	}
}

func containers(r *report, root string) compose.Lib {
	r.section("containers")
	lib, err := compose.Load(root)
	if err != nil {
		r.note("%v - the checks below use compose's defaults", err)
	}
	ps, _ := compose.Command(root, lib.Env(), "ps", "--status", "running", "--format", "{{.Service}}").Output()
	running := strings.Fields(string(ps))
	slices.Sort(running)
	running = slices.Compact(running)
	if len(running) == 0 {
		r.note("nothing running - start with skelet up")
		return lib
	}
	r.pass("running: %s", strings.Join(running, " "))

	// The suite follows core's switch, and unset means on - as the backend reads it
	v, _, _ := envfile.Get(filepath.Join(root, "env", "go.env"), "SIGNALS_ENABLED")
	if slices.Contains([]string{"false", "0", "f", "no", "off"}, strings.ToLower(strings.TrimSpace(v))) {
		return lib
	}
	// The spine lives in ClickHouse - with the suite on, a missing server or schema means the beacons write into a hole
	if slices.Contains(running, "clickhouse") {
		q, _ := compose.Command(root, lib.Env(), "exec", "-T", "clickhouse", "clickhouse-client",
			"--user="+envOr("CLICKHOUSE_USER", "skelet"), "--password="+envOr("CLICKHOUSE_PASSWORD", "skelet"),
			"--query", "EXISTS TABLE signals.analytics_events").Output()
		if strings.TrimSpace(string(q)) == "1" {
			r.pass("clickhouse spine up, schema present")
		} else {
			r.bad("clickhouse is up but signals.analytics_events is missing - the signals process migrates it as it boots, see skelet logs signals")
		}
	} else {
		r.bad("SIGNALS_ENABLED is on but no clickhouse container - the spine has nowhere to write (skelet up)")
	}
	// Core dials the suite whenever the switch is on - without it every Signals screen answers "out of reach"
	if slices.Contains(running, "signals") {
		r.pass("signals process up")
	} else {
		r.bad("SIGNALS_ENABLED is on but no signals container - core has nowhere to dial (skelet up starts it)")
	}
	return lib
}

// ports probes what the stack should be serving, and returns the ports this checkout's containers publish
func ports(r *report, root string, lib compose.Lib, p probes) []string {
	r.section("ports (only meaningful when the stack should be up)")
	type spec struct{ port, name string }
	// skelet up may have moved Postgres off 5432 through .env when something else held it
	db := "5432"
	if v, ok, _ := envfile.Get(filepath.Join(root, ".env"), "DB_HOST_PORT"); ok && regexp.MustCompile(`^[0-9]+$`).MatchString(v) {
		db = v
	}
	specs := []spec{{"8080", "backend"}, {"5173", "dashboard"}, {db, "postgres"}, {"9900", "garage-s3"}, {"9902", "garage-web"}}
	// The dev tools are opt-in, so only the picked ones are expected to answer
	toolPorts := map[string]string{"mailpit": "8025", "webhook-tester": "8081", "grafana": "3030", "garage-webui": "9901"}
	for t := range strings.SplitSeq(lib.Tools, ",") {
		if port, ok := toolPorts[t]; ok {
			specs = append(specs, spec{port, t})
		}
	}
	// A storefront's published port is its own business, so compose is asked rather than assuming the demo's 3000
	dirs, _ := filepath.Glob(filepath.Join(root, "apps", "channels", "*"))
	for _, d := range dirs {
		name := filepath.Base(d)
		out, _ := compose.Command(root, lib.Env(), "port", name, "3000").Output()
		switch s := strings.TrimSpace(string(out)); {
		case s != "":
			specs = append(specs, spec{s[strings.LastIndex(s, ":")+1:], name})
		case lib.Apps == "":
			r.pass("%s not in Docker - run it on the host (skelet serve %s)", name, name)
		default:
			r.note("%s has no published port (not running, or not in .skelet-apps)", name)
		}
	}
	// Only ports this project publishes count as ours. Probing localhost alone can't tell this stack from another
	// checkout's, and reporting somebody else's services as healthy hides that this one never came up
	pub, _ := compose.Command(root, lib.Env(), "ps", "--format", "{{.Publishers}}").Output()
	ours := regexp.MustCompile(`[0-9]+`).FindAllString(string(pub), -1)
	for _, s := range specs {
		switch {
		case !p.open(s.port):
			r.note("%s (:%s) not answering", s.name, s.port)
		case slices.Contains(ours, s.port):
			r.pass("%s (:%s) answering", s.name, s.port)
		default:
			r.note("%s (:%s) answering, but NOT from this checkout - another stack holds it", s.name, s.port)
		}
	}
	return ours
}

func endpoints(r *report, ours []string, p probes) {
	r.section("endpoints")
	// Same trap as the ports - :8080 may belong to another checkout, and "healthy" read off somebody else's backend is
	// the most convincing wrong answer this could give
	if slices.Contains(ours, "8080") {
		status, body, err := p.get("http://localhost:8080/health")
		if err == nil && status/100 == 2 && body == "ok" {
			r.pass("backend /health: ok (db + valkey reachable)")
		} else {
			if err != nil || body == "" {
				body = "no response"
			}
			r.note("backend /health not ok (%s)", body)
		}
	} else {
		r.note("backend /health skipped - this checkout publishes no :8080 (nothing running here)")
	}
	// Any answer proves the name resolved, a 404 included - only a lookup failure means it didn't
	var dns *net.DNSError
	if _, _, err := p.get("http://media.localhost:9902/"); errors.As(err, &dns) {
		r.note("media.localhost doesn't resolve here - browsers handle *.localhost themselves, but host tools won't")
	} else {
		r.pass("*.localhost resolves (public media URLs will work)")
	}
}

// hygiene is the checkout's own state: the opt-in pre-commit hook, and a WSL clone on the Windows drive
func hygiene(r *report, root string) {
	r.section("checkout")
	hook, err := output(root, "git", "rev-parse", "--git-path", "hooks/pre-commit")
	if err == nil && !filepath.IsAbs(hook) {
		hook = filepath.Join(root, hook)
	}
	switch raw, err := os.ReadFile(hook); {
	case err != nil:
		r.pass("no pre-commit hook - skelet hooks adds one that runs skelet check, if you want it")
	case strings.Contains(string(raw), "bin/check"):
		r.pass("pre-commit hook runs skelet check")
	default:
		r.pass("pre-commit hook is your own - skelet hooks would replace it")
	}
	if release, _ := os.ReadFile("/proc/sys/kernel/osrelease"); strings.Contains(strings.ToLower(string(release)), "microsoft") {
		if strings.HasPrefix(root, "/mnt/") {
			r.note("this checkout is on the Windows drive (%s) - file watching misses changes and disk access crawls there; clone it into the Linux filesystem", root)
		} else {
			r.pass("on WSL's own filesystem")
		}
	}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
