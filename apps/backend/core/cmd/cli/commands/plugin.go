package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"slices"

	"github.com/spf13/cobra"

	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/pluginmigrate"
	"github.com/skeletg/skelet/apps/backend/core/internal/pluginset"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Plugin-aware behaviour (config registration, provider registry) lives wherever the plugin's init() runs - the CLI only scaffolds and migrates
var PluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Tier 1 plugin scaffolding and management",
}

var pluginNewCmd = &cobra.Command{
	Use:   "new <slug>",
	Short: "Generate boilerplate for a new Tier 1 plugin",
	Long: `Creates plugins/<slug>/ with a starter set of files matching
the plugin contract.

Layout produced (depending on flags):

  plugins/<slug>/
    plugin.go                          init() - registry calls
    provider.go                        (--payment-provider)
    migrations.go                      (--with-migrations)
    migrations/
      0001_initial.up.sql              (--with-migrations)
      0001_initial.down.sql            (--with-migrations)

The plugin is enabled in plugins/plugins.json and plugins/firstparty is
regenerated, so there is nothing to paste by hand - the command finishes
with a checklist of what is left.`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginNew,
}

func init() {
	PluginCmd.AddCommand(pluginMigrateCmd)
	PluginCmd.AddCommand(pluginNewCmd)
	PluginCmd.AddCommand(pluginUninstallCmd)
	PluginCmd.AddCommand(pluginListCmd)
	PluginCmd.AddCommand(pluginEnableCmd)
	PluginCmd.AddCommand(pluginDisableCmd)

	pluginNewCmd.Flags().String("name", "", "Display name shown in admin UIs (default: title-cased slug)")
	pluginNewCmd.Flags().Bool("payment-provider", false, "Scaffold a payment provider (provider.go + provider registrations in plugin.go)")
	pluginNewCmd.Flags().Bool("with-migrations", false, "Scaffold SQL migrations dir + embed glue")
	pluginNewCmd.Flags().Bool("force", false, "Overwrite an existing plugins/<slug>/ directory")

	pluginUninstallCmd.Flags().Bool("yes", false, "Skip the confirmation prompt")
	pluginUninstallCmd.Flags().Bool("keep-config", false, "Don't delete the plugin's stored config rows")
	pluginUninstallCmd.Flags().StringSlice("config-prefix", nil, "Extra config path prefixes to delete, e.g. payment/stripe - the default sweep only covers plugin/<namespace>/*, and payment plugins register under payment/<provider-slug>/* by SDK convention")
}

var pluginUninstallCmd = &cobra.Command{
	Use:   "uninstall <namespace>",
	Short: "Roll back every migration + delete config rows for one plugin",
	Long: `Reverses the plugin's persistent footprint:

  1. Runs down migrations for every applied version (newest → oldest).
  2. Deletes every config_values row under "plugin/<namespace>/*" and
     anything else under that namespace's prefix (default scope only;
     per-store overrides are deleted too).

Does NOT remove the plugin's Go code - "skelet cli plugin disable <slug>" does
that, then rebuild.
Run uninstall BEFORE disabling or deleting the plugin's directory; the down
migrations need the plugin's FS to find the .down.sql files, and a plugin whose
code is already gone can only be cleaned up by hand via skelet psql.

Payment plugins keep credentials under payment/<provider-slug>/* rather than
plugin/<namespace>/* - pass --config-prefix payment/<provider-slug> to sweep
those too (encrypted secrets included).

Confirmation prompts before destructive action unless --yes is set.`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginUninstall,
}

var pluginEnableCmd = &cobra.Command{
	Use:   "enable <slug>",
	Short: "Compile a plugin into the build",
	Long: `Sets the plugin to 1 in plugins/plugins.json and regenerates plugins/firstparty.

plugins.json is the decision, firstparty is derived from it. Rebuild to pick the change up.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return setPluginState(args[0], true) },
}

var pluginDisableCmd = &cobra.Command{
	Use:   "disable <slug>",
	Short: "Leave a plugin in the tree but out of the build",
	Long: `Sets the plugin to 0 in plugins/plugins.json, regenerates plugins/firstparty, and
moves the plugin to plugins/_<slug>, where the go tool ignores it.

The move is not tidiness. A plugin that ships GraphQL has resolvers returning core's
generated model types, and go build ./... compiles every package in the module
whether the binary links it or not - so a plugin left in place stops compiling the
moment its schema is dropped.

Its code stays on disk either way, so "skelet cli plugin uninstall" can still reach the
down migrations - run that BEFORE disabling if you also want its tables and config
rows gone.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return setPluginState(args[0], false) },
}

