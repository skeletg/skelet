package setup

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/plugins"
	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
	"github.com/skeletg/skelet/cmd/skelet/internal/secret"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
)

const (
	CoreEnv    = "apps/backend/core/.env"
	SignalsEnv = "apps/backend/signals/.env"
)

// What a write leaves for the boot and the summary
type written struct {
	pluginsChanged bool
	adminSlug      string
}

// Write turns answers into files: the target's env files with their secrets, the
// root .env's knobs and plugins.json. It starts nothing and asks nothing. Anything already holding a real value is
// left alone, so a second run with the same answers changes no file
func Write(root string, a answers, c catalog, rnd io.Reader, out io.Writer) (written, error) {
	var w written
	path := func(rel string) string { return filepath.Join(root, rel) }

	core := a.hasCore()
	if a.target == "dev" {
		fmt.Fprintf(out, "==> Apps: %s\n", orNone(a.apps, "backend only"))
		fmt.Fprintf(out, "==> Dev tools: %s  (start one ad hoc with skelet up <name>)\n", orNone(a.tools, "none"))
	} else {
		fmt.Fprintf(out, "==> On this machine: %s\n", join(a.apps))
	}
	if core {
		fmt.Fprintf(out, "==> Shape: %s, signals=%t ai=%t, search=%s, small=%t\n", a.target, a.signals, a.ai, a.search, a.small)
	}

	backend, files, err := EnvFiles(root, a.target, a.runs)
	if err != nil {
		return w, err
	}
	fmt.Fprintf(out, "==> Env files for %s\n", a.target)
	for _, f := range files {
		created, err := fromTemplate(root, f, f+".example")
		if err != nil {
			return w, err
		}
		if created {
			fmt.Fprintf(out, "    %s\n", f)
		} else {
			fmt.Fprintf(out, "    %s (exists, keeping your values)\n", f)
		}
	}
	if _, err := os.Stat(path(".env")); errors.Is(err, fs.ErrNotExist) {
		// The root compose files read COMPOSE_PROFILES=bundled from the template - without it their datastores never start
		if a.target == "dev" {
			err = envfile.WriteAtomic(path(".env"), nil, 0o600)
		} else {
			_, err = fromTemplate(root, ".env", ".env.example")
		}
		if err != nil {
			return w, err
		}
	}
	e := editor{root: root, rnd: rnd, out: out}
	for _, f := range files {
		if strings.HasSuffix(f, "/.env.production") {
			e.frontends = append(e.frontends, f)
		}
	}
	if a.target != "dev" {
		e.rehome(files)
	}
	if core {
		e.set(backend, "SIGNALS_ENABLED", strconv.FormatBool(a.signals))
		e.set(backend, "AI_ENABLED", strconv.FormatBool(a.ai))
	}
	e.secrets(a, backend)
	if e.err != nil {
		return w, e.err
	}

	fmt.Fprintln(out, "==> Compose knobs (.env)")
	if a.target == "dev" {
		// The dev storefront mints review-vote tokens core verifies and signs the revalidations core sends, so it
		// reads both from the root .env, the only file compose interpolates
		for _, k := range []string{"REVIEW_VOTER_TOKEN_SECRET", "STOREFRONT_REVALIDATE_SECRET"} {
			e.set(".env", k, e.get(backend, k))
		}
	}
	if core {
		w.adminSlug = e.coreKnobs(a, backend, files)
	} else if slug, ok := a.shared["ADMIN_API_SLUG"]; ok && dashboardEnv(a, files) != ".env" {
		e.dashboardAPI(dashboardEnv(a, files), slug)
	}
	e.set(".env", profile.EnvTarget, a.target)
	e.set(".env", profile.EnvApps, join(a.apps))
	if a.target == "dev" {
		e.set(".env", profile.EnvProfiles, profile.DevProfiles(a.tools, a.signals))
	} else {
		e.deployment(a, c, backend, files)
	}
	if e.err != nil {
		return w, e.err
	}

	if core {
		w.pluginsChanged, err = decidePlugins(root, c, a.gateways, out)
	}
	return w, err
}

