package setup

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
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

	fmt.Fprintf(out, "==> Apps: %s\n", orNone(a.apps, "backend only"))
	fmt.Fprintf(out, "==> Dev tools: %s  (start one ad hoc with skelet up <name>)\n", orNone(a.tools, "none"))
	fmt.Fprintf(out, "==> Shape: %s, signals=%t ai=%t, search=%s\n", a.target, a.signals, a.ai, a.search)

	backend, files, err := EnvFiles(root, a.target, func(app string) bool { return slices.Contains(a.apps, app) })
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
		// small and prod read COMPOSE_PROFILES=bundled from the template - without it their datastores never start
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
		e.rehome(a.target)
	}
	e.set(backend, "SIGNALS_ENABLED", strconv.FormatBool(a.signals))
	e.set(backend, "AI_ENABLED", strconv.FormatBool(a.ai))
	e.secrets(backend, a.target)
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
	knobs := ".env"
	if a.target != "dev" {
		knobs = backend
	}
	// Written on every run, not only when they leave the default - otherwise re-running and changing an answer back
	// would silently do nothing
	switch {
	case a.search == "postgres":
		e.set(knobs, "OPENSEARCH_URL", "")
		fmt.Fprintln(out, "    search pinned to the Postgres tier")
	case a.target == "dev":
		// compose reads ${OPENSEARCH_URL-http://opensearch:9200}: absent means the bundled cluster, set-but-empty
		// pins search to Postgres
		e.del(".env", "OPENSEARCH_URL")
		fmt.Fprintln(out, "    search on the bundled OpenSearch")
	default:
		fmt.Fprintf(out, "    search on OpenSearch - OPENSEARCH_URL in %s names the cluster\n", backend)
	}
	if a.adminSlug {
		w.adminSlug = e.get(knobs, "ADMIN_API_SLUG")
		if w.adminSlug == "" {
			w.adminSlug = "admin-" + e.hex(8)
			fmt.Fprintf(out, "    admin API moved to /api/admin/%s/graphql\n", w.adminSlug)
		} else {
			fmt.Fprintln(out, "    admin API slug already set, left alone")
		}
		e.set(knobs, "ADMIN_API_SLUG", w.adminSlug)
		if f := dashboardEnv(a, files); a.target == "dev" || f != ".env" {
			e.set(f, "VITE_GRAPHQL_URL", "/api/admin/"+w.adminSlug+"/graphql")
		}
		e.del(".env", "GRAPHQL_CODEGEN_URL")
	} else {
		// All three read ${VAR:-default} in compose, so deleting them puts the API back at /api/admin/graphql
		for _, k := range []string{"ADMIN_API_SLUG", "VITE_GRAPHQL_URL", "GRAPHQL_CODEGEN_URL"} {
			e.del(".env", k)
		}
		if a.target != "dev" {
			e.set(backend, "ADMIN_API_SLUG", "")
			if f := dashboardEnv(a, files); f != ".env" {
				e.del(f, "VITE_GRAPHQL_URL")
			}
		}
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

	w.pluginsChanged, err = decidePlugins(root, c, a.gateways, out)
	return w, err
}

// deployment points a server's stack at the storefront this clone runs, leaving a value set by hand alone
func (e *editor) deployment(a answers, c catalog, backend string, files []string) {
	names := []string{a.shop}
	for _, s := range c.storefronts {
		names = append(names, s.Name)
	}
	cur := e.get(".env", "COMPOSE_PROFILES")
	profiles := slices.DeleteFunc(split(cur), func(p string) bool { return slices.Contains(names, p) || p == "dashboard" })
	if a.target == "small" {
		profiles = slices.DeleteFunc(profiles, func(p string) bool { return p == "valkey" })
		e.set(backend, "COMPOSE_PROFILES", join(slices.DeleteFunc(split(e.get(backend, "COMPOSE_PROFILES")), func(p string) bool { return p == "valkey" })))
	}
	if join(profiles) != cur {
		e.set(".env", "COMPOSE_PROFILES", join(profiles))
	}
	e.shape(a.target, backend)
	shop := primaryStorefront(a, c)
	if shop == "" {
		return
	}
	ours := func(v string) bool {
		return v == "" || strings.HasPrefix(v, storefrontOnHost) || strings.HasPrefix(v, "http://"+storefrontOnHost) ||
			slices.ContainsFunc(names, func(n string) bool { return n != "" && (v == n+":3000" || v == "http://"+n+":3000/api/revalidate") })
	}
	if ours(e.get(backend, "STOREFRONT_REVALIDATE_URL")) {
		e.set(backend, "STOREFRONT_REVALIDATE_URL", "http://"+storefrontOnHost+"/api/revalidate")
	}
	if caddy := "env/prod-caddy.env"; slices.Contains(files, caddy) && ours(e.get(caddy, "STOREFRONT_UPSTREAM")) {
		e.set(caddy, "STOREFRONT_UPSTREAM", storefrontOnHost)
	}
	fmt.Fprintf(e.out, "    storefront %s - its server on the host's :3000, the edge proxying to it, the backend purging it\n", shop)
}