// Writes the decision, then derives - never the other way round
func setPluginState(slug string, on bool) error {
	set, err := pluginset.Load(".")
	if err != nil {
		return err
	}
	// A module plugin has no directory to look for - the decision file is the only record of it, so
	// that is what the existence check reads
	_, known := set.Plugins[slug]
	inTree := set.InTree(slug)
	if inTree {
		found, err := pluginset.Discover(".")
		if err != nil {
			return err
		}
		parked, err := pluginset.Parked(".")
		if err != nil {
			return err
		}
		if !slices.Contains(found, slug) && !slices.Contains(parked, slug) {
			return fmt.Errorf("plugins/%s registers no plugin - known: %s", slug, strings.Join(found, ", "))
		}
	} else if !known {
		return fmt.Errorf("no plugin %q in %s", slug, pluginset.StateFile)
	}
	set.Set(slug, on)
	if err := set.Save("."); err != nil {
		return err
	}
	state := "disabled"
	if inTree {
		// An in-tree plugin shipping GraphQL cannot merely stop being imported: its resolvers return core's generated model types, so it has to leave the build entirely or it stops compiling once its schema is gone. A module plugin needs none of this - dropping the import is enough, because its source was never part of this module's build in the first place
		if err := pluginset.Park(".", slug, on); err != nil {
			return err
		}
		state = "disabled and parked as plugins/_" + slug
	}
	if err := pluginset.Stage("."); err != nil {
		return err
	}
	if err := pluginset.Generate("."); err != nil {
		return err
	}
	if err := pluginset.WriteGraphSchemas("."); err != nil {
		return err
	}
	if on {
		state = "enabled"
	}
	fmt.Printf("%s %s in %s - run skelet gen, then rebuild\n", slug, state, pluginset.StateFile)
	return nil
}

// The read side of Manifest - without it Capabilities is a field operators are told to audit and given no way to see
var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show every plugin compiled into this binary and what its manifest declares",
	Long: `Prints each plugin's manifest in dependency order: version, SDK range,
what it does when the kill switch is unreadable, its dependencies, the events it
emits and consumes, and its declared capabilities.

Capabilities are a DISCLOSURE, not a sandbox. A Tier 1 plugin is compiled into
the process with full privileges, so no list constrains what it actually does -
read it the way you'd read a README, and audit the code for anything you don't
trust.`,
	Args: cobra.NoArgs,
	RunE: runPluginList,
}

func runPluginList(cmd *cobra.Command, args []string) error {
	manifests := pluginhost.Manifests()
	if len(manifests) == 0 {
		fmt.Println("No plugins are compiled into this binary.")
		return nil
	}
	bySlug := make(map[string]sdk.Manifest, len(manifests))
	for _, m := range manifests {
		bySlug[m.Slug] = m
	}

	// Dependency order, so a plugin is never described before what it builds on
	first := true
	for _, slug := range pluginhost.OrderedManifestSlugs() {
		m, ok := bySlug[slug]
		if !ok {
			continue
		}
		if !first {
			fmt.Println()
		}
		first = false
		printManifest(m)
	}

	// The check the server runs at boot, so an SDK mismatch or missing dep surfaces here instead of as a crash loop
	if err := pluginhost.ValidateManifests(); err != nil {
		fmt.Printf("\nWARNING: this set would fail to boot: %v\n", err)
	}
	return nil
}

func printManifest(m sdk.Manifest) {
	name := m.Name
	if name == "" {
		name = m.Slug
	}
	fmt.Printf("%s  (%s)\n", m.Slug, name)
	fmt.Printf("  version           %s\n", m.Version)
	fmt.Printf("  sdk range         %s   (host SDK %s)\n", m.SDKRange, sdk.Version)
	policy := "fail_open - keeps serving"
	if m.FailurePolicy == sdk.FailClosed {
		policy = "fail_closed - refuses"
	}
	fmt.Printf("  kill switch unreadable    %s\n", policy)

	deps := make([]string, 0, len(m.Dependencies))
	for _, d := range m.Dependencies {
		deps = append(deps, fmt.Sprintf("%s %s", d.Slug, d.Version))
	}
	printManifestList("depends on", deps)
	printManifestList("emits", m.Emits)
	printManifestList("consumes", m.Consumes)
	printManifestList("capabilities (declared, not enforced)", m.Capabilities)
}

func printManifestList(label string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Printf("  %s:\n", label)
	for _, v := range values {
		fmt.Printf("    - %s\n", v)
	}
}

var slugPattern = pluginset.SlugPattern

