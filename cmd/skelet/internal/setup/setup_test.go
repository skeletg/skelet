package setup

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/plugins"
	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
	"github.com/skeletg/skelet/cmd/skelet/internal/secret"
)

const repoRoot = "../../../.."

// A checkout reduced to what setup reads, with the real templates - a template change setup can't handle fails here
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	copyFile := func(rel string) {
		raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		mkfile(t, root, rel, string(raw))
	}
	templates, _ := filepath.Glob(filepath.Join(repoRoot, "env", "*.example"))
	for _, f := range templates {
		copyFile("env/" + filepath.Base(f))
	}
	for _, f := range []string{".env.example", "docker-compose.small.yml", "docker-compose.prod.yml", plugins.File} {
		copyFile(f)
	}
	mkfile(t, root, plugins.Dir+"/example/plugin.go", "package example\nfunc init() { sdk.RegisterPlugin(nil) }\n")
	mkfile(t, root, plugins.Dir+"/payment-stub/plugin.go", "package stub\nfunc init() { sdk.RegisterPlugin(nil) }\n")
	return root
}

func mkfile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func dev() answers {
	return answers{target: "dev", signals: true, ai: true, search: "opensearch", apps: []string{"dashboard"}, dockerApps: []string{"dashboard"}}
}

func write(t *testing.T, root string, a answers) written {
	t.Helper()
	c, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	w, err := Write(root, a, c, rand.Reader, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func env(t *testing.T, root, file string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, file))
	if err != nil {
		t.Fatal(err)
	}
	vals := map[string]string{}
	for line := range strings.Lines(string(raw)) {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && !strings.HasPrefix(k, "#") {
			vals[k] = v
		}
	}
	return vals
}

// The backend's own startup rules (lib/envcheck), restated so a value setup writes can't be one core refuses
func checkSecrets(t *testing.T, where string, v map[string]string) {
	t.Helper()
	for _, k := range []string{"ADMIN_JWT_SECRET", "EMAIL_LINK_SECRET", "REVIEW_VOTER_TOKEN_SECRET", "SIGNALS_SERVICE_SECRET"} {
		if s := v[k]; len(s) < 32 || strings.HasPrefix(s, "change-me") {
			t.Errorf("%s %s = %q is not a strong secret", where, k, s)
		}
	}
	keys := []string{"ADMIN_JWT_SECRET", "EMAIL_LINK_SECRET", "REVIEW_VOTER_TOKEN_SECRET", "SIGNALS_SERVICE_SECRET"}
	for i, a := range keys {
		for _, b := range keys[i+1:] {
			if v[a] == v[b] {
				t.Errorf("%s %s and %s are the same value", where, a, b)
			}
		}
	}
	for _, k := range []string{"CONFIG_ENCRYPTION_KEY", "CUSTOMER_JWT_PRIVATE_KEY", "CUSTOMER_JWT_PUBLIC_KEY"} {
		if b, err := base64.StdEncoding.DecodeString(v[k]); err != nil || len(b) != 32 {
			t.Errorf("%s %s = %q is not base64 of 32 bytes", where, k, v[k])
		}
	}
	if pub, _ := secret.CustomerPublic(v["CUSTOMER_JWT_PRIVATE_KEY"]); pub != v["CUSTOMER_JWT_PUBLIC_KEY"] {
		t.Errorf("%s CUSTOMER_JWT_PUBLIC_KEY isn't the private key's public half", where)
	}
	if _, ok := v["JWT_SECRET"]; ok {
		t.Errorf("%s still holds the retired JWT_SECRET", where)
	}
}