// coreKnobs writes search and the admin API's path every run, so answering back to a default takes effect
func (e *editor) coreKnobs(a answers, backend string, files []string) string {
	knobs := ".env"
	if a.target != "dev" {
		knobs = backend
	}
	switch {
	case a.search == "postgres":
		e.set(knobs, "OPENSEARCH_URL", "")
		fmt.Fprintln(e.out, "    search pinned to the Postgres tier")
	case a.target == "dev":
		// compose reads ${OPENSEARCH_URL-http://opensearch:9200}: absent means the bundled cluster, set-but-empty
		// pins search to Postgres
		e.del(".env", "OPENSEARCH_URL")
		fmt.Fprintln(e.out, "    search on the bundled OpenSearch")
	default:
		fmt.Fprintf(e.out, "    search on OpenSearch - OPENSEARCH_URL in %s names the cluster\n", backend)
	}
	slug := ""
	if a.adminSlug {
		slug = e.get(knobs, "ADMIN_API_SLUG")
		if slug == "" {
			slug = "admin-" + e.hex(8)
			fmt.Fprintf(e.out, "    admin API moved to /api/admin/%s/graphql\n", slug)
		} else {
			fmt.Fprintln(e.out, "    admin API slug already set, left alone")
		}
		e.set(knobs, "ADMIN_API_SLUG", slug)
		e.del(".env", "GRAPHQL_CODEGEN_URL")
	} else {
		// All three read ${VAR:-default} in compose, so deleting them puts the API back at /api/admin/graphql
		for _, k := range []string{"ADMIN_API_SLUG", "VITE_GRAPHQL_URL", "GRAPHQL_CODEGEN_URL"} {
			e.del(".env", k)
		}
		if a.target != "dev" {
			e.set(backend, "ADMIN_API_SLUG", "")
		}
	}
	if f := dashboardEnv(a, files); a.target == "dev" || f != ".env" {
		e.dashboardAPI(f, slug)
	}
	return slug
}

// dashboardAPI points the dashboard's build at the admin API under slug - an absolute address keeps its host
func (e *editor) dashboardAPI(file, slug string) {
	path := "/api/admin/graphql"
	if slug != "" {
		path = "/api/admin/" + slug + "/graphql"
	}
	if u, err := url.Parse(e.get(file, "VITE_GRAPHQL_URL")); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		u.Path = path
		e.set(file, "VITE_GRAPHQL_URL", u.String())
		return
	}
	if slug == "" {
		e.del(file, "VITE_GRAPHQL_URL")
		return
	}
	e.set(file, "VITE_GRAPHQL_URL", path)
}