func runPluginNew(cmd *cobra.Command, args []string) error {
	slug := strings.TrimSpace(args[0])
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("invalid slug %q: must match [a-z][a-z0-9-]{1,39}", slug)
	}

	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		name = titleizeSlug(slug)
	}
	paymentProvider, _ := cmd.Flags().GetBool("payment-provider")
	withMigrations, _ := cmd.Flags().GetBool("with-migrations")
	force, _ := cmd.Flags().GetBool("force")

	pluginDir := filepath.Join("plugins", slug)
	if _, err := os.Stat(pluginDir); err == nil && !force {
		return fmt.Errorf("plugin dir %s already exists (pass --force to overwrite)", pluginDir)
	}
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return err
	}

	tmpl := pluginTemplate{
		Slug:            slug,
		Name:            name,
		PackageName:     pluginPackageName(slug),
		PaymentProvider: paymentProvider,
		WithMigrations:  withMigrations,
	}

	files := tmpl.files()
	for path, content := range files {
		full := filepath.Join(pluginDir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", full, err)
		}
		fmt.Printf("  created %s\n", full)
	}

	fmt.Println()
	if err := setPluginState(slug, true); err != nil { // scaffolding it is the decision to ship it - disable later if that turns out wrong
		return fmt.Errorf("enable %s: %w", slug, err)
	}

	fmt.Println()
	fmt.Println("Plugin scaffolded. Next steps:")
	fmt.Println()
	step := 1
	if withMigrations {
		fmt.Printf("  %d. Apply the starter migration:\n", step)
		fmt.Printf("       skelet cli plugin migrate up %s\n", slug)
		fmt.Println()
		step++
	}
	fmt.Printf("  %d. If it extends GraphQL, drop the SDL in plugins/%s/graphql/{admin,front}/\n", step, slug)
	fmt.Println("     and run skelet gen - the core-side delegation and interfaces are generated,")
	fmt.Println("     so there is no resolver to write outside your own package.")
	fmt.Println()
	step++
	fmt.Printf("  %d. skelet up (or skelet restart app) to load the plugin into the running server.\n", step)
	return nil
}