func TestDevWritesWhatTheBackendAccepts(t *testing.T) {
	root := fixture(t)
	write(t, root, dev())
	g := env(t, root, "env/go.env")
	checkSecrets(t, "env/go.env", g)
	if g["SIGNALS_ENABLED"] != "true" || g["AI_ENABLED"] != "true" {
		t.Errorf("suites: signals=%q ai=%q", g["SIGNALS_ENABLED"], g["AI_ENABLED"])
	}
	s := env(t, root, "env/signals.env")
	for _, k := range []string{"CUSTOMER_JWT_PUBLIC_KEY", "EMAIL_LINK_SECRET", "SIGNALS_SERVICE_SECRET"} {
		if s[k] != g[k] {
			t.Errorf("env/signals.env %s differs from env/go.env's", k)
		}
	}
	if s["CUSTOMER_JWT_PRIVATE_KEY"] != "" {
		t.Error("the Signals tier was handed the private key")
	}
	dotenv := env(t, root, ".env")
	for _, k := range []string{"REVIEW_VOTER_TOKEN_SECRET", "STOREFRONT_REVALIDATE_SECRET"} {
		if dotenv[k] == "" || dotenv[k] != g[k] {
			t.Errorf(".env %s doesn't match env/go.env's, so the dev storefront and core disagree", k)
		}
	}
	for _, f := range []string{"env/go.env", "env/db.env", "env/signals.env", ".env"} {
		if fi, err := os.Stat(filepath.Join(root, f)); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v, mode %v - want 0600", f, err, fi.Mode().Perm())
		}
	}
	prof, _ := profile.Load(filepath.Join(root, profile.File))
	if prof[kTarget] != "dev" || prof[kApps] != "dashboard" || prof[kDocker] != "dashboard" {
		t.Errorf("profile: %v", prof)
	}
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(p)
		fi, _ := d.Info()
		files[p] = fi.Mode().String() + "\n" + string(raw)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestASecondRunChangesNothing(t *testing.T) {
	for _, target := range []string{"dev", "small", "prod"} {
		root := fixture(t)
		a := dev()
		a.target, a.adminSlug, a.gateways = target, true, []string{"payment-stub"}
		first := write(t, root, a)
		before := snapshot(t, root)
		again := write(t, root, a)
		if after := snapshot(t, root); !reflect.DeepEqual(before, after) {
			for p := range after {
				if before[p] != after[p] {
					t.Errorf("%s: the second run changed %s", target, p)
				}
			}
		}
		if again.pluginsChanged || again.adminSlug != first.adminSlug {
			t.Errorf("%s: second run %+v after %+v", target, again, first)
		}
	}
}

var required = regexp.MustCompile(`\$\{([A-Z_]+):\?`)

func TestSmallAndProdFillTheRootEnvComposeReads(t *testing.T) {
	for _, target := range []string{"small", "prod"} {
		root := fixture(t)
		a := dev()
		a.target = target
		write(t, root, a)
		backend := env(t, root, "env/"+target+".env")
		dotenv := env(t, root, ".env")
		checkSecrets(t, target+" .env", dotenv)
		if dotenv["COMPOSE_PROFILES"] != "bundled" {
			t.Errorf("%s: .env COMPOSE_PROFILES = %q - without bundled the datastores never start", target, dotenv["COMPOSE_PROFILES"])
		}
		compose, _ := os.ReadFile(filepath.Join(root, "docker-compose."+target+".yml"))
		for _, m := range required.FindAllSubmatch(compose, -1) {
			if dotenv[string(m[1])] == "" {
				t.Errorf("%s: compose demands %s, and .env leaves it empty", target, m[1])
			}
		}
		for _, k := range []string{"ADMIN_JWT_SECRET", "CUSTOMER_JWT_PRIVATE_KEY", "CUSTOMER_JWT_PUBLIC_KEY", "EMAIL_LINK_SECRET",
			"REVIEW_VOTER_TOKEN_SECRET", "CONFIG_ENCRYPTION_KEY", "SIGNALS_SERVICE_SECRET", "STOREFRONT_REVALIDATE_SECRET"} {
			if dotenv[k] != backend[k] {
				t.Errorf("%s: .env %s differs from env/%s.env's - compose's environment: would override it", target, k, target)
			}
		}
		if b, err := base64.StdEncoding.DecodeString(dotenv["SIGNALS_ENCRYPTION_KEY"]); err != nil || len(b) != 32 || dotenv["SIGNALS_ENCRYPTION_KEY"] == dotenv["CONFIG_ENCRYPTION_KEY"] {
			t.Errorf("%s: SIGNALS_ENCRYPTION_KEY must be its own 32-byte key", target)
		}
		for _, k := range []string{"DB_PASSWORD", "SIGNALS_DB_PASSWORD", "VALKEY_PASSWORD", "CLICKHOUSE_PASSWORD", "STORAGE_SIGNING_KEY"} {
			if len(dotenv[k]) != 64 {
				t.Errorf("%s: .env %s = %q", target, k, dotenv[k])
			}
		}
		if len(backend["BACKUP_ENCRYPTION_PASSPHRASE"]) != 64 {
			t.Errorf("%s: no backup passphrase", target)
		}
		_, files, _ := EnvFiles(root, target)
		if len(files) < 2 {
			t.Errorf("%s: only %v - the compose file's env_file list was not read", target, files)
		}
		for _, f := range files {
			if fi, err := os.Stat(filepath.Join(root, f)); err != nil || fi.Mode().Perm() != 0o600 {
				t.Errorf("%s: %s: %v", target, f, err)
			}
		}
	}
}