// deployment wires the apps on this machine to each other, leaving a value set by hand alone
func (e *editor) deployment(a answers, c catalog, backend string, files []string) {
	names := []string{a.shop}
	for _, s := range c.storefronts {
		names = append(names, s.Name)
	}
	cur := e.get(".env", "COMPOSE_PROFILES")
	profiles := slices.DeleteFunc(split(cur), func(p string) bool { return slices.Contains(names, p) || p == "dashboard" })
	core := a.runs(appCore)
	if core && a.small {
		profiles = slices.DeleteFunc(profiles, func(p string) bool { return p == "valkey" })
		e.set(backend, "COMPOSE_PROFILES", join(slices.DeleteFunc(split(e.get(backend, "COMPOSE_PROFILES")), func(p string) bool { return p == "valkey" })))
	}
	if join(profiles) != cur {
		e.set(".env", "COMPOSE_PROFILES", join(profiles))
	}
	if core {
		e.shape(a.small, backend)
	}
	switch {
	case a.runs(appSignals) && !core:
		fmt.Fprintf(e.out, "    Signals reaches core at CORE_FRONT_API_URL and INGEST_FALLBACK_URL in %s - point them at core\n", SignalsEnv)
	case core && a.signals && !a.runs(appSignals):
		fmt.Fprintf(e.out, "    core reaches Signals at SIGNALS_GRPC_ADDR, SIGNALS_INGEST_URL and SIGNALS_URL in %s - point them at Signals,\n", CoreEnv)
		fmt.Fprintf(e.out, "    and on its machine run skelet setup signals --shared with a copy of %s\n", CoreEnv)
	}
	shops := e.served(a, c, files)
	if !core {
		for _, s := range shops {
			fmt.Fprintf(e.out, "    storefront %s - its server on :%s, reaching core at BACKEND_INTERNAL_URL\n", s.name, s.port)
		}
		return
	}
	if len(shops) == 0 {
		return
	}
	ours := func(v string) bool {
		return v == "" || strings.HasPrefix(v, storefrontOnHost) || strings.HasPrefix(v, "http://"+storefrontOnHost) ||
			slices.ContainsFunc(names, func(n string) bool { return n != "" && (v == n+":3000" || v == "http://"+n+":3000/api/revalidate") })
	}
	purge := ""
	if len(shops) == 1 {
		purge = "http://" + storefrontOnHost + shops[0].port + "/api/revalidate"
	}
	if ours(e.get(backend, "STOREFRONT_REVALIDATE_URL")) {
		e.set(backend, "STOREFRONT_REVALIDATE_URL", purge)
	}
	caddy := "env/prod-caddy.env"
	edge := slices.Contains(files, caddy)
	if edge && ours(e.get(caddy, "STOREFRONT_UPSTREAM")) {
		e.set(caddy, "STOREFRONT_UPSTREAM", storefrontOnHost+shops[0].port)
	}
	for i, s := range shops {
		fmt.Fprintf(e.out, "    storefront %s - its server on the host's :%s\n", s.name, s.port)
		if i > 0 && edge {
			e.site(s)
		}
	}
	if len(shops) > 1 {
		fmt.Fprintln(e.out, "    the backend purges each channel at its public URL, so give every channel one in admin")
	}
}

// Where the containers reach a storefront's server running on the same machine
const storefrontOnHost = "host.docker.internal:"

func dashboardEnv(a answers, files []string) string {
	if a.target != "dev" && slices.Contains(files, "apps/dashboard/.env.production") {
		return "apps/dashboard/.env.production"
	}
	return ".env"
}

func primaryStorefront(a answers, c catalog) string {
	if a.shop != "" {
		return a.shop
	}
	for _, s := range c.storefronts {
		if slices.Contains(a.apps, s.Name) {
			return s.Name
		}
	}
	return ""
}

var envRef = regexp.MustCompile(`env/[a-z0-9.-]+\.env`)

var ServerCompose = []string{"docker-compose.small.yml", "docker-compose.prod.yml"}

// EnvFiles is core's env file - empty without core - and every env file the apps here need, the edge's going with core
func EnvFiles(root, target string, runs func(app string) bool) (string, []string, error) {
	if target == "dev" {
		return "env/go.env", []string{"env/go.env", "env/db.env", "env/signals.env"}, nil
	}
	here := func(app string) bool { return runs == nil || runs(app) }
	backend, files := "", []string{}
	if here(appCore) {
		backend, files = CoreEnv, append(files, CoreEnv)
		var named []string
		for _, compose := range ServerCompose {
			raw, err := os.ReadFile(filepath.Join(root, compose))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return "", nil, err
			}
			for _, m := range envRef.FindAll(raw, -1) {
				if f := string(m); !slices.Contains(named, f) {
					named = append(named, f)
				}
			}
		}
		slices.Sort(named)
		files = append(files, named...)
	}
	if here(appSignals) {
		files = append(files, SignalsEnv)
	}
	apps := map[string]string{"dashboard": "apps/dashboard"}
	for _, s := range storefronts.List(root) {
		apps[s.Name] = s.Dir
	}
	var own []string
	for name, dir := range apps {
		f := dir + "/.env.production"
		if _, err := os.Stat(filepath.Join(root, f+".example")); err == nil && here(name) {
			own = append(own, f)
		}
	}
	slices.Sort(own)
	return backend, append(files, own...), nil
}

