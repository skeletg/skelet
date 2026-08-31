// Package example is a Tier 1 plugin that exists only to hold the SDK contract still - NOT shipped, since nothing blank-imports it, so it compiles on every build and links into no binary
// It touches every surface a real plugin uses, so a change to any of them fails here rather than in someone's private plugin repo - the failures this catches are compositional, where every piece is right on its own and only the wiring between them is wrong
package example

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug                = "example"
	SiblingSlug         = "example-sibling"
	ProviderSlug        = "example-gateway"
	OfflineProviderSlug = "example-offline"
	CarrierSlug         = "example-carrier"
	EventSettled        = "example.settled"
	EventSettling       = "example.settling" // sync - fired pre-commit, handlers may mutate or veto
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var (
	settledSeen        = make(chan []byte, 4) // the subscriber's proof that the bus reached this plugin
	onceSeen           = make(chan []byte, 4) // same proof for the replay-deduped subscriber
	shutdownRan        = make(chan struct{})  // closed by the shutdown hook, for the same reason
	registeredProvider = &provider{}          // so the test can read what the host adapter handed it
	pluginHandle       *sdk.Plugin            // so the test can exercise the handle getters
)

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{
		Slug:     Slug,
		Name:     "Example Plugin",
		Version:  "0.1.0",
		SDKRange: "^0.2.0",
		Emits:    []string{EventSettled, EventSettling},
		Consumes: []string{EventSettled, EventSettling},
		Capabilities: []string{
			"db.write",
			"events.publish:" + EventSettled,
			"events.publish:" + EventSettling,
		},
	})
	pluginHandle = p

	// Claimed BEFORE this plugin registers anything - under ambient attribution everything below would land under SiblingSlug, which is the failure this holds still
	sibling := sdk.RegisterPlugin(sdk.Manifest{
		Slug:     SiblingSlug,
		Name:     "Example Sibling",
		Version:  "0.1.0",
		SDKRange: "^0.2.0",
	})
	sibling.RegisterHealthCheck("probe", func(context.Context) error { return nil })

	p.RegisterPaymentProvider(ProviderSlug, registeredProvider, sdk.WithDisplayName("Example Gateway"))

	// A second provider on the same manifest - the offline flag routes it past gateway authorization, and the kill switch stays keyed on the manifest slug
	p.RegisterPaymentProvider(OfflineProviderSlug, &provider{}, sdk.WithDisplayName("Example Offline"), sdk.WithOffline())

	p.RegisterShippingCarrier(CarrierSlug, &carrier{}, sdk.WithCarrierDisplayName("Example Carrier"))

	// Mounted at /api/v1/plugins/example/ping behind the host's kill switch, rate limit, timeout and panic isolation
	p.RegisterRoute("GET ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})

	// Pre-commit participation - the assertion is structural so this package keeps importing only the SDK
	p.SubscribeSync(EventSettling, 100, func(_ context.Context, e any) error {
		if ev, ok := e.(interface{ MarkSettling() }); ok {
			ev.MarkSettling()
		}
		return nil
	})

	p.RegisterPromotionCondition(minSubtotalCondition{})
	p.RegisterPromotionAction(flatDiscountAction{})
	p.RegisterPromotionRedemptionHook(recordRedemptionHook{})

	p.RegisterHealthCheck("api", func(context.Context) error { return nil })

	// fails until api_key is set, which is what readiness is for
	p.RegisterReadinessCheck("config", func(ctx context.Context) error {
		v, _ := sdk.PluginConfig(ctx, "payment/"+ProviderSlug+"/api_key")
		if v == "" {
			return errors.New("api_key not configured")
		}
		return nil
	})

	p.RegisterScheduledJob("reconcile", "@every 1h", "Reconcile example charges", func(context.Context) error { return nil })

	// A plugin-owned index: the host names it plg-example-charges and wires it into the reindex tooling - errors when OPENSEARCH_URL is unset,
	// since search is a soft dependency. A real plugin keeps the returned writer for event-driven upserts
	p.RegisterSearchIndex(sdk.SearchIndexRegistration{
		Name:        "charges",
		MappingJSON: `{"mappings":{"properties":{"reference":{"type":"keyword"},"amount_cents":{"type":"long"},"settled_at":{"type":"date"}}}}`,
		Reindex: func(ctx context.Context, sink sdk.SearchIndexSink) (int, error) {
			// A real plugin streams its source of truth here - the example has none
			return 0, nil
		},
	})

	p.OnShutdown(func(context.Context) error {
		close(shutdownRan)
		return nil
	})

	p.SubscribeAsync(EventSettled, func(_ context.Context, payload json.RawMessage) error {
		select {
		case settledSeen <- payload:
		default:
		}
		return nil
	})

	// Replay-deduped, keyed on the settled reference - on a host with no dedup store wired it degrades to plain delivery
	p.SubscribeAsyncOnce(EventSettled, func(payload json.RawMessage) (string, error) {
		var e struct{ Reference string }
		if err := json.Unmarshal(payload, &e); err != nil {
			return "", err
		}
		return e.Reference, nil
	}, func(_ context.Context, payload json.RawMessage) error {
		select {
		case onceSeen <- payload:
		default:
		}
		return nil
	})

	p.RegisterEvent(EventSettled, sdk.EventKindAsync)
	p.RegisterEvent(EventSettling, sdk.EventKindSync)

	// Run-once Go against the patch's own tx, applied by `bin/cli data-patch up` after schema migrations
	p.RegisterDataPatch(sdk.DataPatchRegistration{
		Namespace: Slug,
		ID:        Slug + ":seed-example-records",
		Apply:     func(context.Context, *sql.Tx) error { return nil },
	})

	p.RegisterConfigField("payment/"+ProviderSlug+"/api_key", sdk.ConfigField{
		Type:        sdk.TypeSecret,
		Description: "Example gateway API key",
	})
	p.RegisterConfigField("payment/"+ProviderSlug+"/mode", sdk.ConfigField{
		Type:         sdk.TypeString,
		DefaultValue: "test",
		Description:  "Example gateway mode",
	})

	p.RegisterPermission("payments", "example.manage", "Manage the example gateway")

	p.RegisterMigrations(sdk.MigrationsRegistration{Namespace: Slug, FS: migrationsFS})
}