func TestSearchAndAdminSlugFollowTheAnswerBothWays(t *testing.T) {
	root := fixture(t)
	a := dev()
	a.search, a.adminSlug = "postgres", true
	w := write(t, root, a)
	v, ok, _ := envfile.Get(filepath.Join(root, ".env"), "OPENSEARCH_URL")
	if !ok || v != "" {
		t.Errorf("postgres search: OPENSEARCH_URL = %q, %v - want present and empty", v, ok)
	}
	if !regexp.MustCompile(`^admin-[0-9a-f]{16}$`).MatchString(w.adminSlug) {
		t.Errorf("slug %q", w.adminSlug)
	}
	d := env(t, root, ".env")
	if d["ADMIN_API_SLUG"] != w.adminSlug || d["VITE_GRAPHQL_URL"] != "/api/admin/"+w.adminSlug+"/graphql" ||
		d["GRAPHQL_CODEGEN_URL"] != "http://app:8080/api/admin/"+w.adminSlug+"/graphql" {
		t.Errorf(".env slug keys: %v", d)
	}

	a.search, a.adminSlug = "opensearch", false
	write(t, root, a)
	for _, k := range []string{"OPENSEARCH_URL", "ADMIN_API_SLUG", "VITE_GRAPHQL_URL", "GRAPHQL_CODEGEN_URL"} {
		if _, ok, _ := envfile.Get(filepath.Join(root, ".env"), k); ok {
			t.Errorf("answering back left %s in .env", k)
		}
	}
}

func TestOperatorValuesStayAndPlaceholdersGo(t *testing.T) {
	root := fixture(t)
	mine := strings.Repeat("m", 40)
	tmpl, _ := os.ReadFile(filepath.Join(root, "env/go.env.example"))
	mkfile(t, root, "env/go.env", string(tmpl)+"ADMIN_JWT_SECRET="+mine+"\nJWT_SECRET=old\n")
	mkfile(t, root, "env/signals.env", "JWT_SECRET=old\nEMAIL_LINK_SECRET=change-me\n")
	write(t, root, dev())
	g := env(t, root, "env/go.env")
	if g["ADMIN_JWT_SECRET"] != mine {
		t.Errorf("an operator's ADMIN_JWT_SECRET was replaced")
	}
	checkSecrets(t, "env/go.env", g)
	if s := env(t, root, "env/signals.env"); s["JWT_SECRET"] != "" || s["EMAIL_LINK_SECRET"] != g["EMAIL_LINK_SECRET"] {
		t.Errorf("env/signals.env: %v", s)
	}
}

func TestPluginsJSONIsMergedInPluginsetsForm(t *testing.T) {
	root := fixture(t)
	mkfile(t, root, plugins.File, `{"plugins": {"example": 0, "_example": 0, "loyalty": {"enabled": true, "module": "example.com/loyalty"}}}`)
	a := dev()
	a.gateways = []string{"payment-stub"}
	if w := write(t, root, a); !w.pluginsChanged {
		t.Error("picking a gateway didn't change plugins.json")
	}
	raw, _ := os.ReadFile(filepath.Join(root, plugins.File))
	want := `{
  "plugins": {
    "example": {
      "enabled": false
    },
    "loyalty": {
      "enabled": true,
      "module": "example.com/loyalty"
    },
    "payment-stub": {
      "enabled": true
    }
  }
}
`
	if string(raw) != want {
		t.Errorf("plugins.json:\n%s", raw)
	}
	if w := write(t, root, a); w.pluginsChanged {
		t.Error("the same picks changed plugins.json again")
	}
}