// fromTemplate copies tmpl to file when file doesn't exist yet - owner-only, since these hold secrets
func fromTemplate(root, file, tmpl string) (bool, error) {
	if _, err := os.Stat(filepath.Join(root, file)); err == nil {
		return false, nil
	}
	raw, err := os.ReadFile(filepath.Join(root, tmpl))
	if err != nil {
		return false, fmt.Errorf("%s needs its template: %w", file, err)
	}
	return true, envfile.WriteAtomic(filepath.Join(root, file), raw, 0o600)
}

// A value setup may overwrite - anything else is the operator's
func placeholder(v string) bool {
	switch v {
	case "", "change-me", "change-me-too", "changeme", "CHANGE_ME", "change-me-in-prod":
		return true
	}
	return false
}

// editor carries the first error through a run of env edits, so the sequence reads as the list of edits it is
type editor struct {
	root      string
	rnd       io.Reader
	out       io.Writer
	err       error
	frontends []string
}

func (e *editor) get(file, key string) string {
	if e.err != nil {
		return ""
	}
	v, _, err := envfile.Get(filepath.Join(e.root, file), key)
	e.err = err
	return v
}

func (e *editor) set(file, key, val string) {
	if e.err == nil {
		e.err = envfile.Set(filepath.Join(e.root, file), key, val)
	}
}

func (e *editor) del(file, key string) {
	if e.err == nil {
		e.err = envfile.Del(filepath.Join(e.root, file), key)
	}
}

func (e *editor) hex(n int) string {
	if e.err != nil {
		return ""
	}
	v, err := secret.Hex(e.rnd, n)
	e.err = err
	return v
}

func (e *editor) key32() string {
	if e.err != nil {
		return ""
	}
	v, err := secret.Key32(e.rnd)
	e.err = err
	return v
}

// generate makes a value in the encoding key's checks demand: the AES key and the Ed25519 seed are base64 of 32
// bytes, everything else is 32 bytes of hex
func (e *editor) generate(key string) string {
	switch key {
	case "CONFIG_ENCRYPTION_KEY", "CUSTOMER_JWT_PRIVATE_KEY":
		return e.key32()
	}
	return e.hex(32)
}

func (e *editor) secrets(a answers, backend string) {
	fmt.Fprintln(e.out, "==> Secrets")
	if a.hasCore() {
		e.coreSecrets(a, backend)
	}
	if a.target == "dev" {
		// The Signals processes' own file carries what they share with core - the public key, never the private one
		for _, k := range []string{"CUSTOMER_JWT_PUBLIC_KEY", "EMAIL_LINK_SECRET", "SIGNALS_SERVICE_SECRET"} {
			if v := e.get(backend, k); v != "" {
				e.set("env/signals.env", k, v)
			}
		}
		e.del("env/signals.env", "JWT_SECRET")
		return
	}
	core := a.runs(appCore)
	shared := func(k string) (string, bool) {
		if core {
			return e.get(backend, k), true
		}
		v, ok := a.shared[k]
		return v, ok
	}
	if a.runs(appSignals) {
		for _, k := range signalsShares {
			if v, ok := shared(k); ok {
				e.share(SignalsEnv, k, v)
			}
		}
		e.generateMissing(SignalsEnv, "SIGNALS_DB_PASSWORD", "CLICKHOUSE_PASSWORD", "SIGNALS_VALKEY_PASSWORD")
		if core && !a.small && placeholder(e.get(SignalsEnv, "VALKEY_URL")) {
			e.set(SignalsEnv, "VALKEY_URL", "redis://:"+e.get(backend, "VALKEY_PASSWORD")+"@valkey:6379")
		}
	}
	for _, f := range e.frontends {
		for _, k := range frontendShares {
			if _, listed, _ := envfile.Get(filepath.Join(e.root, f), k); !listed {
				continue
			}
			if v, ok := shared(k); ok {
				e.share(f, k, v)
			}
		}
	}
}