func runPluginUninstall(cmd *cobra.Command, args []string) error {
	namespace := strings.TrimSpace(args[0])
	if namespace == "" {
		return fmt.Errorf("namespace required")
	}
	skipPrompt, _ := cmd.Flags().GetBool("yes")
	keepConfig, _ := cmd.Flags().GetBool("keep-config")

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close()

	ctx := context.Background()

	// Counting the revert through the runner's own Status keeps the confirmation prompt showing the same numbers as `plugin migrate status`
	runner := pluginmigrate.New(conn)
	statuses, err := runner.Status(ctx)
	if err != nil {
		return fmt.Errorf("read migration status: %w", err)
	}
	var applied int
	found := false
	for _, s := range statuses {
		if s.Namespace == namespace {
			applied = len(s.AppliedVersions)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("plugin %q is not registered with the running binary - blank-import it first OR use `skelet psql` to drop tables manually", namespace)
	}

	// the plugin's registrations are attributed, so the sweep enumerates its EXACT config paths -
	// prefixes remain as a belt (plugin/<slug>/ convention, --config-prefix for anything unusual)
	prefixes := []string{"plugin/" + namespace + "/"}
	extraPrefixes, _ := cmd.Flags().GetStringSlice("config-prefix")
	for _, p := range extraPrefixes {
		p = strings.TrimSuffix(strings.TrimSpace(p), "/")
		if p == "" {
			continue
		}
		prefixes = append(prefixes, p+"/")
	}
	ownedPaths := pluginhost.ConfigPathsFor(namespace)

	var configRows int // default + per-store rows both live under the swept prefixes and attributed paths
	if !keepConfig {
		for _, prefix := range prefixes {
			var n int
			row := conn.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM config_values WHERE path LIKE $1`,
				prefix+"%",
			)
			if err := row.Scan(&n); err != nil {
				return fmt.Errorf("count config rows: %w", err)
			}
			configRows += n
		}
		for _, path := range ownedPaths {
			var n int
			row := conn.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM config_values WHERE path = $1`, path)
			if err := row.Scan(&n); err != nil {
				return fmt.Errorf("count config rows: %w", err)
			}
			configRows += n
		}
	}

	fmt.Printf("About to uninstall plugin %q:\n", namespace)
	fmt.Printf("  - roll back %d applied migration(s)\n", applied)
	if !keepConfig {
		fmt.Printf("  - delete %d config row(s) under %s\n", configRows, strings.Join(prefixes, "*, ")+"*")
	}
	fmt.Println()
	if !skipPrompt {
		fmt.Print("Proceed? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Down(steps) reverts the most recent N, so the applied count takes it to zero - a missing down.sql errors out here, before any config row is touched
	if applied > 0 {
		if err := runner.Down(ctx, namespace, applied); err != nil {
			return fmt.Errorf("roll back migrations: %w", err)
		}
		fmt.Printf("Rolled back %d migration(s).\n", applied)
	}

	if !keepConfig {
		var deleted int64
		for _, prefix := range prefixes {
			res, err := conn.ExecContext(ctx,
				`DELETE FROM config_values WHERE path LIKE $1`,
				prefix+"%",
			)
			if err != nil {
				return fmt.Errorf("delete config rows: %w", err)
			}
			n, _ := res.RowsAffected()
			deleted += n
		}
		for _, path := range ownedPaths {
			res, err := conn.ExecContext(ctx,
				`DELETE FROM config_values WHERE path = $1`, path)
			if err != nil {
				return fmt.Errorf("delete config rows: %w", err)
			}
			n, _ := res.RowsAffected()
			deleted += n
		}
		fmt.Printf("Deleted %d config row(s).\n", deleted)
	}

	fmt.Println()
	fmt.Printf("Uninstall of %q complete.\n", namespace)
	fmt.Printf("Next: skelet cli plugin disable %s, then rebuild.\n", namespace)
	return nil
}

func titleizeSlug(s string) string { // "payment-stripe" -> "Payment Stripe"
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func pluginPackageName(slug string) string { // "payment-stripe" -> "paymentstripe"
	return strings.NewReplacer("-", "", "_", "").Replace(slug)
}

// File content is interpolated rather than run through text/template - the templates double as documentation of plugin shape, and that only reads well when they look like the file they produce
type pluginTemplate struct {
	Slug            string
	Name            string
	PackageName     string
	PaymentProvider bool
	WithMigrations  bool
}

func (t pluginTemplate) files() map[string]string {
	out := map[string]string{
		"plugin.go": t.pluginGo(),
		"README.md": t.readme(),
	}
	if t.PaymentProvider {
		out["provider.go"] = t.providerGo()
	}
	if t.WithMigrations {
		out["migrations.go"] = t.migrationsGo()
		out["migrations/0001_initial.up.sql"] = t.migrationUpSQL()
		out["migrations/0001_initial.down.sql"] = t.migrationDownSQL()
	}
	return out
}

func (t pluginTemplate) pluginGo() string {
	// Every scaffold opens with a manifest because the handle it returns is the only way to register an identity-bearing extension point - that keeps panic isolation, kill switches and observability keyed to the right plugin
	// SDKRange derives from the running SDK version so the scaffold can't drift when Version bumps
	manifestBlock := fmt.Sprintf(`	p := sdk.RegisterPlugin(sdk.Manifest{
		Slug:     %q,
		Name:     %q,
		Version:  "0.1.0",
		SDKRange: %q,
		Capabilities: []string{
			// "http.outbound:api.example.com",
			// "db.write",
		},
	})

`, t.Slug, t.Name, "^"+sdk.Version)

	regBlock := ""
	if t.PaymentProvider {
		regBlock = fmt.Sprintf(`	p.RegisterPaymentProvider(%q, &provider{}, sdk.WithDisplayName(%q))

	p.RegisterConfigField(%q, sdk.ConfigField{
		Type:        sdk.TypeSecret,
		Description: "API key for the %s provider.",
	})
`, t.Slug, t.Name, "payment/"+t.Slug+"/api_key", t.Name)
	} else {
		regBlock = "	_ = p\n\n" // without a provider nothing uses the handle yet and the generated file wouldn't compile
	}
	return fmt.Sprintf(`// Package %s is the %s plugin
//
// Tier 1 contract: listed in plugins/firstparty, which the server and the CLI both read - init() registers this plugin's
// extension points through the SDK and the host adapter (internal/plugin) replays them into the core registries at boot
//
// Every identity-bearing registration goes through the handle RegisterPlugin returns, so the host knows which plugin a hook
// belongs to regardless of init() order - and don't rename Slug after the first deploy, it doubles as the namespace for
// migrations, config (plugin/<slug>/*) and the runtime kill switch
package %s

import "github.com/skeletg/skelet/apps/backend/sdk"

func init() {
%s%s	// Examples of additional extension points - uncomment what you need, and note every Register* call panics on a duplicate
	// registration so conflicts surface the first time the binary boots
	//
	// Off the handle (attributed to this plugin):
	//
	//   p.RegisterScheduledJob("sync", "@every 1h", "Sync upstream", syncFn)
	//   p.SubscribeAsync("order.placed", onOrderPlaced)
	//
	// A browser-reachable endpoint - public and unauthenticated, served at
	// /api/v1/plugins/<slug>/client-config, so treat every input as hostile:
	//
	//   p.RegisterRoute("GET client-config", handleClientConfig)
	//
	// Anything started here that outlives a request needs stopping, or it
	// leaks through every restart:
	//
	//   p.OnShutdown(func(ctx context.Context) error { return pool.Close() })
	//
	// RegisterHealthCheck needs a context import - add it to the
	// import block above when you uncomment this:
	//
	//   p.RegisterHealthCheck("api", func(ctx context.Context) error { return nil })
	//
	// Package-level (keyed by their own path, no plugin identity):
	//
	//   p.RegisterPermission("group", "%s.view", "View %s")
	//   p.RegisterConfigField("section/%s/key", sdk.ConfigField{Type: sdk.TypeString})
	//   p.RegisterEvent("%s.something_happened", sdk.EventKindAsync)
}
`, t.PackageName, t.Name, t.PackageName, manifestBlock, regBlock, t.Slug, t.Name, t.Slug, t.Slug)
}

func (t pluginTemplate) providerGo() string {
	return fmt.Sprintf(`package %s

import (
	"context"
	"errors"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// A stub - replace each method body with real upstream API calls, reading the API key (and any other config field registered
// in plugin.go) via req.Config.Get, and using req.HTTPRequest in HandleWebhook to verify the signature
type provider struct{}

var errNotImplemented = errors.New(%q + ": not implemented")

func (p *provider) Authorize(ctx context.Context, req sdk.AuthorizeRequest) (sdk.AuthorizeResult, error) {
	return sdk.AuthorizeResult{}, errNotImplemented
}

func (p *provider) Capture(ctx context.Context, req sdk.CaptureRequest) (sdk.CaptureResult, error) {
	return sdk.CaptureResult{}, errNotImplemented
}

func (p *provider) Refund(ctx context.Context, req sdk.RefundRequest) (sdk.RefundResult, error) {
	return sdk.RefundResult{}, errNotImplemented
}

func (p *provider) Void(ctx context.Context, req sdk.VoidRequest) (sdk.VoidResult, error) {
	return sdk.VoidResult{}, errNotImplemented
}

func (p *provider) HandleWebhook(ctx context.Context, req sdk.WebhookRequest) (sdk.WebhookResult, error) {
	return sdk.WebhookResult{}, errNotImplemented
}
`, t.PackageName, t.Slug)
}

func (t pluginTemplate) migrationsGo() string {
	return fmt.Sprintf(`package %s

import (
	"embed"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Plugin-owned migrations, embedded at build time - apply with: skelet cli plugin migrate up %s
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

func init() {
	p.RegisterMigrations(sdk.MigrationsRegistration{
		Namespace: %q,
		FS:        migrationsFS,
	})
}
`, t.PackageName, t.Slug, t.Slug)
}

func (t pluginTemplate) migrationUpSQL() string {
	return fmt.Sprintf(`-- Initial migration for the %s plugin.
-- Add CREATE TABLE / ALTER TABLE statements below; skelet cli plugin
-- migrate up applies the whole file in a single transaction.
--
-- Replace the SELECT 1 line - it's only there so the file is
-- non-empty (migrate refuses to apply an empty file).

SELECT 1;
`, t.Slug)
}

func (t pluginTemplate) migrationDownSQL() string {
	return fmt.Sprintf(`-- Rollback for 0001_initial. Apply with:
--   skelet cli plugin migrate down %s 1
--
-- DROP whatever the up migration created. Keep the SELECT 1 if you
-- intentionally have no rollback (e.g. data-only seed) - it satisfies
-- migrate's non-empty requirement.

SELECT 1;
`, t.Slug)
}

func (t pluginTemplate) readme() string {
	return fmt.Sprintf(`# %s plugin

Skelet Tier 1 plugin. Compiled in via its blank-import in
plugins/firstparty.

## Layout

- plugin.go - registers this plugin's extension points (provider,
  permissions, config fields, events).
%s%s
## Wiring

After scaffolding the plugin, add the blank-import line to:

    plugins/firstparty/firstparty.go

Then run:

    skelet cli plugin migrate up %s    # if you have migrations
    skelet up         # restart server to load the plugin
`,
		t.Name,
		conditional(t.PaymentProvider, "- provider.go - PaymentProvider implementation (Authorize/Capture/Refund/Void/HandleWebhook).\n"),
		conditional(t.WithMigrations, "- migrations/ - SQL migrations tracked per-namespace in plugin_migrations.\n"),
		t.Slug,
	)
}

func conditional(cond bool, s string) string {
	if cond {
		return s
	}
	return ""
}