// Where the containers reach a storefront's server running on the same machine
const storefrontOnHost = "host.docker.internal:3000"

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

// EnvFiles is the backend env file for target and every env file it needs. Beyond dev that is whatever the target's
// compose file names, so a shop added with skelet add channel is set up like the one it was forked from
func EnvFiles(root, target string, runs func(app string) bool) (string, []string, error) {
	if target == "dev" {
		return "env/go.env", []string{"env/go.env", "env/db.env", "env/signals.env"}, nil
	}
	files := []string{CoreEnv}
	if target == "prod" {
		files = append(files, SignalsEnv)
	}
	raw, err := os.ReadFile(filepath.Join(root, "docker-compose."+target+".yml"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", nil, err
	}
	var named []string
	for _, m := range envRef.FindAll(raw, -1) {
		if f := string(m); !slices.Contains(named, f) {
			named = append(named, f)
		}
	}
	slices.Sort(named)
	frontends := map[string]string{"dashboard": "apps/dashboard"}
	for _, s := range storefronts.List(root) {
		frontends[s.Name] = s.Dir
	}
	var own []string
	for name, dir := range frontends {
		f := dir + "/.env.production"
		if _, err := os.Stat(filepath.Join(root, f+".example")); err == nil && (runs == nil || runs(name)) {
			own = append(own, f)
		}
	}
	slices.Sort(own)
	return CoreEnv, append(append(files, named...), own...), nil
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

func (e *editor) secrets(backend, target string) {
	fmt.Fprintln(e.out, "==> Secrets")
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

	if target == "dev" {
		// The Signals processes' own file carries what they share with core - the public key, never the private one
		for _, k := range []string{"CUSTOMER_JWT_PUBLIC_KEY", "EMAIL_LINK_SECRET", "SIGNALS_SERVICE_SECRET"} {
			if v := e.get(backend, k); v != "" {
				e.set("env/signals.env", k, v)
			}
		}
		e.del("env/signals.env", "JWT_SECRET")
		return
	}

	if placeholder(e.get(backend, "BACKUP_ENCRYPTION_PASSPHRASE")) {
		e.set(backend, "BACKUP_ENCRYPTION_PASSPHRASE", e.hex(32))
		fmt.Fprintln(e.out, "    BACKUP_ENCRYPTION_PASSPHRASE generated - store it somewhere you can restore from")
	}
	own := []string{"TRUSTED_PROXY_KEY", "STORAGE_SIGNING_KEY", "DB_PASSWORD"}
	if target == "prod" {
		own = append(own, "VALKEY_PASSWORD")
	}
	e.generateMissing(backend, own...)
	if target == "prod" {
		for _, k := range []string{"CUSTOMER_JWT_PUBLIC_KEY", "EMAIL_LINK_SECRET", "SIGNALS_SERVICE_SECRET", "TRUSTED_PROXY_KEY"} {
			e.set(SignalsEnv, k, e.get(backend, k))
		}
		e.generateMissing(SignalsEnv, "SIGNALS_DB_PASSWORD", "CLICKHOUSE_PASSWORD", "SIGNALS_VALKEY_PASSWORD")
		if placeholder(e.get(SignalsEnv, "VALKEY_URL")) {
			e.set(SignalsEnv, "VALKEY_URL", "redis://:"+e.get(backend, "VALKEY_PASSWORD")+"@valkey:6379")
		}
	}
	for _, f := range e.frontends {
		for _, k := range []string{"REVIEW_VOTER_TOKEN_SECRET", "STOREFRONT_REVALIDATE_SECRET", "TRUSTED_PROXY_KEY"} {
			if _, listed, _ := envfile.Get(filepath.Join(e.root, f), k); listed {
				e.set(f, k, e.get(backend, k))
			}
		}
	}
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

func (e *editor) rehome(target string) {
	for _, r := range rehomed {
		v, found, err := envfile.Get(filepath.Join(e.root, ".env"), r.key)
		if e.err == nil {
			e.err = err
		}
		if !found {
			continue
		}
		if v != "" && (r.app == CoreEnv || target == "prod") && placeholder(e.get(r.app, r.key)) {
			e.set(r.app, r.key, v)
			fmt.Fprintf(e.out, "    %s moved from .env to %s\n", r.key, r.app)
		}
		if !r.stays {
			e.del(".env", r.key)
		}
	}
	for _, old := range []string{"env/" + target + ".env", "env/prod-signals.env"} {
		if _, err := os.Stat(filepath.Join(e.root, old)); err == nil {
			fmt.Fprintf(e.out, "    %s is no longer read - its settings belong in %s or %s\n", old, CoreEnv, SignalsEnv)
		}
	}
}

// The small shape is one app container doing every job against Postgres alone - a later prod run puts the tiers back
var smallShape = [][2]string{{"WORKER_REPLICAS", "0"}, {"MEDIA_REPLICAS", "0"}, {"JOBS_EXTERNAL", "false"},
	{"VALKEY_URL", ""}, {"VALKEY_CACHE_URL", ""}, {"VALKEY_LIMITS_URL", ""}}

func (e *editor) shape(target, backend string) {
	if target == "small" {
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
