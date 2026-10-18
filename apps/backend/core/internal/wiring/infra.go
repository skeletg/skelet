package wiring

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type ReadPool struct{ *sql.DB } // both pools are *sql.DB so the graph needs a distinct type; the commerce primary keeps the untagged slot

type ReadQueries struct{ *db.Queries } // same problem, same fix, for the sqlc query set over the replica

// Pools is the datastore layer alone - shareable by every entrypoint, because nothing in it is specific to serving HTTP
// Only the commerce database: everything of the suite's is reached over the contract, never opened here
var Pools = fx.Module("pools",
	fx.Provide(
		newQueryTracer,
		newPrimaryPool,
		newReadPool,
		newQueries,
		newReadQueries,
		newTxManager,
	),
)

var Cache = fx.Module("cache", fx.Provide(newValkey, newValkeyCache, newValkeyLimits)) // Valkey on its own - optional infrastructure, nil without VALKEY_URL, so every consumer degrades rather than fails

type CacheRedis struct{ *redis.Client } // evictable Valkey for caches/limiters/APQ - kept off the noeviction primary since these namespaces are anonymous-writable and unbounded - a flood evicts here instead of failing auth-token writes

type LimitsRedis struct{ *redis.Client } // rate-limit counters, off the cache instance so an eviction there can't reset a limit

// Registries is the process-global bootstrap - side effects rather than values, which is why these are invokes
var Registries = fx.Module("registries",
	fx.Invoke( // fx runs invokes in registration order, and that matters: plugins install before anything validates the registries they contribute to, and permissions validate before they sync
		bootstrapRegistries,
		applyTrustedProxies,
		syncPermissions,
	),
)

func newQueryTracer(cfg Config) pgx.QueryTracer { // dev wraps the OTel tracer in a counter so X-DB-Queries can report statements per request
	tracer := otelx.PGXTracer()
	if cfg.IsDev() {
		tracer = dbconn.CountingTracer(tracer)
	}
	return tracer
}

func openPool(lc fx.Lifecycle, label, dsn string, tracer pgx.QueryTracer, maxConns int) (*sql.DB, error) {
	conn, err := dbconn.Connect(dsn, dbconn.WithQueryTracer(tracer), dbconn.WithMaxConns(maxConns))
	if err != nil {
		return nil, fmt.Errorf("connect to %s database: %w", label, err)
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return conn.Close() }})
	return conn, nil
}

func newPrimaryPool(lc fx.Lifecycle, cfg Config, tracer pgx.QueryTracer) (*sql.DB, error) {
	return openPool(lc, "primary", cfg.DatabaseURL, tracer, cfg.MaxConns)
}

// Bulkhead so heavy Signals aggregations can't starve the primary's budget, and so DATABASE_REPLICA_URL can point them at a replica without code changes - writes stay on the primary
func newReadPool(lc fx.Lifecycle, cfg Config, tracer pgx.QueryTracer) (ReadPool, error) {
	conn, err := openPool(lc, "analytics read", cfg.ReplicaURL, tracer, cfg.AnalyticsMaxConns)
	return ReadPool{conn}, err
}

// OPTIONAL - empty VALKEY_URL runs on Postgres alone: weaker caches, per-process limits, no draft previews - a malformed URL is still fatal, that's a typo rather than a deployment choice
func newValkey(lc fx.Lifecycle, cfg Config) (*redis.Client, error) {
	if cfg.ValkeyURL == "" {
		slog.Warn("VALKEY_URL is unset: running Postgres-only",
			"caches", "disabled",
			"rate_limits", "per-process",
			"auth_tokens", "postgres",
			"draft_previews", "unavailable")
		return nil, nil
	}
	opt, err := redis.ParseURL(cfg.ValkeyURL)
	if err != nil {
		return nil, fmt.Errorf("parse valkey url: %w", err)
	}
	client := redis.NewClient(opt)
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return client.Close() }})
	if err := otelx.InstrumentRedis(client); err != nil {
		slog.Warn("instrument redis", "err", err)
	}
	return client, nil
}

// empty or same-as-primary VALKEY_CACHE_URL shares the primary's pool with no extra close hook - a distinct URL opens its own client that prod runs as a second allkeys-lru instance
func newValkeyCache(lc fx.Lifecycle, cfg Config, primary *redis.Client) (CacheRedis, error) {
	url := strings.TrimSpace(cfg.ValkeyCacheURL)
	if url == "" || url == cfg.ValkeyURL {
		return CacheRedis{primary}, nil
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return CacheRedis{}, fmt.Errorf("parse valkey cache url: %w", err)
	}
	client := redis.NewClient(opt)
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return client.Close() }})
	if err := otelx.InstrumentRedis(client); err != nil {
		slog.Warn("instrument redis (cache)", "err", err)
	}
	slog.Info("VALKEY_CACHE_URL set: caches and rate limiters use a separate Valkey from the auth-token primary")
	return CacheRedis{client}, nil
}

func newValkeyLimits(lc fx.Lifecycle, cfg Config, primary *redis.Client, cache CacheRedis) (LimitsRedis, error) {
	url := strings.TrimSpace(cfg.ValkeyLimitsURL)
	switch {
	case url == "" || url == strings.TrimSpace(cfg.ValkeyCacheURL):
		return LimitsRedis(cache), nil
	case url == strings.TrimSpace(cfg.ValkeyURL):
		return LimitsRedis{primary}, nil
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return LimitsRedis{}, fmt.Errorf("parse valkey limits url: %w", err)
	}
	client := redis.NewClient(opt)
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return client.Close() }})
	if err := otelx.InstrumentRedis(client); err != nil {
		slog.Warn("instrument redis (limits)", "err", err)
	}
	return LimitsRedis{client}, nil
}

func newQueries(conn *sql.DB) *db.Queries { return db.New(conn) }

func newTxManager(conn *sql.DB) *txrunner.Manager {
	return txrunner.New(conn)
}

func newReadQueries(p ReadPool) ReadQueries { return ReadQueries{db.New(p.DB)} }
