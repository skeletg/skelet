package wiring

import (
	"fmt"
	"strings"

	graphadmin "github.com/skeletg/skelet/graph/admin"
	graphfront "github.com/skeletg/skelet/graph/front"

	"github.com/skeletg/skelet/internal/action"
	"github.com/skeletg/skelet/internal/address"
	authpkg "github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/cache"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/consent"
	"github.com/skeletg/skelet/internal/currency"
	"github.com/skeletg/skelet/internal/customer"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/inventory"
	"github.com/skeletg/skelet/internal/permission"
	pluginhost "github.com/skeletg/skelet/internal/plugin"
	"github.com/skeletg/skelet/internal/search"
	"github.com/skeletg/skelet/internal/tx"
)

// All in-memory, so it runs before a pool is opened - a bad manifest fails the deploy with no network round trip
func bootstrapRegistries() error {
	pluginhost.Install() // replays the Register* calls blank-imported plugins queued, so the validators below see them

	if err := pluginhost.ValidateManifests(); err != nil { // first, so a plugin demanding a future SDK errors clearly rather than as a confusing config failure
		return fmt.Errorf("plugin manifests: %w", err)
	}

	if err := graphadmin.ValidatePluginResolvers(); err != nil { // matched structurally, so drift from a plugin's own SDL would resolve fine until the first query hit it
		return fmt.Errorf("plugin graph resolvers (admin): %w", err)
	}
	if err := graphfront.ValidatePluginResolvers(); err != nil {
		return fmt.Errorf("plugin graph resolvers (front): %w", err)
	}

	if err := config.Validate(); err != nil {
		return fmt.Errorf("config registry: %w", err)
	}
	if err := permission.Validate(); err != nil {
		return fmt.Errorf("permission registry: %w", err)
	}
	if err := consent.Validate(); err != nil {
		return fmt.Errorf("consent registry: %w", err)
	}
	return nil
}

func RegisterConfigSources(cust *customer.Service, inv *inventory.Service, cur *currency.Service) { // most domains declare their config paths from init() - these can't, because their sources read through a constructed service
	customer.RegisterConfigSources(cust)
	inventory.RegisterConfigSources(inv)
	currency.RegisterConfigSources(cur)
	address.RegisterConfigSources() // reads the validator registry itself, so it needs no service

}

// Can't join bootstrapRegistries - config / permission / consent register from init(), but action handlers close over service references, so they wait until the graph has built them
func RegisterAdminActions(cfg *config.Service, tier SearchTier, vk CacheRedis, wcfg Config) error { // the flush-caches action targets the evictable client - that's where every read-through namespace lives now
	config.RegisterActions(cfg)
	pluginhost.SetSearchTier(tier.Indexes, tier.Client) // BEFORE RegisterActions so plugin indexes queued at init() make the reindex action's target options - a nil client drops them with a log line
	search.RegisterActions(tier.Indexes)
	cache.RegisterActions(vk.Client, wcfg.IsDev())

	if err := action.Validate(); err != nil {
		return fmt.Errorf("action registry: %w", err)
	}
	return nil
}

func applyTrustedProxies(cfg Config) error { // honored only when the TCP peer is trusted, else any client spoofs a fresh IP per request and sidesteps every limit
	if err := authpkg.SetTrustedProxies(strings.Split(cfg.TrustedProxies, ",")); err != nil {
		return fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}
	return nil
}

func syncPermissions(rc RootContext, queries *db.Queries, txMgr *tx.Manager) error { // one transaction, idempotent - grants every declared permission to the administrator role
	if err := permission.Sync(rc.Context, queries, txMgr); err != nil { // root ctx, not Background - this runs inside fx.New where no timeout applies, so a wedged Postgres would ignore SIGTERM
		return fmt.Errorf("permission sync: %w", err)
	}
	return nil
}
