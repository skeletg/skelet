package example

import (
	"context"
	"database/sql"
	"errors"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/skeletg/skelet/internal/config"
	dbsqlc "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/payment"
	pluginhost "github.com/skeletg/skelet/internal/plugin"
	"github.com/skeletg/skelet/internal/pluginmigrate"
	"github.com/skeletg/skelet/internal/shipping"
	"github.com/skeletg/skelet/internal/testdb"
	"github.com/skeletg/skelet/internal/tx"
)

// One boot per binary, since plugin.Install panics on a second call and exposes no reset - the conn is process-scoped for the same reason, because SetConfigService is single-shot and a per-test conn would leave the host holding a service over a closed handle
var testDB *sql.DB

func TestMain(m *testing.M) {
	pluginhost.Install()
	if conn, _ := testdb.Open("DATABASE_URL"); conn != nil {
		testDB = conn
		pluginhost.SetConfigService(config.NewService(config.NewRepository(dbsqlc.New(conn), tx.New(conn))))
	}
	code := m.Run()
	if testDB != nil {
		_ = testDB.Close()
	}
	os.Exit(code)
}

// doc.go says plugins don't import internal/* and nothing enforced it - a plugin reaching past the SDK means the surface has a hole
func TestPluginImportsOnlySDK(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(p, "/internal/") {
				t.Errorf("%s imports %s - plugins may only reach core through the SDK", path, p)
			}
		}
	}
}

// The boot gate - SDK range admits the host, deps resolve, no cycles, every Consumes resolves
func TestManifestValidates(t *testing.T) {
	if err := pluginhost.ValidateManifests(); err != nil {
		t.Fatalf("boot would abort: %v", err)
	}
	var found bool
	for _, m := range pluginhost.Manifests() {
		if m.Slug == Slug {
			found = true
			if m.Version == "" || m.SDKRange == "" {
				t.Error("manifest registered with empty version or SDK range")
			}
		}
	}
	if !found {
		t.Fatalf("manifest %q never reached the host", Slug)
	}
}

// The two slugs differ on purpose - conflating them is how the kill switch fails open
func TestPaymentProviderRegisteredAndAttributed(t *testing.T) {
	reg, err := payment.GetProvider(ProviderSlug)
	if err != nil {
		t.Fatalf("provider %q not in the registry: %v", ProviderSlug, err)
	}
	if reg.DisplayName != "Example Gateway" {
		t.Errorf("display name: got %q, want Example Gateway", reg.DisplayName)
	}
	if reg.IsOffline {
		t.Error("a gateway provider must not be marked offline")
	}
	if _, err := payment.GetProvider(Slug); err == nil {
		t.Error("the manifest slug leaked into the payment registry as a provider")
	}
}

// The other in-process registry the SDK bridges to - a quote-only carrier also pins the host's capability type-assertion, since wrapping it
// as a Tracker or Labeler it doesn't implement would surface as a nil-method panic at checkout rather than at boot
func TestShippingCarrierRegistered(t *testing.T) {
	reg, err := shipping.GetCarrier(CarrierSlug)
	if err != nil {
		t.Fatalf("carrier %q not in the registry: %v", CarrierSlug, err)
	}
	if reg.DisplayName != "Example Carrier" {
		t.Errorf("display name: got %q", reg.DisplayName)
	}
	if _, isTracker := reg.Carrier.(shipping.Tracker); isTracker {
		t.Error("quote-only carrier was exposed as a Tracker")
	}
	if _, isLabeler := reg.Carrier.(shipping.Labeler); isLabeler {
		t.Error("quote-only carrier was exposed as a Labeler")
	}
}

// Both qualified by the MANIFEST slug, and the job name is persisted as the scheduled_jobs row key, so getting it wrong isn't something a later release can fix
func TestProbesAndJobsCarryTheManifestSlug(t *testing.T) {
	rec := httptest.NewRecorder()
	pluginhost.HealthHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	body := rec.Body.String()
	// a sibling manifest is claimed between this plugin's RegisterPlugin and its registrations, so ambient attribution files both probes under the sibling - two plugins is the smallest case that can tell them apart
	if !strings.Contains(body, Slug+".api") {
		t.Errorf("health probe not qualified as %q: %s", Slug+".api", body)
	}
	if !strings.Contains(body, SiblingSlug+".probe") {
		t.Errorf("sibling probe not qualified as %q: %s", SiblingSlug+".probe", body)
	}
	if strings.Contains(body, SiblingSlug+".api") {
		t.Errorf("probe attributed to the sibling - registration order leaked into identity: %s", body)
	}

	var found bool
	for _, j := range pluginhost.ScheduledJobs() {
		if j.Name == Slug+".reconcile" {
			found = true
			if j.Schedule != "@every 1h" {
				t.Errorf("schedule: got %q, want @every 1h", j.Schedule)
			}
		}
	}
	if !found {
		t.Errorf("scheduled job not registered as %q", Slug+".reconcile")
	}
}

// Fields queue in init() and only reach the registry on Install, with the payment group coming from the host adapter rather than the plugin - a definition whose section or group is missing aborts the server at boot
// Only THIS plugin's paths are checked rather than config.Validate over the global registry, since a test binary links a subset of core and the global view is partial
func TestConfigFieldsRegisterAndValidate(t *testing.T) {
	for _, path := range []string{
		"payment/" + ProviderSlug + "/api_key",
		"payment/" + ProviderSlug + "/mode",
		"plugin/" + Slug + "/enabled",
		"plugin/" + Slug + "/hook_timeout_ms",
		"plugin/" + Slug + "/rate_limit_per_min",
	} {
		if _, ok := config.Find(path); !ok {
			t.Errorf("config field %q not registered", path)
			continue
		}
		section, group, _, ok := config.ParsePath(path)
		if !ok {
			t.Errorf("config path %q is malformed", path)
			continue
		}
		if _, ok := config.FindSection(section); !ok {
			t.Errorf("%q references unregistered section %q - boot would abort", path, section)
		}
		if _, ok := config.FindGroup(section, group); !ok {
			t.Errorf("%q references unregistered group %q/%q - boot would abort", path, section, group)
		}
	}
}

