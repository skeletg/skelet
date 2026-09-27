package sdk

// Per-plugin probes aggregated into /healthz and /readyz, so no plugin has to expose its own endpoint
// The k8s split decides who gets paged vs who gets pulled from the load balancer - health failing means a dependency is down but the replica still serves, readiness failing means this replica can't do the work and should leave rotation mid-deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Reachability of the plugin's external dependency - error text becomes the failure message ("api.stripe.com unreachable: timeout after 2s")
type HealthCheck func(ctx context.Context) error

// Whether the plugin is configured to do useful work at all, e.g. an API key is set
type ReadinessCheck func(ctx context.Context) error

// name is dot-prefixed with the manifest slug, so a Stripe plugin's "api" shows as "payment-stripe.api"
func (pl *Plugin) RegisterHealthCheck(name string, check HealthCheck) {
	withHost(func(h Host) error { return h.RegisterHealthCheck(pl.slug, name, check) })
}

func (pl *Plugin) RegisterReadinessCheck(name string, check ReadinessCheck) {
	withHost(func(h Host) error { return h.RegisterReadinessCheck(pl.slug, name, check) })
}

// Default-scope config read for code with no request-scoped ConfigReader - health probes, scheduled jobs, startup validators
// path is FULL and unprefixed by the plugin slug, since probes commonly cross-check fields outside their own namespace - which also means this reads ANY plugin's config, secrets included, so ConfigReader's prefixing is convenience rather than containment
// Fail-open, returning "" before the host is installed
func PluginConfig(ctx context.Context, path string) (string, error) {
	h := hostNow()
	if h == nil {
		return "", nil
	}
	return h.ReadConfigString(ctx, path)
}

// The typed siblings, resolving through the registered field type so a TypeBool default parses once in core instead of per-plugin - same full-path, fail-open contract as PluginConfig
func PluginConfigBool(ctx context.Context, path string) (bool, error) {
	h := hostNow()
	if h == nil {
		return false, nil
	}
	return h.ReadConfigBool(ctx, path)
}

func PluginConfigInt(ctx context.Context, path string) (int64, error) {
	h := hostNow()
	if h == nil {
		return 0, nil
	}
	return h.ReadConfigInt(ctx, path)
}

// NamespacedConfig is a default-scope ConfigReader rooted at one namespace, e.g.
// NamespacedConfig("payment/stripe") - the reader for health probes and resolvers, which have no
// request-scoped reader of their own. It prefixes exactly like the host's bound
// provider reader, so short keys work identically through both, and it refuses anything with a
// slash for the same reason the host reader does: a short key never has one, and a full path
// double-prefixes into one that doesn't exist, silently
func NamespacedConfig(namespace string) ConfigReader {
	return namespacedConfig{prefix: strings.TrimSuffix(namespace, "/") + "/"}
}

type namespacedConfig struct{ prefix string }

func (n namespacedConfig) Get(ctx context.Context, key string, _ uuid.UUID) (string, error) {
	if strings.Contains(key, "/") {
		return "", fmt.Errorf("sdk: config key %q looks like a full path - this reader already prefixes %q, pass the short key", key, n.prefix)
	}
	return PluginConfig(ctx, n.prefix+key)
}