func (e *editor) coreSecrets(a answers, backend string) {
	for _, k := range []string{"ADMIN_JWT_SECRET", "EMAIL_LINK_SECRET", "CONFIG_ENCRYPTION_KEY", "SIGNALS_SERVICE_SECRET",
		"REVIEW_VOTER_TOKEN_SECRET", "STOREFRONT_REVALIDATE_SECRET", "EMAIL_WEBHOOK_TOKEN", "CUSTOMER_JWT_PRIVATE_KEY"} {
		if placeholder(e.get(backend, k)) {
			e.set(backend, k, e.generate(k))
			fmt.Fprintf(e.out, "    %s generated\n", k)
		} else {
			fmt.Fprintf(e.out, "    %s already set, left alone\n", k)
		}
	}
	// The public half is always re-derived, so it can never drift from the private key it pairs with
	pub, err := secret.CustomerPublic(e.get(backend, "CUSTOMER_JWT_PRIVATE_KEY"))
	if e.err == nil && err != nil {
		e.err = fmt.Errorf("%s: %w", backend, err)
	}
	e.set(backend, "CUSTOMER_JWT_PUBLIC_KEY", pub)
	// JWT_SECRET signed both session types and is refused at boot now - its replacements are generated above
	if e.get(backend, "JWT_SECRET") != "" {
		e.del(backend, "JWT_SECRET")
		fmt.Fprintln(e.out, "    JWT_SECRET removed - ADMIN_JWT_SECRET, CUSTOMER_JWT_PRIVATE_KEY and EMAIL_LINK_SECRET replace it")
	}
	if a.target == "dev" {
		return
	}
	if placeholder(e.get(backend, "BACKUP_ENCRYPTION_PASSPHRASE")) {
		e.set(backend, "BACKUP_ENCRYPTION_PASSPHRASE", e.hex(32))
		fmt.Fprintln(e.out, "    BACKUP_ENCRYPTION_PASSPHRASE generated - store it somewhere you can restore from")
	}
	own := []string{"TRUSTED_PROXY_KEY", "STORAGE_SIGNING_KEY", "DB_PASSWORD"}
	if !a.small {
		own = append(own, "VALKEY_PASSWORD")
	}
	e.generateMissing(backend, own...)
}

func (e *editor) share(file, key, v string) {
	if cur := e.get(file, key); !placeholder(cur) && cur != v {
		fmt.Fprintf(e.out, "    %s %s replaced with core's\n", file, key)
	}
	e.set(file, key, v)
}

func (e *editor) generateMissing(file string, keys ...string) {
	for _, k := range keys {
		if placeholder(e.get(file, k)) {
			e.set(file, k, e.generate(k))
			fmt.Fprintf(e.out, "    %s %s generated\n", file, k)
		}
	}
}

// What a root .env held before each backend had its own file - left there it would override the app's, so it moves
var rehomed = []struct {
	key, app string
	stays    bool
}{
	{"ADMIN_JWT_SECRET", CoreEnv, false}, {"CUSTOMER_JWT_PRIVATE_KEY", CoreEnv, false}, {"CUSTOMER_JWT_PUBLIC_KEY", CoreEnv, false},
	{"EMAIL_LINK_SECRET", CoreEnv, false}, {"CONFIG_ENCRYPTION_KEY", CoreEnv, false}, {"SIGNALS_SERVICE_SECRET", CoreEnv, false},
	{"STORAGE_SIGNING_KEY", CoreEnv, false}, {"DB_PASSWORD", CoreEnv, false}, {"VALKEY_PASSWORD", CoreEnv, false},
	{"DATABASE_URL", CoreEnv, false}, {"VALKEY_URL", CoreEnv, false}, {"VALKEY_CACHE_URL", CoreEnv, false}, {"VALKEY_LIMITS_URL", CoreEnv, false},
	{"VALKEY_CACHE_MAXMEMORY", CoreEnv, false}, {"VALKEY_CACHE_MEMORY_LIMIT", CoreEnv, false},
	{"REVIEW_VOTER_TOKEN_SECRET", CoreEnv, false}, {"STOREFRONT_REVALIDATE_SECRET", CoreEnv, false}, {"TRUSTED_PROXY_KEY", CoreEnv, false},
	{"SIGNALS_DB_PASSWORD", SignalsEnv, false}, {"CLICKHOUSE_PASSWORD", SignalsEnv, false}, {"NATS_PASSWORD", SignalsEnv, false},
	{"SIGNALS_DB_URL", SignalsEnv, false}, {"SIGNALS_QUEUE_URL", SignalsEnv, false}, {"SIGNALS_CH_URL", SignalsEnv, false},
}

