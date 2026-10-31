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

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/plugins"
	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
	"github.com/skeletg/skelet/cmd/skelet/internal/secret"
)

// What a write leaves for the boot and the summary
type written struct {
	pluginsChanged bool
	adminSlug      string
}

// Write turns answers into files: the selection files, the profile, the target's env files with their secrets, the
// root .env's knobs and plugins.json. It starts nothing and asks nothing. Anything already holding a real value is
// left alone, so a second run with the same answers changes no file
func Write(root string, a answers, c catalog, rnd io.Reader, out io.Writer) (written, error) {
	var w written
	path := func(rel string) string { return filepath.Join(root, rel) }

	if err := writeLine(path(".skelet-apps"), join(a.dockerApps)); err != nil {
		return w, err
	}
	fmt.Fprintf(out, "==> Apps: %s\n", orNone(a.apps, "backend only"))
	fmt.Fprintf(out, "    in Docker: %s  (saved to .skelet-apps)\n", orNone(a.dockerApps, "none - they run on the host with `skelet serve <app>`"))
	if err := writeLine(path(".skelet-tools"), join(a.tools)); err != nil {
		return w, err
	}
	fmt.Fprintf(out, "==> Dev tools: %s  (saved to .skelet-tools; start one ad hoc with skelet up <name>)\n", orNone(a.tools, "none"))

	if err := profile.Write(path(profile.File), a.profile(), profileKeys); err != nil {
		return w, err
	}
	fmt.Fprintf(out, "==> Profile: %s, signals=%t ai=%t, search=%s  (saved to %s)\n", a.target, a.signals, a.ai, a.search, profile.File)

	backend, files, err := EnvFiles(root, a.target)
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
	// Written on every run, not only when they leave the default - otherwise re-running and changing an answer back
	// would silently do nothing
	if a.search == "postgres" {
		e.set(".env", "OPENSEARCH_URL", "")
		fmt.Fprintln(out, "    search pinned to the Postgres tier")
	} else {
		// compose reads ${OPENSEARCH_URL-http://opensearch:9200}: absent means the bundled cluster, set-but-empty
		// pins search to Postgres
		e.del(".env", "OPENSEARCH_URL")
		fmt.Fprintln(out, "    search on the bundled OpenSearch")
	}
	if a.adminSlug {
		w.adminSlug = e.get(".env", "ADMIN_API_SLUG")
		if w.adminSlug == "" {
			w.adminSlug = "admin-" + e.hex(8)
			fmt.Fprintf(out, "    admin API moved to /api/admin/%s/graphql\n", w.adminSlug)
		} else {
			fmt.Fprintln(out, "    admin API slug already set, left alone")
		}
		e.set(".env", "ADMIN_API_SLUG", w.adminSlug)
		e.set(".env", "VITE_GRAPHQL_URL", "/api/admin/"+w.adminSlug+"/graphql")
		e.set(".env", "GRAPHQL_CODEGEN_URL", "http://app:8080/api/admin/"+w.adminSlug+"/graphql")
	} else {
		// All three read ${VAR:-default} in compose, so deleting them puts the API back at /api/admin/graphql
		for _, k := range []string{"ADMIN_API_SLUG", "VITE_GRAPHQL_URL", "GRAPHQL_CODEGEN_URL"} {
			e.del(".env", k)
		}
	}
	if e.err != nil {
		return w, e.err
	}

	w.pluginsChanged, err = decidePlugins(root, c, a.gateways, out)
	return w, err
}

func (a answers) profile() profile.Profile {
	slug := "0"
	if a.adminSlug {
		slug = "1"
	}
	return profile.Profile{
		kTarget: a.target, kSignals: strconv.FormatBool(a.signals), kAI: strconv.FormatBool(a.ai), kSearch: a.search,
		kGates: join(a.gateways), kSlug: slug, kApps: join(a.apps), kDocker: join(a.dockerApps), kTools: join(a.tools),
	}
}

var envRef = regexp.MustCompile(`env/[a-z0-9.-]+\.env`)

// EnvFiles is the backend env file for target and every env file it needs. Beyond dev that is whatever the target's
// compose file names, so a shop added with skelet add channel is set up like the one it was forked from
func EnvFiles(root, target string) (string, []string, error) {
	if target == "dev" {
		return "env/go.env", []string{"env/go.env", "env/db.env", "env/signals.env"}, nil
	}
	backend := "env/" + target + ".env"
	files := []string{backend}
	raw, err := os.ReadFile(filepath.Join(root, "docker-compose."+target+".yml"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", nil, err
	}
	var named []string
	for _, m := range envRef.FindAll(raw, -1) {
		if f := string(m); !slices.Contains(files, f) && !slices.Contains(named, f) {
			named = append(named, f)
		}
	}
	slices.Sort(named)
	return backend, append(files, named...), nil
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

func writeLine(path, line string) error {
	data := line + "\n"
	if old, err := os.ReadFile(path); err == nil && string(old) == data {
		return nil
	}
	return envfile.WriteAtomic(path, []byte(data), 0o644)
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
	root string
	rnd  io.Reader
	out  io.Writer
	err  error
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

// generate makes a value in the encoding key's checks demand: the two AES keys and the Ed25519 seed are base64 of 32
// bytes, everything else is 32 bytes of hex
func (e *editor) generate(key string) string {
	switch key {
	case "CONFIG_ENCRYPTION_KEY", "SIGNALS_ENCRYPTION_KEY", "CUSTOMER_JWT_PRIVATE_KEY":
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
	// The root .env is the only file compose interpolates, and small and prod build their datastore URLs and their
	// containers' secrets out of it - a secret only in env_file is invisible to ${VAR:?}, and compose's environment:
	// beats env_file, so one missing here reaches the container empty
	e.del(".env", "JWT_SECRET")
	shared := []string{"ADMIN_JWT_SECRET", "CUSTOMER_JWT_PRIVATE_KEY", "CUSTOMER_JWT_PUBLIC_KEY", "EMAIL_LINK_SECRET",
		"REVIEW_VOTER_TOKEN_SECRET", "CONFIG_ENCRYPTION_KEY", "SIGNALS_SERVICE_SECRET", "STOREFRONT_REVALIDATE_SECRET"}
	for _, k := range append(slices.Clone(shared), "SIGNALS_ENCRYPTION_KEY", "STORAGE_SIGNING_KEY", "DB_PASSWORD",
		"SIGNALS_DB_PASSWORD", "VALKEY_PASSWORD", "CLICKHOUSE_PASSWORD") {
		if !placeholder(e.get(".env", k)) {
			fmt.Fprintf(e.out, "    .env %s already set, left alone\n", k)
			continue
		}
		// The ones the backend also reads must match it there, not be generated twice
		v := ""
		if slices.Contains(shared, k) {
			v = e.get(backend, k)
		}
		if v == "" {
			v = e.generate(k)
		}
		e.set(".env", k, v)
		fmt.Fprintf(e.out, "    .env %s set\n", k)
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