func TestParseFlags(t *testing.T) {
	c := catalog{storefronts: []string{"vetrina-demo"}}
	f, err := parseFlags([]string{"--apps", "vetrina-demo,dashboard", "--docker-apps=none", "--tools", "mailpit", "--keep-code", "-y"}, c)
	if err != nil || !reflect.DeepEqual(f.apps, list{true, []string{"vetrina-demo", "dashboard"}}) ||
		!reflect.DeepEqual(f.dockerApps, list{true, nil}) || !reflect.DeepEqual(f.tools, list{true, []string{"mailpit"}}) || !f.keepCode || !f.yes {
		t.Errorf("parsed %+v, %v", f, err)
	}
	if f, _ := parseFlags([]string{"--backend-only"}, c); !f.apps.set || f.apps.items != nil {
		t.Errorf("--backend-only: %+v", f.apps)
	}
	if f, _ := parseFlags([]string{"--all"}, c); !reflect.DeepEqual(f.apps.items, []string{"dashboard", "vetrina-demo"}) {
		t.Errorf("--all: %+v", f.apps)
	}
	for args, want := range map[string]string{
		"--apps":          "--apps needs a value (e.g. dashboard,vetrina-demo or none)",
		"--apps shop":     `Unknown app "shop" (valid: dashboard, vetrina-demo, none)`,
		"--tools jaeger":  `Unknown tool "jaeger" (valid: mailpit, webhook-tester, grafana, garage-webui, none)`,
		"--profile":       "--profile needs a file",
		"--profile /nope": "No such profile: /nope",
		"--bogus":         "Unknown flag: --bogus (try --all, --backend-only, --apps LIST, --docker-apps LIST, --tools LIST, --keep-code, --yes, --profile FILE)",
	} {
		if _, err := parseFlags(strings.Fields(args), c); err == nil || err.Error() != want {
			t.Errorf("%s: %v\nwant %s", args, err, want)
		}
	}
}

func quiet() prompter { return prompter{in: bufio.NewReader(strings.NewReader("")), out: io.Discard} }

// A profile from before the placement question has no SKELET_DOCKER_APPS - a --yes run used to empty .skelet-apps then,
// moving every containerized app to a host that may not have Node
func TestYesKeepsTheDockerPlacementTheProfileDoesntRecord(t *testing.T) {
	root := fixture(t)
	mkfile(t, root, ".skelet-apps", "dashboard\n")
	c, _ := scan(root)
	a, err := answer(root, c, flags{yes: true}, profile.Profile{kTarget: "dev"}, "p", quiet(), io.Discard)
	if err != nil || !reflect.DeepEqual(a.dockerApps, []string{"dashboard"}) {
		t.Errorf("docker apps %v, %v - want the dashboard kept in Docker", a.dockerApps, err)
	}
}

func TestProfileAnswers(t *testing.T) {
	root := fixture(t)
	c, _ := scan(root)
	p := profile.Profile{kTarget: "small", kSignals: "false", kSearch: "postgres", kApps: "dashboard,gone", kDocker: "dashboard", kSlug: "1", kTools: "mailpit"}
	a, err := answer(root, c, flags{yes: true}, p, "p", quiet(), io.Discard)
	if err != nil || a.target != "small" || a.signals || a.search != "postgres" || !a.adminSlug ||
		!reflect.DeepEqual(a.apps, []string{"dashboard"}) || !a.fromProfile || a.prune || !reflect.DeepEqual(a.tools, []string{"mailpit"}) {
		t.Errorf("answers %+v, %v", a, err)
	}
	for k, v := range map[string]string{kTarget: "staging", kSignals: "yes", kSearch: "elastic", kSlug: "true"} {
		if _, err := answer(root, c, flags{yes: true}, profile.Profile{k: v}, "p", quiet(), io.Discard); err == nil {
			t.Errorf("%s=%s accepted", k, v)
		}
	}
}

// --profile replays a file without touching it - the answers land in .skelet-profile, where the next run reads them
func TestRunReplaysAProfileIntoSkeletProfile(t *testing.T) {
	root := fixture(t)
	replay := filepath.Join(t.TempDir(), "shop.profile")
	body := "SKELET_TARGET=prod\nSKELET_APPS_SELECTED=dashboard\nSKELET_DOCKER_APPS=dashboard\n"
	if err := os.WriteFile(replay, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run(root, []string{"--profile", replay}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if raw, _ := os.ReadFile(replay); string(raw) != body {
		t.Error("the replayed file was rewritten")
	}
	saved, _ := profile.Load(filepath.Join(root, profile.File))
	if saved[kTarget] != "prod" || saved[kApps] != "dashboard" {
		t.Errorf(".skelet-profile: %v", saved)
	}
	if code := Run(root, []string{"--apps", "nope"}); code != 2 {
		t.Errorf("a bad flag: exit %d, want 2", code)
	}
}