func (e *editor) rehome(files []string) {
	for _, r := range rehomed {
		v, found, err := envfile.Get(filepath.Join(e.root, ".env"), r.key)
		if e.err == nil {
			e.err = err
		}
		if !found {
			continue
		}
		if v != "" && slices.Contains(files, r.app) && placeholder(e.get(r.app, r.key)) {
			e.set(r.app, r.key, v)
			fmt.Fprintf(e.out, "    %s moved from .env to %s\n", r.key, r.app)
		}
		if !r.stays {
			e.del(".env", r.key)
		}
	}
	for _, old := range []string{"env/small.env", "env/prod.env", "env/prod-signals.env"} {
		if _, err := os.Stat(filepath.Join(e.root, old)); err == nil {
			fmt.Fprintf(e.out, "    %s is no longer read - its settings belong in %s or %s\n", old, CoreEnv, SignalsEnv)
		}
	}
}

// The small shape is one app container doing every job against Postgres alone - a later run without it puts the tiers back
var smallShape = [][2]string{{"WORKER_REPLICAS", "0"}, {"MEDIA_REPLICAS", "0"}, {"JOBS_EXTERNAL", "false"},
	{"VALKEY_URL", ""}, {"VALKEY_CACHE_URL", ""}, {"VALKEY_LIMITS_URL", ""}}

func (e *editor) shape(small bool, backend string) {
	if small {
		for _, kv := range smallShape {
			e.set(backend, kv[0], kv[1])
		}
		if e.get(backend, "CORE_PORT") == "" {
			e.set(backend, "CORE_PORT", "18080")
		}
		return
	}
	for _, kv := range smallShape {
		if v, found, _ := envfile.Get(filepath.Join(e.root, backend), kv[0]); !found || v != kv[1] {
			return
		}
	}
	e.set(backend, "WORKER_REPLICAS", "1")
	e.set(backend, "MEDIA_REPLICAS", "1")
	e.set(backend, "JOBS_EXTERNAL", "true")
	for _, k := range []string{"VALKEY_URL", "VALKEY_CACHE_URL", "VALKEY_LIMITS_URL"} {
		e.del(backend, k)
	}
}

// decidePlugins merges the gateway picks into plugins.json - merged, never rewritten from a list held here, since
// skelet cli plugin enable|disable|install write the same file
func decidePlugins(root string, c catalog, picked []string, out io.Writer) (bool, error) {
	set, before, err := plugins.Load(root)
	if err != nil {
		return false, err
	}
	for _, slug := range set.Decide(c.plugins, picked) {
		fmt.Fprintf(out, "==> Dropped %q from plugins.json - a parked directory's name, which no build reads\n", slug)
	}
	data, err := set.Marshal()
	if err != nil {
		return false, err
	}
	changed, err := plugins.Save(root, data, before)
	if err != nil {
		return false, err
	}
	var on []string
	for _, g := range c.gateways {
		if set.Plugins[g].Enabled {
			on = append(on, g)
		}
	}
	if changed {
		fmt.Fprintf(out, "==> Plugin set changed - gateways: %s\n", orNone(on, "none"))
	} else {
		fmt.Fprintln(out, "==> Plugin set unchanged")
	}
	return changed, nil
}