// The window between plugin init() and main.go building the bus is the whole reason the queue exists
func TestEventSubscriberReplaysOntoTheBus(t *testing.T) {
	bus := sharedBus() // SetEventBus is single-shot, and the sync-subscriber test needs the same bus

	bus.Publish(context.Background(), settledEvent{Reference: "ref-1"})
	for { // drain past other tests' publishes - every settled event fans into this channel
		select {
		case payload := <-settledSeen:
			if strings.Contains(string(payload), "ref-1") {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatal("subscriber never fired - the init()-time subscription did not replay")
		}
	}
}

type settledEvent struct{ Reference string }

func (settledEvent) Type() string { return EventSettled }

// Postgres from here down - the config service backs the scoped reader, the kill switch and the migration runner, none of which can be faked without testing the fake
func withDB(t *testing.T) *sql.DB {
	t.Helper()
	if testDB == nil {
		t.Skip("set DATABASE_URL")
	}
	return testDB
}

// The full placement path - core request in, SDK request out, scoped config read, result translated back
func TestAuthorizeThroughTheHostAdapter(t *testing.T) {
	withDB(t)

	reg, err := payment.GetProvider(ProviderSlug)
	if err != nil {
		t.Fatal(err)
	}
	orderID := uuid.New()
	res, err := reg.Provider.Authorize(context.Background(), payment.AuthorizeRequest{
		OrderID:      orderID,
		CurrencyCode: "USD",
		AmountCents:  1234,
		Email:        "buyer@example.invalid",
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if res.ProviderReference != "auth_"+orderID.String() {
		t.Errorf("provider reference: got %q", res.ProviderReference)
	}

	seen, mode := registeredProvider.lastAuthorize()
	if seen.OrderID != orderID || seen.AmountCents != 1234 {
		t.Errorf("request did not survive translation: %+v", seen)
	}
	if seen.Config == nil {
		t.Error("host did not inject a scoped config reader")
	}
	// registered default, resolved through the payment/<provider>/ prefix
	if mode != "test" {
		t.Errorf("scoped config read: got %q, want the registered default \"test\"", mode)
	}
}

// The operator's only lever over a compiled-in plugin, so it has to hold against a real config store rather than a stub
func TestKillSwitchStopsTheHook(t *testing.T) {
	conn := withDB(t)
	svc := config.NewService(config.NewRepository(dbsqlc.New(conn), tx.New(conn)))
	ctx := context.Background()

	path := "plugin/" + Slug + "/enabled"
	if _, err := svc.Set(ctx, path, "false", config.DefaultScope); err != nil {
		t.Fatalf("disable: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM config_values WHERE path = $1`, path)
	})

	reg, err := payment.GetProvider(ProviderSlug)
	if err != nil {
		t.Fatal(err)
	}
	_, err = reg.Provider.Authorize(ctx, payment.AuthorizeRequest{OrderID: uuid.New(), AmountCents: 100})
	if !errors.Is(err, pluginhost.ErrPluginDisabled) {
		t.Fatalf("disabled plugin still authorized: %v", err)
	}
	var perr *pluginhost.PluginError
	if !errors.As(err, &perr) || perr.Slug != Slug {
		t.Errorf("error attributed to %v, want the manifest slug %q", err, Slug)
	}

	if _, err := svc.Set(ctx, path, "true", config.DefaultScope); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if _, err := reg.Provider.Authorize(ctx, payment.AuthorizeRequest{OrderID: uuid.New(), AmountCents: 100}); err != nil {
		t.Fatalf("re-enabled plugin still refused: %v", err)
	}
}

// Runs last on purpose: it fires the plugin's hook, and nothing after it should assume the plugin
// is still live - which is exactly the ordering main.go gives real shutdown
func TestShutdownHookRuns(t *testing.T) {
	pluginhost.Shutdown(context.Background())
	select {
	case <-shutdownRan:
	default:
		t.Fatal("shutdown hook never ran - a plugin's goroutines would leak across restarts")
	}
}

// Embedded SQL applies, records itself, and no-ops on re-run
func TestMigrationsApply(t *testing.T) {
	conn := withDB(t)
	ctx := context.Background()

	t.Cleanup(func() {
		_, _ = conn.Exec(`DROP TABLE IF EXISTS example_plugin_records`)
		_, _ = conn.Exec(`DELETE FROM plugin_migrations WHERE namespace = $1`, Slug)
	})
	_, _ = conn.Exec(`DROP TABLE IF EXISTS example_plugin_records`)
	_, _ = conn.Exec(`DELETE FROM plugin_migrations WHERE namespace = $1`, Slug)

	runner := pluginmigrate.New(conn)
	if err := runner.UpInOrder(ctx, pluginhost.OrderedManifestSlugs()); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	var exists bool
	if err := conn.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'example_plugin_records')`,
	).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("migration recorded but the table it creates is missing")
	}
	if err := runner.UpInOrder(ctx, pluginhost.OrderedManifestSlugs()); err != nil {
		t.Fatalf("re-run should be a no-op: %v", err)
	}
}
